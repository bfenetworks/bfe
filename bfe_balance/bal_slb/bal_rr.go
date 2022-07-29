// Copyright (c) 2019 The BFE Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// weighted round-robin balance
//
// Algorithm:
//   smooth Weighted Round Robin algorithm is as follows: on each backend selection,
//   1. increase CurrentWeight of each eligible backend by its weight,
//   2. select backend with greatest CurrentWeight and reduce its CurrentWeight
//      by total number of weight points distributed among backends.
//
// Example:
//   In case of {a:5, b:1, c:1} weights this gives the following sequence 'aabacaa'
//   instead of 'abcaaaa'.
//
//      a  b  c
//      0  0  0  initial state
//      5  1  1  a selected; 5-7+5 1+1   1+1   -> 3  2  2
//      3  2  2  a selected; 3-7+5 2+1   2+1   -> 1  3  3
//      1  3  3  b selected; 1+5   3-7+1 3+1   -> 6 -3  4
//      6 -3  4  a selected; 6-7+5 -3+1  5+1   -> 4 -2  5
//      4 -2  5  c selected; 4+5   -2+1  5-7+1 -> 9 -1 -1
//      9 -1 -1  a selected; 9-7+5 -1+1  -1+1  -> 7  0  0
//      7  0  0  a selected; 7-7+5 0+1   0+1   -> 5  1  1

package bal_slb

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/spaolacci/murmur3"
)

import (
	"github.com/bfenetworks/bfe/bfe_balance/backend"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
	"github.com/bfenetworks/bfe/bfe_debug"
)

// implementation versions of weighted round-robin algorithm
const (
	WrrSimple = 0
	WrrSmooth = 1
	WrrSticky = 2
	WlcSimple = 3
	WlcSmooth = 4
)

type BackendList []*BackendRR

func (bl *BackendList) ResetWeight() {
	for _, backendRR := range *bl {
		backendRR.current = backendRR.weight
	}
}

type BackendListSorter struct {
	l BackendList
}

func (s BackendListSorter) Len() int {
	return len(s.l)
}

func (s BackendListSorter) Swap(i, j int) {
	s.l[i], s.l[j] = s.l[j], s.l[i]
}

func (s BackendListSorter) Less(i, j int) bool {
	return s.l[i].backend.AddrInfo < s.l[j].backend.AddrInfo
}

type BalanceRR struct {
	sync.Mutex
	Name     string
	backends BackendList // list of BackendRR
	sorted   bool        // list of BackendRR sorted or not
	next     int         // next backend to schedule

	slowStartNum  int // number of backends in slow_start phase
	slowStartTime int // time for backend increases the weight to the full value, in seconds
}

func NewBalanceRR(name string) *BalanceRR {
	brr := new(BalanceRR)
	brr.Name = name
	return brr
}

// Init initializes RRList with config.
func (brr *BalanceRR) Init(conf cluster_table_conf.SubClusterBackend) {
	for _, backendConf := range conf {
		backendRR := NewBackendRR()
		backendRR.Init(brr.Name, backendConf)
		// add to backends
		brr.backends = append(brr.backends, backendRR)
	}
	brr.sorted = false
	brr.next = 0
}

func (brr *BalanceRR) SetSlowStart(ssTime int) {
	brr.Lock()
	brr.slowStartTime = ssTime
	brr.Unlock()
}

func (brr *BalanceRR) checkSlowStart() {
	brr.Lock()
	defer brr.Unlock()
	if brr.slowStartTime > 0 {
		for _, backendRR := range brr.backends {
			backend := backendRR.backend
			if backend.GetRestart() {
				backend.SetRestart(false)
				backendRR.initSlowStart(brr.slowStartTime)
			}
			backendRR.updateSlowStart()
		}
	}
}

// Release releases backend list.
func (brr *BalanceRR) Release() {
	for _, back := range brr.backends {
		back.Release()
	}
}

func confMapMake(conf cluster_table_conf.SubClusterBackend) map[string]*cluster_table_conf.BackendConf {
	retVal := make(map[string]*cluster_table_conf.BackendConf)

	for _, backend := range conf {
		retVal[backend.AddrInfo()] = backend
	}

	return retVal
}

// Update updates BalanceRR with new config.
func (brr *BalanceRR) Update(conf cluster_table_conf.SubClusterBackend) {
	// create new BackendList
	var backendsNew BackendList

	// create map for config
	confMap := confMapMake(conf)

	brr.Lock()
	defer brr.Unlock()

	// go through backendsOld, make update and delete
	for index := 0; index < len(brr.backends); index++ {
		backendRR := brr.backends[index]

		backendKey := backendRR.backend.GetAddrInfo()
		bkConf, ok := confMap[backendKey]
		if ok && backendRR.MatchAddrPort(*bkConf.Addr, *bkConf.Port) {
			// found existing backend
			backendRR.UpdateWeight(*bkConf.Weight)
			backendsNew = append(backendsNew, backendRR)
			delete(confMap, backendKey)
		} else {
			// tell health-check to stop
			backendRR.Release()
		}
	}

	// add new backend to backendsNew
	for _, bkConf := range confMap {
		backendRR := NewBackendRR()
		backendRR.Init(brr.Name, bkConf)
		backend := backendRR.backend
		backend.SetRestart(true)
		// add to backendsNew
		backendsNew = append(backendsNew, backendRR)
	}

	// point brr.backends to backendsNew
	brr.backends = backendsNew
	brr.sorted = false
	brr.next = 0
}

// initWeight initializes all backendRR.current to backendRR.weight.
func (brr *BalanceRR) initWeight() {
	brr.backends.ResetWeight()
}

func moveToNext(next int, backends BackendList) int {
	// move to next. if at end of list, move back to 0
	next += 1
	if next >= len(backends) {
		next = 0
	}
	return next
}

func (brr *BalanceRR) ensureSortedUnlocked() {
	if !brr.sorted {
		sort.Sort(BackendListSorter{brr.backends})
		brr.sorted = true
	}
}

// Balance select one backend from sub cluster in round-robin manner.
func (brr *BalanceRR) Balance(algor int, key []byte) (*backend.BfeBackend, error) {
	// Slow start is not supported when session sticky is enabled
	if algor != WrrSticky {
		brr.checkSlowStart()
	}
	switch algor {
	case WrrSimple:
		return brr.simpleBalance()
	case WrrSmooth:
		return brr.smoothBalance()
	case WrrSticky:
		return brr.stickyBalance(key)
	case WlcSimple:
		return brr.leastConnsSimpleBalance()
	case WlcSmooth:
		return brr.leastConnsSmoothBalance()
	default:
		return brr.smoothBalance()
	}
}

func (brr *BalanceRR) smoothBalance() (*backend.BfeBackend, error) {
	brr.Lock()
	defer brr.Unlock()

	return smoothBalance(brr.backends)
}

func smoothBalance(backs BackendList) (*backend.BfeBackend, error) {
	var best *BackendRR
	total, max := 0, 0

	for _, backendRR := range backs {
		backend := backendRR.backend
		// skip ineligible backend
		if !backend.Avail() || backendRR.weight <= 0 {
			continue
		}

		// select backend with the greatest current weight
		if best == nil || backendRR.current > max {
			best = backendRR
			max = backendRR.current
		}
		total += backendRR.current

		// update current weight
		backendRR.current += backendRR.weight
	}

	if best == nil {
		if bfe_debug.DebugBal {
			log.Logger.Debug("rr_bal:reset backend weight")
		}
		return nil, fmt.Errorf("rr_bal:all backend is down")
	}

	// update current weight for chosen backend
	best.current -= total

	return best.backend, nil
}

func (brr *BalanceRR) leastConnsSmoothBalance() (*backend.BfeBackend, error) {
	brr.Lock()
	defer brr.Unlock()

	// select available candidates
	candidates, err := leastConnsBalance(brr.backends)
	if err != nil {
		return nil, err
	}

	// single candidate, return directly
	if len(candidates) == 1 {
		return candidates[0].backend, nil
	}

	// select backends by smooth balance
	return smoothBalance(candidates)
}

func (brr *BalanceRR) leastConnsSimpleBalance() (*backend.BfeBackend, error) {
	brr.Lock()
	defer brr.Unlock()

	// select candidates
	candidates, err := leastConnsBalance(brr.backends)
	if err != nil {
		return nil, err
	}

	// single candidate, return directly
	if len(candidates) == 1 {
		return candidates[0].backend, nil
	}

	// random select
	return randomBalance(candidates)
}

func leastConnsBalance(backs BackendList) (BackendList, error) {
	var best *BackendRR
	candidates := make(BackendList, 0, len(backs))

	// select available candidates
	singleBackend := true
	for _, backendRR := range backs {
		if !backendRR.backend.Avail() || backendRR.weight <= 0 {
			continue
		}

		if best == nil {
			best = backendRR
			singleBackend = true
			continue
		}

		// compare backends
		ret := compLCWeight(best, backendRR)
		if ret > 0 {
			best = backendRR
			singleBackend = true
		} else if ret == 0 {
			singleBackend = false
			if len(candidates) > 0 {
				candidates = append(candidates, backendRR)
			} else {
				candidates = append(candidates, best, backendRR)
			}

		}
	}

	if best == nil {
		return nil, fmt.Errorf("rr_bal:all backend is down")
	}

	// single backend, return directly
	if singleBackend {
		return BackendList{best}, nil
	}
	// more than one backend have same connections/weight,
	// return all the candidates
	return candidates, nil
}

func randomBalance(backs BackendList) (*backend.BfeBackend, error) {
	i := rand.Int() % len(backs)
	return backs[i].backend, nil
}

func (brr *BalanceRR) simpleBalance() (*backend.BfeBackend, error) {
	var backend *backend.BfeBackend
	var backendRR *BackendRR

	brr.Lock()
	defer brr.Unlock()

	backends := brr.backends
	allBackendDown := true

	next := brr.next
	for {
		backendRR = backends[next]
		backend = backendRR.backend

		avail := backend.Avail()
		if avail && backendRR.current > 0 {
			// find one available backend
			break
		}

		if bfe_debug.DebugBal {
			log.Logger.Debug("backend[%s],avail[%d],weight[%d]",
				backend.Name, avail, backendRR.weight)
		}

		if avail && backendRR.weight != 0 {
			allBackendDown = false
		}

		// move to next
		next = moveToNext(next, backends)

		if next == brr.next {
			// all backends have been checked
			if allBackendDown {
				if bfe_debug.DebugBal {
					log.Logger.Debug("rr_bal:all backend is down")
				}
				return backend, fmt.Errorf("rr_bal:all backend is down")
			} else {
				if bfe_debug.DebugBal {
					log.Logger.Debug("rr_bal:reset backend weight")
				}
				brr.initWeight()
				brr.next = 0
				next = 0
			}
		}
	}

	// modify current
	backendRR.current--

	// modify brr.next, for next use
	next = moveToNext(next, backends)
	brr.next = next

	if bfe_debug.DebugBal {
		log.Logger.Debug("rr.Balance: backend[%s] weight[%d]current[%d]",
			backend.Name, backendRR.weight, backendRR.current)
	}
	return backend, nil
}

func (brr *BalanceRR) stickyBalance(key []byte) (*backend.BfeBackend, error) {
	candidates := make(BackendList, 0, brr.Len())
	totalWeight := 0

	brr.Lock()
	defer brr.Unlock()

	// select available candidates
	brr.ensureSortedUnlocked()
	for _, backendRR := range brr.backends {
		if backendRR.backend.Avail() && backendRR.weight > 0 {
			candidates = append(candidates, backendRR)
			totalWeight += backendRR.weight
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("rr_bal:all backend is down")
	}

	// select backend from candidates
	value := GetHash(key, uint(totalWeight))
	for _, backendRR := range candidates {
		value -= backendRR.weight
		if value < 0 {
			return backendRR.backend, nil
		}
	}

	/* never come here */
	return nil, fmt.Errorf("rr_bal:stickyBalance fail")
}

// compLCWeight returns an integer comparing two backends by connNum/Weight.
// result will be 0 if a == b, -1 if a < b, +1 if a > b
func compLCWeight(a, b *BackendRR) int {
	// compare a.backend.ConnNum() / a.weight and b.backend.ConnNum() / b.weight
	// to avoid compare floating num, both multiple a.weight * b.weight
	ret := a.backend.ConnNum()*b.weight - b.backend.ConnNum()*a.weight

	// a.backend.ConnNum() / a.weight > b.backend.ConnNum() / b.weight
	if ret > 0 {
		return 1
	}

	// a.backend.ConnNum() / a.weight == b.backend.ConnNum() / b.weight
	if ret == 0 {
		return 0
	}

	return -1
}

func (brr *BalanceRR) Len() int {
	return len(brr.backends)
}

func GetHash(value []byte, base uint) int {
	var hash uint64

	if value == nil {
		hash = uint64(rand.Uint32())
	} else {
		hash = murmur3.Sum64(value)
	}

	return int(hash % uint64(base))
}

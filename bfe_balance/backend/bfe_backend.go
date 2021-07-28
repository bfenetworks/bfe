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

// backend framework for bfe

package backend

import (
	"fmt"
	"sync"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
)

// BfeBackend is a backend server.
type BfeBackend struct {
	// immutable
	Name       string // backend's name
	Addr       string // backend's address, e.g., "10.1.1.1"
	Port       int    // backend's port, e.g., 8080
	AddrInfo   string // backend's address and port, e.g., "10.1.1.1:8080"
	SubCluster string // name of sub-cluster

	sync.RWMutex      // guards following fields
	avail        bool // whether the backend is usable
	restarted    bool // indicate if this backend is new bring-up by health-check
	connNum      int  // number of connections backend hold
	failNum      int  // number of consecutive failures of normal requests
	succNum      int  // number of consecutive successes of health-check request

	closeChan chan bool // tell health-check to stop

}

func NewBfeBackend() *BfeBackend {
	backend := new(BfeBackend)
	backend.avail = true
	backend.closeChan = make(chan bool)

	return backend
}

// Init initializes BfeBackend with BackendConf
func (back *BfeBackend) Init(subCluster string, conf *cluster_table_conf.BackendConf) {
	back.Name = *conf.Name
	back.Addr = *conf.Addr
	back.Port = *conf.Port
	back.AddrInfo = fmt.Sprintf("%s:%d", back.Addr, back.Port)
	back.SubCluster = subCluster
}

func (back *BfeBackend) GetAddr() string {
	return back.Addr
}

func (back *BfeBackend) GetAddrInfo() string {
	return back.AddrInfo
}

func (back *BfeBackend) Avail() bool {
	back.RLock()
	avail := back.avail
	back.RUnlock()

	return avail
}

func (back *BfeBackend) SetAvail(avail bool) {
	back.Lock()
	back.setAvail(avail)
	back.Unlock()
}

func (back *BfeBackend) setAvail(avail bool) {
	// no lock, caller to call lock
	back.avail = avail
	if back.avail {
		back.failNum = 0
	}
}

func (back *BfeBackend) SetRestart(restart bool) {
	back.Lock()
	back.restarted = restart
	back.Unlock()
}

func (back *BfeBackend) GetRestart() bool {
	back.RLock()
	restart := back.restarted
	back.RUnlock()
	return restart
}

func (back *BfeBackend) ConnNum() int {
	back.RLock()
	connNum := back.connNum
	back.RUnlock()

	return connNum
}

func (back *BfeBackend) IncConnNum() {
	back.Lock()
	back.connNum++
	back.Unlock()
}

func (back *BfeBackend) DecConnNum() {
	back.Lock()
	back.connNum--
	back.Unlock()
}

func (back *BfeBackend) AddFailNum() {
	back.Lock()
	back.failNum++
	back.Unlock()
}

func (back *BfeBackend) ResetFailNum() {
	back.Lock()
	back.failNum = 0
	back.Unlock()
}

func (back *BfeBackend) FailNum() int {
	back.RLock()
	failNum := back.failNum
	back.RUnlock()

	return failNum
}

func (back *BfeBackend) AddSuccNum() {
	back.Lock()
	back.succNum++
	back.Unlock()
}

func (back *BfeBackend) ResetSuccNum() {
	back.Lock()
	back.succNum = 0
	back.Unlock()
}

func (back *BfeBackend) SuccNum() int {
	back.Lock()
	succNum := back.succNum
	back.Unlock()

	return succNum
}

// CheckAvail check whether backend becomes available.
func (back *BfeBackend) CheckAvail(succThreshold int) bool {
	back.Lock()
	defer back.Unlock()

	if back.succNum >= succThreshold {
		back.succNum = 0
		return true
	}

	return false
}

func (back *BfeBackend) UpdateStatus(failThreshold int) bool {
	back.Lock()
	defer back.Unlock()

	prevStatus := back.avail

	// set status to false when failNum >= threshold and
	// return true if status flip to false.
	if back.failNum >= failThreshold {
		back.setAvail(false)
		if prevStatus {
			return true
		}
	}

	return false
}

func (back *BfeBackend) Release() {
	back.Close()
}

func (back *BfeBackend) Close() {
	close(back.closeChan)
}

func (back *BfeBackend) CloseChan() <-chan bool {
	return back.closeChan
}

// OnSuccess is called when request backend success
func (back *BfeBackend) OnSuccess() {
	// reset backend failnum
	back.ResetFailNum()
}

// OnFail is called when request backend fail
func (back *BfeBackend) OnFail(cluster string) {
	back.AddFailNum()
	UpdateStatus(back, cluster)
}

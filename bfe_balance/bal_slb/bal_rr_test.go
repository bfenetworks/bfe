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

package bal_slb

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/bfenetworks/bfe/bfe_balance/backend"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

func populateBackend(name, addr string, port int, avail bool) *backend.BfeBackend {
	b := backend.NewBfeBackend()
	b.Name = name
	b.Addr = addr
	b.Port = port
	b.AddrInfo = fmt.Sprintf("%s:%d", addr, port)
	b.SetAvail(avail)
	return b
}

func prepareBalanceRR() *BalanceRR {
	b1 := populateBackend("b1", "127.0.0.1", 80, true)
	b2 := populateBackend("b2", "127.0.0.1", 81, true)
	b3 := populateBackend("b3", "127.0.0.1", 82, true)

	rr := &BalanceRR{
		backends: []*BackendRR{
			{
				weight:  300,
				current: 300,
				backend: b1,
			},
			{
				weight:  200,
				current: 200,
				backend: b2,
			},
			{
				weight:  100,
				current: 100,
				backend: b3,
			},
		},
	}
	return rr
}

func prepareBalanceRRLcw() *BalanceRR {
	b1 := populateBackend("b1", "127.0.0.1", 80, true)
	b2 := populateBackend("b2", "127.0.0.1", 81, true)
	b3 := populateBackend("b3", "127.0.0.1", 82, true)
	b4 := populateBackend("b4", "127.0.0.1", 83, true)

	rr := &BalanceRR{
		backends: []*BackendRR{
			{
				weight:  300,
				current: 300,
				backend: b1,
			},
			{
				weight:  200,
				current: 200,
				backend: b2,
			},
			{
				weight:  100,
				current: 100,
				backend: b3,
			},
			{
				weight:  50,
				current: 50,
				backend: b4,
			},
		},
	}
	return rr
}

func processBalance(t *testing.T, label string, algor int, key []byte, rr *BalanceRR, result []string) {
	var l []string
	for i := 1; i < 10; i++ {
		r, err := rr.Balance(algor, key)
		if err != nil {
			t.Errorf("should not error")
		}
		r.IncConnNum()
		l = append(l, r.Name)
	}

	if !reflect.DeepEqual(l, result) {
		t.Errorf("balance error [%s] %v, expect %v", label, l, result)
	}
}

func processBalancLoopTwenty(t *testing.T, label string, algor int, key []byte, rr *BalanceRR, result []string) {
	var l []string
	for i := 1; i < 20; i++ {
		r, err := rr.Balance(algor, key)
		if err != nil {
			t.Errorf("should not error")
		}
		r.IncConnNum()
		l = append(l, r.Name)
	}

	if !reflect.DeepEqual(l, result) {
		t.Errorf("balance error [%s] %v, expect %v", label, l, result)
	}
}

func processSimpleBalance(t *testing.T, label string, algor int, key []byte, rr *BalanceRR, result []string) {
	var l []string
	loopCount := (300 + 200 + 100) + 4

	for i := 1; i < loopCount; i++ {
		r, err := rr.Balance(algor, key)
		if err != nil {
			t.Errorf("should not error")
		}
		r.IncConnNum()
		// append the end of backend b3
		if (i > 297) && (i <= 303) {
			l = append(l, r.Name)
		}
		// append the end of backend b1
		if (i > 600) && (i <= 603) {
			l = append(l, r.Name)
		}
	}

	if !reflect.DeepEqual(l, result) {
		t.Errorf("balance error [%s] %v, expect %v", label, l, result)
	}
}

func processSimpleBalance3(t *testing.T, label string, algor int, key []byte, rr *BalanceRR, result []string) {
	var l []string
	loopCount := (200+100)*3 + 4

	for i := 1; i < loopCount; i++ {
		r, err := rr.Balance(algor, key)
		if err != nil {
			t.Errorf("should not error")
		}
		r.IncConnNum()
		// append the end of backend b3
		if (i > 198) && (i <= 201) {
			l = append(l, r.Name)
		}
		if (i > 498) && (i <= 501) {
			l = append(l, r.Name)
		}
		if (i > 798) && (i <= 801) {
			l = append(l, r.Name)
		}
	}

	if !reflect.DeepEqual(l, result) {
		t.Errorf("balance error [%s] %v, expect %v", label, l, result)
	}
}

func TestBalance(t *testing.T) {
	// case 1
	rr := prepareBalanceRR()
	expectResult := []string{"b1", "b2", "b3", "b1", "b2", "b1", "b1", "b2", "b3"}
	processSimpleBalance(t, "case 1", WrrSimple, nil, rr, expectResult)

	// case 2
	rr = prepareBalanceRR()
	expectResult = []string{"b1", "b2", "b1", "b3", "b2", "b1", "b1", "b2", "b1"}
	processBalance(t, "case 2", WrrSmooth, nil, rr, expectResult)

	// case 3
	rr = prepareBalanceRR()
	rr.backends[0].backend.SetAvail(false)
	expectResult = []string{"b2", "b3", "b2", "b2", "b3", "b2", "b2", "b3", "b2"}
	processSimpleBalance3(t, "case 3", WrrSimple, nil, rr, expectResult)

	// case 4
	rr = prepareBalanceRR()
	rr.backends[0].backend.SetAvail(false)
	expectResult = []string{"b2", "b3", "b2", "b2", "b3", "b2", "b2", "b3", "b2"}
	processBalance(t, "case 4", WrrSmooth, nil, rr, expectResult)

	// case 5
	rr = prepareBalanceRR()
	expectResult = []string{"b1", "b1", "b1", "b1", "b1", "b1", "b1", "b1", "b1"}
	processBalance(t, "case 5", WrrSticky, []byte{1}, rr, expectResult)

	rr.backends[0], rr.backends[2] = rr.backends[2], rr.backends[0]
	rr.sorted = false
	processBalance(t, "case 5", WrrSticky, []byte{1}, rr, expectResult)

	// case 6
	rr = prepareBalanceRR()
	rr.backends[0].backend.SetAvail(false)
	// after scale up 100, the hash result changed
	expectResult = []string{"b3", "b3", "b3", "b3", "b3", "b3", "b3", "b3", "b3"}
	//	expectResult = []string{"b2", "b2", "b2", "b2", "b2", "b2", "b2", "b2", "b2"}
	processBalance(t, "case 6", WrrSticky, []byte{1}, rr, expectResult)

	// case 7, lcw balance
	rr = prepareBalanceRR()
	expectResult = []string{"b1", "b2", "b3", "b1", "b2", "b1", "b2", "b3", "b1"}
	processBalance(t, "case 7", WlcSmooth, []byte{1}, rr, expectResult)

	// case 8, lcw balance same weight
	rr = prepareBalanceRRLcw()
	expectResult = []string{"b1", "b2", "b3", "b4", "b1", "b2", "b1", "b2", "b3", "b1", "b1",
		"b2", "b1", "b1", "b4", "b2", "b3", "b1", "b2"}
	processBalancLoopTwenty(t, "case 8", WlcSmooth, []byte{1}, rr, expectResult)
}

func TestUpdate(t *testing.T) {
	b1 := populateBackend("b1", "127.0.0.1", 80, true)
	b2 := populateBackend("b2", "127.0.0.1", 81, true)
	b3 := populateBackend("b3", "127.0.0.1", 82, true)
	rr := &BalanceRR{
		backends: []*BackendRR{
			{
				weight:  3,
				current: 3,
				backend: b1,
			},
			{
				weight:  2,
				current: 2,
				backend: b2,
			},
			{
				weight:  1,
				current: 1,
				backend: b3,
			},
		},
	}

	var newConf cluster_table_conf.SubClusterBackend
	buf := []byte(`[{"name":"b1", "Addr":"12", "Port":10, "weight":10}, {"name":"b2", "Addr":"127.0.0.1", "Port":81, "weight":20}, {"name":"b4", "Addr":"13", "Port":90, "weight":10}]`)
	if err := json.Unmarshal(buf, &newConf); err != nil {
		t.Errorf("unmarshal error")
	}

	rr.Update(newConf)
	if len(rr.backends) != 3 {
		t.Errorf("backend len %d", len(rr.backends))
	}

	b, _ := rr.Balance(WlcSmooth, []byte{1})
	b.IncConnNum()
	b, _ = rr.Balance(WlcSmooth, []byte{1})
	b.IncConnNum()

	for i := 0; i < len(rr.backends); i++ {
		brr := rr.backends[i]
		switch brr.backend.Name {
		case "b1":
			checkBackend(t, rr.backends[i], "b1", "12", 10, 10, -1)
		case "b2":
			checkBackend(t, rr.backends[i], "b2", "127.0.0.1", 81, 20, 1)
		case "b4":
			checkBackend(t, rr.backends[i], "b4", "13", 90, 10, -1)
		default:
			t.Errorf("should not contail backend %v", brr)
		}
	}
}

func checkBackend(t *testing.T, brr *BackendRR, name string, addr string, port int, weight int, connNum int) {
	b := brr.backend
	if b.Name != name {
		t.Errorf("backend name wrong, expect %s, actual %s", name, b.Name)
	}
	if b.Addr != addr {
		t.Errorf("backend addr wrong, expect %s, actual %s", addr, b.Addr)
	}
	if b.Port != port {
		t.Errorf("backend port wrong, expect %d, actual %d", port, b.Port)
	}
	if brr.weight != weight*100 {
		t.Errorf("backend weight wrong, expect %d, actual %d", weight, brr.weight)
	}
	if connNum != -1 && b.ConnNum() != connNum {
		t.Errorf("backend connNum wrong, expect %d, actual %d", connNum, b.ConnNum())
	}
}

func prepareBalanceRRForBench() *BalanceRR {
	rr := new(BalanceRR)
	rr.backends = make([]*BackendRR, 0)
	for i := 0; i < 100; i++ {
		addr := fmt.Sprintf("10.10.0.%d", i)
		backendRR := new(BackendRR)
		backendRR.weight = 1 + rand.Intn(5)
		backendRR.current = backendRR.weight
		backendRR.backend = populateBackend(addr, addr, 80, true)
		rr.backends = append(rr.backends, backendRR)
	}
	return rr
}

func BenchmarkSmoothBalance(b *testing.B) {
	rr := prepareBalanceRRForBench()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr.smoothBalance()
	}
}

func BenchmarkSimpleBalance(b *testing.B) {
	rr := prepareBalanceRRForBench()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr.simpleBalance()
	}
}

func BenchmarkWlcBalance(b *testing.B) {
	rr := prepareBalanceRRForBench()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr.leastConnsSmoothBalance()
	}
}

func BenchmarkStickyBalance(b *testing.B) {
	rr := prepareBalanceRRForBench()
	key := []byte{100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr.stickyBalance(key)
	}
}

func TestSlowStart(t *testing.T) {
	// case 1
	rr := prepareBalanceRR()
	rr.SetSlowStart(30)
}

func prepareBackendRR(name string, port int, weight int, connNum int, avail bool) *BackendRR {
	b := populateBackend(name, "127.0.0.1", port, avail)
	for i := 0; i < connNum; i++ {
		b.IncConnNum()
	}
	return &BackendRR{
		weight:  weight,
		current: weight,
		backend: b,
	}
}

func candidateNames(backs BackendList) []string {
	names := make([]string, 0, len(backs))
	for _, b := range backs {
		names = append(names, b.backend.Name)
	}
	return names
}

func TestLeastConnsBalance(t *testing.T) {
	// case 1: all backends have equal connNum/weight, return all candidates
	candidates, err := leastConnsBalance(BackendList{
		prepareBackendRR("b1", 80, 100, 0, true),
		prepareBackendRR("b2", 81, 100, 0, true),
		prepareBackendRR("b3", 82, 100, 0, true),
	})
	if err != nil {
		t.Errorf("case 1 should not error, got %v", err)
	}
	if !reflect.DeepEqual(candidateNames(candidates), []string{"b1", "b2", "b3"}) {
		t.Errorf("case 1 wrong candidates, expect [b1 b2 b3], got %v", candidateNames(candidates))
	}

	// case 2: single backend with min connNum/weight
	candidates, err = leastConnsBalance(BackendList{
		prepareBackendRR("b1", 80, 100, 0, true),
		prepareBackendRR("b2", 81, 100, 1, true),
		prepareBackendRR("b3", 82, 100, 2, true),
	})
	if err != nil {
		t.Errorf("case 2 should not error, got %v", err)
	}
	if !reflect.DeepEqual(candidateNames(candidates), []string{"b1"}) {
		t.Errorf("case 2 wrong candidates, expect [b1], got %v", candidateNames(candidates))
	}

	// case 3: new best found in the middle of loop
	candidates, err = leastConnsBalance(BackendList{
		prepareBackendRR("b1", 80, 100, 5, true),
		prepareBackendRR("b2", 81, 100, 3, true),
		prepareBackendRR("b3", 82, 100, 1, true),
	})
	if err != nil {
		t.Errorf("case 3 should not error, got %v", err)
	}
	if !reflect.DeepEqual(candidateNames(candidates), []string{"b3"}) {
		t.Errorf("case 3 wrong candidates, expect [b3], got %v", candidateNames(candidates))
	}

	// case 4: skip ineligible backends (weight <= 0, avail false)
	candidates, err = leastConnsBalance(BackendList{
		prepareBackendRR("b1", 80, 100, 0, true),
		prepareBackendRR("b2", 81, 0, 0, true),
		prepareBackendRR("b3", 82, 100, 0, false),
	})
	if err != nil {
		t.Errorf("case 4 should not error, got %v", err)
	}
	if !reflect.DeepEqual(candidateNames(candidates), []string{"b1"}) {
		t.Errorf("case 4 wrong candidates, expect [b1], got %v", candidateNames(candidates))
	}

	// case 5: all backends ineligible, return error
	candidates, err = leastConnsBalance(BackendList{
		prepareBackendRR("b1", 80, 100, 0, false),
		prepareBackendRR("b2", 81, 0, 0, true),
	})
	if err == nil {
		t.Errorf("case 5 should error, got candidates %v", candidateNames(candidates))
	}

	// case 6: equal ratio with different weight
	candidates, err = leastConnsBalance(BackendList{
		prepareBackendRR("b1", 80, 100, 1, true),
		prepareBackendRR("b2", 81, 200, 2, true),
		prepareBackendRR("b3", 82, 100, 3, true),
	})
	if err != nil {
		t.Errorf("case 6 should not error, got %v", err)
	}
	if !reflect.DeepEqual(candidateNames(candidates), []string{"b1", "b2"}) {
		t.Errorf("case 6 wrong candidates, expect [b1 b2], got %v", candidateNames(candidates))
	}

	// case 7: equal ratio discovered after new best
	candidates, err = leastConnsBalance(BackendList{
		prepareBackendRR("b1", 80, 100, 5, true),
		prepareBackendRR("b2", 81, 100, 1, true),
		prepareBackendRR("b3", 82, 200, 2, true),
	})
	if err != nil {
		t.Errorf("case 7 should not error, got %v", err)
	}
	if !reflect.DeepEqual(candidateNames(candidates), []string{"b2", "b3"}) {
		t.Errorf("case 7 wrong candidates, expect [b2 b3], got %v", candidateNames(candidates))
	}

	// case 8: all weight equal to 1, equal connNum, return all candidates
	candidates, err = leastConnsBalance(BackendList{
		prepareBackendRR("b1", 80, 1, 0, true),
		prepareBackendRR("b2", 81, 1, 0, true),
		prepareBackendRR("b3", 82, 1, 0, true),
	})
	if err != nil {
		t.Errorf("case 8 should not error, got %v", err)
	}
	if !reflect.DeepEqual(candidateNames(candidates), []string{"b1", "b2", "b3"}) {
		t.Errorf("case 8 wrong candidates, expect [b1 b2 b3], got %v", candidateNames(candidates))
	}

	// case 9: all weight equal to 1, min connNum is first backend
	candidates, err = leastConnsBalance(BackendList{
		prepareBackendRR("b1", 80, 1, 0, true),
		prepareBackendRR("b2", 81, 1, 1, true),
		prepareBackendRR("b3", 82, 1, 2, true),
	})
	if err != nil {
		t.Errorf("case 9 should not error, got %v", err)
	}
	if !reflect.DeepEqual(candidateNames(candidates), []string{"b1"}) {
		t.Errorf("case 9 wrong candidates, expect [b1], got %v", candidateNames(candidates))
	}

	// case 10: all weight equal to 1, tie at min connNum
	candidates, err = leastConnsBalance(BackendList{
		prepareBackendRR("b1", 80, 1, 2, true),
		prepareBackendRR("b2", 81, 1, 1, true),
		prepareBackendRR("b3", 82, 1, 1, true),
	})
	if err != nil {
		t.Errorf("case 10 should not error, got %v", err)
	}
	if !reflect.DeepEqual(candidateNames(candidates), []string{"b2", "b3"}) {
		t.Errorf("case 10 wrong candidates, expect [b2 b3], got %v", candidateNames(candidates))
	}

	// case 11: all weight equal to 1, skip ineligible backend
	candidates, err = leastConnsBalance(BackendList{
		prepareBackendRR("b1", 80, 1, 0, false),
		prepareBackendRR("b2", 81, 1, 0, true),
		prepareBackendRR("b3", 82, 1, 1, true),
	})
	if err != nil {
		t.Errorf("case 11 should not error, got %v", err)
	}
	if !reflect.DeepEqual(candidateNames(candidates), []string{"b2"}) {
		t.Errorf("case 11 wrong candidates, expect [b2], got %v", candidateNames(candidates))
	}

	// case 12: all weight equal to 1, min connNum discovered in the middle of loop
	candidates, err = leastConnsBalance(BackendList{
		prepareBackendRR("b1", 80, 1, 3, true),
		prepareBackendRR("b2", 81, 1, 1, true),
		prepareBackendRR("b3", 82, 1, 2, true),
	})
	if err != nil {
		t.Errorf("case 12 should not error, got %v", err)
	}
	if !reflect.DeepEqual(candidateNames(candidates), []string{"b2"}) {
		t.Errorf("case 12 wrong candidates, expect [b2], got %v", candidateNames(candidates))
	}
}

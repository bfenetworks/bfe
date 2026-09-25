// Copyright (c) 2026 The BFE Authors.
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

package mod_traffic_mirror

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_balance"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
)

func newTestSenderConf() *ConfModTrafficMirror {
	conf := &ConfModTrafficMirror{}
	conf.Basic.ConnectTimeoutMs = 500
	conf.Basic.TTFBTimeoutMs = 2000
	conf.Basic.TotalTimeoutMs = 5000
	conf.Basic.MaxMirrorBodyBytes = DefaultMaxMirrorBodyBytes
	conf.Basic.MaxResponseBodyBytes = DefaultMaxResponseBodyBytes
	conf.Basic.MaxConcurrent = 2
	conf.Basic.QueueCapacity = 4
	conf.Basic.CircuitBreakerFailThreshold = 2
	conf.Basic.CircuitBreakerCooldownSec = 60
	return conf
}

func newTestSender(conf *ConfModTrafficMirror) *mirrorSender {
	table := newMirrorRuleTable()
	breaker := newMirrorCircuitBreaker(
		conf.Basic.CircuitBreakerFailThreshold, conf.Basic.CircuitBreakerCooldownSec)
	pms := newPrometheusStates()
	return newMirrorSender(conf, table, breaker, pms)
}

// setupGlobalBalTable registers a balance table with one cluster pointing at
// the given address, so resolveTarget can pick a backend.
func setupGlobalBalTable(t *testing.T, cluster string, addr string, port int) {
	dir := t.TempDir()
	gslbFile := filepath.Join(dir, "gslb.data")
	clusterTableFile := filepath.Join(dir, "cluster_table.data")

	gslbConf := fmt.Sprintf(`{
		"Clusters": {"%s": {"sub0": 100, "GSLB_BLACKHOLE": 0}},
		"Hostname": "testhost",
		"Ts": "20260924000000"
	}`, cluster)
	clusterTableConf := fmt.Sprintf(`{
		"Version": "1.0",
		"Config": {"%s": {"sub0": [{"name": "b0", "addr": "%s", "port": %d, "weight": 10}]}}
	}`, cluster, addr, port)

	if err := ioutil.WriteFile(gslbFile, []byte(gslbConf), 0644); err != nil {
		t.Fatalf("write gslb conf: %v", err)
	}
	if err := ioutil.WriteFile(clusterTableFile, []byte(clusterTableConf), 0644); err != nil {
		t.Fatalf("write cluster table conf: %v", err)
	}

	fetcher := func(name string) (*cluster_conf.BackendCheck, *cluster_conf.BackendHTTPS) {
		return nil, nil
	}
	balTable := bfe_balance.NewBalTable(fetcher)
	if err := balTable.Init(gslbFile, clusterTableFile); err != nil {
		t.Fatalf("balTable init: %v", err)
	}
	bfe_balance.SetGlobalBalTable(balTable)
}

func newSenderTestTask(cluster string) *mirrorTask {
	return &mirrorTask{
		Method:  http.MethodPost,
		URI:     "/v1/chat/completions",
		Host:    "example.com",
		Header:  http.Header{"Content-Type": []string{"application/json"}},
		Body:    []byte(`{"model":"gpt-4o"}`),
		Cluster: cluster,
		Product: "default",
		Model:   "gpt-4o",
		StartAt: time.Now(),
	}
}

func TestExecEndToEndSSE(t *testing.T) {
	const cluster = "cluster_shadow_e2e"

	received := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("mirror path = %s", r.URL.Path)
		}
		if !strings.Contains(string(body), "gpt-4o") {
			t.Errorf("mirror body = %s", body)
		}
		received <- r

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseFullStream)
	}))
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("parse server addr: %v", err)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatalf("parse server port: %v", err)
	}
	setupGlobalBalTable(t, cluster, host, port)

	sender := newTestSender(newTestSenderConf())
	sender.exec(newSenderTestTask(cluster))

	// the mirror request must have reached the target
	select {
	case r := <-received:
		if r.Host != "example.com" {
			t.Errorf("mirror Host = %s, want example.com", r.Host)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("mirror Content-Type header missing")
		}
	default:
		t.Fatal("mirror target did not receive the request")
	}

	snap := sender.table.snapshotCounters()
	if snap.reqTotal != 1 {
		t.Errorf("reqTotal = %d, want 1", snap.reqTotal)
	}
	if snap.sendFail != 0 {
		t.Errorf("sendFail = %d, want 0", snap.sendFail)
	}
	if sender.breaker.Open(cluster) {
		t.Error("circuit should stay closed after success")
	}
}

func TestExecResolveFailureOpensCircuit(t *testing.T) {
	conf := newTestSenderConf()
	conf.Basic.CircuitBreakerFailThreshold = 1
	sender := newTestSender(conf)

	// no global balTable registered (or cluster missing): resolve fails; the
	// first failure opens the circuit (threshold 1), the second exec is
	// dropped by the circuit breaker
	bfe_balance.SetGlobalBalTable(nil)
	task := newSenderTestTask("cluster_no_such_cluster")

	sender.exec(task)
	sender.exec(task)

	snap := sender.table.snapshotCounters()
	if snap.sendFail != 1 {
		t.Errorf("sendFail = %d, want 1", snap.sendFail)
	}
	if snap.circuitOpen != 1 {
		t.Errorf("circuitOpen = %d, want 1", snap.circuitOpen)
	}
	if !sender.breaker.Open(task.Cluster) {
		t.Error("circuit should open after reaching the failure threshold")
	}
}

func TestExecCircuitOpenDropsFast(t *testing.T) {
	conf := newTestSenderConf()
	sender := newTestSender(conf)

	// pre-open the circuit
	sender.breaker.OnFail("cluster_open")
	sender.breaker.OnFail("cluster_open")

	sender.exec(newSenderTestTask("cluster_open"))

	snap := sender.table.snapshotCounters()
	if snap.circuitOpen != 1 {
		t.Errorf("circuitOpen = %d, want 1", snap.circuitOpen)
	}
	if snap.sendFail != 0 {
		t.Errorf("sendFail = %d, want 0 (dropped before send)", snap.sendFail)
	}
}

func TestExecNonStreamResponse(t *testing.T) {
	const cluster = "cluster_shadow_json"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
	}))
	defer server.Close()

	host, portStr, _ := net.SplitHostPort(server.Listener.Addr().String())
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	setupGlobalBalTable(t, cluster, host, port)

	sender := newTestSender(newTestSenderConf())
	sender.exec(newSenderTestTask(cluster))

	snap := sender.table.snapshotCounters()
	if snap.reqTotal != 1 || snap.sendFail != 0 {
		t.Errorf("reqTotal=%d sendFail=%d, want 1/0", snap.reqTotal, snap.sendFail)
	}
}

func TestSubmitDropsWhenQueueFull(t *testing.T) {
	sender := newTestSender(newTestSenderConf())
	sender.queue = make(chan *mirrorTask, 1)

	if !sender.Submit(&mirrorTask{Cluster: "c"}) {
		t.Fatal("first submit should succeed")
	}
	if sender.Submit(&mirrorTask{Cluster: "c"}) {
		t.Error("second submit should be dropped when queue is full")
	}
}

func TestMain(m *testing.M) {
	// make sure no global balTable leaks between tests
	bfe_balance.SetGlobalBalTable(nil)
	os.Exit(m.Run())
}

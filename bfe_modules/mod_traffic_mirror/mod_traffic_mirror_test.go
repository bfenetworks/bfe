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
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

// prepareTestModule builds a module with the test rule table. Workers are
// not started, so submitted tasks stay in the queue for inspection.
func prepareTestModule(t *testing.T) *ModuleTrafficMirror {
	m := NewModuleTrafficMirror()
	conf, err := ConfLoad("testdata/mod_traffic_mirror/mod_traffic_mirror.conf", "./testdata")
	if err != nil {
		t.Fatalf("ConfLoad failed: %v", err)
	}
	m.conf = conf
	m.breaker = newMirrorCircuitBreaker(
		conf.Basic.CircuitBreakerFailThreshold, conf.Basic.CircuitBreakerCooldownSec)
	m.sender = newMirrorSender(conf, m.ruleTable, m.breaker, m.pmsStates)

	m.conf.Basic.ProductRulePath = "testdata/mod_traffic_mirror/mirror_rule.data"
	if _, err := m.loadProductRuleTable(nil); err != nil {
		t.Fatalf("loadProductRuleTable failed: %v", err)
	}
	return m
}

func newMirrorTestRequest(product, body string) *bfe_basic.Request {
	httpReq, _ := bfe_http.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions",
		ioutil.NopCloser(bytes.NewBufferString(body)))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer secret")
	httpReq.Header.Set("Cookie", "session=abc")
	httpReq.Host = "example.com"
	req := bfe_basic.NewRequest(httpReq, nil, nil, &bfe_basic.Session{}, nil)
	req.Route = bfe_basic.RequestRoute{Product: product}
	req.LogId = "logid-123"
	req.InitAiBasicInfo()
	return req
}

func chatBodyModel(model string) string {
	return `{"model":"` + model + `","messages":[{"role":"user","content":"hi"}]}`
}

func TestMirrorHandlerMatchAndSubmit(t *testing.T) {
	m := prepareTestModule(t)
	req := newMirrorTestRequest("default", chatBodyModel("gpt-4o"))

	ret := m.mirrorHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("handler should always return BfeHandlerGoOn, got %d", ret)
	}

	aiInfo := req.GetAiBasicInfo()
	if aiInfo == nil || !aiInfo.MirrorHit {
		t.Error("MirrorHit should be true after submit")
	}
	if aiInfo.MirrorCluster != "cluster_shadow_v2" {
		t.Errorf("MirrorCluster = %s, want cluster_shadow_v2", aiInfo.MirrorCluster)
	}

	if len(m.sender.queue) != 1 {
		t.Fatalf("queue should hold 1 task, got %d", len(m.sender.queue))
	}
	task := <-m.sender.queue

	// header handling (FR-5)
	if task.Header.Get("Authorization") != "" {
		t.Error("Authorization should be stripped")
	}
	if task.Header.Get("Cookie") != "" {
		t.Error("Cookie should be stripped")
	}
	if task.Header.Get(DefaultMirrorHeader) != MirrorHeaderValue {
		t.Error("X-Bfe-Mirror header should be injected")
	}
	if task.Header.Get("X-Bfe-Logid") != "logid-123" {
		t.Error("X-Bfe-Logid should be injected")
	}
	if task.Header.Get("X-Env") != "shadow" {
		t.Error("rule SetHeaders should be injected")
	}

	// body model rewrite (FR-6)
	if !bytes.Contains(task.Body, []byte(`"model":"deepseek-v3"`)) {
		t.Errorf("mirror body should rewrite model, got %s", task.Body)
	}

	// fallback dedup marker
	if req.GetContext(CtxMirrored) == nil {
		t.Error("CtxMirrored should be set after submit")
	}
}

func TestMirrorHandlerFallbackRetryDedup(t *testing.T) {
	m := prepareTestModule(t)
	req := newMirrorTestRequest("default", chatBodyModel("gpt-4o"))

	m.mirrorHandler(req)
	m.mirrorHandler(req) // simulates a fallback retry re-entering HandleForward

	if len(m.sender.queue) != 1 {
		t.Errorf("request should be mirrored only once, queue = %d", len(m.sender.queue))
	}
}

func TestMirrorHandlerSamplingZero(t *testing.T) {
	m := prepareTestModule(t)
	req := newMirrorTestRequest("default", chatBodyModel("deepseek-chat"))

	m.mirrorHandler(req)

	if len(m.sender.queue) != 0 {
		t.Errorf("percentage-0 rule should not mirror, queue = %d", len(m.sender.queue))
	}
	snap := m.ruleTable.snapshotCounters()
	if snap.skipSample != 1 {
		t.Errorf("skipSample = %d, want 1", snap.skipSample)
	}
}

func TestMirrorHandlerProductMiss(t *testing.T) {
	m := prepareTestModule(t)
	req := newMirrorTestRequest("unknown_product", chatBodyModel("gpt-4o"))

	m.mirrorHandler(req)

	if len(m.sender.queue) != 0 {
		t.Errorf("unknown product should not mirror, queue = %d", len(m.sender.queue))
	}
	if req.GetContext(CtxMirrored) != nil {
		t.Error("CtxMirrored should not be set when no rule matched")
	}
}

func TestMirrorHandlerBodyLimit(t *testing.T) {
	m := prepareTestModule(t)
	m.conf.Basic.MaxMirrorBodyBytes = 8 // smaller than the request body
	req := newMirrorTestRequest("default", chatBodyModel("gpt-4o"))

	m.mirrorHandler(req)

	if len(m.sender.queue) != 0 {
		t.Errorf("oversized body should be skipped, queue = %d", len(m.sender.queue))
	}
	snap := m.ruleTable.snapshotCounters()
	if snap.skipBodyLimit != 1 {
		t.Errorf("skipBodyLimit = %d, want 1", snap.skipBodyLimit)
	}
}

func TestMirrorHandlerRewriteFallback(t *testing.T) {
	m := prepareTestModule(t)

	// rewrite-only rule: the request body has no model field, so the
	// rewrite fails and the original body is mirrored instead
	rules := MirrorRuleConfList{
		{
			Cond:          "",
			MirrorCluster: "cluster_shadow_rw",
			Percentage:    100,
			BodyRewrites:  []*MirrorBodyRewriteConf{{Path: "model", Value: "deepseek-v3"}},
		},
	}
	if err := m.ruleTable.load(&MirrorRuleConfData{
		Version: strPtr("test"),
		Config:  &map[string]*MirrorRuleConfList{"default": &rules},
	}); err != nil {
		t.Fatalf("rule table load failed: %v", err)
	}

	req := newMirrorTestRequest("default", `{"messages":[{"role":"user","content":"hi"}]}`)
	m.mirrorHandler(req)

	snap := m.ruleTable.snapshotCounters()
	if snap.skipRewrite != 1 {
		t.Errorf("skipRewrite = %d, want 1", snap.skipRewrite)
	}
	if len(m.sender.queue) != 1 {
		t.Fatalf("request should still be mirrored with original body, queue = %d", len(m.sender.queue))
	}
	task := <-m.sender.queue
	if !bytes.Contains(task.Body, []byte(`"messages"`)) {
		t.Errorf("fallback body should keep original content, got %s", task.Body)
	}
}

func TestMirrorHandlerSubmitDropWhenQueueFull(t *testing.T) {
	m := prepareTestModule(t)
	m.sender.queue = make(chan *mirrorTask, 1) // shrink the queue
	m.sender.Submit(&mirrorTask{Cluster: "dummy"})

	req := newMirrorTestRequest("default", chatBodyModel("gpt-4o"))
	m.mirrorHandler(req)

	snap := m.ruleTable.snapshotCounters()
	if snap.submitDrop != 1 {
		t.Errorf("submitDrop = %d, want 1", snap.submitDrop)
	}
	// main request must be unaffected
	aiInfo := req.GetAiBasicInfo()
	if aiInfo == nil || aiInfo.MirrorHit {
		t.Error("MirrorHit should stay false when submit is dropped")
	}
}

func TestSampleHit(t *testing.T) {
	if !sampleHit(100) {
		t.Error("100 should always hit")
	}
	if sampleHit(0) {
		t.Error("0 should never hit")
	}
	if sampleHit(-1) {
		t.Error("negative percentage should never hit")
	}

	// 50% should hit roughly half over many samples
	hits := 0
	for i := 0; i < 2000; i++ {
		if sampleHit(50) {
			hits++
		}
	}
	if hits < 700 || hits > 1300 {
		t.Errorf("50%% sampling hit %d/2000, want roughly 1000", hits)
	}
}

func strPtr(s string) *string {
	return &s
}

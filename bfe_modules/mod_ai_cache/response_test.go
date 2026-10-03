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

package mod_ai_cache

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_modules/mod_ai_cache/provider/vector"
)

func TestExtractAnswerNonStream(t *testing.T) {
	ctx := &aiCacheContext{
		Stream: false,
		Rule: &ProductRuleConf{
			CacheValueFrom:  DefaultCacheValuePath,
			CacheStreamFrom: DefaultCacheStreamPath,
		},
	}

	body := []byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"the answer"}}]}`)
	if got := extractAnswer(body, ctx); got != "the answer" {
		t.Errorf("expected %q, got %q", "the answer", got)
	}

	if got := extractAnswer([]byte(`not json`), ctx); got != "" {
		t.Errorf("invalid json should give empty answer, got %q", got)
	}
}

func TestExtractAnswerStream(t *testing.T) {
	ctx := &aiCacheContext{
		Stream: true,
		Rule: &ProductRuleConf{
			CacheValueFrom:  DefaultCacheValuePath,
			CacheStreamFrom: DefaultCacheStreamPath,
		},
	}

	body := []byte("data:{\"choices\":[{\"index\":0,\"delta\":{\"content\":\"streamed\"}}]}\n\n" +
		"data:[DONE]\n\n")
	if got := extractAnswer(body, ctx); got != "streamed" {
		t.Errorf("expected %q, got %q", "streamed", got)
	}
}

func TestCacheCaptureBodyPassthroughAndComplete(t *testing.T) {
	src := io.NopCloser(bytes.NewReader([]byte("hello world")))
	var captured []byte
	var complete bool

	body := newCacheCaptureBody(src, 1024, func(b []byte, c bool) {
		captured = append(captured, b...)
		complete = c
	})

	got, err := ioutil.ReadAll(body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("passthrough mismatch: %q", got)
	}

	if err := body.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if !complete {
		t.Error("complete should be true after reading to EOF")
	}
	if string(captured) != "hello world" {
		t.Errorf("captured mismatch: %q", captured)
	}
}

func TestCacheCaptureBodyIncompleteOnAbort(t *testing.T) {
	src := io.NopCloser(&errorReader{})
	var complete = true

	body := newCacheCaptureBody(src, 1024, func(b []byte, c bool) {
		complete = c
	})

	_, err := ioutil.ReadAll(body)
	if err == nil {
		t.Fatal("expected read error")
	}
	_ = body.Close()

	if complete {
		t.Error("complete should be false when the read fails")
	}
}

func TestCacheCaptureBodyLimit(t *testing.T) {
	src := io.NopCloser(bytes.NewReader([]byte("0123456789")))
	var captured []byte

	body := newCacheCaptureBody(src, 4, func(b []byte, c bool) {
		captured = append(captured, b...)
	})

	if _, err := ioutil.ReadAll(body); err != nil {
		t.Fatalf("read failed: %v", err)
	}
	_ = body.Close()

	if string(captured) != "0123" {
		t.Errorf("accumulation should stop at the limit, got %q", captured)
	}
}

func TestCacheResponseHandlerNoContext(t *testing.T) {
	m := prepareTestModule(t)
	req := newTestRequest("default", chatBody("q", false))
	req.InitAiBasicInfo()

	res := &bfe_http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte("x")))}
	if ret := m.cacheResponseHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected go on without cache context, got %d", ret)
	}
}

func TestCacheResponseHandlerWrapsAndWritesBack(t *testing.T) {
	fake := newFakeRedisClient("")
	m := prepareTestModule(t)
	m.redisCache = newRedisCache(m.name, fake, m.ruleTable)

	req := newTestRequest("default", chatBody("q", false))
	req.InitAiBasicInfo().ClientKeyId = "key_001"

	// request phase saves the context
	if _, _ = m.cacheRequestHandler(req); getAiCacheContext(req) == nil {
		t.Fatal("cache context expected")
	}
	ctx := getAiCacheContext(req)

	res := &bfe_http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"choices":[{"message":{"content":"answer from upstream"}}]}`))),
	}
	if ret := m.cacheResponseHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected go on, got %d", ret)
	}

	// simulate the reverse proxy reading the body to the client
	if _, err := ioutil.ReadAll(res.Body); err != nil {
		t.Fatalf("response read failed: %v", err)
	}
	if err := res.Body.Close(); err != nil {
		t.Fatalf("response close failed: %v", err)
	}

	if fake.setexCalls != 1 {
		t.Fatalf("expected 1 setex, got %d", fake.setexCalls)
	}
	if fake.lastSetKey != ctx.Key {
		t.Errorf("setex key mismatch: %s vs %s", fake.lastSetKey, ctx.Key)
	}
	if string(fake.lastSetVal) != "answer from upstream" {
		t.Errorf("setex value mismatch: %q", fake.lastSetVal)
	}
	if fake.lastSetTTL != 60 {
		t.Errorf("expected TTL 60, got %d", fake.lastSetTTL)
	}
}

func TestCacheResponseHandlerSkipStatus(t *testing.T) {
	fake := newFakeRedisClient("")
	m := prepareTestModule(t)
	m.redisCache = newRedisCache(m.name, fake, m.ruleTable)

	req := newTestRequest("default", chatBody("q", false))
	req.InitAiBasicInfo().ClientKeyId = "key_001"
	req.HttpRequest.Header.Set(HeaderSkipAiCache, SkipCacheOn)

	// skip header: no context is saved, so the response phase does nothing
	if _, _ = m.cacheRequestHandler(req); getAiCacheContext(req) != nil {
		t.Fatal("no cache context expected for skip")
	}

	res := &bfe_http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte("x")))}
	_ = m.cacheResponseHandler(req, res)
	_, _ = ioutil.ReadAll(res.Body)
	_ = res.Body.Close()

	if fake.setexCalls != 0 {
		t.Error("skip request should never write cache")
	}
}

type errorReader struct{}

func (e *errorReader) Read(p []byte) (int, error) {
	return 0, errors.New("simulated upstream abort")
}

// waitFor polls cond until it returns true or the deadline expires.
func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func TestCacheResponseHandlerSemanticUpload(t *testing.T) {
	fake := newFakeRedisClient("")
	emb := &fakeEmbeddingProvider{vec: []float32{0.1, 0.2}}
	vec := &fakeVectorProvider{} // semantic miss: no neighbors
	m := prepareSemanticTestModule(t, emb, vec)
	m.redisCache = newRedisCache(m.name, fake, m.ruleTable)

	req := newTestRequest("default", chatBody("what is bfe", false))
	req.InitAiBasicInfo().ClientKeyId = "key_001"
	if _, _ = m.cacheRequestHandler(req); getAiCacheContext(req) == nil {
		t.Fatal("cache context expected")
	}

	res := &bfe_http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"choices":[{"message":{"content":"answer from upstream"}}]}`))),
	}
	if ret := m.cacheResponseHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected go on, got %d", ret)
	}
	if _, err := ioutil.ReadAll(res.Body); err != nil {
		t.Fatalf("response read failed: %v", err)
	}
	if err := res.Body.Close(); err != nil {
		t.Fatalf("response close failed: %v", err)
	}

	// redis write-back is synchronous
	if fake.setexCalls != 1 {
		t.Fatalf("expected 1 setex, got %d", fake.setexCalls)
	}

	// vector upload is asynchronous and reuses the request-phase embedding
	if !waitFor(func() bool { return vec.uploadCalls == 1 }) {
		t.Fatal("async vector upload did not happen")
	}
	if emb.calls != 1 {
		t.Errorf("embedding must be computed exactly once (reused at write-back), got %d", emb.calls)
	}
	item := vec.lastUpload
	if item.ID != vector.ItemID("key_001", "what is bfe") {
		t.Errorf("upload id must be deterministic, got %s", item.ID)
	}
	if item.Tenant != "key_001" || item.Question != "what is bfe" || item.Answer != "answer from upstream" {
		t.Errorf("unexpected upload item: %+v", item)
	}
	if len(item.Embedding) != 2 || item.Embedding[0] != 0.1 {
		t.Errorf("upload must reuse the request-phase embedding, got %v", item.Embedding)
	}
}

func TestCacheResponseHandlerNoEmbeddingNoUpload(t *testing.T) {
	fake := newFakeRedisClient("")
	emb := &fakeEmbeddingProvider{err: errors.New("embedding down")} // embedding unavailable
	vec := &fakeVectorProvider{}
	m := prepareSemanticTestModule(t, emb, vec)
	m.redisCache = newRedisCache(m.name, fake, m.ruleTable)

	req := newTestRequest("default", chatBody("what is bfe", false))
	req.InitAiBasicInfo().ClientKeyId = "key_001"
	if _, _ = m.cacheRequestHandler(req); getAiCacheContext(req) == nil {
		t.Fatal("cache context expected")
	}
	if getAiCacheContext(req).Embedding != nil {
		t.Fatal("embedding should be nil after the embedding failure")
	}

	res := &bfe_http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"choices":[{"message":{"content":"answer from upstream"}}]}`))),
	}
	_ = m.cacheResponseHandler(req, res)
	_, _ = ioutil.ReadAll(res.Body)
	_ = res.Body.Close()

	if fake.setexCalls != 1 {
		t.Errorf("redis write-back must still happen, got %d setex", fake.setexCalls)
	}
	time.Sleep(50 * time.Millisecond)
	if vec.uploadCalls != 0 {
		t.Error("no vector upload without a computed embedding (no recomputation)")
	}
}

func TestCacheResponseHandlerUploadFailureSwallowed(t *testing.T) {
	fake := newFakeRedisClient("")
	emb := &fakeEmbeddingProvider{vec: []float32{0.1}}
	vec := &fakeVectorProvider{err: errors.New("vector down")}
	m := prepareSemanticTestModule(t, emb, vec)
	m.redisCache = newRedisCache(m.name, fake, m.ruleTable)

	req := newTestRequest("default", chatBody("what is bfe", false))
	req.InitAiBasicInfo().ClientKeyId = "key_001"
	if _, _ = m.cacheRequestHandler(req); getAiCacheContext(req) == nil {
		t.Fatal("cache context expected")
	}

	res := &bfe_http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"choices":[{"message":{"content":"answer from upstream"}}]}`))),
	}
	_ = m.cacheResponseHandler(req, res)
	_, _ = ioutil.ReadAll(res.Body)
	_ = res.Body.Close()

	// the upload failure is counted and swallowed: the response was already
	// delivered, nothing may panic or propagate
	if !waitFor(func() bool { return m.ruleTable.snapshotCounters().vectorErr >= 1 }) {
		t.Error("VECTOR_ERR counter should be incremented on upload failure")
	}
	if fake.setexCalls != 1 {
		t.Error("redis write-back must still happen")
	}
}

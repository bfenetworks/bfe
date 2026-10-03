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

package mod_ai_context

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func prepareTestModule(t *testing.T) *ModuleAiContext {
	m := NewModuleAiContext()
	m.productConfPath = "testdata/mod_ai_context/context_rule.data"
	if _, err := m.loadProductRuleTable(nil); err != nil {
		t.Fatalf("loadProductRuleTable failed: %s", err)
	}
	return m
}

func newAIRequest(t *testing.T, product, body string) *bfe_basic.Request {
	httpReq, err := bfe_http.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions",
		ioutil.NopCloser(bytes.NewBufferString(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Content-Length", strconv.Itoa(len(body)))
	httpReq.ContentLength = int64(len(body))

	req := bfe_basic.NewRequest(httpReq, nil, nil, nil, nil)
	req.Route = bfe_basic.RequestRoute{Product: product}

	// buffer the body on the original request up front, mirroring
	// reverseproxy prepareRequestBodyForRetry / mod_ai_cache: the OutRequest
	// shallow copy below then SHARES the bytes_body with HttpRequest
	if _, err := httpReq.GetBodyAccessor(); err != nil {
		t.Fatalf("wrap request body failed: %v", err)
	}

	// mirror reverseproxy: OutRequest is a shallow copy of HttpRequest, so
	// the body object and the header maps are SHARED with the original
	outReq := new(bfe_http.Request)
	*outReq = *httpReq
	req.OutRequest = outReq
	return req
}

// aiChatBody builds a chat completions body with an oversized tool result.
func aiChatBody(toolResultLen int) string {
	return `{"model":"gpt-test","stream":false,"messages":[` +
		`{"role":"system","content":"sys"},` +
		`{"role":"assistant","content":"a1","tool_calls":[{"id":"c1","type":"function","function":{"name":"f","arguments":"{}"}}]},` +
		`{"role":"tool","tool_call_id":"c1","content":"` + strings.Repeat("x", toolResultLen) + `"},` +
		`{"role":"user","content":"current question"}` +
		`]}`
}

func initAIInfo(req *bfe_basic.Request) *bfe_basic.AiBasicInfo {
	ai := req.InitAiBasicInfo()
	ai.Mode = bfe_basic.ModeChat
	ai.AuthStyle = bfe_basic.AuthStyleOpenAI
	ai.TargetModel = "gpt-test"
	return ai
}

func outRequestBody(t *testing.T, req *bfe_basic.Request) string {
	accessor, err := req.OutRequest.GetBodyAccessor()
	require.NoError(t, err)
	body, _ := accessor.GetBytes()
	return string(body)
}

func TestHandlerCompressFlow(t *testing.T) {
	m := prepareTestModule(t)

	// tiny_budget_product: maxContextTokens=1000, reserve=100 => budget 900,
	// trigger 630; a 4000-char tool result (~1000 tokens) exceeds it
	body := aiChatBody(4000)
	req := newAIRequest(t, "tiny_budget_product", body)
	basic := initAIInfo(req)

	ret, res := m.contextCompressHandler(req)
	assert.Equal(t, bfe_module.BfeHandlerGoOn, ret)
	assert.Nil(t, res)

	// status and AiBasicInfo fields
	assert.Equal(t, StatusTrim, basic.ContextCompressStatus)
	assert.Equal(t, ModeBalanced, basic.ContextCompressMode)
	assert.True(t, basic.ContextTokensBefore > basic.ContextTokensAfter)
	assert.True(t, basic.ContextTokensAfter > 0)

	// the forwarded body is rewritten, the last user question intact
	newBody := outRequestBody(t, req)
	assert.NotEqual(t, body, newBody)
	assert.Contains(t, newBody, "current question")
	assert.Contains(t, newBody, `"model":"gpt-test"`)
	assert.Contains(t, newBody, `"stream":false`)
	assert.Contains(t, newBody, toolTruncatedMarker)
	assert.NotContains(t, newBody, strings.Repeat("x", 100))

	// forwarding headers fixed (precedent: reverseproxy model rewrite)
	assert.Equal(t, int64(-1), req.OutRequest.ContentLength)
	assert.Equal(t, "", req.OutRequest.Header.Get("Content-Length"))

	// the original HttpRequest shares the body buffer with OutRequest
	// (shallow copy in reverseproxy); its ContentLength is reset so a
	// fallback retry replays the compressed body consistently (precedent:
	// reverseproxy model rewrite)
	assert.Equal(t, int64(-1), req.HttpRequest.ContentLength)
	assert.Equal(t, "", req.HttpRequest.Header.Get("Content-Length"))
	origAccessor, err := req.HttpRequest.GetBodyAccessor()
	require.NoError(t, err)
	orig, _ := origAccessor.GetBytes()
	assert.Contains(t, string(orig), toolTruncatedMarker, "shared buffer now holds the compressed body for retry replay")
	assert.NotEqual(t, body, string(orig))

	// counters
	assert.Equal(t, int64(1), m.state.GetCounter("CTX_TOTAL"))
	assert.Equal(t, int64(1), m.state.GetCounter("CTX_TRIGGERED"))
	assert.Equal(t, int64(1), m.state.GetCounter("CTX_DONE_TRIM"))
	assert.Equal(t, int64(0), m.state.GetCounter("CTX_REPAIR_ROLLBACK"))

	// response annotation
	resp := &bfe_http.Response{Header: make(bfe_http.Header)}
	ret = m.responseAnnotationHandler(req, resp)
	assert.Equal(t, bfe_module.BfeHandlerGoOn, ret)
	annotation := resp.Header.Get(annotationHeader)
	assert.Contains(t, annotation, "tokens=")
	assert.Contains(t, annotation, "mode=balanced")
	assert.Contains(t, annotation, "->")
}

func TestHandlerIdempotencyGuard(t *testing.T) {
	m := prepareTestModule(t)

	body := aiChatBody(4000)
	req := newAIRequest(t, "tiny_budget_product", body)
	initAIInfo(req)

	m.contextCompressHandler(req)
	first := outRequestBody(t, req)
	before := m.state.GetCounter("CTX_TRIGGERED")

	// second cluster attempt (fallback/retry) must not compress again
	ret, _ := m.contextCompressHandler(req)
	assert.Equal(t, bfe_module.BfeHandlerGoOn, ret)
	assert.Equal(t, first, outRequestBody(t, req), "body must stay unchanged on the second attempt")
	assert.Equal(t, before, m.state.GetCounter("CTX_TRIGGERED"))

	// and the annotation stays stable
	resp := &bfe_http.Response{Header: make(bfe_http.Header)}
	m.responseAnnotationHandler(req, resp)
	assert.NotEqual(t, "", resp.Header.Get(annotationHeader))
}

func TestHandlerFallbackReplaysCompressedOnce(t *testing.T) {
	m := prepareTestModule(t)

	body := aiChatBody(4000)
	req := newAIRequest(t, "tiny_budget_product", body)
	initAIInfo(req)

	// first cluster attempt compresses; the shared body buffer and the
	// original ContentLength are rewritten for retry consistency
	m.contextCompressHandler(req)
	compressed := outRequestBody(t, req)
	if !strings.Contains(compressed, toolTruncatedMarker) {
		t.Fatalf("first attempt should compress, got: %.120q", compressed)
	}

	// retry builds a fresh OutRequest as a shallow copy of the original
	// request, exactly like doSingleAIForward does
	retryOut := new(bfe_http.Request)
	*retryOut = *req.HttpRequest
	req.OutRequest = retryOut
	ret, _ := m.contextCompressHandler(req)
	assert.Equal(t, bfe_module.BfeHandlerGoOn, ret)

	// the guard skips recompression: the retried attempt replays the
	// once-compressed body, byte-identical, with no second marker
	replayed := outRequestBody(t, req)
	assert.Equal(t, compressed, replayed, "fallback retry must replay the once-compressed body")
	assert.Equal(t, 1, strings.Count(replayed, toolTruncatedMarker))
}

func TestHandlerSkipNoRule(t *testing.T) {
	m := prepareTestModule(t)

	req := newAIRequest(t, "no_such_product", aiChatBody(4000))
	basic := initAIInfo(req)

	m.contextCompressHandler(req)
	assert.Equal(t, StatusSkipNoRule, basic.ContextCompressStatus)
	assert.Equal(t, int64(1), m.state.GetCounter("CTX_SKIP_NO_RULE"))
	assert.Equal(t, bodyOf(t, req), aiChatBody(4000))

	// mode=off rule also records SKIP_NO_RULE
	req2 := newAIRequest(t, "off_product", aiChatBody(4000))
	basic2 := initAIInfo(req2)
	m.contextCompressHandler(req2)
	assert.Equal(t, StatusSkipNoRule, basic2.ContextCompressStatus)
}

func TestHandlerSkipProtocol(t *testing.T) {
	m := prepareTestModule(t)

	// Anthropic-style request
	req := newAIRequest(t, "tiny_budget_product", aiChatBody(4000))
	basic := initAIInfo(req)
	basic.AuthStyle = bfe_basic.AuthStyleAnthropic
	m.contextCompressHandler(req)
	assert.Equal(t, StatusSkipProtocol, basic.ContextCompressStatus)
	assert.Equal(t, int64(1), m.state.GetCounter("CTX_SKIP_PROTOCOL"))

	// non-chat mode
	req2 := newAIRequest(t, "tiny_budget_product", aiChatBody(4000))
	basic2 := initAIInfo(req2)
	basic2.Mode = bfe_basic.ModeEmbedding
	m.contextCompressHandler(req2)
	assert.Equal(t, StatusSkipProtocol, basic2.ContextCompressStatus)
}

func TestHandlerSkipParseErr(t *testing.T) {
	m := prepareTestModule(t)

	req := newAIRequest(t, "tiny_budget_product", "not-a-json-body")
	basic := initAIInfo(req)
	m.contextCompressHandler(req)
	assert.Equal(t, StatusSkipParseErr, basic.ContextCompressStatus)
	assert.Equal(t, int64(1), m.state.GetCounter("CTX_SKIP_PARSE_ERR"))
	assert.Equal(t, "not-a-json-body", outRequestBody(t, req), "fail-open: original body forwarded")
}

func TestHandlerSkipBodyIncomplete(t *testing.T) {
	m := prepareTestModule(t)

	// nil OutRequest hits the body-incomplete guard (fail-open)
	req := newAIRequest(t, "tiny_budget_product", aiChatBody(4000))
	req.OutRequest = nil
	basic := initAIInfo(req)
	m.contextCompressHandler(req)
	assert.Equal(t, StatusSkipBodyIncomplete, basic.ContextCompressStatus)
}

func TestHandlerSkipUnderThreshold(t *testing.T) {
	m := prepareTestModule(t)

	// small body: budget 900, trigger 630, estimate far below
	body := aiChatBody(10)
	req := newAIRequest(t, "tiny_budget_product", body)
	basic := initAIInfo(req)
	m.contextCompressHandler(req)
	assert.Equal(t, StatusSkipUnderThreshold, basic.ContextCompressStatus)
	assert.Equal(t, body, outRequestBody(t, req))
	assert.Equal(t, int64(0), basic.ContextTokensAfter)
}

func TestHandlerBadBudgetSkips(t *testing.T) {
	m := prepareTestModule(t)

	// bad_budget_product: maxContextTokens=50 < reserve=1000 => budget <= 0
	req := newAIRequest(t, "bad_budget_product", aiChatBody(4000))
	basic := initAIInfo(req)
	m.contextCompressHandler(req)
	assert.Equal(t, StatusSkipNoRule, basic.ContextCompressStatus)
	assert.Equal(t, aiChatBody(4000), outRequestBody(t, req))
}

func TestHandlerRepairRollback(t *testing.T) {
	m := prepareTestModule(t)

	// unknown role: repair cannot fix it, whole body rolls back
	badBody := `{"model":"m","messages":[{"role":"wizard","content":"` + strings.Repeat("x", 4000) + `"},{"role":"user","content":"q"}]}`
	req := newAIRequest(t, "tiny_budget_product", badBody)
	basic := initAIInfo(req)
	m.contextCompressHandler(req)
	assert.Equal(t, StatusRepairRollback, basic.ContextCompressStatus)
	assert.Equal(t, int64(1), m.state.GetCounter("CTX_REPAIR_ROLLBACK"))
	assert.Equal(t, badBody, outRequestBody(t, req), "rollback: original body forwarded unchanged")
}

func TestHandlerNilAIInfo(t *testing.T) {
	m := prepareTestModule(t)

	req := newAIRequest(t, "tiny_budget_product", aiChatBody(4000))
	// no InitAiBasicInfo: the callback never fires for non-AI flows, but the
	// handler must not panic
	ret, res := m.contextCompressHandler(req)
	assert.Equal(t, bfe_module.BfeHandlerGoOn, ret)
	assert.Nil(t, res)
}

func TestHandlerResponseAnnotationOnlyWhenCompressed(t *testing.T) {
	m := prepareTestModule(t)

	// skipped request: no annotation
	req := newAIRequest(t, "tiny_budget_product", aiChatBody(10))
	basic := initAIInfo(req)
	m.contextCompressHandler(req)
	resp := &bfe_http.Response{Header: make(bfe_http.Header)}
	m.responseAnnotationHandler(req, resp)
	assert.Equal(t, "", resp.Header.Get(annotationHeader))

	// nil response: no panic
	assert.Equal(t, bfe_module.BfeHandlerGoOn, m.responseAnnotationHandler(req, nil))

	// rollback status: no annotation
	basic.ContextCompressStatus = StatusRepairRollback
	resp2 := &bfe_http.Response{Header: make(bfe_http.Header)}
	m.responseAnnotationHandler(req, resp2)
	assert.Equal(t, "", resp2.Header.Get(annotationHeader))
}

func TestHandlerUnknownFieldsCounted(t *testing.T) {
	m := prepareTestModule(t)
	// Defaults.summary + tiny_budget_product.override
	assert.Equal(t, int64(2), m.state.GetCounter("CTX_CFG_UNKNOWN_FIELD"))
}

func bodyOf(t *testing.T, req *bfe_basic.Request) string {
	return outRequestBody(t, req)
}

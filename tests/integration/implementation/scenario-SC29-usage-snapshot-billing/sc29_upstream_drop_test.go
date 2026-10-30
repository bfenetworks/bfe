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

package sc29

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
)

// Injected gateway error payloads (issue #1474). The estimator counts every
// forwarded event's data length, so the injected frame contributes to the
// completion estimate and the expected amounts below must include it.
var (
	openAIInjectedErrorData    = `{"error":{"message":"upstream stream terminated abnormally.","type":"server_error","param":null,"code":"upstream_stream_truncated"}}`
	anthropicInjectedErrorData = `{"type":"error","error":{"type":"api_error","message":"upstream stream terminated abnormally."}}`
)

// sendStreamTolerant sends a streaming request and drains the body, tolerating
// a transport error at the end: when the upstream drops mid-stream BFE finishes
// the client response without a clean chunked terminator (issue #1474).
func (e *testEnv) sendStreamTolerant(t *testing.T, path string, headers map[string]string, body []byte) (string, error) {
	t.Helper()

	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, path)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Host = apiHost
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		e.logBFEException()
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	return string(respBody), err
}

// TestTC07 verifies issue #1474 (OpenAI arm): an upstream that dies mid-stream
// (no [DONE], no terminating chunk) must be billed by the forwarded-content
// estimate, must carry the ai_usage_source=upstream_dropped signal, must be
// recorded with err_code=BK_READ_RESP_BODY (not CLIENT_WRITE), and the client
// must receive a protocol error event telling it the answer is incomplete.
func TestTC07_UpstreamDropBilledAndErrorEventInjected(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	// Three content frames, then the mock aborts the connection abruptly —
	// the kill -9 shape: no usage, no [DONE], no chunked terminator.
	e.backend.SSERaw = []string{
		"data: " + openAIContentChunk + "\n\n",
		"data: " + openAIContentChunk + "\n\n",
		"data: " + openAIContentChunk + "\n\n",
	}
	e.backend.SSEAbortAfter = 3

	respBody, _ := e.sendStreamTolerant(t, "/v1/chat/completions",
		map[string]string{"Authorization": "Bearer " + apiKey, "Content-Type": "application/json"},
		openAIStreamBody)

	if !strings.Contains(respBody, "chatcmpl-1") {
		t.Errorf("client must receive the forwarded content frames, got: %s", respBody)
	}
	if strings.Contains(respBody, "[DONE]") {
		t.Errorf("the upstream sent no [DONE]; the client must not receive one: %s", respBody)
	}
	if !strings.Contains(respBody, "upstream_stream_truncated") {
		t.Errorf("client must receive the injected error event (issue #1474), got: %s", respBody)
	}

	promptEst := int64(len(openAIStreamBody)) / 4
	completionEst := int64(3*len(openAIContentChunk)+len(openAIInjectedErrorData)) / 4
	expectedCost := promptEst*300 + completionEst*900

	remaining := e.quotaRemaining(t)
	if remaining != initialQuota-expectedCost {
		t.Fatalf("billing by forwarded-content estimate: remaining = %d, want %d (promptEst=%d completionEst=%d)",
			remaining, initialQuota-expectedCost, promptEst, completionEst)
	}

	reqLogs := e.stopAndParseLogs()
	reqLog := reqLogs[len(reqLogs)-1]
	if got := logInt64(reqLog.AiTotalTokens); got != promptEst+completionEst {
		t.Errorf("ai_total_tokens = %d, want %d", got, promptEst+completionEst)
	}
	if got := reqLog.GetAiUsageSource(); got != bfe_basic.UsageSourceUpstreamDropped {
		t.Errorf("ai_usage_source = %q, want %q", got, bfe_basic.UsageSourceUpstreamDropped)
	}
	if got := reqLog.GetErrCode(); got != bfe_basic.ErrBkReadRespBody.Error() {
		t.Errorf("err_code = %q, want %q (an upstream drop is not a client write)",
			got, bfe_basic.ErrBkReadRespBody)
	}
}

// TestTC08 verifies issue #1474 (Anthropic arm): the injected event uses the
// protocol-native "event: error" envelope and the drop is billed by the
// forwarded-content estimate.
func TestTC08_AnthropicUpstreamDropInjectsEventError(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	e.backend.SSERaw = []string{
		"event: message_start\ndata: " + anthropicMessageStartData + "\n\n",
		"event: content_block_delta\ndata: " + anthropicDeltaData + "\n\n",
	}
	e.backend.SSEAbortAfter = 2

	respBody, _ := e.sendStreamTolerant(t, "/v1/messages",
		map[string]string{
			"x-api-key":         apiKey,
			"anthropic-version": "2023-06-01",
			"Content-Type":      "application/json",
		},
		anthropicMessagesBody)

	if !strings.Contains(respBody, "msg_01") {
		t.Errorf("client must receive the forwarded content frames, got: %s", respBody)
	}
	if !strings.Contains(respBody, "event: error") {
		t.Errorf("client must receive the injected anthropic error event (issue #1474), got: %s", respBody)
	}

	promptEst := int64(len(anthropicMessagesBody)) / 4
	completionEst := int64(len(anthropicMessageStartData)+len(anthropicDeltaData)+len(anthropicInjectedErrorData)) / 4
	expectedCost := promptEst*300 + completionEst*900

	remaining := e.quotaRemaining(t)
	if remaining != initialQuota-expectedCost {
		t.Fatalf("billing by forwarded-content estimate: remaining = %d, want %d (promptEst=%d completionEst=%d)",
			remaining, initialQuota-expectedCost, promptEst, completionEst)
	}

	reqLogs := e.stopAndParseLogs()
	reqLog := reqLogs[len(reqLogs)-1]
	if got := reqLog.GetAiUsageSource(); got != bfe_basic.UsageSourceUpstreamDropped {
		t.Errorf("ai_usage_source = %q, want %q", got, bfe_basic.UsageSourceUpstreamDropped)
	}
}

// TestTC09 verifies issue #1474 stays observable when estimation is disabled:
// the dropped stream cannot be billed, but the row keeps the unrecognized
// usage signal and the dedicated upstream_dropped zero-cost reason instead of
// being silently free or misreported as a client abort.
func TestTC09_UpstreamDropEstimateOffKeepsSignal(t *testing.T) {
	e := newTestEnvWithEstimateToken(t, false)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	e.backend.SSERaw = []string{"data: " + openAIContentChunk + "\n\n"}
	e.backend.SSEAbortAfter = 1

	e.sendStreamTolerant(t, "/v1/chat/completions",
		map[string]string{"Authorization": "Bearer " + apiKey, "Content-Type": "application/json"},
		openAIStreamBody)

	remaining := e.quotaRemaining(t)
	if remaining != initialQuota {
		t.Fatalf("EstimateToken=false must not bill: remaining = %d, want %d", remaining, initialQuota)
	}

	reqLogs := e.stopAndParseLogs()
	reqLog := reqLogs[len(reqLogs)-1]
	if got := reqLog.GetAiUsageSource(); got != bfe_basic.UsageSourceUnrecognized {
		t.Errorf("ai_usage_source = %q, want %q (unbillable drop must keep its signal)",
			got, bfe_basic.UsageSourceUnrecognized)
	}
	if got := reqLog.GetAiZeroCostReason(); got != bfe_basic.ZeroCostReasonUpstreamDropped {
		t.Errorf("ai_zero_cost_reason = %q, want %q", got, bfe_basic.ZeroCostReasonUpstreamDropped)
	}
	if got := reqLog.GetErrCode(); got != bfe_basic.ErrBkReadRespBody.Error() {
		t.Errorf("err_code = %q, want %q", got, bfe_basic.ErrBkReadRespBody)
	}
}

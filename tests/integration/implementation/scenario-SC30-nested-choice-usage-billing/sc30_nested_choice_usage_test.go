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

package sc30

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"

	bfe_access_pb "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

// SC30 (issue #1401): Kimi-style nested usage. kimi-for-coding embeds the
// final usage inside the streaming finish chunk's choice
// (choices[0].usage, cache field named cached_tokens) and ignores
// stream_options.include_usage. Before the nested fallback in
// ParseOpenAIUsageFields the real usage parsed as all-zero (isguess), so
// EstimateToken=true logged/billed a forged estimate and EstimateToken=false
// logged (0, -1, 0) and billed 0.

const (
	apiHost  = "nested-usage.example.org"
	apiKey   = "ak_user_a"
	apiKeyId = "user_a_key_id"

	clusterNestedUsage = "cluster_nested_usage"

	planRMB     = "plan_rmb"
	redisKeyRMB = "quota:plan_rmb"

	initialQuota = int64(10000000000)
)

var (
	openAIStreamBody = []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	openAIContentChunk = `{"id":"chatcmpl-kimi","choices":[{"index":0,"delta":{"role":"assistant","content":"hello"}}]}`
	// Kimi nested finish chunk: the final usage is embedded in
	// choices[0].usage (top-level usage absent), cache field named
	// cached_tokens. Values mirror the issue #1401 pcap evidence.
	openAINestedUsageChunk = `{"id":"chatcmpl-kimi","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls","usage":{"prompt_tokens":77207,"completion_tokens":168,"total_tokens":77375,"cached_tokens":73472}}]}`

	nestedPromptTokens     = int64(77207)
	nestedCompletionTokens = int64(168)
	nestedTotalTokens      = int64(77375)
	nestedCacheReadTokens  = int64(73472)

	// expectedActualCost: prompt 77207 * 300 + completion 168 * 900 =
	// 23313300 fixed-point units (input 0.000003 yuan/token, output
	// 0.000009). No cache-read price is configured, so calcChatCost bills
	// the full prompt at the input price (cache split only kicks in with a
	// cache price key).
	expectedActualCost = int64(77207*300 + 168*900)
)

type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backend    *common.MockBackend
	redis      *common.RedisServer
	bfePort    int
	logDir     string
	stopBFE    func()
}

func newTestEnv(t *testing.T, estimateToken bool) *testEnv {
	e := &testEnv{t: t}

	e.backend = common.NewMockBackend(clusterNestedUsage, http.StatusOK, "")
	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	confDir := filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	tokenRule := &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": {
				{
					Id:          planRMB,
					Unlimited:   false,
					PassNoQuota: false,
					RedisKey:    redisKeyRMB,
					ExpiredTime: -1,
					Quota:       initialQuota,
					Unit:        "RMB",
				},
			},
		},
		Tokens: map[string]map[string]common.TokenFile{
			"ai_product": {
				apiKey: {
					Key:            apiKey,
					KeyId:          apiKeyId,
					Enabled:        true,
					ExpiredTime:    -1,
					UnlimitedQuota: false,
					QuotaPlans:     []string{planRMB},
				},
			},
		},
		Config: map[string][]common.TokenRule{
			"ai_product": {
				{
					Cond:   "default_t()",
					Action: common.ActionFile{Cmd: "CHECK_TOKEN"},
				},
			},
		},
	}

	aiConfs := map[string]*cluster_conf.AIConf{
		clusterNestedUsage: nestedUsageAIConf(),
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: confDir,
		Backends:      map[string]*common.MockBackend{clusterNestedUsage: e.backend},
		AIConfs:       aiConfs,
		RedisAddr:     e.redis.Addr(),
		TokenRuleData: tokenRule,
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	if !estimateToken {
		// builder.Build copies the template again: re-apply the flip.
		rewriteEstimateToken(t, confDir, false)
	}

	e.bfePort, _, e.stopBFE = e.processEnv.StartBFE(confDir, e.logDir)
	return e
}

// rewriteEstimateToken sets the EstimateToken line in the generated bfe.conf
// of the scenario workdir.
func rewriteEstimateToken(t *testing.T, confDir string, v bool) {
	t.Helper()
	path := filepath.Join(confDir, "bfe.conf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bfe.conf failed: %v", err)
	}
	repl := "EstimateToken = false"
	if v {
		repl = "EstimateToken = true"
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "EstimateToken") {
			lines[i] = repl
			found = true
		}
	}
	if !found {
		t.Fatalf("EstimateToken not found in %s", path)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatalf("write bfe.conf failed: %v", err)
	}
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
	}
	if e.backend != nil {
		e.backend.Close()
	}
	if e.redis != nil {
		e.redis.Close()
	}
}

// stopAndParseLogs stops BFE (flushing the pb3 access log) and parses it.
func (e *testEnv) stopAndParseLogs() []*bfe_access_pb.RequestLog {
	e.t.Helper()
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	reqLogs, err := common.ParseAccessLogAfterStop(e.logDir)
	if err != nil {
		e.t.Fatalf("parse access log failed: %v", err)
	}
	if len(reqLogs) == 0 {
		e.t.Fatal("expected at least 1 access log, got 0")
	}
	return reqLogs
}

func (e *testEnv) logBFEException() {
	data, err := os.ReadFile(filepath.Join(e.processEnv.WorkDir(), "log", "exception.log"))
	if err == nil && len(data) > 0 {
		e.t.Logf("bfe exception log:\n%s", string(data))
	}
}

func nestedUsageAIConf() *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type: 0,
		ModelMapping: &map[string]string{
			"gpt-4": "kimi-for-coding",
		},
		Provider: "mock-provider",
		Keys: []cluster_conf.AIKey{
			{Name: "key-primary", Key: "sk-primary", Weight: 100},
		},
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:            "weighted_random",
			MaxRetries:          0,
			RetryBackoffInitial: 50,
			RetryBackoffMax:     200,
		},
		ModelTable: &cluster_conf.ModelTable{
			Currency: "RMB",
			Models: []cluster_conf.ModelPrice{
				{
					Provider:            "mock-provider",
					Model:               "kimi-for-coding",
					BaseModel:           "kimi-for-coding",
					Mode:                "chat",
					Capabilities:        []string{"chat"},
					SupportedParameters: []string{"temperature", "max_tokens"},
					Prices: cluster_conf.PriceMap{
						"input_cost_per_token":  0.000003,
						"output_cost_per_token": 0.000009,
					},
				},
			},
		},
	}
}

// sendOpenAIStream sends a chat completion request and drains the SSE stream.
func (e *testEnv) sendOpenAIStream(t *testing.T, body []byte) string {
	t.Helper()

	url := fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", e.bfePort)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

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
	if err != nil {
		t.Fatalf("read response failed: %v", err)
	}
	return string(respBody)
}

// lastBackendBody returns the most recent request body captured by the mock
// backend.
func (e *testEnv) lastBackendBody(t *testing.T) string {
	t.Helper()
	bodies := e.backend.RequestBodies()
	if len(bodies) == 0 {
		t.Fatal("backend received no request bodies")
	}
	return string(bodies[len(bodies)-1])
}

func (e *testEnv) quotaRemaining(t *testing.T) int64 {
	t.Helper()
	// Deduction happens synchronously in the request finish handler; give
	// the request a moment to settle before checking redis.
	time.Sleep(1500 * time.Millisecond)
	return e.redis.GetQuota(redisKeyRMB)
}

func logInt64(p *int64) int64 {
	if p == nil {
		return -1
	}
	return *p
}

// assertNestedUsageBilledAndLogged asserts the regression target shared by
// both switch arms: the request is billed by the REAL nested usage (never an
// estimate, never 0) and the access log records the real fields.
func (e *testEnv) assertNestedUsageBilledAndLogged(t *testing.T) {
	t.Helper()

	remaining := e.quotaRemaining(t)
	if remaining != initialQuota-expectedActualCost {
		t.Fatalf("billing by real nested usage: remaining = %d, want %d", remaining, initialQuota-expectedActualCost)
	}

	reqLogs := e.stopAndParseLogs()
	reqLog := reqLogs[len(reqLogs)-1]
	if got := logInt64(reqLog.AiInputTokens); got != nestedPromptTokens {
		t.Errorf("ai_input_tokens = %d, want %d (real usage, not the ContentLength/4 estimate)", got, nestedPromptTokens)
	}
	if got := logInt64(reqLog.AiOutputTokens); got != nestedCompletionTokens {
		t.Errorf("ai_output_tokens = %d, want %d (never the -1 sentinel)", got, nestedCompletionTokens)
	}
	if got := logInt64(reqLog.AiTotalTokens); got != nestedTotalTokens {
		t.Errorf("ai_total_tokens = %d, want %d", got, nestedTotalTokens)
	}
	if got := logInt64(reqLog.AiCacheReadTokens); got != nestedCacheReadTokens {
		t.Errorf("ai_cache_read_tokens = %d, want %d (Kimi cached_tokens mapping)", got, nestedCacheReadTokens)
	}
}

// TestTC01 verifies issue #1401 with EstimateToken=true: the mock upstream
// ignores the injected stream_options.include_usage (Kimi behavior) and
// returns the nested choices[0].usage finish chunk; BFE must parse the real
// usage, bill it, and log it — not the ContentLength/4 estimate forgery.
func TestTC01_NestedChoiceUsageEstimateTokenTrue(t *testing.T) {
	e := newTestEnv(t, true)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	e.backend.SSEEvents = []string{openAIContentChunk, openAINestedUsageChunk, "[DONE]"}

	respBody := e.sendOpenAIStream(t, openAIStreamBody)
	// The finish chunk passes through to the client byte-for-byte.
	if !strings.Contains(respBody, `"total_tokens":77375`) || !strings.Contains(respBody, "[DONE]") {
		t.Errorf("client must receive the nested usage chunk passed through, got: %s", respBody)
	}

	// The mitigation fired: include_usage was injected (the upstream
	// ignores it, which is exactly the facet-2 trigger condition).
	gotBody := e.lastBackendBody(t)
	includeUsage := gjson.Get(gotBody, "stream_options.include_usage")
	if !includeUsage.Exists() || !includeUsage.Bool() {
		t.Errorf("backend body must carry stream_options.include_usage=true, got: %s", gotBody)
	}

	// Without the fix this arm would log/bill the estimate forgery
	// (prompt = len(body)/4 = %d, completion = chunk bytes/4), proving the
	// assertion discriminates.
	t.Logf("discriminator: ContentLength/4 prompt estimate would be %d, real is %d",
		int64(len(openAIStreamBody))/4, nestedPromptTokens)

	e.assertNestedUsageBilledAndLogged(t)
}

// TestTC02 verifies issue #1401 with EstimateToken=false: the parser
// fallback is switch-independent. Without the fix this arm logged
// (0, -1, 0) — negative output tokens — and billed 0.
func TestTC02_NestedChoiceUsageEstimateTokenFalse(t *testing.T) {
	e := newTestEnv(t, false)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	e.backend.SSEEvents = []string{openAIContentChunk, openAINestedUsageChunk, "[DONE]"}

	respBody := e.sendOpenAIStream(t, openAIStreamBody)
	if !strings.Contains(respBody, `"total_tokens":77375`) {
		t.Errorf("client must receive the nested usage chunk passed through, got: %s", respBody)
	}

	e.assertNestedUsageBilledAndLogged(t)
}

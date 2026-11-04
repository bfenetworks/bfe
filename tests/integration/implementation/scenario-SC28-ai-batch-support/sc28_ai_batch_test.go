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

// Package sc28 verifies mod_ai_batch (OpenAI Batch API passthrough):
// lifecycle equivalence, access-log batch fields, post-paid settlement at
// batch prices, batch-level key affinity, batch rate limits, cancel-release
// and the balance pre-check. See 测试设计文档/scenario-SC28-批量任务支持.
package sc28

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost     = "batch.example.org"
	apiKey      = "ak_batch"
	apiKeyID    = "ak_batch"
	apiKeyEvil  = "ak_evil"
	clusterName = "cluster_batch"

	planRMB     = "plan-rmb-sc28"
	redisKeyRMB = "QUOTA_RMB_sc28"

	// result file: two gpt-4o rows, prompt/completion = 10/5 and 3/2. Usage
	// lives under response.body (the OpenAI batch result contract); a flat
	// response.usage fixture would hide the parser defect (issue #254).
	resultJSONL = `{"custom_id":"req-1","response":{"status_code":200,"body":{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}},"error":null}` + "\n" +
		`{"custom_id":"req-2","response":{"status_code":200,"body":{"model":"gpt-4o","usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}},"error":null}` + "\n"

	// resultJSONLMixed covers every row shape the download scanner must
	// handle (TC-15): contract shape, legacy flat shape, a response without
	// usage, a plain error row and a malformed line.
	resultJSONLMixed = `{"custom_id":"body","response":{"status_code":200,"body":{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}},"error":null}` + "\n" +
		`{"custom_id":"flat","response":{"model":"gpt-4o","usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}},"error":null}` + "\n" +
		`{"custom_id":"no-usage","response":{"status_code":200,"body":{"model":"gpt-4o"}},"error":null}` + "\n" +
		`{"custom_id":"error-row","response":null,"error":{"message":"upstream failed"}}` + "\n" +
		`not-a-json-line` + "\n"

	inputJSONL = `{"custom_id":"req-1","method":"POST","url":"/v1/chat/completions","body":{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}}` + "\n" +
		`{"custom_id":"req-2","method":"POST","url":"/v1/chat/completions","body":{"model":"gpt-4o","messages":[{"role":"user","content":"yo"}]}}` + "\n"

	fileInID  = "file-in-1"
	fileOutID = "file-out-1"
	batchID   = "batch-1"

	// uploadFileName is echoed by the mock File object; the BATCH_FILE
	// binding must record it (control-plane contract batch_files[].filename).
	uploadFileName = "batch-input.jsonl"

	// providerBatchCreatedAt is the batch object creation time (unix seconds)
	// recorded as BATCH_TASK.created_at.
	providerBatchCreatedAt = 1760000000

	// batch prices: in 1e-6/token, out 2e-6/token -> (13*1 + 7*2)e-6 = 27e-6 yuan
	// = 2700 units (1e-8). chat prices are double, so any chat-priced charge
	// would be 5400 and is distinguishable in assertions.
	wantSettleUnits = int64(2700)

	// billable totals of resultJSONL / resultJSONLMixed: (10+3) prompt and
	// (5+2) completion tokens.
	wantUsageIn  = int64(13)
	wantUsageOut = int64(7)
)

// mockBatchProvider routes the files/batches endpoints like an OpenAI-style
// batch provider and records the uploaded content for equivalence checks.
// Note: MockBackend consumes r.Body before invoking ResponseFunc, so the
// multipart content is parsed from the recorded bodies instead.
type mockBatchProvider struct {
	uploaded []byte

	// contentTypes/contentLengths record what actually arrived at the
	// provider per request, in arrival order (issue #1426: passthrough
	// must preserve the client's Content-Type and Content-Length).
	contentTypes   []string
	contentLengths []int64

	// test behavior switches
	expiredOnGet  bool   // GET /v1/batches/{id} answers status=expired
	failKeyAOnGet bool   // GET /v1/batches/{id} answers 404 when authorized as key-a
	resultBody    string // overrides the result file served by the download
}

func (p *mockBatchProvider) handler(r *http.Request, count int, bodies [][]byte) (int, string) {
	p.contentTypes = append(p.contentTypes, r.Header.Get("Content-Type"))
	p.contentLengths = append(p.contentLengths, r.ContentLength)
	switch {
	case r.Method == "POST" && r.URL.Path == "/v1/files":
		if len(bodies) > 0 {
			p.uploaded = extractMultipartFile(r.Header.Get("Content-Type"), bodies[len(bodies)-1])
		}
		return 200, fmt.Sprintf(
			`{"id":%q,"object":"file","purpose":"batch","bytes":%d,"filename":%q}`,
			fileInID, len(p.uploaded), uploadFileName)
	case r.Method == "POST" && r.URL.Path == "/v1/chat/completions":
		// echo the model the gateway actually forwarded, so a test can
		// distinguish "target model override applied" (chat) from
		// "body rewritten" (the issue #1426 defect on passthrough modes)
		model := ""
		if len(bodies) > 0 {
			var body map[string]interface{}
			if json.Unmarshal(bodies[len(bodies)-1], &body) == nil {
				if m, ok := body["model"].(string); ok {
					model = m
				}
			}
		}
		return 200, fmt.Sprintf(
			`{"id":"chatcmpl-sc28","object":"chat.completion","model":%q,"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			model)
	case r.Method == "POST" && r.URL.Path == "/v1/batches":
		return 200, fmt.Sprintf(
			`{"id":%q,"object":"batch","status":"in_progress","input_file_id":%q,"output_file_id":%q,`+
				`"endpoint":"/v1/chat/completions","created_at":%d,"request_counts":{"completed":0,"failed":0,"total":2}}`,
			batchID, fileInID, fileOutID, providerBatchCreatedAt)
	case r.Method == "GET" && r.URL.Path == "/v1/batches/"+batchID:
		if p.failKeyAOnGet && strings.Contains(r.Header.Get("Authorization"), "sk-key-a") {
			return 404, `{"error":{"message":"batch not found on this key"}}`
		}
		status := "completed"
		if p.expiredOnGet {
			status = "expired"
		}
		return 200, fmt.Sprintf(
			`{"id":%q,"object":"batch","status":%q,"input_file_id":%q,"output_file_id":%q,`+
				`"endpoint":"/v1/chat/completions","created_at":%d,"request_counts":{"completed":2,"failed":0,"total":2}}`,
			batchID, status, fileInID, fileOutID, providerBatchCreatedAt)
	case r.Method == "POST" && r.URL.Path == "/v1/batches/"+batchID+"/cancel":
		return 200, fmt.Sprintf(`{"id":%q,"object":"batch","status":"cancelling"}`, batchID)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/files/") && strings.HasSuffix(r.URL.Path, "/content"):
		if p.resultBody != "" {
			return 200, p.resultBody
		}
		return 200, resultJSONL
	default:
		return 404, `{"error":{"message":"not found"}}`
	}
}

// extractMultipartFile pulls the first file part out of a recorded multipart
// body (test-side only; mirrors what a provider would see after BFE转发).
func extractMultipartFile(contentType string, body []byte) []byte {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil
	}
	mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil {
			return nil
		}
		if part.FileName() != "" {
			data, _ := io.ReadAll(part)
			return data
		}
	}
}

type testEnv struct {
	provider   *common.MockBackend
	providerMu *mockBatchProvider
	redis      *common.RedisServer

	processEnv  *common.ProcessEnv
	confDir     string
	logDir      string
	bfePort     int
	monitorPort int
	stopBFE     func()
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	e := &testEnv{}
	e.providerMu = &mockBatchProvider{}
	e.provider = common.NewMockBackend(clusterName, 200, "")
	e.provider.ResponseFunc = func(r *http.Request, count int) (int, string) {
		return e.providerMu.handler(r, count, e.provider.RequestBodies())
	}
	e.redis = common.NewRedisServer(t)
	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()
	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		nl := string(rune(10))
		for _, name := range []string{"exception.log", "bfe.log"} {
			if data, err := os.ReadFile(filepath.Join(e.logDir, name)); err == nil {
				lines := strings.Split(string(data), nl)
				if len(lines) > 40 {
					lines = lines[len(lines)-40:]
				}
				t.Logf("---- %s tail ----%s%s", name, nl, strings.Join(lines, nl))
			}
		}
	})
	return e
}

type startOpts struct {
	twoKeys          bool
	noBatchPrice     bool
	routeTargetModel string // non-empty: set the ai-route target Model (issue #1426)
}

type startOption func(*startOpts)

func withTwoKeys() startOption      { return func(o *startOpts) { o.twoKeys = true } }
func withNoBatchPrice() startOption { return func(o *startOpts) { o.noBatchPrice = true } }

// withRouteTargetModel gives the ai-route target a non-empty model override,
// the exact condition that triggered the issue #1426 body rewrite.
func withRouteTargetModel(model string) startOption {
	return func(o *startOpts) { o.routeTargetModel = model }
}

// startBFE assembles the conf from testdata and starts the server.
func (e *testEnv) startBFE(t *testing.T, balance int64, rl *common.RateLimitPolicyData, opts ...startOption) {
	var so startOpts
	for _, opt := range opts {
		opt(&so)
	}
	t.Helper()

	e.redis.SetQuota(redisKeyRMB, balance)

	aiConf := &cluster_conf.AIConf{
		Provider: "mock-provider",
		Keys: []cluster_conf.AIKey{
			{Name: "key-a", Key: "sk-key-a", Weight: 50},
		},
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:                     "weighted_random",
			MaxRetries:                   0,
			RetryBackoffInitial:          50,
			RetryBackoffMax:              200,
			SessionAffinity:              false,
			SessionAffinityPenaltyEnable: true,
		},
		ModelTable: &cluster_conf.ModelTable{
			Currency: "RMB",
			Models: []cluster_conf.ModelPrice{
				{
					Provider: "mock-provider", Model: "gpt-4o", BaseModel: "gpt-4o",
					Mode: "chat", Capabilities: []string{"chat"},
					Prices: cluster_conf.PriceMap{
						"input_cost_per_token":  0.000002,
						"output_cost_per_token": 0.000004,
					},
				},
				{
					Provider: "mock-provider", Model: "gpt-4o", BaseModel: "gpt-4o",
					Mode: "batch", Capabilities: []string{"chat"},
					Prices: cluster_conf.PriceMap{
						"input_cost_per_token":  0.000001,
						"output_cost_per_token": 0.000002,
					},
				},
			},
		},
	}
	if so.twoKeys {
		aiConf.Keys = append(aiConf.Keys, cluster_conf.AIKey{Name: "key-b", Key: "sk-key-b", Weight: 50})
	}
	if so.noBatchPrice {
		// price table without a batch row: settlement lookup must miss and
		// bill 0 (never fall back to the chat price)
		models := aiConf.ModelTable.Models[:1]
		aiConf.ModelTable.Models = models
	}

	tokenRule := &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": {
				{
					Id: planRMB, Unlimited: false, PassNoQuota: false,
					RedisKey: redisKeyRMB, ExpiredTime: -1,
					Quota: balance, Unit: "RMB",
				},
			},
		},
		Tokens: map[string]map[string]common.TokenFile{
			"ai_product": {
				apiKey: {
					Key: apiKey, KeyId: apiKeyID, Enabled: true, ExpiredTime: -1,
					UnlimitedQuota: false, QuotaPlans: []string{planRMB},
				},
				// a second tenant for ownership checks; same plan keeps
				// balance assertions unaffected (unused in those TCs)
				apiKeyEvil: {
					Key: apiKeyEvil, KeyId: apiKeyEvil, Enabled: true, ExpiredTime: -1,
					UnlimitedQuota: false, QuotaPlans: []string{planRMB},
				},
			},
		},
		Config: map[string][]common.TokenRule{
			"ai_product": {
				{Cond: "default_t()", Action: common.ActionFile{Cmd: "CHECK_TOKEN"}},
			},
		},
	}

	if rl == nil {
		// an empty policy set keeps mod_ai_rate_limit happy without
		// restricting anything
		rl = &common.RateLimitPolicyData{
			Version: "1.0",
			Config: map[string][]common.RateLimitProductRule{
				"ai_product": {{Cond: "default_t()", HitAction: struct {
					Cmd    string   `json:"cmd"`
					Params []string `json:"params,omitempty"`
				}{Cmd: "FINISH"}}},
			},
			RateLimitPolicies:             map[string]common.RateLimitPolicy{},
			ApikeyRateLimitPolicyBindings: map[string][]string{},
		}
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:         "testdata",
		TargetConfDir:       e.confDir,
		Backends:            map[string]*common.MockBackend{clusterName: e.provider},
		RedisAddr:           e.redis.Addr(),
		TokenRuleData:       tokenRule,
		RateLimitPolicyData: rl,
		AIConfs:             map[string]*cluster_conf.AIConf{clusterName: aiConf},
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build conf: %v", err)
	}

	if so.routeTargetModel != "" {
		// give the ai-route target a non-empty model in the BUILT conf,
		// leaving the shared testdata untouched for the other TCs
		routePath := filepath.Join(e.confDir, "mod_ai_route", "ai_route.data")
		data, err := os.ReadFile(routePath)
		if err != nil {
			t.Fatalf("read ai_route.data: %v", err)
		}
		old := `"Model": ""`
		if strings.Count(string(data), old) != 1 {
			t.Fatalf("ai_route.data should contain exactly one %q, got:\n%s", old, data)
		}
		data = bytes.Replace(data, []byte(old), []byte(fmt.Sprintf(`"Model": %q`, so.routeTargetModel)), 1)
		if err := os.WriteFile(routePath, data, 0644); err != nil {
			t.Fatalf("write ai_route.data: %v", err)
		}
	}

	var err error
	e.bfePort, e.monitorPort, e.stopBFE, err = e.processEnv.StartBFERaw(e.confDir, e.logDir)
	if err != nil {
		t.Fatalf("start bfe: %v", err)
	}
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
	}
	e.provider.Close()
	e.redis.Close()
}

func (e *testEnv) url(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, path)
}

// do sends one request through the gateway and returns status + body.
func (e *testEnv) do(t *testing.T, method, path string, body io.Reader, contentType string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, e.url(path), body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

// upload sends a multipart batch file upload and returns the provider file id.
func (e *testEnv) upload(t *testing.T, content string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "batch.jsonl")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	fw.Write([]byte(content))
	w.WriteField("purpose", "batch")
	w.Close()
	status, respBody := e.do(t, "POST", "/v1/files", &buf, w.FormDataContentType())
	if status/100 != 2 {
		return status, ""
	}
	var fr struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &fr); err != nil {
		t.Fatalf("parse file response %q: %v", respBody, err)
	}
	return status, fr.Id
}

// runLifecycle executes upload -> create -> get -> download and returns the
// download body. Any step failure fails the test.
func (e *testEnv) runLifecycle(t *testing.T) []byte {
	t.Helper()
	e.runLifecycleToDownload(t)
	return e.downloadResult(t)
}

// runLifecycleToDownload executes upload -> create -> get, i.e. the lifecycle
// steps that do not settle the batch. It lets a test read the module counters
// after the control requests and before the settlement runs (TC-11).
func (e *testEnv) runLifecycleToDownload(t *testing.T) {
	t.Helper()

	if status, id := e.upload(t, inputJSONL); status != 200 || id != fileInID {
		t.Fatalf("upload: status=%d id=%q", status, id)
	}

	status, respBody := e.do(t, "POST", "/v1/batches",
		strings.NewReader(fmt.Sprintf(`{"input_file_id":%q,"endpoint":"/v1/chat/completions","completion_window":"24h"}`, fileInID)),
		"application/json")
	if status != 200 || !strings.Contains(string(respBody), batchID) {
		t.Fatalf("create: status=%d body=%s", status, respBody)
	}

	status, respBody = e.do(t, "GET", "/v1/batches/"+batchID, nil, "")
	if status != 200 || !strings.Contains(string(respBody), `"completed"`) {
		t.Fatalf("get: status=%d body=%s", status, respBody)
	}
}

// downloadResult downloads the batch output file, which triggers the
// post-paid settlement.
func (e *testEnv) downloadResult(t *testing.T) []byte {
	t.Helper()

	status, dl := e.do(t, "GET", "/v1/files/"+fileOutID+"/content", nil, "")
	if status != 200 {
		t.Fatalf("download: status=%d", status)
	}
	return dl
}

// waitForRedis polls a redis condition: BFE executes the settlement/release
// in HandleRequestFinish, which runs after the response is flushed, so the
// client observes the effects asynchronously.
func waitForRedis(t *testing.T, e *testEnv, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return cond()
}

// batchLog is the subset of access-log getters this scenario asserts on.
type batchLog interface {
	GetAiMode() string
	GetAiBatchOp() string
	GetAiBatchId() string
	GetAiFileId() string
	GetAiBatchStatus() string
	GetAiBatchSettle() string
}

func findBatchLog(t *testing.T, e *testEnv, op string) *batchLogEntry {
	t.Helper()
	var found *batchLogEntry
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && found == nil {
		records, err := common.ParseAccessLog(e.logDir, 2*time.Second)
		if err == nil {
			for i := len(records) - 1; i >= 0; i-- {
				r := records[i]
				if r.GetAiBatchOp() == op && r.GetAiApikeyId() == apiKeyID {
					found = &batchLogEntry{
						mode: r.GetAiMode(), op: r.GetAiBatchOp(), batchId: r.GetAiBatchId(),
						fileId: r.GetAiFileId(), status: r.GetAiBatchStatus(), settle: r.GetAiBatchSettle(),
						targetModel:    r.GetAiTargetModel(),
						requestedModel: r.GetAiRequestedModel(),
						// issue #1414: the settlement records why a batch was
						// released without billing (price_miss / batch_release)
						// and reports the cost for observed rows explicitly.
						zeroCostReason:    r.GetAiZeroCostReason(),
						zeroCostReasonSet: r.AiZeroCostReason != nil,
						costValue:         r.GetAiCostValue(),
						costValueSet:      r.AiCostValue != nil,
						priceMiss:         r.GetAiPriceMiss(),
						priceMissSet:      r.AiPriceMiss != nil,
					}
					break
				}
			}
		}
		if found == nil {
			time.Sleep(200 * time.Millisecond)
		}
	}
	if found == nil {
		t.Fatalf("no access log record for op=%s", op)
	}
	return found
}

type batchLogEntry struct {
	mode, op, batchId, fileId, status, settle string
	targetModel                               string
	requestedModel                            string
	zeroCostReason                            string
	zeroCostReasonSet                         bool
	costValue                                 int64
	costValueSet                              bool
	priceMiss                                 bool
	priceMissSet                              bool
}

func (b *batchLogEntry) GetAiMode() string        { return b.mode }
func (b *batchLogEntry) GetAiBatchOp() string     { return b.op }
func (b *batchLogEntry) GetAiBatchId() string     { return b.batchId }
func (b *batchLogEntry) GetAiFileId() string      { return b.fileId }
func (b *batchLogEntry) GetAiBatchStatus() string { return b.status }
func (b *batchLogEntry) GetAiBatchSettle() string { return b.settle }

// tokenAuthMetric reads a mod_ai_token_auth counter by its Go field name
// (issue #1414: BillingZeroCost added next to the pre-existing
// PriceMissZeroBilled signal for a batch settlement that charges nothing).
func (e *testEnv) tokenAuthMetric(t *testing.T, field string) float64 {
	t.Helper()
	v, err := common.ModuleMetric(e.monitorPort, "mod_ai_token_auth", field)
	if err != nil {
		t.Fatalf("read mod_ai_token_auth.%s failed: %v", field, err)
	}
	return v
}

// waitTokenAuthMetric polls a counter until it reaches want (the settlement
// runs in HandleRequestFinish, i.e. after the download response is flushed).
func (e *testEnv) waitTokenAuthMetric(t *testing.T, field string, want float64) float64 {
	t.Helper()
	got, err := common.WaitForModuleMetric(e.monitorPort, "mod_ai_token_auth", field,
		10*time.Second, func(v float64) bool { return v >= want })
	if err != nil {
		t.Fatalf("read mod_ai_token_auth.%s failed: %v", field, err)
	}
	return got
}

// batchMetric reads a mod_ai_batch counter by its state key (the module
// declares its counters in upper snake case). mod_ai_batch exposes state via
// module_state2.StateData.FormatOutput, which has no prometheus renderer, so
// the counter is read from the JSON document.
func (e *testEnv) batchMetric(t *testing.T, key string) float64 {
	t.Helper()
	v, err := common.ModuleCounterJSON(e.monitorPort, "mod_ai_batch", key)
	if err != nil {
		t.Fatalf("read mod_ai_batch.%s failed: %v", key, err)
	}
	return v
}

// waitBatchMetric polls a mod_ai_batch counter until it reaches want (the
// settlement write-back runs in HandleRequestFinish, after the response).
func (e *testEnv) waitBatchMetric(t *testing.T, key string, want float64) float64 {
	t.Helper()
	got, err := common.WaitForModuleCounterJSON(e.monitorPort, "mod_ai_batch", key,
		10*time.Second, func(v float64) bool { return v >= want })
	if err != nil {
		t.Fatalf("read mod_ai_batch.%s failed: %v", key, err)
	}
	if got < want {
		t.Fatalf("mod_ai_batch.%s = %v, want >= %v", key, got, want)
	}
	return got
}

// batchTaskHash / batchFileHash read the BATCH_* records the control plane
// syncs from (nil when absent).
func (e *testEnv) batchTaskHash(batchId string) map[string]string {
	return e.redis.HGetAll("BATCH_TASK:" + batchId)
}

func (e *testEnv) batchFileHash(fileId string) map[string]string {
	return e.redis.HGetAll("BATCH_FILE:" + clusterName + ":" + fileId)
}

// requireField asserts one hash field, failing with the whole record so a
// missing write is debuggable.
func requireField(t *testing.T, rec map[string]string, key, want string) {
	t.Helper()
	if got := rec[key]; got != want {
		t.Fatalf("%s = %q, want %q; record=%v", key, got, want, rec)
	}
}

// TC-14: the completed batch exposes the full terminal record the control
// plane syncs (output_file_id / request_counts / endpoint / created_at /
// reserved_units / settlement fields) and BATCH_FILE describes file CONTENT,
// not the multipart envelope (issue #254).
func TestTC14_TerminalFieldsAndFileContentStats(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	e.startBFE(t, 10_000_000_000, nil)

	// upload + create only: the poll below overwrites status/request_counts,
	// so the create-time record is asserted before it runs.
	if status, id := e.upload(t, inputJSONL); status != 200 || id != fileInID {
		t.Fatalf("upload: status=%d id=%q", status, id)
	}
	status, respBody := e.do(t, "POST", "/v1/batches",
		strings.NewReader(fmt.Sprintf(`{"input_file_id":%q,"endpoint":"/v1/chat/completions","completion_window":"24h"}`, fileInID)),
		"application/json")
	if status != 200 || !strings.Contains(string(respBody), batchID) {
		t.Fatalf("create: status=%d body=%s", status, respBody)
	}

	// A. input file binding: content lines/bytes, provider filename
	in := e.batchFileHash(fileInID)
	if in == nil {
		t.Fatalf("BATCH_FILE binding for the input file missing")
	}
	requireField(t, in, "lines", "2")
	requireField(t, in, "bytes", strconv.Itoa(len(inputJSONL)))
	requireField(t, in, "filename", uploadFileName)
	requireField(t, in, "lines_src", "exact")
	requireField(t, in, "bytes_src", "exact")
	requireField(t, in, "purpose", "batch")
	requireField(t, in, "dir", "input")
	requireField(t, in, "api_key_id", apiKeyID)

	// B. create-time task record: identity, endpoint, counts, reserve mirror
	task := e.batchTaskHash(batchID)
	if task == nil {
		t.Fatalf("BATCH_TASK record missing")
	}
	requireField(t, task, "status", "in_progress")
	requireField(t, task, "input_file_id", fileInID)
	requireField(t, task, "output_file_id", fileOutID)
	requireField(t, task, "endpoint", "/v1/chat/completions")
	requireField(t, task, "request_counts", `{"completed":0,"failed":0,"total":2}`)
	requireField(t, task, "created_at", strconv.Itoa(providerBatchCreatedAt))
	requireField(t, task, "est_lines", "2")
	requireField(t, task, "reserved", "1")
	if units := parseIntField(t, task, "reserved_units"); units <= 0 {
		t.Fatalf("reserved_units = %d, want > 0; record=%v", units, task)
	}
	// not settled yet
	for _, k := range []string{"usage_in", "usage_out", "settle_units", "settle_status", "usage_source"} {
		if _, ok := task[k]; ok {
			t.Fatalf("%s must be absent before the download settles; record=%v", k, task)
		}
	}

	// poll: status and provider counts advance, the output binding appears
	status, respBody = e.do(t, "GET", "/v1/batches/"+batchID, nil, "")
	if status != 200 || !strings.Contains(string(respBody), `"completed"`) {
		t.Fatalf("get: status=%d body=%s", status, respBody)
	}
	task = e.batchTaskHash(batchID)
	requireField(t, task, "status", "completed")
	requireField(t, task, "request_counts", `{"completed":2,"failed":0,"total":2}`)
	out := e.batchFileHash(fileOutID)
	if out == nil {
		t.Fatalf("BATCH_FILE binding for the output file missing")
	}
	requireField(t, out, "dir", "output")
	requireField(t, out, "batch_id", batchID)

	// C. download settles: usage/settle fields land, counters flip
	dl := e.downloadResult(t)
	if string(dl) != resultJSONL {
		t.Fatalf("download body differs from provider bytes:\n got %q", dl)
	}
	if !waitForRedis(t, e, func() bool {
		rec := e.batchTaskHash(batchID)
		return rec != nil && rec["settle_status"] == "settled"
	}) {
		t.Fatalf("BATCH_TASK never reached settle_status=settled; record=%v", e.batchTaskHash(batchID))
	}
	task = e.batchTaskHash(batchID)
	requireField(t, task, "status", "completed")
	requireField(t, task, "request_counts", `{"completed":2,"failed":0,"total":2}`)
	requireField(t, task, "usage_in", strconv.FormatInt(wantUsageIn, 10))
	requireField(t, task, "usage_out", strconv.FormatInt(wantUsageOut, 10))
	requireField(t, task, "settle_units", strconv.FormatInt(wantSettleUnits, 10))
	requireField(t, task, "settle_status", "settled")
	requireField(t, task, "usage_source", "download")
	if !e.redis.Exists("BATCH_SETTLED:" + batchID) {
		t.Fatalf("settled marker missing")
	}

	// D. counters: parsed once, written once, no degraded upload counting
	e.waitBatchMetric(t, "SETTLE_USAGE_PARSE", 1)
	e.waitBatchMetric(t, "TASK_SETTLE_WRITE", 1)
	for field, want := range map[string]float64{
		"SETTLE_NO_USAGE":        0,
		"SETTLE_USAGE_PARSE_ERR": 0,
		"UPLOAD_COUNT_APPROX":    0,
	} {
		if got := e.batchMetric(t, field); got != want {
			t.Fatalf("mod_ai_batch.%s = %v, want %v", field, got, want)
		}
	}
}

// TC-15: the download scanner bills the contract shape (response.body.usage)
// and the legacy flat shape equally, and reports rows it cannot bill instead
// of silently zeroing them (issue #254).
func TestTC15_ResultRowShapesAndAnomalySignals(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	e.providerMu.resultBody = resultJSONLMixed
	e.startBFE(t, 10_000_000_000, nil)

	e.runLifecycleToDownload(t)

	dl := e.downloadResult(t)
	if string(dl) != resultJSONLMixed {
		t.Fatalf("download body differs from provider bytes:\n got %q", dl)
	}

	if !waitForRedis(t, e, func() bool {
		rec := e.batchTaskHash(batchID)
		return rec != nil && rec["settle_status"] == "settled"
	}) {
		t.Fatalf("BATCH_TASK never reached settle_status=settled; record=%v", e.batchTaskHash(batchID))
	}
	task := e.batchTaskHash(batchID)
	// only the two billable rows count: contract shape + legacy flat shape
	requireField(t, task, "usage_in", strconv.FormatInt(wantUsageIn, 10))
	requireField(t, task, "usage_out", strconv.FormatInt(wantUsageOut, 10))
	requireField(t, task, "settle_units", strconv.FormatInt(wantSettleUnits, 10))
	requireField(t, task, "settle_status", "settled")
	requireField(t, task, "usage_source", "download")

	// anomaly rows are counted, error rows are not
	for field, want := range map[string]float64{
		"SETTLE_USAGE_PARSE":     1,
		"SETTLE_NO_USAGE":        1,
		"SETTLE_USAGE_PARSE_ERR": 1,
		"TASK_SETTLE_WRITE":      1,
	} {
		if got := e.waitBatchMetric(t, field, want); got != want {
			t.Fatalf("mod_ai_batch.%s = %v, want %v", field, got, want)
		}
	}

	dlLog := findBatchLog(t, e, "download")
	if dlLog.settle != "settle" {
		t.Fatalf("ai_batch_settle = %q, want settle", dlLog.settle)
	}
}

// parseIntField reads an integer hash field (0 when absent or malformed).
func parseIntField(t *testing.T, rec map[string]string, key string) int64 {
	t.Helper()
	raw := rec[key]
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatalf("field %s = %q is not an integer", key, raw)
	}
	return v
}

// TC-01: lifecycle passes through byte-exact; batch access-log fields land.
func TestTC01_LifecycleEquivalenceAndLogFields(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	e.startBFE(t, 10_000_000_000, nil)

	dl := e.runLifecycle(t)
	if string(dl) != resultJSONL {
		t.Fatalf("download body differs from provider bytes:\n got %q\nwant %q", dl, resultJSONL)
	}
	if !bytes.Equal(e.providerMu.uploaded, []byte(inputJSONL)) {
		t.Fatalf("uploaded content changed in transit:\n got %q", e.providerMu.uploaded)
	}

	paths := strings.Join(e.provider.URLPaths(), " ")
	for _, want := range []string{"/v1/files", "/v1/batches", "/v1/batches/" + batchID, "/v1/files/" + fileOutID + "/content"} {
		if !strings.Contains(paths, want) {
			t.Fatalf("provider never saw %s; paths=%s", want, paths)
		}
	}

	up := findBatchLog(t, e, "upload")
	if up.mode != "file" || up.fileId != fileInID {
		t.Fatalf("upload log wrong: %+v", up)
	}
	cr := findBatchLog(t, e, "create")
	if cr.mode != "batch" || cr.batchId != batchID || cr.status != "in_progress" {
		t.Fatalf("create log wrong: %+v", cr)
	}
	get := findBatchLog(t, e, "get")
	if get.batchId != batchID || get.status != "completed" {
		t.Fatalf("get log wrong: %+v", get)
	}
	dlLog := findBatchLog(t, e, "download")
	if dlLog.mode != "file" || dlLog.fileId != fileOutID || dlLog.batchId != batchID {
		t.Fatalf("download log wrong: %+v", dlLog)
	}
}

// TC-02: downloading the result file settles at batch prices; the reserve
// mirrors are released; the log carries ai_batch_settle=settle.
func TestTC02_SettleAtBatchPriceAndReserveRelease(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	const balance = int64(10_000_000_000) // 100 yuan in 1e-8 units
	e.startBFE(t, balance, nil)

	e.runLifecycle(t)

	if !waitForRedis(t, e, func() bool { return e.redis.GetQuota(redisKeyRMB) == balance-wantSettleUnits }) {
		got := e.redis.GetQuota(redisKeyRMB)
		t.Fatalf("balance = %d, want %d (settled %d; a chat-priced charge would be %d)",
			got, balance-wantSettleUnits, balance-got, wantSettleUnits*2)
	}
	if !waitForRedis(t, e, func() bool { return e.redis.GetQuota("BATCH_RESERVE:"+redisKeyRMB) == 0 }) {
		t.Fatalf("reserve mirror = %d, want 0", e.redis.GetQuota("BATCH_RESERVE:"+redisKeyRMB))
	}
	if e.redis.Exists("BATCH_RESERVE_BATCH:" + batchID) {
		t.Fatalf("per-batch reserve record should be deleted after settle")
	}
	if !e.redis.Exists("BATCH_SETTLED:" + batchID) {
		t.Fatalf("settled marker missing")
	}

	dlLog := findBatchLog(t, e, "download")
	if dlLog.settle != "settle" {
		t.Fatalf("ai_batch_settle = %q, want settle", dlLog.settle)
	}
}

// TC-03: all four lifecycle operations stick to the same upstream key via
// the batch-level affinity binding (client session affinity is disabled).
func TestTC03_FourStepsSameUpstreamKey(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	e.startBFE(t, 10_000_000_000, nil, withTwoKeys())

	e.runLifecycle(t)

	auths := e.provider.AuthHeaders()
	if len(auths) < 4 {
		t.Fatalf("provider saw %d requests, want >= 4", len(auths))
	}
	first := auths[0]
	for i, a := range auths[:4] {
		if a != first {
			t.Fatalf("request %d used auth %q, want %q (all four steps on one key)", i, a, first)
		}
	}
	if !strings.HasPrefix(first, "Bearer sk-key-") {
		t.Fatalf("unexpected upstream auth %q", first)
	}

	if !e.redis.Exists("bfe:ai:key_affinity:batch:cluster_batch:" + batchID) {
		t.Fatalf("batch affinity binding missing in redis")
	}
}

// TC-04a: max_file_bytes rejects oversized uploads with 413.
func TestTC04a_BatchLimitFileBytes(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	e.startBFE(t, 10_000_000_000, batchLimitPolicy(&common.RateLimitBatchLimits{
		MaxFileBytes: 100, MaxFileLines: 50000,
		MaxCreateRPM: 100, RedisKey: "RL_BATCH_rlp-batch_rpm",
	}))

	status, body := e.do(t, "POST", "/v1/files",
		bytes.NewReader(make([]byte, 200)), "application/octet-stream")
	if status != 413 || !strings.Contains(string(body), "BATCH_FILE_TOO_LARGE") {
		t.Fatalf("oversized upload: status=%d body=%s, want 413 BATCH_FILE_TOO_LARGE", status, body)
	}
}

// TC-04b: max_create_rpm meters batch creations per minute per policy.
func TestTC04b_BatchLimitCreateRPM(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	e.startBFE(t, 10_000_000_000, batchLimitPolicy(&common.RateLimitBatchLimits{
		MaxCreateRPM: 1, RedisKey: "RL_BATCH_rlp-batch_rpm",
	}))

	if status, id := e.upload(t, inputJSONL); status != 200 || id == "" {
		t.Fatalf("upload failed: status=%d", status)
	}
	createBody := fmt.Sprintf(`{"input_file_id":%q,"endpoint":"/v1/chat/completions","completion_window":"24h"}`, fileInID)
	if status, _ := e.do(t, "POST", "/v1/batches", strings.NewReader(createBody), "application/json"); status != 200 {
		t.Fatalf("first create rejected: status=%d", status)
	}
	status, body := e.do(t, "POST", "/v1/batches", strings.NewReader(createBody), "application/json")
	if status != 429 {
		t.Fatalf("second create: status=%d body=%s, want 429", status, body)
	}
	if !strings.Contains(string(body), "ratelimitBatch") {
		t.Fatalf("reject body should name the policy: %s", body)
	}
}

// batchLimitPolicy builds a rate limit policy with only the batch section
// configured, bound to the scenario apikey.
func batchLimitPolicy(b *common.RateLimitBatchLimits) *common.RateLimitPolicyData {
	return &common.RateLimitPolicyData{
		Version: "1.0",
		Config: map[string][]common.RateLimitProductRule{
			"ai_product": {{Cond: "default_t()", HitAction: struct {
				Cmd    string   `json:"cmd"`
				Params []string `json:"params,omitempty"`
			}{Cmd: "FINISH"}}},
		},
		RateLimitPolicies: map[string]common.RateLimitPolicy{
			"rlp-batch": {Name: "ratelimitBatch", Enabled: true, Rules: struct {
				TPM            []common.RateLimitRule       `json:"tpm,omitempty"`
				RPM            []common.RateLimitRule       `json:"rpm,omitempty"`
				MaxConcurrency *int64                       `json:"max_concurrency,omitempty"`
				Batch          *common.RateLimitBatchLimits `json:"batch,omitempty"`
			}{
				Batch: b,
			}},
		},
		ApikeyRateLimitPolicyBindings: map[string][]string{apiKey: {"rlp-batch"}},
	}
}

// TC-05: cancel releases the reserve without settlement.
func TestTC05_CancelReleasesReserve(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	const balance = int64(10_000_000_000)
	e.startBFE(t, balance, nil)

	if status, id := e.upload(t, inputJSONL); status != 200 || id == "" {
		t.Fatalf("upload failed: status=%d", status)
	}
	createBody := fmt.Sprintf(`{"input_file_id":%q,"endpoint":"/v1/chat/completions","completion_window":"24h"}`, fileInID)
	if status, _ := e.do(t, "POST", "/v1/batches", strings.NewReader(createBody), "application/json"); status != 200 {
		t.Fatalf("create failed: status=%d", status)
	}
	if got := e.redis.GetQuota("BATCH_RESERVE:" + redisKeyRMB); got <= 0 {
		t.Fatalf("reserve mirror should be positive after create, got %d", got)
	}

	status, body := e.do(t, "POST", "/v1/batches/"+batchID+"/cancel", strings.NewReader("{}"), "application/json")
	if status != 200 {
		t.Fatalf("cancel: status=%d body=%s", status, body)
	}
	if !waitForRedis(t, e, func() bool { return e.redis.GetQuota("BATCH_RESERVE:"+redisKeyRMB) == 0 }) {
		t.Fatalf("reserve mirror = %d after cancel, want 0", e.redis.GetQuota("BATCH_RESERVE:"+redisKeyRMB))
	}
	if e.redis.Exists("BATCH_RESERVE_BATCH:" + batchID) {
		t.Fatalf("per-batch reserve record should be deleted after cancel")
	}
	if !e.redis.Exists("BATCH_SETTLED:" + batchID) {
		t.Fatalf("release marker missing")
	}
	if got := e.redis.GetQuota(redisKeyRMB); got != balance {
		t.Fatalf("balance changed by %d without settlement", balance-got)
	}

	cancelLog := findBatchLog(t, e, "cancel")
	if cancelLog.settle != "release" {
		t.Fatalf("ai_batch_settle = %q, want release", cancelLog.settle)
	}
}

// TC-06: insufficient balance rejects the creation before any forwarding.
func TestTC06_BalancePreCheckRejectsCreate(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	// far below the reserve estimate (lines x ReservePerLineMicros x 100)
	e.startBFE(t, 1000, nil)

	if status, id := e.upload(t, inputJSONL); status != 200 || id == "" {
		t.Fatalf("upload failed: status=%d", status)
	}
	createBody := fmt.Sprintf(`{"input_file_id":%q,"endpoint":"/v1/chat/completions","completion_window":"24h"}`, fileInID)
	status, body := e.do(t, "POST", "/v1/batches", strings.NewReader(createBody), "application/json")
	if status != 401 && status != 403 && !strings.Contains(string(body), "QUOTA_EXHAUSTED") {
		t.Fatalf("create: status=%d body=%s, want QUOTA_EXHAUSTED", status, body)
	}
	for _, p := range e.provider.URLPaths() {
		if p == "/v1/batches" {
			t.Fatalf("provider must not see the create request on pre-check reject")
		}
	}
}

// TC-07: a file bound to another apikey is rejected with 404 before forwarding.
func TestTC07_OwnershipReject(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	e.startBFE(t, 10_000_000_000, nil)

	// owner runs upload + create so the output binding exists
	e.runLifecycle(t)

	req, err := http.NewRequest("GET", e.url("/v1/files/"+fileOutID+"/content"), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apiKeyEvil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("evil download: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 404 || !strings.Contains(string(body), "BATCH_FILE_FORBIDDEN") {
		t.Fatalf("evil download: status=%d body=%s, want 404 BATCH_FILE_FORBIDDEN", resp.StatusCode, body)
	}
	contentHits := 0
	for _, pp := range e.provider.URLPaths() {
		if pp == "/v1/files/"+fileOutID+"/content" {
			contentHits++
		}
	}
	if contentHits != 1 { // only the owner's lifecycle download
		t.Fatalf("rejected download must not reach the provider; contentHits=%d", contentHits)
	}
}

// TC-08: downloading a file without any binding follows the allow_log policy.
func TestTC08_OwnershipMissAllow(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	e.startBFE(t, 10_000_000_000, nil)

	status, body := e.do(t, "GET", "/v1/files/file-unknown-9/content", nil, "")
	if status != 200 || string(body) != resultJSONL {
		t.Fatalf("unbound download: status=%d, want 200 with provider bytes", status)
	}
	if e.redis.Exists("BATCH_FILE:cluster_batch:file-unknown-9") {
		t.Fatalf("no binding should be created for an unknown file")
	}
}

// TC-09: a terminal no-settle status (expired) observed via poll releases the reserve.
func TestTC09_ExpiredReleasesReserve(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	e.providerMu.expiredOnGet = true
	const balance = int64(10_000_000_000)
	e.startBFE(t, balance, nil)

	if status, id := e.upload(t, inputJSONL); status != 200 || id == "" {
		t.Fatalf("upload failed: status=%d", status)
	}
	createBody := fmt.Sprintf(`{"input_file_id":%q,"endpoint":"/v1/chat/completions","completion_window":"24h"}`, fileInID)
	if status, _ := e.do(t, "POST", "/v1/batches", strings.NewReader(createBody), "application/json"); status != 200 {
		t.Fatalf("create failed: status=%d", status)
	}
	if got := e.redis.GetQuota("BATCH_RESERVE:" + redisKeyRMB); got <= 0 {
		t.Fatalf("reserve mirror should be positive after create, got %d", got)
	}

	status, body := e.do(t, "GET", "/v1/batches/"+batchID, nil, "")
	if status != 200 || !strings.Contains(string(body), `"expired"`) {
		t.Fatalf("get: status=%d body=%s, want expired", status, body)
	}

	if !waitForRedis(t, e, func() bool { return e.redis.GetQuota("BATCH_RESERVE:"+redisKeyRMB) == 0 }) {
		t.Fatalf("reserve mirror = %d after expired, want 0", e.redis.GetQuota("BATCH_RESERVE:"+redisKeyRMB))
	}
	if !e.redis.Exists("BATCH_SETTLED:" + batchID) {
		t.Fatalf("release marker missing")
	}
	if got := e.redis.GetQuota(redisKeyRMB); got != balance {
		t.Fatalf("balance changed by %d without settlement", balance-got)
	}
	getLog := findBatchLog(t, e, "get")
	if getLog.settle != "release" {
		t.Fatalf("ai_batch_settle = %q, want release", getLog.settle)
	}
}

// TC-10: a chunked upload exceeding the line limit aborts mid-stream.
func TestTC10_ChunkedUploadLineLimitAbort(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	e.startBFE(t, 10_000_000_000, batchLimitPolicy(&common.RateLimitBatchLimits{
		MaxFileLines: 3, RedisKey: "RL_BATCH_rlp-batch_lines",
	}))

	// chunked (no Content-Length): the gateway buffers the body while
	// extracting the model, so the exact line count is known pre-forward and
	// the upload is rejected before reaching the provider
	var tenLines string
	for i := 0; i < 10; i++ {
		tenLines += `{"custom_id":"req-1","method":"POST","url":"/v1/chat/completions","body":{"model":"gpt-4o","messages":[]}}` + "\n"
	}
	req, err := http.NewRequest("POST", e.url("/v1/files"), io.NopCloser(strings.NewReader(tenLines)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/jsonl")
	req.TransferEncoding = []string{"chunked"} // no Content-Length
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("chunked upload: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 413 || !strings.Contains(string(respBody), "BATCH_FILE_TOO_LARGE") {
		t.Fatalf("chunked oversize upload: status=%d body=%s, want 413 BATCH_FILE_TOO_LARGE", resp.StatusCode, respBody)
	}
	if len(e.provider.RequestBodies()) != 0 {
		t.Fatalf("rejected chunked upload must not reach the provider")
	}
}

// TC-11: without a batch price row the settlement bills 0 (price lookup miss,
// never the chat price).
func TestTC11_NoBatchPriceBillsZero(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	const balance = int64(10_000_000_000)
	e.startBFE(t, balance, nil, withNoBatchPrice())

	e.runLifecycleToDownload(t)

	// The lifecycle control requests (upload / create / get) hold the API key
	// and its RMB plan but never became a model call (no target model), so the
	// BillingZeroCost checkpoint deliberately excludes them: their zero cost is
	// by design, and counting them would inflate the alert baseline of every
	// batch lifecycle. Waiting for the "get" record guarantees their finish
	// handlers ran, so the baseline below is meaningful rather than merely
	// early; together with the "exactly 1 after the settlement" assertion below
	// it is the reverse proof that none of the three control requests counted.
	getLog := findBatchLog(t, e, "get")
	baseZeroCost := e.tokenAuthMetric(t, "BillingZeroCost")
	basePriceMiss := e.tokenAuthMetric(t, "PriceMissZeroBilled")
	t.Logf("before the settlement: BillingZeroCost = %v, PriceMissZeroBilled = %v (control requests excluded)",
		baseZeroCost, basePriceMiss)
	if baseZeroCost != 0 {
		t.Errorf("BillingZeroCost = %v before the settlement, want 0 (a request without a target model is not counted)",
			baseZeroCost)
	}
	if basePriceMiss != 0 {
		t.Errorf("PriceMissZeroBilled = %v before the settlement, want 0 (only the settlement looks up a batch price)",
			basePriceMiss)
	}
	// Residual, reported but deliberately not asserted: the control row still
	// carries ai_zero_cost_reason, because calcCostUnits marks
	// config_missing at its own early return (no target model) and the access
	// log writes any marked reason. The counter exclusion above is what keeps
	// the alert baseline clean; the row marker is not gated on the target
	// model yet (see 测试设计文档/scenario-SC28-批量任务支持/TC-11).
	if getLog.zeroCostReasonSet {
		t.Logf("note: control request (get) still logs ai_zero_cost_reason = %q (counter excluded, row marker not)",
			getLog.zeroCostReason)
	}

	e.downloadResult(t)

	if !waitForRedis(t, e, func() bool { return e.redis.GetQuota(redisKeyRMB) == balance }) {
		t.Fatalf("balance = %d, want unchanged %d (batch price missing must bill 0, not the chat price)",
			e.redis.GetQuota(redisKeyRMB), balance)
	}
	dlLog := findBatchLog(t, e, "download")
	if dlLog.settle != "settle" {
		t.Fatalf("ai_batch_settle = %q, want settle", dlLog.settle)
	}

	// issue #1414: a settlement that charges nothing must be countable and
	// attributable, and the observed row must report its cost explicitly.
	// PriceMissZeroBilled is the pre-existing (#1408) signal for the missing
	// batch price row; BillingZeroCost is the single zero-cost checkpoint of
	// issue #1414, which the batch settlement reaches with its own reason code
	// (its own checkpoint in batchSettle, unaffected by the target-model
	// exclusion). The result file has two rows of the same model, so the
	// settlement walks one model group: each counter ends at exactly 1.
	if got := e.waitTokenAuthMetric(t, "PriceMissZeroBilled", 1); got != 1 {
		t.Errorf("PriceMissZeroBilled = %v, want 1 (batch price entry missing, one model group)", got)
	}
	if got := e.waitTokenAuthMetric(t, "BillingZeroCost", 1); got != 1 {
		t.Errorf("BillingZeroCost = %v, want 1 (RMB plan bound, settlement charged 0)", got)
	}
	if got := e.tokenAuthMetric(t, "UsageUnrecognized"); got != 0 {
		t.Errorf("UsageUnrecognized = %v, want 0: a missing batch price is not an unrecognized-usage problem", got)
	}

	if dlLog.zeroCostReason != bfe_basic.ZeroCostReasonPriceMiss {
		t.Errorf("ai_zero_cost_reason = %q, want %q (the price-miss branch marks the reason)",
			dlLog.zeroCostReason, bfe_basic.ZeroCostReasonPriceMiss)
	}
	if !dlLog.priceMissSet || !dlLog.priceMiss {
		t.Errorf("ai_price_miss = %v (set=%v), want true (issue #1414 marks the batch price miss per row)",
			dlLog.priceMiss, dlLog.priceMissSet)
	}
	if !dlLog.costValueSet {
		t.Error("ai_cost_value is absent, want an explicit 0 (issue #1414: observed 2xx rows always report the cost)")
	} else if dlLog.costValue != 0 {
		t.Errorf("ai_cost_value = %d, want 0", dlLog.costValue)
	}
	// ai_usage_source is deliberately NOT asserted: the batch settlement runs
	// before the usage-source decision point, so the field stays absent on the
	// download row (see 测试设计文档/scenario-SC28-批量任务支持/TC-11).
}

// TC-12: an underestimated reserve settles to the actual usage (which exceeds
// the reserve). Also exercises the mod_ai_batch.data hot reload.
func TestTC12_UnderestimatedReserveSettlesActual(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	const balance = int64(10_000_000_000)
	e.startBFE(t, balance, nil)

	// hot-reload the global knobs: ReservePerLineMicros 200000 -> 1
	dataPath := filepath.Join(e.confDir, "mod_ai_batch", "mod_ai_batch.data")
	data, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatalf("read mod_ai_batch.data: %v", err)
	}
	data = bytes.Replace(data, []byte(`"ReservePerLineMicros": 200000`), []byte(`"ReservePerLineMicros": 1`), 1)
	if err := os.WriteFile(dataPath, data, 0644); err != nil {
		t.Fatalf("write mod_ai_batch.data: %v", err)
	}
	reloadURL := fmt.Sprintf("http://127.0.0.1:%d/reload/mod_ai_batch", e.monitorPort)
	if resp, err := http.Get(reloadURL); err != nil {
		t.Fatalf("hot reload: %v", err)
	} else {
		resp.Body.Close()
	}

	e.runLifecycle(t)

	// reserve is tiny (counted lines x 1 micro x 100), settlement is exact:
	// the balance must drop by the actual usage at batch price
	if !waitForRedis(t, e, func() bool { return e.redis.GetQuota(redisKeyRMB) == balance-wantSettleUnits }) {
		got := e.redis.GetQuota(redisKeyRMB)
		t.Fatalf("balance = %d, want %d (actual settle %d exceeds the tiny reserve)",
			got, balance-wantSettleUnits, balance-got)
	}
	if !waitForRedis(t, e, func() bool { return e.redis.GetQuota("BATCH_RESERVE:"+redisKeyRMB) == 0 }) {
		t.Fatalf("reserve mirror = %d, want 0", e.redis.GetQuota("BATCH_RESERVE:"+redisKeyRMB))
	}
}

// TC-13: a 404 from the bound upstream key deletes the batch binding and
// penalizes the key; the next poll deterministically lands on the other key.
func TestTC13_Failover404Rebind(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	e.providerMu.failKeyAOnGet = true
	e.startBFE(t, 10_000_000_000, nil, withTwoKeys())

	// pre-seed the input binding so the whole chain is pinned to key-a
	e.redis.Set("bfe:ai:key_affinity:batch:cluster_batch:"+fileInID, "key-a")

	if status, id := e.upload(t, inputJSONL); status != 200 || id == "" {
		t.Fatalf("upload failed: status=%d", status)
	}
	createBody := fmt.Sprintf(`{"input_file_id":%q,"endpoint":"/v1/chat/completions","completion_window":"24h"}`, fileInID)
	if status, _ := e.do(t, "POST", "/v1/batches", strings.NewReader(createBody), "application/json"); status != 200 {
		t.Fatalf("create failed: status=%d", status)
	}
	if got := e.redis.Get("bfe:ai:key_affinity:batch:cluster_batch:" + batchID); got != "key-a" {
		t.Fatalf("batch binding = %q, want key-a", got)
	}

	// first poll: bound to key-a -> provider 404, binding deleted + penalty
	status, _ := e.do(t, "GET", "/v1/batches/"+batchID, nil, "")
	if status != 404 {
		t.Fatalf("first get: status=%d, want 404 from the bound key", status)
	}
	if e.redis.Exists("bfe:ai:key_affinity:batch:cluster_batch:" + batchID) {
		t.Fatalf("stale batch binding should be deleted after 404")
	}

	// second poll: key-a is penalized (60s) -> filtered -> key-b answers 200
	status, body := e.do(t, "GET", "/v1/batches/"+batchID, nil, "")
	if status != 200 || !strings.Contains(string(body), `"completed"`) {
		t.Fatalf("second get: status=%d body=%s, want 200 via the other key", status, body)
	}
}

// TC-16: with a non-empty AI route target model, provider-native batch
// endpoints pass the request body through byte-for-byte (issue #1426):
// /v1/files multipart is not replaced by {"model":...}, /v1/batches JSON
// gets no injected model field, while the chat-mode target model override
// still applies.
func TestTC16_PassthroughBodyNotRewritten(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()
	const targetModel = "controlled-batch-model"
	e.startBFE(t, 10_000_000_000, nil, withRouteTargetModel(targetModel))

	// 1. chat: target model override must still rewrite the body
	status, respBody := e.do(t, "POST", "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`),
		"application/json")
	if status != 200 {
		t.Fatalf("chat: status=%d body=%s", status, respBody)
	}
	var chatResp struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		t.Fatalf("parse chat response %q: %v", respBody, err)
	}
	if chatResp.Model != targetModel {
		t.Fatalf("chat body model = %q, want %q (target model override must still apply)",
			chatResp.Model, targetModel)
	}

	// 2. files: multipart body sent byte-exact; Content-Type/Length must
	//    survive the gateway unchanged
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "batch.jsonl")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	fw.Write([]byte(inputJSONL))
	w.WriteField("purpose", "batch")
	w.Close()
	clientBody := buf.Bytes()

	status, respBody = e.do(t, "POST", "/v1/files", bytes.NewReader(clientBody), w.FormDataContentType())
	if status != 200 || !strings.Contains(string(respBody), fileInID) {
		t.Fatalf("upload: status=%d body=%s (multipart body must not be rewritten)", status, respBody)
	}

	// 3. batches: JSON body must not gain a model field
	createBody := fmt.Sprintf(`{"input_file_id":%q,"endpoint":"/v1/chat/completions","completion_window":"24h"}`, fileInID)
	status, respBody = e.do(t, "POST", "/v1/batches", strings.NewReader(createBody), "application/json")
	if status != 200 || !strings.Contains(string(respBody), batchID) {
		t.Fatalf("create: status=%d body=%s", status, respBody)
	}

	bodies := e.provider.RequestBodies()
	if len(bodies) != 3 {
		t.Fatalf("provider saw %d requests, want 3 (chat, files, batches)", len(bodies))
	}

	// B. byte-exact multipart passthrough, headers intact
	if !bytes.Equal(bodies[1], clientBody) {
		t.Fatalf("upload body changed in transit:\n got %q\nwant %q", bodies[1], clientBody)
	}
	if got := e.providerMu.uploaded; !bytes.Equal(got, []byte(inputJSONL)) {
		t.Fatalf("uploaded file content changed in transit:\n got %q", got)
	}
	if ct := e.providerMu.contentTypes[1]; !strings.HasPrefix(ct, "multipart/form-data; boundary=") {
		t.Fatalf("upload Content-Type = %q, want multipart/form-data with boundary", ct)
	}
	if cl := e.providerMu.contentLengths[1]; cl != int64(len(clientBody)) {
		t.Fatalf("upload Content-Length = %d, want %d (body forwarded unchanged, not chunked)", cl, len(clientBody))
	}

	// C. no model injection into the batches create body
	var gotCreate map[string]interface{}
	if err := json.Unmarshal(bodies[2], &gotCreate); err != nil {
		t.Fatalf("batches body is not valid JSON: %v; body=%q", err, bodies[2])
	}
	if _, ok := gotCreate["model"]; ok {
		t.Fatalf("batches body gained a model field: %q", bodies[2])
	}
	if gotCreate["input_file_id"] != fileInID || gotCreate["endpoint"] != "/v1/chat/completions" {
		t.Fatalf("batches body fields changed: %q", bodies[2])
	}

	// D. access log: target model recorded on the side, requested model empty
	upLog := findBatchLog(t, e, "upload")
	if upLog.mode != "file" || upLog.targetModel != targetModel || upLog.requestedModel != "" {
		t.Fatalf("upload log wrong: %+v", upLog)
	}
	crLog := findBatchLog(t, e, "create")
	if crLog.mode != "batch" || crLog.targetModel != targetModel || crLog.requestedModel != "" {
		t.Fatalf("create log wrong: %+v", crLog)
	}
}

// TC-17: max_file_lines is measured on the multipart `file` part content, not
// on the request envelope (issue #1434). A 2-line / 274-byte file arrives in a
// 12-line / 638-byte envelope, so an envelope-based check rejected (or aborted)
// uploads whose file was well within the limit.
func TestTC17_BatchLimitFileLinesMultipart(t *testing.T) {
	fileLines := int64(strings.Count(inputJSONL, "\n"))

	t.Run("file lines at limit accepted", func(t *testing.T) {
		e := newTestEnv(t)
		defer e.Close()
		e.startBFE(t, 10_000_000_000, batchLimitPolicy(&common.RateLimitBatchLimits{
			MaxFileLines: fileLines, MaxFileBytes: 1048576,
			RedisKey: "RL_BATCH_rlp-batch_lines",
		}))

		status, id := e.upload(t, inputJSONL)
		if status != 200 || id != fileInID {
			t.Fatalf("upload with file lines == limit: status=%d id=%q", status, id)
		}
		// binding describes the file content, not the envelope
		in := e.batchFileHash(fileInID)
		requireField(t, in, "lines", strconv.FormatInt(fileLines, 10))
		requireField(t, in, "bytes", strconv.Itoa(len(inputJSONL)))
		requireField(t, in, "lines_src", "exact")
		requireField(t, in, "bytes_src", "exact")
		// the provider received the file content unchanged
		if got := string(e.providerMu.uploaded); got != inputJSONL {
			t.Fatalf("provider file content = %q, want %q", got, inputJSONL)
		}
		if got := e.batchMetric(t, "UPLOAD_REJECT_SIZE"); got != 0 {
			t.Fatalf("UPLOAD_REJECT_SIZE = %v, want 0", got)
		}
	})

	t.Run("file lines over limit rejected", func(t *testing.T) {
		e := newTestEnv(t)
		defer e.Close()
		e.startBFE(t, 10_000_000_000, batchLimitPolicy(&common.RateLimitBatchLimits{
			MaxFileLines: fileLines - 1, MaxFileBytes: 1048576,
			RedisKey: "RL_BATCH_rlp-batch_lines",
		}))

		status, respBody := e.uploadMultipart(t, inputJSONL)
		if status != 413 || !strings.Contains(string(respBody), "BATCH_FILE_TOO_LARGE") {
			t.Fatalf("upload with file lines over limit: status=%d body=%s, want 413 BATCH_FILE_TOO_LARGE",
				status, respBody)
		}
		if !strings.Contains(string(respBody), `"limit_type":"batch_file"`) {
			t.Fatalf("reject body should carry limit_type=batch_file: %s", respBody)
		}
		if len(e.provider.RequestBodies()) != 0 {
			t.Fatalf("rejected upload must not reach the provider")
		}
		e.waitBatchMetric(t, "UPLOAD_REJECT_SIZE", 1)
		if in := e.batchFileHash(fileInID); in != nil {
			t.Fatalf("rejected upload must not write a BATCH_FILE binding: %v", in)
		}
	})

	t.Run("chunked within limit accepted", func(t *testing.T) {
		e := newTestEnv(t)
		defer e.Close()
		e.startBFE(t, 10_000_000_000, batchLimitPolicy(&common.RateLimitBatchLimits{
			MaxFileLines: fileLines, MaxFileBytes: 1048576,
			RedisKey: "RL_BATCH_rlp-batch_lines",
		}))

		status, respBody := e.uploadMultipartChunked(t, inputJSONL)
		if status != 200 || !strings.Contains(string(respBody), fileInID) {
			t.Fatalf("chunked upload within limit: status=%d body=%s", status, respBody)
		}
		// the streaming gate counted the file part, not the envelope
		in := e.batchFileHash(fileInID)
		requireField(t, in, "lines", strconv.FormatInt(fileLines, 10))
		requireField(t, in, "bytes", strconv.Itoa(len(inputJSONL)))
		requireField(t, in, "lines_src", "exact")
		if got := e.batchMetric(t, "UPLOAD_COUNT_APPROX"); got != 0 {
			t.Fatalf("UPLOAD_COUNT_APPROX = %v, want 0 (file part located)", got)
		}
	})

	t.Run("chunked over limit rejected on file content", func(t *testing.T) {
		e := newTestEnv(t)
		defer e.Close()
		e.startBFE(t, 10_000_000_000, batchLimitPolicy(&common.RateLimitBatchLimits{
			MaxFileLines: 3, MaxFileBytes: 1048576,
			RedisKey: "RL_BATCH_rlp-batch_lines",
		}))

		tenLines := strings.Repeat(
			`{"custom_id":"req-1","method":"POST","url":"/v1/chat/completions","body":{"model":"gpt-4o","messages":[]}}`+"\n", 10)
		status, respBody := e.uploadMultipartChunked(t, tenLines)
		if status != 413 || !strings.Contains(string(respBody), "BATCH_FILE_TOO_LARGE") {
			t.Fatalf("chunked upload over the file line limit: status=%d body=%s, want 413", status, respBody)
		}
		// the message must describe the FILE (10 lines), not the envelope (20)
		if !strings.Contains(string(respBody), "lines 10 exceed") {
			t.Fatalf("reject body should report file lines 10, not the envelope: %s", respBody)
		}
		e.waitBatchMetric(t, "UPLOAD_REJECT_SIZE", 1)
		if len(e.provider.RequestBodies()) != 0 {
			t.Fatalf("rejected upload must not reach the provider")
		}
	})

	t.Run("oversized stream aborts on file content", func(t *testing.T) {
		e := newTestEnv(t)
		defer e.Close()
		e.startBFE(t, 10_000_000_000, batchLimitPolicy(&common.RateLimitBatchLimits{
			MaxFileLines: 3, MaxFileBytes: 1048576,
			RedisKey: "RL_BATCH_rlp-batch_lines",
		}))

		// a file part larger than accessibleBodySize (4 MB in this scenario's
		// bfe.conf) cannot be located pre-forward: the streaming gate counts
		// the `file` part and aborts mid-stream
		line := `{"custom_id":"req-1","method":"POST","url":"/v1/chat/completions","body":{"model":"gpt-4o","messages":[]}}` + "\n"
		huge := strings.Repeat(line, 40000) // ~5.4 MB
		status, _ := e.uploadMultipartChunked(t, huge)
		if status/100 == 2 {
			t.Fatalf("oversized streamed upload: status=%d, want failure", status)
		}
		e.waitBatchMetric(t, "UPLOAD_ABORT_STREAM", 1)
		if in := e.batchFileHash(fileInID); in != nil {
			t.Fatalf("aborted upload must not write a BATCH_FILE binding: %v", in)
		}
	})
}

// TC-18: max_file_bytes is measured on the multipart `file` part content, never
// on the envelope Content-Length (issue #1434); raw uploads keep the early
// Content-Length rejection and per-policy pre-checks stay correct.
func TestTC18_BatchLimitFileBytesMultipart(t *testing.T) {
	fileBytes := int64(len(inputJSONL))

	t.Run("file bytes at limit accepted", func(t *testing.T) {
		e := newTestEnv(t)
		defer e.Close()
		e.startBFE(t, 10_000_000_000, batchLimitPolicy(&common.RateLimitBatchLimits{
			MaxFileBytes: fileBytes, RedisKey: "RL_BATCH_rlp-batch_bytes",
		}))

		status, id := e.upload(t, inputJSONL)
		if status != 200 || id != fileInID {
			t.Fatalf("upload with file bytes == limit: status=%d id=%q", status, id)
		}
		in := e.batchFileHash(fileInID)
		requireField(t, in, "bytes", strconv.FormatInt(fileBytes, 10))
		requireField(t, in, "bytes_src", "exact")
	})

	t.Run("file bytes over limit rejected", func(t *testing.T) {
		e := newTestEnv(t)
		defer e.Close()
		e.startBFE(t, 10_000_000_000, batchLimitPolicy(&common.RateLimitBatchLimits{
			MaxFileBytes: fileBytes - 1, RedisKey: "RL_BATCH_rlp-batch_bytes",
		}))

		status, respBody := e.uploadMultipart(t, inputJSONL)
		if status != 413 || !strings.Contains(string(respBody), "BATCH_FILE_TOO_LARGE") {
			t.Fatalf("upload with file bytes over limit: status=%d body=%s", status, respBody)
		}
		if len(e.provider.RequestBodies()) != 0 {
			t.Fatalf("rejected upload must not reach the provider")
		}
	})

	t.Run("raw upload keeps the Content-Length reject", func(t *testing.T) {
		e := newTestEnv(t)
		defer e.Close()
		e.startBFE(t, 10_000_000_000, batchLimitPolicy(&common.RateLimitBatchLimits{
			MaxFileBytes: 100, RedisKey: "RL_BATCH_rlp-batch_bytes",
		}))

		status, respBody := e.do(t, "POST", "/v1/files", bytes.NewReader(make([]byte, 200)), "application/octet-stream")
		if status != 413 || !strings.Contains(string(respBody), "BATCH_FILE_TOO_LARGE") {
			t.Fatalf("raw oversized upload: status=%d body=%s", status, respBody)
		}
		if len(e.provider.RequestBodies()) != 0 {
			t.Fatalf("rejected raw upload must not reach the provider")
		}
	})

	t.Run("raw upload at limit accepted", func(t *testing.T) {
		e := newTestEnv(t)
		defer e.Close()
		e.startBFE(t, 10_000_000_000, batchLimitPolicy(&common.RateLimitBatchLimits{
			MaxFileBytes: 100, RedisKey: "RL_BATCH_rlp-batch_bytes",
		}))

		status, respBody := e.do(t, "POST", "/v1/files", bytes.NewReader(make([]byte, 100)), "application/jsonl")
		if status != 200 || !strings.Contains(string(respBody), fileInID) {
			t.Fatalf("raw upload exactly at the limit: status=%d body=%s, want 200", status, respBody)
		}
	})

	t.Run("per-policy precheck ignores the envelope", func(t *testing.T) {
		e := newTestEnv(t)
		defer e.Close()
		// two bound policies: the looser byte limit must not be evaluated on the
		// envelope Content-Length (638) before mod_ai_batch measures the file
		rl := batchLimitPolicy(&common.RateLimitBatchLimits{
			MaxFileBytes: fileBytes, RedisKey: "RL_BATCH_rlp-batch_bytes_a",
		})
		rl.RateLimitPolicies["rlp-batch-loose"] = common.RateLimitPolicy{
			Name: "ratelimitBatchLoose", Enabled: true,
			Rules: struct {
				TPM            []common.RateLimitRule       `json:"tpm,omitempty"`
				RPM            []common.RateLimitRule       `json:"rpm,omitempty"`
				MaxConcurrency *int64                       `json:"max_concurrency,omitempty"`
				Batch          *common.RateLimitBatchLimits `json:"batch,omitempty"`
			}{Batch: &common.RateLimitBatchLimits{
				MaxFileBytes: 400, RedisKey: "RL_BATCH_rlp-batch_bytes_b",
			}},
		}
		rl.ApikeyRateLimitPolicyBindings[apiKey] = append(rl.ApikeyRateLimitPolicyBindings[apiKey], "rlp-batch-loose")
		e.startBFE(t, 10_000_000_000, rl)

		if status, id := e.upload(t, inputJSONL); status != 200 || id != fileInID {
			t.Fatalf("upload with a 400-byte envelope-sized policy: status=%d id=%q", status, id)
		}
	})
}

// uploadMultipart sends a multipart batch upload and returns the raw status +
// body, so rejection paths can be asserted too.
func (e *testEnv) uploadMultipart(t *testing.T, content string) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "batch.jsonl")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	fw.Write([]byte(content))
	w.WriteField("purpose", "batch")
	w.Close()
	return e.do(t, "POST", "/v1/files", &buf, w.FormDataContentType())
}

// uploadMultipartChunked sends the same multipart upload without a
// Content-Length, exercising the streaming gate (file part counting).
func (e *testEnv) uploadMultipartChunked(t *testing.T, content string) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "batch.jsonl")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	fw.Write([]byte(content))
	w.WriteField("purpose", "batch")
	w.Close()

	req, err := http.NewRequest("POST", e.url("/v1/files"), io.NopCloser(bytes.NewReader(buf.Bytes())))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.TransferEncoding = []string{"chunked"} // no Content-Length
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("chunked upload: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

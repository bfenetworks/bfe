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
	"strings"
	"testing"
	"time"

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

	// result file: two gpt-4o rows, prompt/completion = 10/5 and 3/2
	resultJSONL = `{"custom_id":"req-1","response":{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}},"error":null}` + "\n" +
		`{"custom_id":"req-2","response":{"model":"gpt-4o","usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}},"error":null}` + "\n"

	inputJSONL = `{"custom_id":"req-1","method":"POST","url":"/v1/chat/completions","body":{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}}` + "\n" +
		`{"custom_id":"req-2","method":"POST","url":"/v1/chat/completions","body":{"model":"gpt-4o","messages":[{"role":"user","content":"yo"}]}}` + "\n"

	fileInID  = "file-in-1"
	fileOutID = "file-out-1"
	batchID   = "batch-1"

	// batch prices: in 1e-6/token, out 2e-6/token -> (13*1 + 7*2)e-6 = 27e-6 yuan
	// = 2700 units (1e-8). chat prices are double, so any chat-priced charge
	// would be 5400 and is distinguishable in assertions.
	wantSettleUnits = int64(2700)
)

// mockBatchProvider routes the files/batches endpoints like an OpenAI-style
// batch provider and records the uploaded content for equivalence checks.
// Note: MockBackend consumes r.Body before invoking ResponseFunc, so the
// multipart content is parsed from the recorded bodies instead.
type mockBatchProvider struct {
	uploaded []byte

	// test behavior switches
	expiredOnGet  bool // GET /v1/batches/{id} answers status=expired
	failKeyAOnGet bool // GET /v1/batches/{id} answers 404 when authorized as key-a
}

func (p *mockBatchProvider) handler(r *http.Request, count int, bodies [][]byte) (int, string) {
	switch {
	case r.Method == "POST" && r.URL.Path == "/v1/files":
		if len(bodies) > 0 {
			p.uploaded = extractMultipartFile(r.Header.Get("Content-Type"), bodies[len(bodies)-1])
		}
		return 200, fmt.Sprintf(`{"id":%q,"object":"file","purpose":"batch","bytes":%d}`,
			fileInID, len(p.uploaded))
	case r.Method == "POST" && r.URL.Path == "/v1/batches":
		return 200, fmt.Sprintf(`{"id":%q,"object":"batch","status":"in_progress","input_file_id":%q,"output_file_id":%q}`,
			batchID, fileInID, fileOutID)
	case r.Method == "GET" && r.URL.Path == "/v1/batches/"+batchID:
		if p.failKeyAOnGet && strings.Contains(r.Header.Get("Authorization"), "sk-key-a") {
			return 404, `{"error":{"message":"batch not found on this key"}}`
		}
		status := "completed"
		if p.expiredOnGet {
			status = "expired"
		}
		return 200, fmt.Sprintf(`{"id":%q,"object":"batch","status":%q,"input_file_id":%q,"output_file_id":%q}`,
			batchID, status, fileInID, fileOutID)
	case r.Method == "POST" && r.URL.Path == "/v1/batches/"+batchID+"/cancel":
		return 200, fmt.Sprintf(`{"id":%q,"object":"batch","status":"cancelling"}`, batchID)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/files/") && strings.HasSuffix(r.URL.Path, "/content"):
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
	twoKeys      bool
	noBatchPrice bool
}

type startOption func(*startOpts)

func withTwoKeys() startOption      { return func(o *startOpts) { o.twoKeys = true } }
func withNoBatchPrice() startOption { return func(o *startOpts) { o.noBatchPrice = true } }

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
}

func (b *batchLogEntry) GetAiMode() string        { return b.mode }
func (b *batchLogEntry) GetAiBatchOp() string     { return b.op }
func (b *batchLogEntry) GetAiBatchId() string     { return b.batchId }
func (b *batchLogEntry) GetAiFileId() string      { return b.fileId }
func (b *batchLogEntry) GetAiBatchStatus() string { return b.status }
func (b *batchLogEntry) GetAiBatchSettle() string { return b.settle }

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

	e.runLifecycle(t)

	if !waitForRedis(t, e, func() bool { return e.redis.GetQuota(redisKeyRMB) == balance }) {
		t.Fatalf("balance = %d, want unchanged %d (batch price missing must bill 0, not the chat price)",
			e.redis.GetQuota(redisKeyRMB), balance)
	}
	dlLog := findBatchLog(t, e, "download")
	if dlLog.settle != "settle" {
		t.Fatalf("ai_batch_settle = %q, want settle", dlLog.settle)
	}
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

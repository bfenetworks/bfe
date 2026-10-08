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

package mod_ai_batch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_modules/mod_ai_token_auth"
)

// batchTerminalStatuses are provider states after which no settlement will
// happen (the batch produced no billable result file).
var batchTerminalNoSettle = map[string]bool{
	"expired":   true,
	"failed":    true,
	"cancelled": true,
}

// targetModelHandler classifies the batch operation and runs the pre-forward
// checks (registered after mod_ai_rate_limit, so batch limits are already
// resolved into AiBasicInfo and local rejections have already happened).
func (m *ModuleAiBatch) targetModelHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	aiMeta := req.GetAiBasicInfo()
	if aiMeta == nil || (aiMeta.Mode != bfe_basic.ModeFile && aiMeta.Mode != bfe_basic.ModeBatch) {
		return bfe_module.BfeHandlerGoOn, nil
	}

	op := bfe_basic.ClassifyBatchOp(req.HttpRequest.Method, req.HttpRequest.URL.Path)
	if op == "" {
		return bfe_module.BfeHandlerGoOn, nil
	}

	ctx := &batchContext{
		op:           op,
		fileId:       bfe_basic.BatchPathFileId(req.HttpRequest.URL.Path),
		batchId:      bfe_basic.BatchPathBatchId(req.HttpRequest.URL.Path),
		usageByModel: make(map[string]*bfe_basic.TokenUsage),
	}
	setBatchContext(req, ctx)
	m.state.Inc("REQ_TOTAL", 1)

	// route hint for batch-level key affinity (read by chooseAIKeyWithAffinity)
	req.SetAiBatchRouteHint(&bfe_basic.AiBatchRouteHint{BatchId: ctx.batchId, FileId: ctx.fileId})

	data := m.getData()

	switch op {
	case bfe_basic.BatchOpUpload:
		return m.preCheckUpload(req, ctx, aiMeta)
	case bfe_basic.BatchOpCreate:
		return m.preCheckCreate(req, ctx, aiMeta, data)
	case bfe_basic.BatchOpDownload:
		return m.preCheckDownload(req, ctx, aiMeta, data)
	case bfe_basic.BatchOpGet, bfe_basic.BatchOpCancel:
		// task lookup happens at finish (post-response); the route hint
		// above already covers affinity for these operations
	}
	return bfe_module.BfeHandlerGoOn, nil
}

// preCheckUpload gates uploads by the effective byte limit when
// Content-Length is known, and installs the counting reader on the outgoing
// request body (OutRequest: the shallow copy made before this callback is
// what the transport actually reads).
func (m *ModuleAiBatch) preCheckUpload(req *bfe_basic.Request, ctx *batchContext,
	aiMeta *bfe_basic.AiBasicInfo) (int, *bfe_http.Response) {

	outreq := req.OutRequest
	if outreq == nil {
		return bfe_module.BfeHandlerGoOn, nil
	}
	ctx.contentLength = outreq.ContentLength
	ctx.knownContentLen = outreq.ContentLength >= 0

	if ctx.knownContentLen && ctx.contentLength > 0 {
		if lim := aiMeta.BatchEffMaxFileBytes; lim > 0 && ctx.contentLength > lim {
			m.state.Inc("UPLOAD_REJECT_SIZE", 1)
			ctx.overSized = true
			return m.batchLimitReject(req, aiMeta, "file bytes "+strconv.FormatInt(ctx.contentLength, 10)+
				" exceeds limit "+strconv.FormatInt(lim, 10))
		}
	}

	// the AI gateway already buffered the body while extracting the model
	// (ReqBodyJsonFetch wraps it into bytes_body): for fully buffered uploads
	// (including chunked ones under the buffer cap) the exact line/byte count
	// is known here, so reject before forwarding instead of mid-stream. This
	// must run BEFORE installing the counting wrapper: GetBodyAccessor on a
	// wrapped body would buffer a second time and GetBytes would report
	// all=false (source not exhausted).
	if outreq.Body != nil {
		if accessor, err := outreq.GetBodyAccessor(); err == nil && accessor != nil {
			if buf, all := accessor.GetBytes(); all && len(buf) > 0 {
				lines := int64(bytes.Count(buf, []byte{10}))
				if overBatchFileLimit(int64(len(buf)), lines, aiMeta) {
					m.state.Inc("UPLOAD_REJECT_SIZE", 1)
					ctx.overSized = true
					return m.batchLimitReject(req, aiMeta,
						fmt.Sprintf("file bytes %d/lines %d exceed effective limits", len(buf), lines))
				}
			}
		}
		outreq.Body = newCountingBody(outreq.Body, ctx, aiMeta)
	}
	return bfe_module.BfeHandlerGoOn, nil
}

// overBatchFileLimit reports whether bytes/lines exceed the effective limits.
func overBatchFileLimit(bytesCount, lines int64, aiMeta *bfe_basic.AiBasicInfo) bool {
	if lim := aiMeta.BatchEffMaxFileBytes; lim > 0 && bytesCount > lim {
		return true
	}
	if lim := aiMeta.BatchEffMaxFileLines; lim > 0 && lines > lim {
		return true
	}
	return false
}

// preCheckCreate reads the create request body (KB-level JSON) for
// input_file_id, resolves the input file's line count, and performs the
// balance pre-check (the actual reserve is written after the upstream 2xx,
// two-phase: check first, reserve later).
func (m *ModuleAiBatch) preCheckCreate(req *bfe_basic.Request, ctx *batchContext,
	aiMeta *bfe_basic.AiBasicInfo, data *BatchDataConf) (int, *bfe_http.Response) {

	outreq := req.OutRequest
	if outreq == nil || outreq.Body == nil {
		return bfe_module.BfeHandlerGoOn, nil
	}
	accessor, err := outreq.GetBodyAccessor()
	if err != nil || accessor == nil {
		return bfe_module.BfeHandlerGoOn, nil
	}
	body, all := accessor.GetBytes()
	if !all || len(body) == 0 {
		// unbuffered/oversized create body: fail-open, no pre-check
		return bfe_module.BfeHandlerGoOn, nil
	}
	defer func() {
		if rb, ok := outreq.Body.(bfe_http.Rewindable); ok {
			rb.Rewind()
		}
	}()

	var createReq struct {
		InputFileId string `json:"input_file_id"`
	}
	if err := json.Unmarshal(body, &createReq); err != nil || createReq.InputFileId == "" {
		return bfe_module.BfeHandlerGoOn, nil
	}
	ctx.inputFileId = createReq.InputFileId
	// the create path carries no file id; publish the referenced input file
	// so the key affinity lookup (which runs before this callback's module
	// order matters, in aiClusterInvoke) can chain upload -> create
	req.SetAiBatchRouteHint(&bfe_basic.AiBatchRouteHint{BatchId: ctx.batchId, FileId: createReq.InputFileId})

	// input file metadata for the reserve estimate
	cluster := req.Route.ClusterName
	meta, rerr := m.readFileBinding(cluster, createReq.InputFileId)
	if rerr == nil && meta != nil {
		ctx.estLines = meta.Lines
	} else {
		ctx.estLines = 0 // unknown: reserve falls back to a floor below
	}
	if ctx.estLines <= 0 {
		ctx.estLines = 1
	}

	rmbUnits := ctx.estLines * data.ReservePerLineMicros * 100 // micro (1e-6) -> 1e-8 units
	tokenUnits := ctx.estLines * (data.ReserveInputTokens + data.ReserveOutputTokens)
	if !mod_ai_token_auth.BatchPreCheck(req, rmbUnits, tokenUnits) {
		m.state.Inc("RESERVE_PRECHECK_MISS", 1)
		return m.quotaReject(req, aiMeta)
	}
	ctx.reserved = true // pre-check passed; actual reserve written at finish
	ctx.rmbUnits = rmbUnits
	ctx.tokenUnits = tokenUnits
	return bfe_module.BfeHandlerGoOn, nil
}

// preCheckDownload enforces the file ownership: a BATCH_FILE binding that
// belongs to another apikey is rejected with 404; a missing binding follows
// the OwnerCheckMissPolicy (allow_log by default).
func (m *ModuleAiBatch) preCheckDownload(req *bfe_basic.Request, ctx *batchContext,
	aiMeta *bfe_basic.AiBasicInfo, data *BatchDataConf) (int, *bfe_http.Response) {

	ctx.ownerChecked = true
	cluster := req.Route.ClusterName
	meta, err := m.readFileBinding(cluster, ctx.fileId)
	if err != nil {
		// redis error: fail-open per policy, counted
		m.ownerMissPolicy(req, ctx, aiMeta, data)
		return bfe_module.BfeHandlerGoOn, nil
	}
	if meta == nil {
		m.ownerMissPolicy(req, ctx, aiMeta, data)
		return bfe_module.BfeHandlerGoOn, nil
	}
	if meta.ApiKeyId != "" && aiMeta.ClientKeyId != "" && meta.ApiKeyId != aiMeta.ClientKeyId {
		m.state.Inc("OWNER_CHECK_REJECT", 1)
		ctx.ownerDenied = true
		return m.ownerReject(req, aiMeta)
	}
	// an output binding resolves the owning batch: the settle contract and
	// the reserve release key by batch id, not by file id
	if meta.BatchId != "" {
		ctx.batchId = meta.BatchId
	}
	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleAiBatch) ownerMissPolicy(req *bfe_basic.Request, ctx *batchContext,
	aiMeta *bfe_basic.AiBasicInfo, data *BatchDataConf) {
	if data.OwnerCheckMissPolicy == OwnerCheckMissDeny {
		m.state.Inc("OWNER_CHECK_REJECT", 1)
		ctx.ownerDenied = true
		return
	}
	m.state.Inc("OWNER_CHECK_MISS", 1)
}

func (m *ModuleAiBatch) batchLimitReject(req *bfe_basic.Request, aiMeta *bfe_basic.AiBasicInfo,
	msg string) (int, *bfe_http.Response) {
	aiErr := bfe_basic.NewAiErrorWithDetails(
		bfe_basic.CodeBatchFileTooLarge,
		bfe_basic.TypeRateLimitError,
		"Batch file rejected: "+msg,
		&bfe_basic.AiErrorDetail{
			ApiKey:    aiMeta.ClientApiKey,
			KeyId:     aiMeta.ClientKeyId,
			LimitType: bfe_basic.LimitTypeBatchFile,
		},
	)
	return bfe_module.BfeHandlerFinish, aiErr.CreateErrorResponse(req)
}

func (m *ModuleAiBatch) ownerReject(req *bfe_basic.Request, aiMeta *bfe_basic.AiBasicInfo) (int, *bfe_http.Response) {
	aiErr := bfe_basic.NewAiErrorWithDetails(
		bfe_basic.CodeBatchFileForbidden,
		bfe_basic.TypeAuthenticationError,
		"Batch file not found",
		&bfe_basic.AiErrorDetail{ApiKey: aiMeta.ClientApiKey, KeyId: aiMeta.ClientKeyId},
	)
	return bfe_module.BfeHandlerFinish, aiErr.CreateErrorResponse(req)
}

func (m *ModuleAiBatch) quotaReject(req *bfe_basic.Request, aiMeta *bfe_basic.AiBasicInfo) (int, *bfe_http.Response) {
	aiErr := bfe_basic.NewAiErrorWithDetails(
		bfe_basic.CodeQuotaExhausted,
		bfe_basic.TypeQuotaError,
		"Insufficient balance for batch reserve",
		&bfe_basic.AiErrorDetail{ApiKey: aiMeta.ClientApiKey, KeyId: aiMeta.ClientKeyId},
	)
	return bfe_module.BfeHandlerFinish, aiErr.CreateErrorResponse(req)
}

// readResponseHandler installs the per-operation response wrapper.
func (m *ModuleAiBatch) readResponseHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	ctx := getBatchContext(req)
	if ctx == nil || res == nil || res.Body == nil {
		return bfe_module.BfeHandlerGoOn
	}
	if res.StatusCode/100 != 2 {
		// failed operations carry no billable state; keep the body untouched
		return bfe_module.BfeHandlerGoOn
	}

	aiMeta := req.GetAiBasicInfo()
	data := m.getData()

	switch ctx.op {
	case bfe_basic.BatchOpUpload, bfe_basic.BatchOpCreate, bfe_basic.BatchOpGet, bfe_basic.BatchOpCancel:
		// 2xx-only (see caller): bindings and task records are written at
		// stream EOF, before the client sees the completed response
		res.Body = newCaptureBody(res.Body, smallJsonCap, res.ContentLength, func(buf []byte) {
			m.finishSmallJSON(req, ctx, aiMeta, buf)
		})
	case bfe_basic.BatchOpDownload:
		settleKey := ctx.batchId
		if settleKey == "" {
			settleKey = "file:" + ctx.fileId
		}
		res.Body = newUsageScanBody(res.Body, m, ctx, aiMeta, settleKey, data.MaxLineParseBytes, res.ContentLength)
	}
	return bfe_module.BfeHandlerGoOn
}

// finishSmallJSON writes the bindings/task records for upload/create/get/
// cancel once the response body has been fully buffered (stream EOF). It is
// fail-open: bookkeeping failures never affect the response.
func (m *ModuleAiBatch) finishSmallJSON(req *bfe_basic.Request, ctx *batchContext,
	aiMeta *bfe_basic.AiBasicInfo, buf []byte) {

	keyName, cluster := lastClusterKeyName(aiMeta, req)
	ttl := defaultBatchStateTTL

	switch ctx.op {
	case bfe_basic.BatchOpUpload:
		m.finishUpload(ctx, aiMeta, keyName, cluster, ttl, buf)
	case bfe_basic.BatchOpCreate:
		m.finishCreate(req, ctx, aiMeta, keyName, cluster, ttl, buf)
	case bfe_basic.BatchOpGet:
		m.finishGet(req, ctx, aiMeta, ttl, buf)
	case bfe_basic.BatchOpCancel:
		m.finishCancel(req, ctx, aiMeta, ttl, buf)
	}
}

// requestFinishHandler only fills the access-log fields: all Redis
// bookkeeping already happened at stream EOF (see finishSmallJSON and the
// usage scanner), which is strictly before this callback.
func (m *ModuleAiBatch) requestFinishHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	ctx := getBatchContext(req)
	if ctx == nil {
		return bfe_module.BfeHandlerGoOn
	}
	aiMeta := req.GetAiBasicInfo()
	if aiMeta == nil {
		return bfe_module.BfeHandlerGoOn
	}

	aiMeta.BatchOp = ctx.op
	if ctx.batchId != "" {
		aiMeta.BatchId = ctx.batchId
	}
	if ctx.fileId != "" {
		aiMeta.BatchFileId = ctx.fileId
	}
	if ctx.op == bfe_basic.BatchOpUpload {
		aiMeta.BatchLines = ctx.uploadLines
		aiMeta.BatchBytes = ctx.uploadBytes
		if ctx.uploadAborted {
			m.state.Inc("UPLOAD_ABORT_STREAM", 1)
		}
	}
	return bfe_module.BfeHandlerGoOn
}

// finishUpload records the input file binding (id from the response body,
// lines/bytes from the counting reader) and the affinity chain start.
func (m *ModuleAiBatch) finishUpload(ctx *batchContext, aiMeta *bfe_basic.AiBasicInfo,
	keyName, cluster string, ttl int, buf []byte) {

	fr := parseFileResponse(buf)
	if fr == nil || fr.Id == "" {
		return
	}
	ctx.fileId = fr.Id
	aiMeta.BatchFileId = fr.Id

	meta := map[string]string{
		"api_key_id": aiMeta.ClientKeyId,
		"key_name":   keyName,
		"lines":      strconv.FormatInt(ctx.uploadLines, 10),
		"bytes":      strconv.FormatInt(ctx.uploadBytes, 10),
		"purpose":    fr.Purpose,
		"dir":        "input",
	}
	m.writeFileBinding(cluster, fr.Id, meta, ttl)
	m.writeBatchAffinity(cluster, fr.Id, keyName, ttl)
}

// finishCreate writes the reserve (only after the upstream 2xx: two-phase),
// the BATCH_TASK record, the active-batch marker and the affinity chain.
func (m *ModuleAiBatch) finishCreate(req *bfe_basic.Request, ctx *batchContext, aiMeta *bfe_basic.AiBasicInfo,
	keyName, cluster string, ttl int, buf []byte) {

	if ctx.inputFileId == "" {
		return
	}
	br := parseBatchResponse(buf)
	if br == nil || br.Id == "" {
		return
	}
	ctx.batchId = br.Id
	aiMeta.BatchId = br.Id
	aiMeta.BatchStatus = br.Status

	// reserve: pass_when_no_enough_quota semantics live inside the primitive
	if ctx.reserved {
		if mod_ai_token_auth.BatchReserve(req, br.Id, ctx.rmbUnits, ctx.tokenUnits) {
			m.state.Inc("RESERVE_OK", 1)
		} else {
			ctx.reserved = false // record anyway; settle bills the actual usage
		}
	}
	m.activeAdd(aiMeta.ClientKeyId, br.Id, ttl)

	fields := map[string]string{
		"api_key_id":    aiMeta.ClientKeyId,
		"cluster":       cluster,
		"key_name":      keyName,
		"input_file_id": ctx.inputFileId,
		"est_lines":     strconv.FormatInt(ctx.estLines, 10),
		"status":        br.Status,
		"reserved":      boolStr(ctx.reserved),
	}
	m.writeTaskRecord(br.Id, fields, ttl)
	m.writeBatchAffinity(cluster, br.Id, keyName, ttl)
	if br.OutputFileId != "" {
		ctx.outputFileId = br.OutputFileId
		m.writeOutputFileBinding(br.OutputFileId, br.Id, aiMeta, keyName, cluster, ttl)
	}
}

// writeOutputFileBinding registers a provider-side output file so that a
// later download passes the ownership check and resolves the batch id.
func (m *ModuleAiBatch) writeOutputFileBinding(fileId, batchId string, aiMeta *bfe_basic.AiBasicInfo,
	keyName, cluster string, ttl int) {
	meta := map[string]string{
		"api_key_id": aiMeta.ClientKeyId,
		"key_name":   keyName,
		"batch_id":   batchId,
		"dir":        "output",
	}
	m.writeFileBinding(cluster, fileId, meta, ttl)
	m.writeBatchAffinity(cluster, fileId, keyName, ttl)
}

// finishGet updates the task status; terminal no-settle statuses release the
// reserve (the actual release runs in mod_ai_token_auth's finish handler via
// the release contract set here).
func (m *ModuleAiBatch) finishGet(req *bfe_basic.Request, ctx *batchContext,
	aiMeta *bfe_basic.AiBasicInfo, ttl int, buf []byte) {

	if ctx.batchId == "" {
		return
	}
	br := parseBatchResponse(buf)
	if br == nil {
		return
	}
	aiMeta.BatchStatus = br.Status
	m.writeTaskRecord(br.Id, map[string]string{"status": br.Status}, ttl)

	if br.OutputFileId != "" && ctx.outputFileId == "" {
		ctx.outputFileId = br.OutputFileId
		keyName, cluster := lastClusterKeyName(aiMeta, req)
		m.writeOutputFileBinding(br.OutputFileId, br.Id, aiMeta, keyName, cluster, ttl)
	}

	if batchTerminalNoSettle[br.Status] {
		m.markRelease(ctx, aiMeta, br.Id, ttl)
	}
}

// finishCancel releases the reserve when the provider accepted the cancel.
func (m *ModuleAiBatch) finishCancel(req *bfe_basic.Request, ctx *batchContext,
	aiMeta *bfe_basic.AiBasicInfo, ttl int, buf []byte) {

	if ctx.batchId == "" {
		return
	}
	br := parseBatchResponse(buf)
	if br != nil && br.Id != "" {
		aiMeta.BatchStatus = br.Status
		m.writeTaskRecord(br.Id, map[string]string{"status": br.Status}, ttl)
	}
	m.markRelease(ctx, aiMeta, ctx.batchId, ttl)
}

// markRelease publishes the release contract (executed by mod_ai_token_auth
// in its finish handler) and removes the active-batch marker.
func (m *ModuleAiBatch) markRelease(ctx *batchContext, aiMeta *bfe_basic.AiBasicInfo, batchId string, ttl int) {
	if ctx.released {
		return
	}
	ctx.released = true
	if aiMeta.BatchSettle == bfe_basic.BatchSettleNone || aiMeta.BatchSettle == "" {
		aiMeta.BatchSettle = bfe_basic.BatchSettleRelease
		aiMeta.BatchSettleId = batchId
	}
	m.activeRemove(aiMeta.ClientKeyId, batchId)
	m.state.Inc("RELEASE_OK", 1)
}

// writeBatchAffinity claims the batch-level key affinity binding consumed by
// chooseAIKeyWithAffinity (bfe_server). Claim-first (SET NX): once a chain
// step binds an id to a key, a later write (e.g. a delayed upload record)
// must not move it; failover rebind happens after the 404 path deletes the
// binding.
func (m *ModuleAiBatch) writeBatchAffinity(cluster, id, keyName string, ttl int) {
	if id == "" || keyName == "" || m.redisClient == nil {
		return
	}
	_, err := m.runScript(luaSetNxValueWithTtl, bfe_basic.BatchAffinityKey(cluster, id), strconv.Itoa(ttl), keyName)
	if err != nil {
		m.state.Inc("REDIS_ERR", 1)
		log.Logger.Warn("mod_ai_batch: write batch affinity %s err: %v", id, err)
	}
}

// lastClusterKeyName resolves the finally selected upstream key (for
// bindings and BATCH_* records) and the cluster.
func lastClusterKeyName(aiMeta *bfe_basic.AiBasicInfo, req *bfe_basic.Request) (string, string) {
	keyName := ""
	if n := len(aiMeta.ClusterKeyNames); n > 0 {
		keyName = aiMeta.ClusterKeyNames[n-1].KeyName
	}
	cluster := req.Route.ClusterName
	return keyName, cluster
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// isTerminalStatus reports whether the provider status is a no-settle final
// state (helper for tests and future use).
func isTerminalStatus(s string) bool {
	return batchTerminalNoSettle[strings.ToLower(s)]
}

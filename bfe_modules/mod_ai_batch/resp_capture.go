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
	"io"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_basic"
)

// captureBody buffers the response of a small-JSON batch operation
// (upload/create/get/cancel; their response bodies are KB-level JSON) while
// passing bytes through unchanged. onEOF runs when the upstream body is
// exhausted: the buffered copy is complete there, which happens BEFORE the
// response is finalized to the client and before HandleRequestFinish, so
// binding writes triggered from onEOF are visible to an immediately
// following lifecycle step (upload -> create -> poll -> download).
type captureBody struct {
	source   io.ReadCloser
	buf      *bytes.Buffer
	cap      int
	expected int64 // response Content-Length; -1 = unknown (fall back to EOF)
	onEOF    func([]byte)
	done     bool
}

func newCaptureBody(source io.ReadCloser, cap int, expected int64, onEOF func([]byte)) *captureBody {
	return &captureBody{source: source, buf: bytes.NewBuffer(nil), cap: cap, expected: expected, onEOF: onEOF}
}

func (c *captureBody) Read(p []byte) (int, error) {
	n, err := c.source.Read(p)
	if n > 0 && c.buf.Len() < c.cap {
		rem := c.cap - c.buf.Len()
		if rem > n {
			rem = n
		}
		c.buf.Write(p[:rem])
	}
	// Proxies do not read past a Content-Length-sized body, so the EOF that
	// used to trigger onEOF may never arrive. Firing once the buffered bytes
	// reach the declared length makes the callback deterministic: it runs
	// during the last body read, strictly before the response completes to
	// the client and before the next lifecycle request is processed.
	if !c.done && ((c.expected >= 0 && int64(c.buf.Len()) >= c.expected) || err == io.EOF) {
		c.done = true
		if c.onEOF != nil {
			c.onEOF(c.buf.Bytes())
		}
	}
	return n, err
}

func (c *captureBody) Close() error {
	return c.source.Close()
}

// usageScanBody wraps the batch result file download: it streams every byte
// to the client unchanged while scanning jsonl lines for per-model usage.
// When the stream ends (EOF) and usage was parsed, it finalizes the settle
// contract in AiBasicInfo so that mod_ai_token_auth's HandleRequestFinish
// (which runs after this module's callbacks) prices the settlement.
// A single line's parse buffer is bounded by maxLineBytes; lines that do not
// parse are skipped (the counter records them) and never affect billing
// beyond that line.
type usageScanBody struct {
	source       io.ReadCloser
	m            *ModuleAiBatch
	ctx          *batchContext
	aiMeta       *bfe_basic.AiBasicInfo
	settleKey    string // batch_id, or "file:<id>" when the binding is unknown
	maxLineBytes int64
	expected     int64 // response Content-Length; -1 = unknown (fall back to EOF)
	got          int64
	finalized    bool

	pending   []byte // unterminated line fragment
	lineDirty bool   // current line overflowed maxLineBytes
}

func newUsageScanBody(source io.ReadCloser, m *ModuleAiBatch, ctx *batchContext, aiMeta *bfe_basic.AiBasicInfo,
	settleKey string, maxLineBytes, expected int64) *usageScanBody {
	return &usageScanBody{
		source:       source,
		m:            m,
		ctx:          ctx,
		aiMeta:       aiMeta,
		settleKey:    settleKey,
		maxLineBytes: maxLineBytes,
		expected:     expected,
	}
}

func (u *usageScanBody) Read(p []byte) (int, error) {
	// Bytes flow to the client verbatim; usage extraction piggybacks on the
	// reads (the same bytes are fed to the line parser). Finalization is
	// length-driven for the same reason as captureBody: a CL-sized body may
	// never produce a trailing EOF read.
	n, err := u.source.Read(p)
	if n > 0 {
		u.got += int64(n)
		u.feed(p[:n])
	}
	if !u.finalized && ((u.expected >= 0 && u.got >= u.expected) || err == io.EOF) {
		u.finalized = true
		u.finalize()
	}
	return n, err
}

// finalize publishes the settle contract. Idempotency (BATCH_SETTLED) is
// handled inside mod_ai_token_auth's settle primitive; a client-aborted
// stream simply never finalizes and the control-plane reconcile job settles
// from the provider side instead.
//
// The BATCH_TASK settlement fields are NOT written here: the SETNX claim that
// decides whether this download really settles happens later, in
// mod_ai_token_auth's finish handler. Writing them here would mark a batch
// settled even when another path (reconcile backstop) already owns it. The
// write-back runs in requestFinishHandler once the contract has executed.
func (u *usageScanBody) finalize() {
	if !u.ctx.usageParsed || u.aiMeta == nil {
		return
	}
	usage := make(map[string]bfe_basic.TokenUsage, len(u.ctx.usageByModel))
	for model, tu := range u.ctx.usageByModel {
		usage[model] = *tu
	}
	u.aiMeta.BatchSettle = bfe_basic.BatchSettleSettle
	u.aiMeta.BatchSettleId = u.settleKey
	u.aiMeta.BatchUsageByModel = usage
	u.aiMeta.BatchLines = u.ctx.parsedLines

	u.m.state.Inc("SETTLE_USAGE_PARSE", 1)
}

// feed pushes bytes into the line parser.
func (u *usageScanBody) feed(b []byte) {
	u.pending = append(u.pending, b...)
	for {
		idx := bytes.IndexByte(u.pending, '\n')
		if idx < 0 {
			if int64(len(u.pending)) > u.maxLineBytes {
				// abnormal line without terminator: drop the overflow and
				// mark the current line dirty so it is not billed
				u.lineDirty = true
				u.pending = u.pending[:0]
			}
			return
		}
		u.handleLine(u.pending[:idx])
		u.pending = u.pending[idx+1:]
	}
}

func (u *usageScanBody) handleLine(line []byte) {
	dirty := u.lineDirty
	u.lineDirty = false
	if dirty {
		return // overflowed fragment of an abnormal line: skip
	}
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return
	}
	var row batchResultRow
	if err := json.Unmarshal(trimmed, &row); err != nil {
		// malformed line: dropped and counted, never billed
		u.m.state.Inc("SETTLE_USAGE_PARSE_ERR", 1)
		return
	}
	if row.Response == nil {
		// error rows ({"custom_id":...,"error":{...}}) carry no usage at all
		return
	}
	usage, model := row.usageAndModel()
	if usage == nil {
		// a response without usage cannot be billed; distinguish it from a
		// malformed line so "provider wrote no usage" is observable
		u.m.state.Inc("SETTLE_NO_USAGE", 1)
		return
	}
	if model == "" {
		model = "unknown"
	}
	tu, ok := u.ctx.usageByModel[model]
	if !ok {
		tu = &bfe_basic.TokenUsage{}
		u.ctx.usageByModel[model] = tu
	}
	tu.PromptTokens += usage.PromptTokens
	tu.CompletionTokens += usage.CompletionTokens
	if usage.TotalTokens > 0 {
		tu.UsedQuota += usage.TotalTokens
	} else {
		tu.UsedQuota += usage.PromptTokens + usage.CompletionTokens
	}
	u.ctx.usageParsed = true
	u.ctx.parsedLines++
}

func (u *usageScanBody) Close() error {
	return u.source.Close()
}

// parse helpers for the small-JSON responses (shared by the finish handler).

type fileResponse struct {
	Id       string `json:"id"`
	Purpose  string `json:"purpose"`
	Bytes    int64  `json:"bytes"`
	Filename string `json:"filename"`
}

// batchResponse is the minimal read view of a provider batch object. The
// fields mirror what the control plane (ai-gateway-api batch_tasks) needs, so
// the BATCH_TASK record can expose the batch terminal facts.
type batchResponse struct {
	Id           string `json:"id"`
	Status       string `json:"status"`
	InputFileId  string `json:"input_file_id"`
	OutputFileId string `json:"output_file_id"`
	Endpoint     string `json:"endpoint"`
	// RequestCounts is kept verbatim (contract: provider counts as-is); nil
	// when the provider omitted it, in which case nothing is written.
	RequestCounts json.RawMessage `json:"request_counts"`
	CreatedAt     int64           `json:"created_at"`
}

// usageFields is the token usage object carried by a batch result row.
type usageFields struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// batchResultRow is one line of a batch result file. An OpenAI-style row
// nests the chat completion under response.body, so model/usage live at
// response.body.model / response.body.usage; a flat response.model /
// response.usage layout is still accepted for non-standard providers.
type batchResultRow struct {
	Response *struct {
		Model string       `json:"model"`
		Usage *usageFields `json:"usage"`
		Body  *struct {
			Model string       `json:"model"`
			Usage *usageFields `json:"usage"`
		} `json:"body"`
	} `json:"response"`
}

// usageAndModel returns the row's usage object and model, preferring the
// nested response.body shape.
func (r *batchResultRow) usageAndModel() (*usageFields, string) {
	if r == nil || r.Response == nil {
		return nil, ""
	}
	if body := r.Response.Body; body != nil && body.Usage != nil {
		return body.Usage, body.Model
	}
	if r.Response.Usage != nil {
		return r.Response.Usage, r.Response.Model
	}
	return nil, ""
}

func parseFileResponse(buf []byte) *fileResponse {
	if len(buf) == 0 {
		return nil
	}
	var fr fileResponse
	if err := json.Unmarshal(buf, &fr); err != nil || fr.Id == "" {
		if openDebug {
			log.Logger.Debug("mod_ai_batch: parse file response failed: %v", err)
		}
		return nil
	}
	return &fr
}

func parseBatchResponse(buf []byte) *batchResponse {
	if len(buf) == 0 {
		return nil
	}
	var br batchResponse
	if err := json.Unmarshal(buf, &br); err != nil || br.Id == "" {
		if openDebug {
			log.Logger.Debug("mod_ai_batch: parse batch response failed: %v", err)
		}
		return nil
	}
	return &br
}

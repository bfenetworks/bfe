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

package bfe_server

import (
	"bytes"
	"encoding/base64"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_model_protocol"
	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
	"github.com/bfenetworks/bfe/bfe_route/bfe_cluster"
	"github.com/bfenetworks/go-lib/web-monitor/metrics"
)

// redactMask replaces credential material in outgoing upstream error
// content (client response and access log).
const redactMask = "••••••••"

// normalizeUpstreamError rewrites the final upstream error response into
// the unified AiError catalog (AIConf.NormalizeUpstreamError). It runs
// after the fallback loop settles — so it never affects retry/fallback
// decisions — and before any byte is written to the client or the EPP
// filter wraps res.Body, so both the EPP chain and the access log observe
// the final form. No-op unless the last cluster enables the feature;
// every failure path is fail-open (the original response is preserved).
func normalizeUpstreamError(res *bfe_http.Response, lastCluster *bfe_cluster.BfeCluster,
	aiMeta *bfe_basic.AiBasicInfo, state *ProxyState) {
	if res == nil || aiMeta == nil {
		return
	}
	var cfg *cluster_conf.UpstreamErrorNormalizeConf
	if lastCluster != nil && lastCluster.AIConf != nil {
		cfg = lastCluster.AIConf.NormalizeUpstreamError
	}
	eff := cfg.Effective()
	if !eff.Enabled && !eff.StreamEnabled {
		return
	}
	// Gateway-generated responses (auth/limit/quota/protocol-mismatch) are
	// already in the unified format; skip them.
	if res.Header.Get(bfe_basic.HeaderBfeGwError) != "" {
		return
	}

	adapter := bfe_model_protocol.Get(aiMeta.AuthStyle)
	isStream := res.IsSse || strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream")
	if isStream {
		if !eff.StreamEnabled {
			return
		}
		res.Body = &aiErrorStreamFilter{
			src:     res.Body,
			adapter: adapter,
			cfg:     eff,
			aiMeta:  aiMeta,
			state:   state,
		}
		return
	}

	if !eff.Enabled || res.StatusCode < 400 {
		return
	}
	normalizeUpstreamErrorBody(res, adapter, eff, aiMeta, state)
}

// normalizeUpstreamErrorBody handles a non-streaming upstream error
// response: buffer the body (bounded), parse it with the protocol's
// ErrorParser, then rewrite (recognized), generic-rewrite or pass through
// (unrecognized) with credential redaction applied to every egress path.
func normalizeUpstreamErrorBody(res *bfe_http.Response, adapter bfe_model_protocol.ProtocolAdapter,
	cfg cluster_conf.UpstreamErrorNormalizeConf, aiMeta *bfe_basic.AiBasicInfo, state *ProxyState) {
	limit := cfg.MaxBodyBytes
	if limit <= 0 {
		limit = 64 * 1024
	}
	body, err := ioutil.ReadAll(io.LimitReader(res.Body, limit+1))
	res.Body.Close()
	if err != nil {
		// fail-open: restore whatever was read, keep the original response
		res.Body = ioutil.NopCloser(bytes.NewReader(body))
		return
	}

	var perr *utils.ProtocolError
	if int64(len(body)) <= limit {
		perr = adapter.ErrorParser().ParseError(res.StatusCode, body, http.Header(res.Header))
	}

	if perr == nil {
		// Unrecognized envelope: passthrough (default) or generic rewrite.
		aiMeta.UpstreamStatus = int32(res.StatusCode)
		aiMeta.ErrNormalizeMiss = true
		incStateCounter(state.ErrNormalizeMiss)

		out := body
		if cfg.RedactSecretsEnabled() {
			if s, changed := redactSecretMaterial(string(body), aiMeta.UpstreamKey); changed {
				out = []byte(s)
				incStateCounter(state.ErrNormalizeRedact)
			}
		}
		if cfg.UnrecognizedAction == cluster_conf.NormalizeActionRewriteGeneric {
			gerr := &utils.ProtocolError{
				Code:       bfe_basic.CodeUpstreamUnknown,
				StatusCode: res.StatusCode,
				IsUpstream: true,
				Message:    "upstream error unrecognized.",
			}
			rewriteResponseWithUnified(res, buildUnifiedAiError(gerr, cfg, aiMeta, state))
			aiMeta.ErrNormalized = true
			return
		}
		res.Body = ioutil.NopCloser(bytes.NewReader(out))
		res.ContentLength = int64(len(out))
		res.Header.Set("Content-Length", strconv.Itoa(len(out)))
		return
	}

	// Recognized envelope: rewrite into the unified error body.
	rewriteResponseWithUnified(res, buildUnifiedAiError(perr, cfg, aiMeta, state))
	aiMeta.ErrNormalized = true
	aiMeta.UpstreamStatus = int32(perr.StatusCode)
	aiMeta.UpstreamErrCode = perr.UpstreamCode
	incStateCounter(state.ErrNormalizeHit)
}

// rewriteResponseWithUnified replaces status/headers/body of res with the
// unified AiError response, mirroring AiError.CreateErrorResponse.
func rewriteResponseWithUnified(res *bfe_http.Response, aiErr *bfe_basic.AiError) {
	bodyStr := aiErr.GenRespBodyStr()
	res.StatusCode = bfe_basic.ErrorCodeToStatusCode[aiErr.Code]
	if res.Header == nil {
		res.Header = make(bfe_http.Header)
	}
	res.Header.Set("Server", "bfe")
	res.Header.Set("Content-Type", "application/json")
	res.Header.Set(bfe_basic.HeaderBfeGwError, "1")
	res.ContentLength = int64(len(bodyStr))
	res.Header.Set("Content-Length", strconv.Itoa(len(bodyStr)))
	res.Body = ioutil.NopCloser(strings.NewReader(bodyStr))
	res.IsSse = false
}

// buildUnifiedAiError converts a normalized ProtocolError into an AiError,
// redacting the embedded upstream message when configured.
func buildUnifiedAiError(perr *utils.ProtocolError, cfg cluster_conf.UpstreamErrorNormalizeConf,
	aiMeta *bfe_basic.AiBasicInfo, state *ProxyState) *bfe_basic.AiError {
	msg := perr.Message
	if cfg.RedactSecretsEnabled() {
		if s, changed := redactSecretMaterial(msg, aiMeta.UpstreamKey); changed {
			msg = s
			incStateCounter(state.ErrNormalizeRedact)
		}
	}
	if msg == "" {
		msg = defaultNormalizedMessage(perr.Code)
	}

	detail := &bfe_basic.AiErrorDetail{
		Model: aiMeta.ClientModel,
	}
	if perr.StatusCode > 0 {
		status := perr.StatusCode
		detail.UpstreamStatus = &status
	}
	if perr.UpstreamCode != "" {
		detail.UpstreamCode = perr.UpstreamCode
	}
	if perr.RetryAfterSeconds > 0 {
		detail.RetryAfterSeconds = perr.RetryAfterSeconds
	}

	aiErr := bfe_basic.NewAiErrorWithDetails(perr.Code, bfe_basic.ErrorTypeForCode(perr.Code), msg, detail)
	if perr.Param != nil {
		aiErr.Param = perr.Param
	}
	return aiErr
}

// defaultNormalizedMessage provides the fallback message when the upstream
// error carries none.
func defaultNormalizedMessage(code string) string {
	switch code {
	case bfe_basic.CodeUpstreamRateLimited:
		return "upstream rate limited."
	case bfe_basic.CodeUpstreamQuotaExhausted:
		return "upstream quota exhausted."
	case bfe_basic.CodeUpstreamAuthError:
		return "upstream auth failed: credential rejected by provider."
	case bfe_basic.CodeUpstreamModelNotFound:
		return "upstream model not found."
	case bfe_basic.CodeUpstreamOverloaded:
		return "upstream overloaded."
	case bfe_basic.CodeContextLengthExceeded:
		return "upstream context length exceeded."
	case bfe_basic.CodeContentFiltered:
		return "upstream content filtered."
	case bfe_basic.CodeModelInternalError:
		return "upstream model internal error."
	case bfe_basic.CodeBackendTimeout:
		return "upstream backend timeout."
	case bfe_basic.CodeUpstreamUnknown:
		return "upstream error unrecognized."
	default:
		return "upstream request invalid."
	}
}

// secretPatterns expands the upstream credential into the encodings an
// upstream error body may echo: raw, base64 (standard and URL), URL-query
// escaped, and JSON-escaped forms.
func secretPatterns(key string) []string {
	if key == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	add(key)
	add(base64.StdEncoding.EncodeToString([]byte(key)))
	add(base64.URLEncoding.EncodeToString([]byte(key)))
	add(url.QueryEscape(key))
	if q := strconv.Quote(key); len(q) >= 2 {
		add(q[1 : len(q)-1])
	}
	return out
}

// redactSecretMaterial replaces every occurrence of the credential's
// encoded forms in s with the mask. Fail-open by contract: the caller
// never aborts the error path because of redaction.
func redactSecretMaterial(s, key string) (string, bool) {
	changed := false
	for _, p := range secretPatterns(key) {
		if strings.Contains(s, p) {
			s = strings.ReplaceAll(s, p, redactMask)
			changed = true
		}
	}
	return s, changed
}

// incStateCounter is a nil-safe counter increment (unit tests construct
// partial ProxyState structs).
func incStateCounter(c *metrics.Counter) {
	if c != nil {
		c.Inc(1)
	}
}

// aiErrorStreamFilter normalizes in-stream (SSE) error events: recognized
// error events get their data payload rewritten with the unified error
// JSON (the SSE structure is preserved); all other events pass through
// byte-identical. At EOF the filter marks stream truncation when the
// protocol's terminal event never arrived (protocols that end at EOF,
// e.g. Gemini, are never marked).
type aiErrorStreamFilter struct {
	src     io.ReadCloser
	adapter bfe_model_protocol.ProtocolAdapter
	cfg     cluster_conf.UpstreamErrorNormalizeConf
	aiMeta  *bfe_basic.AiBasicInfo
	state   *ProxyState

	raw     []byte // unconsumed source bytes
	pending []byte // transformed bytes not yet returned to the caller
	sawTerm bool   // a protocol terminal event was seen
	hit     bool   // at least one error event was rewritten
	eof     bool
	done    bool
}

func (f *aiErrorStreamFilter) Close() error {
	return f.src.Close()
}

func (f *aiErrorStreamFilter) Read(p []byte) (int, error) {
	if len(f.pending) == 0 {
		if f.done {
			return 0, io.EOF
		}
		if err := f.fill(); err != nil && len(f.pending) == 0 {
			return 0, err
		}
		if len(f.pending) == 0 {
			return 0, io.EOF
		}
	}
	n := copy(p, f.pending)
	f.pending = f.pending[n:]
	return n, nil
}

// fill pulls from the source, transforms complete SSE blocks, and
// finalizes (truncation marking) when the source reaches EOF.
func (f *aiErrorStreamFilter) fill() error {
	if f.eof {
		if len(f.raw) > 0 {
			// trailing partial block without a blank-line terminator:
			// flush it unchanged
			f.pending = append(f.pending, f.raw...)
			f.raw = nil
		}
		f.finalize()
		f.done = true
		return io.EOF
	}

	buf := make([]byte, 32*1024)
	n, err := f.src.Read(buf)
	if n > 0 {
		f.raw = append(f.raw, buf[:n]...)
		f.pending = append(f.pending, f.processBlocks()...)
	}
	if err == io.EOF {
		f.eof = true
		if len(f.raw) > 0 {
			f.pending = append(f.pending, f.raw...)
			f.raw = nil
		}
		f.finalize()
		f.done = true
		return io.EOF
	}
	return err
}

// splitSSEBlock finds the first blank-line separator ("\n\n" or
// "\r\n\r\n") and returns the block's start index and the separator bytes.
func splitSSEBlock(b []byte) (int, []byte) {
	for i := 0; i < len(b); i++ {
		// CRLF blank line "\r\n\r\n": the block ends before its first '\r'
		if b[i] == '\r' && i+3 < len(b) &&
			b[i+1] == '\n' && b[i+2] == '\r' && b[i+3] == '\n' {
			return i, b[i : i+4]
		}
		if b[i] == '\n' && i+1 < len(b) && b[i+1] == '\n' {
			return i, b[i : i+2]
		}
	}
	return -1, nil
}

// processBlocks consumes every complete SSE block from f.raw and returns
// the transformed wire bytes.
func (f *aiErrorStreamFilter) processBlocks() []byte {
	var out []byte
	for {
		idx, sep := splitSSEBlock(f.raw)
		if idx < 0 {
			return out
		}
		block := f.raw[:idx]
		out = append(out, f.processBlock(block, sep)...)
		f.raw = f.raw[idx+len(sep):]
	}
}

// processBlock transforms a single SSE block (without its terminating
// blank line). Unrecognized events are returned byte-identical (sep
// re-appended); error events are rebuilt with the unified error JSON as
// the data payload.
func (f *aiErrorStreamFilter) processBlock(block, sep []byte) []byte {
	eol := "\n"
	if len(sep) == 4 { // "\r\n\r\n"
		eol = "\r\n"
	}
	lines := strings.Split(string(block), eol)

	var dataLines []string
	for _, ln := range lines {
		if ln == "" || strings.HasPrefix(ln, ":") {
			continue
		}
		if strings.HasPrefix(ln, "data:") {
			d := strings.TrimPrefix(ln, "data:")
			d = strings.TrimPrefix(d, " ")
			dataLines = append(dataLines, d)
		}
	}
	data := strings.Join(dataLines, "\n")
	ev := utils.StreamEvent{
		Type: gjson.Get(data, "type").String(),
		Data: data,
	}

	perr := f.adapter.StreamErrorParser().ParseStreamError(ev)
	if perr == nil {
		if f.adapter.IsStreamTerminal(ev) {
			f.sawTerm = true
		}
		out := make([]byte, 0, len(block)+len(sep))
		out = append(out, block...)
		return append(out, sep...)
	}

	// Error event: rewrite the data payload, keep every other line and the
	// block's original line endings.
	aiErr := buildUnifiedAiError(perr, f.cfg, f.aiMeta, f.state)
	unified := aiErr.GenRespBodyStr()

	var nb bytes.Buffer
	for _, ln := range lines {
		if strings.HasPrefix(ln, "data:") {
			continue
		}
		nb.WriteString(ln)
		nb.WriteString(eol)
	}
	nb.WriteString("data: ")
	nb.WriteString(unified)
	nb.WriteString(eol)
	nb.Write(sep)

	f.hit = true
	f.aiMeta.ErrNormalized = true
	f.aiMeta.UpstreamStatus = int32(perr.StatusCode)
	f.aiMeta.UpstreamErrCode = perr.UpstreamCode
	return nb.Bytes()
}

// finalize records per-stream results on AiBasicInfo and the counters.
func (f *aiErrorStreamFilter) finalize() {
	if f.hit {
		f.aiMeta.StreamErrorRewritten = true
		incStateCounter(f.state.ErrStreamErrorRewritten)
	}
	if f.sawTerm {
		return
	}
	// Protocols whose streams end at HTTP EOF (e.g. Gemini) never report
	// truncation.
	if r, ok := f.adapter.(utils.StreamEndsAtEOFReporter); ok && r.StreamEndsAtEOF() {
		return
	}
	f.aiMeta.StreamTruncated = true
	incStateCounter(f.state.ErrStreamTruncated)
}

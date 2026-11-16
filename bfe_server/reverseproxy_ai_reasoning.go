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

// reverseproxy_ai_reasoning.go implements the provider-level compensating
// request normalization driven by AIConf.EnsureReasoningContent.
//
// Some thinking-mode upstreams require every assistant message after the last
// user message to carry reasoning_content (a missing field or null returns
// 400), yet have turns that produce no reasoning text. A client mirroring the
// upstream response therefore fails on the next turn; routing a session to a
// different provider makes the first request after the switch fail, because
// the assistant turn came from the previous backend. The gateway fills the
// empty string (the value the upstream itself emits for "no reasoning this
// turn") inside the validated window.
//
// Design: docs/zh_cn/sys_design/ai_reasoning_content_normalization.md

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_debug"
	"github.com/bfenetworks/bfe/bfe_http"
)

// errInvalidJSONBody reports a request body that is not valid JSON; the caller
// passes it through unchanged (fail-open).
var errInvalidJSONBody = errors.New("request body is not valid JSON")

// anthropicThinkingBlock is the empty thinking block injected at the head of
// an anthropic assistant content array. An empty signature is accepted by the
// upstream only if the same validation rule also tolerates an empty
// reasoning_content string; the anthropic protocol stays out of the default
// configuration until that is verified against the upstream.
const anthropicThinkingBlock = `{"type":"thinking","thinking":"","signature":""}`

// ensureReasoningContent injects an empty reasoning_content on the assistant
// messages inside the upstream validation window when the cluster opts in for
// the request's protocol. It reports whether the body was changed.
//
// It must run after the HandleAfterAITargetModel callbacks (mod_ai_context
// re-marshals the whole body there and drops an empty reasoning_content) and
// after the model rewrite, which also rewrites the body.
func (p *ReverseProxy) ensureReasoningContent(basicReq *bfe_basic.Request, outreq *bfe_http.Request,
	cfg *cluster_conf.AIConf, aiMeta *bfe_basic.AiBasicInfo) bool {
	if cfg == nil || len(cfg.EnsureReasoningContent) == 0 || outreq == nil || aiMeta == nil {
		return false
	}
	if !protocolListed(cfg.EnsureReasoningContent, aiMeta.AuthStyle) {
		return false
	}

	switch aiMeta.Mode {
	case bfe_basic.ModeChat:
	case bfe_basic.ModeFile, bfe_basic.ModeBatch:
		// file/batch bodies are streamed through unbuffered by mod_ai_batch
		incStateCounter(p.proxyState.ReasoningSkip)
		return false
	default:
		// /responses and other modes have no messages[].reasoning_content
		// injection point; leave them untouched
		return false
	}

	if isNonJsonContentType(outreq.Header) || hasRequestBodyEncoding(outreq.Header) {
		incStateCounter(p.proxyState.ReasoningSkip)
		return false
	}

	bodyAccessor, err := outreq.GetBodyAccessor()
	if err != nil || bodyAccessor == nil {
		incStateCounter(p.proxyState.ReasoningSkip)
		return false
	}
	body, all := bodyAccessor.GetBytes()
	if !all {
		// body larger than Server.AccessibleBodySize: only a prefix is
		// buffered, so it cannot be rewritten safely
		incStateCounter(p.proxyState.ReasoningSkip)
		return false
	}

	newBody, patched, err := patchReasoningContent(body, aiMeta.AuthStyle)
	if err != nil {
		incStateCounter(p.proxyState.ReasoningFail)
		log.Logger.Warn("ensureReasoningContent: skip, protocol[%s] err[%v] body[%.512s]",
			aiMeta.AuthStyle, err, body)
		return false
	}
	if patched == 0 {
		return false
	}

	bodyAccessor.SetBytes(newBody, false)

	// The body length changed: drop Content-Length on both the outbound request
	// and the original one, so a fallback/retry builds a consistent request
	// (same compensation as the model rewrite).
	outreq.ContentLength = -1
	outreq.Header.Del("Content-Length")
	if basicReq != nil && basicReq.HttpRequest != nil && basicReq.HttpRequest.ContentLength >= 0 {
		basicReq.HttpRequest.ContentLength = -1
		basicReq.HttpRequest.Header.Del("Content-Length")
	}

	aiMeta.ReasoningPatched = true
	incStateCounter(p.proxyState.ReasoningPatched)

	if bfe_debug.DebugServHTTP {
		log.Logger.Debug("ensureReasoningContent: patched[%d] protocol[%s] target_model[%s]",
			patched, aiMeta.AuthStyle, aiMeta.TargetModel)
	}

	return true
}

// patchReasoningContent adds an empty reasoning_content to the assistant
// messages that sit after the last user message and either miss the field or
// carry null. Existing non-empty values are never touched. The rewrite is
// in place (sjson): field order and every other byte of the body are
// preserved.
//
// Any parse or structure error returns the original body, so a caller can
// fail open without a second copy.
func patchReasoningContent(body []byte, protocol string) ([]byte, int, error) {
	// A truncated body still parses partially under gjson, and sjson would
	// append to it, turning a malformed request into a longer malformed one.
	// Validate first so such bodies are passed through untouched.
	if !gjson.ValidBytes(body) {
		return body, 0, errInvalidJSONBody
	}

	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body, 0, nil
	}

	msgs := messages.Array()

	lastUser := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Get("role").String() == "user" {
			lastUser = i
			break
		}
	}
	if lastUser < 0 {
		return body, 0, nil
	}

	orig := body
	patched := 0
	for i := lastUser + 1; i < len(msgs); i++ {
		if msgs[i].Get("role").String() != "assistant" {
			continue
		}
		reasoning := msgs[i].Get("reasoning_content")
		if reasoning.Exists() && reasoning.Type != gjson.Null {
			continue
		}

		var (
			next []byte
			hit  bool
			err  error
		)
		if protocol == bfe_basic.AuthStyleAnthropic {
			next, hit, err = prependThinkingBlock(body, fmt.Sprintf("messages.%d.content", i))
		} else {
			next, err = sjson.SetBytes(body, fmt.Sprintf("messages.%d.reasoning_content", i), "")
			hit = err == nil
		}
		if err != nil {
			return orig, 0, err
		}
		if !hit {
			continue
		}
		body = next
		patched++
	}

	return body, patched, nil
}

// prependThinkingBlock inserts the empty thinking block at the head of the
// assistant content array, shifting the existing elements right by one so
// each part keeps its original bytes. It reports false when the message has
// no content array or already carries a thinking block.
func prependThinkingBlock(body []byte, contentPath string) ([]byte, bool, error) {
	content := gjson.GetBytes(body, contentPath)
	if !content.Exists() || !content.IsArray() {
		return body, false, nil
	}

	parts := content.Array()
	for i := range parts {
		if parts[i].Get("type").String() == "thinking" {
			return body, false, nil
		}
	}

	for i := len(parts) - 1; i >= 0; i-- {
		next, err := sjson.SetRawBytes(body, fmt.Sprintf("%s.%d", contentPath, i+1), []byte(parts[i].Raw))
		if err != nil {
			return body, false, err
		}
		body = next
	}

	next, err := sjson.SetRawBytes(body, contentPath+".0", []byte(anthropicThinkingBlock))
	if err != nil {
		return body, false, err
	}

	return next, true, nil
}

// protocolListed reports whether protocol is in the configured list.
func protocolListed(protocols []string, protocol string) bool {
	for _, p := range protocols {
		if p == protocol {
			return true
		}
	}
	return false
}

// hasRequestBodyEncoding reports whether the request body carries an encoding
// other than identity, in which case the bytes are not the declared media
// type and must not be rewritten as JSON.
func hasRequestBodyEncoding(h bfe_http.Header) bool {
	encoding := strings.ToLower(strings.TrimSpace(h.Get("Content-Encoding")))
	return encoding != "" && encoding != "identity"
}

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
	"io"
	"strings"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_model_protocol"
	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

// streamTruncatedMessage is the message carried by the gateway-injected
// error event when the upstream connection drops mid-stream (issue #1474).
const streamTruncatedMessage = "upstream stream terminated abnormally."

// streamTruncationInjector is a pass-through response body filter that
// appends a protocol error event to the client stream when the upstream
// connection breaks before the protocol's terminal event (issue #1474).
// Unlike aiErrorStreamFilter it does not buffer or re-chunk SSE blocks:
// bytes flow through unmodified and the terminal event is detected with a
// cheap byte-marker scan, so it is safe to install by default.
type streamTruncationInjector struct {
	src     io.ReadCloser
	adapter bfe_model_protocol.ProtocolAdapter
	// cfg/aiMeta/state mirror aiErrorStreamFilter; unified selects the
	// payload form (unified AiError body vs the protocol-native envelope).
	cfg     cluster_conf.UpstreamErrorNormalizeConf
	unified bool
	aiMeta  *bfe_basic.AiBasicInfo
	state   *ProxyState
	markers [][]byte // lazily resolved terminal markers; nil = never inject

	pending []byte // injected event bytes not yet consumed
	saved   error  // source error, returned after pending is drained
	injDone bool   // the injection decision has been made
	marked  bool   // a terminal marker was seen in the passed-through bytes
	tail    []byte // overlap of the previous read, for split markers (issue #1473)
}

func newStreamTruncationInjector(src io.ReadCloser, adapter bfe_model_protocol.ProtocolAdapter,
	cfg cluster_conf.UpstreamErrorNormalizeConf, unified bool,
	aiMeta *bfe_basic.AiBasicInfo, state *ProxyState) *streamTruncationInjector {
	f := &streamTruncationInjector{
		src:     src,
		adapter: adapter,
		cfg:     cfg,
		unified: unified,
		aiMeta:  aiMeta,
		state:   state,
	}
	if adapter == nil {
		return f
	}
	// Protocols whose streams end at HTTP EOF (e.g. Gemini) never report
	// truncation, same exemption aiErrorStreamFilter.finalize applies.
	if r, ok := adapter.(utils.StreamEndsAtEOFReporter); ok && r.StreamEndsAtEOF() {
		return f
	}
	if m, ok := adapter.(utils.StreamTerminalMarkersReporter); ok {
		f.markers = m.StreamTerminalMarkers()
	}
	return f
}

func (f *streamTruncationInjector) Close() error {
	return f.src.Close()
}

func (f *streamTruncationInjector) Read(p []byte) (int, error) {
	if len(f.pending) > 0 {
		n := copy(p, f.pending)
		f.pending = f.pending[n:]
		return n, nil
	}
	if f.saved != nil {
		err := f.saved
		f.saved = nil
		return 0, err
	}
	if f.injDone {
		// After the saved error was delivered, keep the stream settled.
		return 0, io.EOF
	}

	n, err := f.src.Read(p)
	if n > 0 {
		f.scanTerminal(p[:n])
	}
	if err == io.EOF {
		// issue #1473: a clean EOF without the protocol's terminal event is a
		// truncated stream — the definition of the ai_stream_truncated field.
		// Judged only when the terminal markers are known: an unknown adapter
		// would otherwise mark every stream as truncated. Gemini reaches here
		// with empty markers (StreamEndsAtEOF) and stays unmarked. No event is
		// injected on a clean EOF: the client already received every byte.
		//
		// The event parser is authoritative over the byte-level markers: an
		// anthropic-compatible upstream may send only "data:" lines (no
		// "event: message_stop"), and body_process has already recorded the
		// completion in that case — a parsed terminal event is never a
		// truncation.
		if len(f.markers) > 0 && !f.terminated() && f.aiMeta != nil &&
			!f.aiMeta.StreamTruncated {
			f.aiMeta.StreamTruncated = true
			incStateCounter(f.state.ErrStreamTruncated)
		}
		return n, err
	}
	if err == nil {
		return n, err
	}

	// Non-EOF source error: the upstream connection broke. Decide the
	// injection once, queue the event (if any) ahead of the error, and
	// deliver the error only after every queued byte went out.
	f.injDone = true
	f.saved = err
	if len(f.markers) > 0 && !f.terminated() {
		f.pending = f.buildEvent()
		if f.aiMeta != nil && !f.aiMeta.StreamTruncated {
			f.aiMeta.StreamTruncated = true
			incStateCounter(f.state.ErrStreamTruncated)
		}
		incStateCounter(f.state.ErrStreamTruncationInjected)
	}
	if n > 0 {
		return n, nil
	}
	if len(f.pending) > 0 {
		m := copy(p, f.pending)
		f.pending = f.pending[m:]
		return m, nil
	}
	return 0, f.takeSaved()
}

// takeSaved returns and clears the saved source error (io.EOF when none).
func (f *streamTruncationInjector) takeSaved() error {
	if f.saved == nil {
		return io.EOF
	}
	err := f.saved
	f.saved = nil
	return err
}

// scanTerminal records whether a terminal marker appears in the passed bytes.
// The tail of the previous read is prepended so a marker split across two
// reads is still recognized: with issue #1473 a missed marker would also make
// a normally terminated stream look truncated at clean EOF.
func (f *streamTruncationInjector) scanTerminal(b []byte) {
	if f.sawTerminal() || len(f.markers) == 0 {
		return
	}

	buf := b
	if len(f.tail) > 0 {
		buf = make([]byte, 0, len(f.tail)+len(b))
		buf = append(buf, f.tail...)
		buf = append(buf, b...)
	}

	for _, m := range f.markers {
		if len(m) == 0 {
			continue
		}
		if strings.Contains(string(buf), string(m)) {
			f.marked = true
			f.tail = nil
			return
		}
	}

	maxLen := 0
	for _, m := range f.markers {
		if len(m) > maxLen {
			maxLen = len(m)
		}
	}
	keep := maxLen - 1
	if keep <= 0 {
		return
	}
	if len(buf) > keep {
		buf = buf[len(buf)-keep:]
	}
	// Copy: buf may alias the caller's read buffer.
	f.tail = append(f.tail[:0], buf...)
}

func (f *streamTruncationInjector) sawTerminal() bool {
	return f.marked
}

// terminated reports whether the stream had already ended normally: either the
// byte-level terminal marker was seen in the passed-through bytes, or the
// event parser recorded the protocol's terminal event. The second source
// matters for anthropic-compatible upstreams that send only "data:" lines —
// their "event: message_stop" marker never appears, yet the stream did
// terminate, so it must not be reported (nor answered with an injected error
// event) as truncated (issue #1473).
func (f *streamTruncationInjector) terminated() bool {
	if f.marked {
		return true
	}
	return f.aiMeta != nil && f.aiMeta.IsResponseCompleted()
}

// buildEvent builds the protocol error event for the client. When unified
// is set (streaming normalization on) the payload is the unified AiError
// body, matching the shape of rewritten upstream error events.
func (f *streamTruncationInjector) buildEvent() []byte {
	builder, ok := f.adapter.(utils.StreamErrorEventBuilder)
	if !ok {
		return nil
	}
	if f.unified {
		perr := &utils.ProtocolError{
			Code:       utils.CodeModelInternalError,
			StatusCode: 500,
			IsUpstream: true,
			Message:    streamTruncatedMessage,
		}
		aiErr := buildUnifiedAiError(perr, f.cfg, f.aiMeta, f.state)
		return builder.WrapStreamErrorPayload(aiErr.GenRespBodyStr())
	}
	return builder.NativeStreamErrorEvent(streamTruncatedMessage)
}

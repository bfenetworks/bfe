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
	if err == nil || err == io.EOF {
		return n, err
	}

	// Non-EOF source error: the upstream connection broke. Decide the
	// injection once, queue the event (if any) ahead of the error, and
	// deliver the error only after every queued byte went out.
	f.injDone = true
	f.saved = err
	if len(f.markers) > 0 && !f.sawTerminal() {
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

// scanTerminal records whether a terminal marker appears in the passed
// bytes. Markers spanning two reads are missed; the window is the marker
// length and the consequence is only an injected event after an already
// terminated stream, which clients ignore after their terminal event.
func (f *streamTruncationInjector) scanTerminal(b []byte) {
	if f.sawTerminal() {
		return
	}
	for _, m := range f.markers {
		if len(m) == 0 {
			continue
		}
		if strings.Contains(string(b), string(m)) {
			f.marked = true
			return
		}
	}
}

func (f *streamTruncationInjector) sawTerminal() bool {
	return f.marked
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

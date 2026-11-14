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
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/bfenetworks/go-lib/web-monitor/metrics"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_model_protocol/gemini"
	"github.com/bfenetworks/bfe/bfe_model_protocol/openai"
)

// copyErr builds a responseCopyError with the given side attribution, the same
// shape copyResponse hands back to the send path.
func copyErr(err error, readSide, clientClosed bool) error {
	return &responseCopyError{err: err, readSide: readSide, clientClosed: clientClosed}
}

// TestIsClientAbortConsequence pins which copy errors count as "the client
// went away": the close watcher fired, or the write to the client failed.
// A read error with the client still attached is an upstream fault.
func TestIsClientAbortConsequence(t *testing.T) {
	upstreamErr := errors.New("read tcp: use of closed network connection")

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"watcher fired, read error is collateral", copyErr(upstreamErr, true, true), true},
		{"client write failed", copyErr(upstreamErr, false, false), true},
		{"upstream read failed with client attached", copyErr(upstreamErr, true, false), false},
		{"plain error", upstreamErr, false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isClientAbortConsequence(tt.err); got != tt.want {
				t.Errorf("isClientAbortConsequence = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsClientCloseAfterComplete pins the issue #1473 predicate: only a close
// observed by the watcher on an already completed response qualifies. A failed
// write does not: those bytes may never have reached the client.
func TestIsClientCloseAfterComplete(t *testing.T) {
	upstreamErr := errors.New("read tcp: use of closed network connection")
	completed := func() *bfe_basic.AiBasicInfo {
		m := &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeChat}
		m.MarkResponseCompleted()
		return m
	}

	tests := []struct {
		name   string
		err    error
		aiMeta *bfe_basic.AiBasicInfo
		want   bool
	}{
		{"closed after complete", copyErr(upstreamErr, true, true), completed(), true},
		{"closed before complete", copyErr(upstreamErr, true, true), &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeChat}, false},
		{"write failed after complete stays a write error", copyErr(upstreamErr, false, false), completed(), false},
		{"upstream read error keeps drop semantics", copyErr(upstreamErr, true, false), completed(), false},
		{"nil aiMeta", copyErr(upstreamErr, true, true), nil, false},
		{"plain error", upstreamErr, completed(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isClientCloseAfterComplete(tt.err, tt.aiMeta); got != tt.want {
				t.Errorf("isClientCloseAfterComplete = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestClientAbortErrMsg pins the direction-correct access-log text: a client
// that went away must never be reported as an upstream read error.
func TestClientAbortErrMsg(t *testing.T) {
	upstreamErr := errors.New("BodyProcessError: read tcp 10.0.0.1: use of closed network connection")

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"watcher fired", copyErr(upstreamErr, true, true), errMsgClientClosed},
		{"client write failed", copyErr(upstreamErr, false, false), errMsgClientWriteFailed},
		{"upstream read error keeps raw text", copyErr(upstreamErr, true, false), upstreamErr.Error()},
		{"plain error keeps raw text", upstreamErr, upstreamErr.Error()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clientAbortErrMsg(tt.err); got != tt.want {
				t.Errorf("clientAbortErrMsg = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRecordCopyError covers the issue #1473 attribution matrix end to end:
// the err_code, the counters, the AI markers and the err_msg a request ends up
// with.
func TestRecordCopyError(t *testing.T) {
	upstreamErr := errors.New("BodyProcessError: read tcp 10.0.0.1: use of closed network connection")

	completed := func() *bfe_basic.AiBasicInfo {
		m := &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeChat, AuthStyle: bfe_basic.AuthStyleOpenAI}
		m.MarkResponseCompleted()
		return m
	}
	sseRes := func() *bfe_http.Response {
		return &bfe_http.Response{StatusCode: bfe_http.StatusOK, IsSse: true}
	}

	tests := []struct {
		name string
		err  error
		res  *bfe_http.Response
		meta *bfe_basic.AiBasicInfo

		wantErrCode   error
		wantErrMsg    string
		wantClosed    bool
		wantDropped   bool
		wantWriteErr  int64
		wantDropErr   int64
		wantClosedCnt int64
	}{
		{
			name: "complete, then the client closed",
			err:  copyErr(upstreamErr, true, true), res: sseRes(), meta: completed(),
			wantErrCode: nil, wantErrMsg: "", wantClosed: true,
			wantClosedCnt: 1,
		},
		{
			name: "client closed before the terminal event",
			err:  copyErr(upstreamErr, true, true), res: sseRes(), meta: &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeChat},
			wantErrCode: bfe_basic.ErrClientWrite, wantErrMsg: errMsgClientClosed,
			wantWriteErr: 1,
		},
		{
			name: "write failed after the terminal event",
			err:  copyErr(upstreamErr, false, false), res: sseRes(), meta: completed(),
			wantErrCode: bfe_basic.ErrClientWrite, wantErrMsg: errMsgClientWriteFailed,
			wantWriteErr: 1,
		},
		{
			name: "upstream dropped mid-stream",
			err:  copyErr(upstreamErr, true, false), res: sseRes(), meta: &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeChat},
			wantErrCode: bfe_basic.ErrBkReadRespBody, wantErrMsg: upstreamErr.Error(),
			wantDropped: true, wantDropErr: 1,
		},
		{
			name: "non-AI client abort keeps the historical label",
			err:  copyErr(upstreamErr, true, true), res: sseRes(), meta: nil,
			wantErrCode: bfe_basic.ErrClientWrite, wantErrMsg: errMsgClientClosed,
			wantWriteErr: 1,
		},
		{
			name: "non-AI upstream read error keeps the issue #1474 label",
			err:  copyErr(upstreamErr, true, false), res: sseRes(), meta: nil,
			wantErrCode: bfe_basic.ErrBkReadRespBody, wantErrMsg: upstreamErr.Error(),
			wantDropErr: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &ProxyState{
				ErrClientWrite:                 new(metrics.Counter),
				ErrBkReadRespBody:              new(metrics.Counter),
				ReqAiClientClosedAfterComplete: new(metrics.Counter),
			}
			p := &ReverseProxy{proxyState: state}
			req := &bfe_basic.Request{LogId: "test-logid"}

			p.recordCopyError(req, tt.res, tt.meta, tt.err)

			switch {
			case tt.wantErrCode == nil && req.ErrCode != nil:
				t.Errorf("expected no err_code, got %v", req.ErrCode)
			case tt.wantErrCode != nil && req.ErrCode != tt.wantErrCode:
				t.Errorf("err_code = %v, want %v", req.ErrCode, tt.wantErrCode)
			}
			if req.ErrMsg != tt.wantErrMsg {
				t.Errorf("err_msg = %q, want %q", req.ErrMsg, tt.wantErrMsg)
			}
			if got := state.ErrClientWrite.Get(); got != tt.wantWriteErr {
				t.Errorf("ErrClientWrite = %d, want %d", got, tt.wantWriteErr)
			}
			if got := state.ErrBkReadRespBody.Get(); got != tt.wantDropErr {
				t.Errorf("ErrBkReadRespBody = %d, want %d", got, tt.wantDropErr)
			}
			if got := state.ReqAiClientClosedAfterComplete.Get(); got != tt.wantClosedCnt {
				t.Errorf("ReqAiClientClosedAfterComplete = %d, want %d", got, tt.wantClosedCnt)
			}
			if tt.meta != nil {
				if tt.meta.ClientClosedAfterComplete != tt.wantClosed {
					t.Errorf("ClientClosedAfterComplete = %v, want %v",
						tt.meta.ClientClosedAfterComplete, tt.wantClosed)
				}
				if tt.meta.IsUpstreamDropped() != tt.wantDropped {
					t.Errorf("IsUpstreamDropped = %v, want %v", tt.meta.IsUpstreamDropped(), tt.wantDropped)
				}
			}
		})
	}
}

// TestStreamTruncationInjectorCleanEOF covers the issue #1473 half of the
// ai_stream_truncated field: a clean EOF without the protocol terminal event
// marks the stream truncated, while a terminated stream, an EOF-exempt adapter
// and a marker split across two reads do not.
func TestStreamTruncationInjectorCleanEOF(t *testing.T) {
	truncated := func(body string) (*ProxyState, bool) {
		state := &ProxyState{ErrStreamTruncated: new(metrics.Counter)}
		meta := &bfe_basic.AiBasicInfo{}
		f := newStreamTruncationInjector(io.NopCloser(strings.NewReader(body)), openai.New(),
			cluster_conf.UpstreamErrorNormalizeConf{}, false, meta, state)
		if _, err := io.ReadAll(f); err != nil {
			t.Fatalf("ReadAll err: %v", err)
		}
		return state, meta.StreamTruncated
	}

	t.Run("clean EOF without terminal event marks truncated", func(t *testing.T) {
		state, marked := truncated("data: {\"c\":1}\n\n")
		if !marked {
			t.Error("expected StreamTruncated to be set")
		}
		if got := state.ErrStreamTruncated.Get(); got != 1 {
			t.Errorf("ErrStreamTruncated = %d, want 1", got)
		}
	})

	t.Run("terminal event suppresses truncation", func(t *testing.T) {
		state, marked := truncated("data: {\"c\":1}\n\ndata: [DONE]\n\n")
		if marked {
			t.Error("terminated stream must not be marked truncated")
		}
		if got := state.ErrStreamTruncated.Get(); got != 0 {
			t.Errorf("ErrStreamTruncated = %d, want 0", got)
		}
	})

	t.Run("EOF-exempt adapter never marks", func(t *testing.T) {
		state := &ProxyState{ErrStreamTruncated: new(metrics.Counter)}
		meta := &bfe_basic.AiBasicInfo{}
		f := newStreamTruncationInjector(io.NopCloser(strings.NewReader("data: {\"c\":1}\n\n")),
			gemini.New(), cluster_conf.UpstreamErrorNormalizeConf{}, false, meta, state)
		if _, err := io.ReadAll(f); err != nil {
			t.Fatalf("ReadAll err: %v", err)
		}
		if meta.StreamTruncated {
			t.Error("gemini streams end at HTTP EOF and must never be marked truncated")
		}
		if got := state.ErrStreamTruncated.Get(); got != 0 {
			t.Errorf("ErrStreamTruncated = %d, want 0", got)
		}
	})

	t.Run("marker split across reads is still recognized", func(t *testing.T) {
		state := &ProxyState{ErrStreamTruncated: new(metrics.Counter)}
		meta := &bfe_basic.AiBasicInfo{}
		// One byte per read forces "[DONE]" to span many reads.
		f := newStreamTruncationInjector(&byteATimeReader{data: []byte("data: [DONE]\n\n")}, openai.New(),
			cluster_conf.UpstreamErrorNormalizeConf{}, false, meta, state)
		if _, err := io.ReadAll(f); err != nil {
			t.Fatalf("ReadAll err: %v", err)
		}
		if meta.StreamTruncated {
			t.Error("a terminal marker split across reads must still suppress truncation")
		}
	})

	t.Run("parsed terminal event suppresses truncation even without wire markers", func(t *testing.T) {
		// Anthropic-compatible upstreams may send only "data:" lines, so the
		// wire marker ("event: message_stop") is absent; body_process has
		// already parsed the terminal event and marked completion.
		state := &ProxyState{ErrStreamTruncated: new(metrics.Counter)}
		meta := &bfe_basic.AiBasicInfo{}
		meta.MarkResponseCompleted()
		f := newStreamTruncationInjector(io.NopCloser(strings.NewReader("data: {\"type\":\"message_stop\"}\n\n")),
			openai.New(), cluster_conf.UpstreamErrorNormalizeConf{}, false, meta, state)
		if _, err := io.ReadAll(f); err != nil {
			t.Fatalf("ReadAll err: %v", err)
		}
		if meta.StreamTruncated {
			t.Error("a parsed terminal event must never be reported as truncation")
		}
		if got := state.ErrStreamTruncated.Get(); got != 0 {
			t.Errorf("ErrStreamTruncated = %d, want 0", got)
		}
	})
}

// byteATimeReader yields at most one byte per Read, so a terminal marker is
// delivered in pieces.
type byteATimeReader struct {
	data []byte
}

func (r *byteATimeReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}

func (r *byteATimeReader) Close() error { return nil }

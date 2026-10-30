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

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_model_protocol/anthropic"
	"github.com/bfenetworks/bfe/bfe_model_protocol/gemini"
	"github.com/bfenetworks/bfe/bfe_model_protocol/openai"
)

// TestShouldMarkUpstreamDropped pins the issue #1474 upstream-drop mark
// matrix: a 200 response of a billable AI mode that had not already
// completed is marked; non-200, file/batch modes, completed responses and
// nil guards are rejected.
func TestShouldMarkUpstreamDropped(t *testing.T) {
	okRes := func() *bfe_http.Response {
		return &bfe_http.Response{StatusCode: bfe_http.StatusOK, IsSse: true}
	}
	aiMeta := func() *bfe_basic.AiBasicInfo {
		return &bfe_basic.AiBasicInfo{AuthStyle: bfe_basic.AuthStyleOpenAI, Mode: bfe_basic.ModeChat}
	}
	completedMeta := func() *bfe_basic.AiBasicInfo {
		m := aiMeta()
		m.MarkResponseCompleted()
		return m
	}

	tests := []struct {
		name   string
		res    *bfe_http.Response
		aiMeta *bfe_basic.AiBasicInfo
		want   bool
	}{
		{"200 SSE chat marks upstream dropped", okRes(), aiMeta(), true},
		{"non-200 never marks", &bfe_http.Response{StatusCode: bfe_http.StatusBadGateway, IsSse: true}, aiMeta(), false},
		{"non-SSE 200 marks (billing gate is mode-agnostic)", &bfe_http.Response{StatusCode: bfe_http.StatusOK}, aiMeta(), true},
		{"file mode excluded", okRes(), &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeFile}, false},
		{"batch mode excluded", okRes(), &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeBatch}, false},
		{"already-completed response keeps issue #1415 semantics", okRes(), completedMeta(), false},
		{"nil aiMeta rejected", okRes(), nil, false},
		{"nil response rejected", nil, aiMeta(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldMarkUpstreamDropped(tt.res, tt.aiMeta); got != tt.want {
				t.Errorf("shouldMarkUpstreamDropped() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsUpstreamDropErr pins the issue #1474 copy-error attribution: only a
// read-side copy error with a live client is an upstream drop; write-side
// errors and CloseWatcher-triggered (client-abort consequence) read errors
// keep the client-abort semantics, and unwrapped errors stay legacy
// client-write.
func TestIsUpstreamDropErr(t *testing.T) {
	readErr := errors.New("read tcp: connection reset by peer")
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"read-side copy error is an upstream drop", &responseCopyError{err: readErr, readSide: true}, true},
		{"write-side copy error is a client write", &responseCopyError{err: readErr, readSide: false}, false},
		{"watcher-fired read error stays a client abort", &responseCopyError{err: readErr, readSide: true, clientClosed: true}, false},
		{"plain error keeps legacy client-write semantics", readErr, false},
		{"nil is not an upstream drop", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUpstreamDropErr(tt.err); got != tt.want {
				t.Errorf("isUpstreamDropErr() = %v, want %v", got, tt.want)
			}
		})
	}
}

// errorReader yields payload bytes then a permanent non-EOF error,
// simulating an upstream that breaks the stream mid-response.
type errorReader struct {
	payload []byte
	err     error
	done    bool
}

func (r *errorReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	return copy(p, r.payload), nil
}

func (r *errorReader) Close() error { return nil }

// TestStreamTruncationInjector pins the issue #1474 injection behavior:
// pass-through is byte-identical, a mid-stream source error appends the
// protocol error event before the error surfaces, terminal markers suppress
// the injection, and EOF-exempt adapters never inject.
func TestStreamTruncationInjector(t *testing.T) {
	upstreamErr := errors.New("upstream connection broken")

	t.Run("clean EOF passes through untouched", func(t *testing.T) {
		src := io.NopCloser(strings.NewReader("data: {\"x\":1}\n\ndata: [DONE]\n\n"))
		f := newStreamTruncationInjector(src, openai.New(), cluster_conf.UpstreamErrorNormalizeConf{}, false,
			&bfe_basic.AiBasicInfo{}, &ProxyState{})
		got, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll err: %v", err)
		}
		if string(got) != "data: {\"x\":1}\n\ndata: [DONE]\n\n" {
			t.Errorf("pass-through changed bytes: %q", got)
		}
	})

	t.Run("openai upstream drop injects native error event", func(t *testing.T) {
		src := io.NopCloser(&errorReader{payload: []byte("data: {\"c\":1}\n\n"), err: upstreamErr})
		aiMeta := &bfe_basic.AiBasicInfo{}
		state := &ProxyState{}
		f := newStreamTruncationInjector(src, openai.New(), cluster_conf.UpstreamErrorNormalizeConf{}, false, aiMeta, state)
		got, err := io.ReadAll(f)
		if err == nil {
			t.Fatalf("expected the upstream error to surface, got nil")
		}
		if !strings.HasPrefix(string(got), "data: {\"c\":1}\n\n") {
			t.Errorf("forwarded content lost: %q", got)
		}
		if !strings.Contains(string(got), "data: {\"error\":{\"message\":\"upstream stream terminated abnormally.\"") {
			t.Errorf("native openai error event missing: %q", got)
		}
		if !strings.HasSuffix(string(got), "\n\n") {
			t.Errorf("injected event not terminated with a blank line: %q", got)
		}
		if !aiMeta.StreamTruncated {
			t.Errorf("StreamTruncated not marked")
		}
	})

	t.Run("anthropic upstream drop injects event error event", func(t *testing.T) {
		src := io.NopCloser(&errorReader{payload: []byte("event: content_block_delta\ndata: {}\n\n"), err: upstreamErr})
		f := newStreamTruncationInjector(src, anthropic.New(), cluster_conf.UpstreamErrorNormalizeConf{}, false,
			&bfe_basic.AiBasicInfo{}, &ProxyState{})
		got, err := io.ReadAll(f)
		if err == nil {
			t.Fatalf("expected the upstream error to surface, got nil")
		}
		if !strings.Contains(string(got), "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\"") {
			t.Errorf("native anthropic error event missing: %q", got)
		}
	})

	t.Run("terminal marker suppresses injection", func(t *testing.T) {
		src := io.NopCloser(&errorReader{payload: []byte("data: {\"c\":1}\n\ndata: [DONE]\n\n"), err: upstreamErr})
		f := newStreamTruncationInjector(src, openai.New(), cluster_conf.UpstreamErrorNormalizeConf{}, false,
			&bfe_basic.AiBasicInfo{}, &ProxyState{})
		got, err := io.ReadAll(f)
		if err == nil {
			t.Fatalf("expected the upstream error to surface, got nil")
		}
		if strings.Contains(string(got), "upstream stream terminated abnormally.") {
			t.Errorf("injected after a terminal event: %q", got)
		}
	})

	t.Run("anthropic message_stop suppresses injection", func(t *testing.T) {
		payload := "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
		src := io.NopCloser(&errorReader{payload: []byte(payload), err: upstreamErr})
		f := newStreamTruncationInjector(src, anthropic.New(), cluster_conf.UpstreamErrorNormalizeConf{}, false,
			&bfe_basic.AiBasicInfo{}, &ProxyState{})
		got, err := io.ReadAll(f)
		if err == nil {
			t.Fatalf("expected the upstream error to surface, got nil")
		}
		if strings.Contains(string(got), "event: error") {
			t.Errorf("injected after message_stop: %q", got)
		}
	})

	t.Run("gemini EOF-exempt adapter never injects", func(t *testing.T) {
		src := io.NopCloser(&errorReader{payload: []byte("data: {\"c\":1}\n\n"), err: upstreamErr})
		f := newStreamTruncationInjector(src, gemini.New(), cluster_conf.UpstreamErrorNormalizeConf{}, false,
			&bfe_basic.AiBasicInfo{}, &ProxyState{})
		got, err := io.ReadAll(f)
		if err == nil {
			t.Fatalf("expected the upstream error to surface, got nil")
		}
		if strings.Contains(string(got), "upstream stream terminated abnormally.") {
			t.Errorf("gemini must never be injected: %q", got)
		}
	})

	t.Run("unified payload wraps the unified AiError body", func(t *testing.T) {
		src := io.NopCloser(&errorReader{payload: []byte("data: {\"c\":1}\n\n"), err: upstreamErr})
		cfg := cluster_conf.UpstreamErrorNormalizeConf{Enabled: true, StreamEnabled: true}
		f := newStreamTruncationInjector(src, openai.New(), cfg.Effective(), true,
			&bfe_basic.AiBasicInfo{AuthStyle: bfe_basic.AuthStyleOpenAI, ClientModel: "m"}, &ProxyState{})
		got, err := io.ReadAll(f)
		if err == nil {
			t.Fatalf("expected the upstream error to surface, got nil")
		}
		// The unified body carries the gateway catalog code for the
		// internal-error class.
		if !strings.Contains(string(got), "MODEL_INTERNAL_ERROR") {
			t.Errorf("unified payload missing: %q", got)
		}
	})
}

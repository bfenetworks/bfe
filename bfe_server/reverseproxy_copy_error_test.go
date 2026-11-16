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
	"errors"
	"io"
	"testing"

	"github.com/bfenetworks/bfe/bfe_util"
)

// failWriter fails every write, simulating a gone client.
type failWriter struct{}

func (failWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write to client failed")
}

// TestCopyResponseErrorAttribution pins issue #1474: copyResponse wraps the
// failing side of the copy so a mid-stream upstream read error is
// distinguishable from a client write error.
func TestCopyResponseErrorAttribution(t *testing.T) {
	p := &ReverseProxy{bufferPool: bfe_util.NewFixedPool(32 * 1024)}
	upstreamErr := errors.New("read tcp: connection reset by peer")

	t.Run("upstream read error classified read-side", func(t *testing.T) {
		src := io.NopCloser(&errorReader{payload: []byte("data: x\n\n"), err: upstreamErr})
		err := p.copyResponse(&bytes.Buffer{}, src, 0, false)
		if err == nil {
			t.Fatalf("expected copy error, got nil")
		}
		var ce *responseCopyError
		if !errors.As(err, &ce) {
			t.Fatalf("error not wrapped in responseCopyError: %T %v", err, err)
		}
		if !ce.readSide {
			t.Errorf("readSide = false, want true for an upstream read error")
		}
		if !isUpstreamDropErr(err) {
			t.Errorf("isUpstreamDropErr = false, want true")
		}
	})

	t.Run("client write error classified write-side", func(t *testing.T) {
		src := io.NopCloser(&errorReader{payload: []byte("data: x\n\n"), err: upstreamErr})
		err := p.copyResponse(failWriter{}, src, 0, false)
		if err == nil {
			t.Fatalf("expected copy error, got nil")
		}
		var ce *responseCopyError
		if !errors.As(err, &ce) {
			t.Fatalf("error not wrapped in responseCopyError: %T %v", err, err)
		}
		if ce.readSide {
			t.Errorf("readSide = true, want false for a client write error")
		}
		if isUpstreamDropErr(err) {
			t.Errorf("isUpstreamDropErr = true, want false for a client write error")
		}
	})

	t.Run("successful copy stays unwrapped", func(t *testing.T) {
		src := io.NopCloser(bytes.NewReader([]byte("data: x\n\n")))
		if err := p.copyResponse(&bytes.Buffer{}, src, 0, false); err != nil {
			t.Fatalf("copyResponse() = %v, want nil", err)
		}
	})
}

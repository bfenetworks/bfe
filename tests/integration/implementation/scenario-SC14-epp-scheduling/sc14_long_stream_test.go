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

package sc14

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

// TestTC08_LongStreamingResponseClosesAndDrains verifies issue #1475: a long
// SSE response through an EPP cluster must finish cleanly — the connection
// closes after [DONE] and the request is billed. The mock EPP replicates the
// llm-d StreamingServer behavior (a ProcessingResponse per response-body
// chunk); before the fix, bfe never drained those responses and both sides
// deadlocked on gRPC flow control once the body exceeded the HTTP/2 windows:
// the client saw the full body but the connection hung and the finish handler
// (billing point) never ran.
func TestTC08_LongStreamingResponseClosesAndDrains(t *testing.T) {
	// ~1.1MB of SSE frames: well above the gRPC flow-control window so the
	// pre-fix code deadlocks (the production threshold was ~1500 tokens).
	const frames = 5000
	chunk := fmt.Sprintf(`{"id":"chatcmpl-long","choices":[{"delta":{"content":%q}}]}`,
		strings.Repeat("x", 180))

	backend := common.NewMockBackend(clusterName, http.StatusOK, "")
	raw := make([]string, 0, frames)
	for i := 0; i < frames; i++ {
		raw = append(raw, "data: "+chunk+"\n\n")
	}
	backend.SSERaw = raw

	eppMock := common.NewMockEPP(t, common.WithPerChunkResponse())
	eppMock.SetDecisionEndpoint(backend.Addr())

	eppConf := &common.EPPClusterConf{
		Addrs:  []string{eppMock.Addr()},
		EPPTLS: map[string]interface{}{"Insecure": true},
	}

	e := newBackendTestEnv(t, backend, eppMock, eppConf)

	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", e.httpPort),
		strings.NewReader(`{"model":"epp-test-model","stream":true,"messages":[{"role":"user","content":"hello-epp-tc08"}]}`))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Host = testHost
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	// The 25s timeout is the regression tripwire: pre-fix, Close() hung on
	// donech forever and the response never terminated, so this request
	// timed out (production: 90s client timeout).
	resp, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("long streaming request failed (connection not closed / response not terminated): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("reading the long stream failed: %v", err)
	}
	if elapsed > 20*time.Second {
		t.Fatalf("long stream took %v, want well under the 25s timeout (close-after-[DONE] regression)", elapsed)
	}

	wantBytes := frames * len("data: "+chunk+"\n\n")
	if len(body) < wantBytes {
		t.Fatalf("client received %d bytes, want %d (full SSE body)", len(body), wantBytes)
	}
	if !strings.Contains(string(body), "chatcmpl-long") {
		t.Fatalf("client stream missing payload: %q...", body[:min(200, len(body))])
	}

	// the EPP must have received the complete body: with the pre-fix code
	// the exchange stalled mid-stream once the responses backed up
	respBodies := eppMock.ResponseBodies()
	if len(respBodies) != 1 {
		t.Fatalf("epp response bodies count = %d, want 1", len(respBodies))
	}
	if len(respBodies[0]) < len(body) {
		t.Fatalf("body relayed to EPP (%d bytes) is shorter than the client body (%d bytes): exchange did not drain",
			len(respBodies[0]), len(body))
	}
}

// newBackendTestEnv builds a SC14 environment whose cluster backend is a mock
// HTTP backend (instead of the inference-sim) so a test controls the response
// shape exactly; scheduling decisions come from the given mock EPP.
func newBackendTestEnv(t *testing.T, backend *common.MockBackend, eppMock *common.MockEPP, eppConf *common.EPPClusterConf) *testEnv {
	e := &testEnv{t: t}
	defer func() { t.Cleanup(e.Close) }()

	logDir := filepath.Join(t.TempDir(), "log")
	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends:      map[string]*common.MockBackend{clusterName: backend},
		EPPClusters:   map[string]*common.EPPClusterConf{clusterName: eppConf},
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	e.httpPort, e.bfeMonitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, logDir)
	e.epp = []*common.MockEPP{eppMock}
	return e
}

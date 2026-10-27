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

package sc03

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

// issue #1468: a compressed SSE response must be parsed for usage from a
// decompressed copy while the bytes forwarded to the client stay identical.

const (
	initialRMBQuota int64 = 10000000000
	// TC-25 expected deduction, in fixed point units (1e-8 RMB):
	// normal_input = 296476 - 296320 = 156 -> 156*100
	// cache_read   = 296320             -> 296320*2
	// output       = 160                -> 160*200
	compressedStreamDeduction int64 = 640240
)

// cacheHitStreamUsage is the final OpenAI chunk of the field reproduction: a
// 96% prompt-cache hit, so the bill is dominated by the cache_read leg and any
// parse failure (cache_read = 0) overcharges by roughly 46x.
const cacheHitStreamUsage = `{"usage":{"prompt_tokens":296476,"completion_tokens":160,` +
	`"total_tokens":296636,"prompt_tokens_details":{"cached_tokens":296320}}}`

// cacheHitStreamEvents is the same stream split into frames, as the upstream
// flushes them.
var cacheHitStreamEvents = []string{
	`{"choices":[{"delta":{"role":"assistant"}}]}`,
	`{"choices":[{"delta":{"content":"hello"}}]}`,
	cacheHitStreamUsage,
}

// ssePlaintext renders events the way the mock backend's SSE mode does, i.e. the
// plaintext a client sees after decompressing the forwarded bytes.
func ssePlaintext(events []string) string {
	var b strings.Builder
	for _, event := range events {
		b.WriteString("data: " + event + "\n\n")
	}
	return b.String()
}

// compressedStreamAIConf is defaultRMBAIConf with a cache_read price added to
// the deepseek-chat entry, so the cache-hit split can be asserted (the default
// fixture prices the input and output legs only).
func compressedStreamAIConf() *cluster_conf.AIConf {
	conf := defaultRMBAIConf()
	conf.ModelTable.Models[0].Prices = cluster_conf.PriceMap{
		"input_cost_per_token":        0.000001,   // 100 fixed point
		"output_cost_per_token":       0.000002,   // 200
		"cache_read_input_token_cost": 0.00000002, // 2
	}
	return conf
}

// bodyProcessMetricsRaw fetches /monitor/mod_body_process in prometheus format.
func (e *testEnv) bodyProcessMetricsRaw() string {
	e.t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/monitor/mod_body_process?format=prometheus", e.bfeMonitorPort)
	resp, err := http.Get(url)
	if err != nil {
		e.t.Fatalf("get mod_body_process monitor failed: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("mod_body_process monitor status = %d, body: %s", resp.StatusCode, data)
	}
	return string(data)
}

// bodyProcessMetric returns the value of the named mod_body_process counter
// (0 when absent), e.g. "ResStreamDecoded" -> "mod_body_process_RES_STREAM_DECODED".
func (e *testEnv) bodyProcessMetric(name string) float64 {
	e.t.Helper()
	data := e.bodyProcessMetricsRaw()

	fullName := "mod_body_process_" + camelToSnake(name)
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.LastIndex(line, " ")
		if idx < 0 || strings.TrimSpace(line[:idx]) != fullName {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(line[idx+1:]), 64)
		if err != nil {
			continue
		}
		return v
	}
	return 0
}

// waitForQuota polls the RMB quota until it equals want, or reports the last
// observed value when the timeout elapses (the deduction is asynchronous, and a
// fixed sleep makes the assertion flaky under load).
func (e *testEnv) waitForQuota(want int64) int64 {
	e.t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	var got int64
	for time.Now().Before(deadline) {
		got = e.redis.GetQuota(redisKeyRMB)
		if got == want {
			return got
		}
		time.Sleep(50 * time.Millisecond)
	}
	return got
}

// TestTC25 verifies issue #1468: when the upstream returns a gzip-compressed
// streaming SSE response, the usage must be parsed from the decompressed
// plaintext (cache_read_tokens included) while the forwarded bytes and the
// Content-Encoding header stay exactly what the upstream sent.
//
// Before the fix mod_body_process bypassed any Content-Encoding body to
// LineDecoder, so the compressed stream was never parsed: cache_read_tokens was
// always 0 and the whole prompt was billed at the input price.
func TestTC25_RMBQuotaDeduction_CompressedStreamUsage(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: compressedStreamAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(initialRMBQuota)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialRMBQuota)

	backend := e.backends[clusterRMB]
	backend.SSEEvents = cacheHitStreamEvents
	backend.SSECompress = "gzip"

	// Accept-Encoding is set explicitly so net/http does not transparently
	// gunzip the response: the test then observes exactly the bytes BFE
	// forwards.
	resp, body, err := e.sendRequestWithHeaders(apiHost, apiPath, streamBody, map[string]string{
		"Accept-Encoding": "gzip, deflate, br, zstd",
	})
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if backend.Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, backend.Hits())
	}

	// Forwarding stays byte-identical: gzip payload + Content-Encoding header.
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}
	zr, err := gzip.NewReader(strings.NewReader(body))
	if err != nil {
		t.Fatalf("forwarded body is not gzip: %v", err)
	}
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip forwarded body: %v", err)
	}
	if want := ssePlaintext(cacheHitStreamEvents); string(decoded) != want {
		t.Errorf("forwarded plaintext mismatch:\n got: %s\nwant: %s", decoded, want)
	}

	// The decompress-for-parsing tap ran; nothing degraded.
	if got := e.bodyProcessMetric("ResStreamCompressed"); got != 1 {
		t.Errorf("ResStreamCompressed = %v, want 1", got)
	}
	if got := e.bodyProcessMetric("ResStreamDecoded"); got != 1 {
		t.Errorf("ResStreamDecoded = %v, want 1", got)
	}
	if got := e.bodyProcessMetric("ResStreamDecodeSkipped"); got != 0 {
		t.Errorf("ResStreamDecodeSkipped = %v, want 0", got)
	}
	if got := e.bodyProcessMetric("ResStreamRawOnly"); got != 0 {
		t.Errorf("ResStreamRawOnly = %v, want 0", got)
	}

	// The upstream's real usage reached the bill: cache_read is charged at the
	// cache price instead of the whole prompt at the input price.
	want := initialRMBQuota - compressedStreamDeduction
	if remaining := e.waitForQuota(want); remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d (parsed usage, response body: %s)",
			remaining, want, body)
	}
}

// TestTC26 verifies the outbound Accept-Encoding narrowing of issue #1468: the
// upstream must never be offered a coding the gateway cannot decode, because a
// coding it cannot parse silently turns the bill into an estimate. The header is
// only rewritten when the intersection with the client's list is non-empty.
func TestTC26_RMBQuotaDeduction_AcceptEncodingNarrowed(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: compressedStreamAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(initialRMBQuota)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialRMBQuota)

	backend := e.backends[clusterRMB]
	backend.SSEEvents = cacheHitStreamEvents
	backend.SSECompress = "gzip"

	// The v1.8.8 MockBackend does not record request headers, so the header the
	// upstream actually received is captured through ResponseFunc (it is called
	// for every request, including SSE mode).
	var mu sync.Mutex
	var received []string
	backend.ResponseFunc = func(r *http.Request, count int) (int, string) {
		mu.Lock()
		received = append(received, r.Header.Get("Accept-Encoding"))
		mu.Unlock()
		return http.StatusOK, ""
	}

	arms := []struct {
		name   string
		client string
		wantUp string
	}{
		{"narrows the client list", "gzip, deflate, br, zstd", "gzip, deflate"},
		{"keeps q values", "gzip;q=0.5, zstd", "gzip;q=0.5"},
		{"empty intersection is not rewritten", "zstd", "zstd"},
	}

	for _, arm := range arms {
		resp, body, err := e.sendRequestWithHeaders(apiHost, apiPath, streamBody, map[string]string{
			"Accept-Encoding": arm.client,
		})
		if err != nil {
			t.Fatalf("%s: send request failed: %v", arm.name, err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("%s: expected status 200, got %d, body: %s", arm.name, resp.StatusCode, body)
		}

		mu.Lock()
		got := ""
		if n := len(received); n > 0 {
			got = received[n-1]
		}
		mu.Unlock()
		if got != arm.wantUp {
			t.Errorf("%s: upstream Accept-Encoding = %q, want %q", arm.name, got, arm.wantUp)
		}
		// The forwarded bytes are still gzip and still parseable.
		if gotCE := resp.Header.Get("Content-Encoding"); gotCE != "gzip" {
			t.Errorf("%s: Content-Encoding = %q, want gzip", arm.name, gotCE)
		}
	}

	mu.Lock()
	armCount := len(received)
	mu.Unlock()
	if armCount != len(arms) {
		t.Fatalf("recorded %d upstream requests, want %d", armCount, len(arms))
	}

	// All three arms bill by the parsed usage (the mock always answers gzip).
	want := initialRMBQuota - 3*compressedStreamDeduction
	if remaining := e.waitForQuota(want); remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d", remaining, want)
	}
	if got := e.bodyProcessMetric("ResStreamDecoded"); got != 3 {
		t.Errorf("ResStreamDecoded = %v, want 3", got)
	}
}

// TestTC27 verifies the fail-open path of issue #1468: when the upstream ignores
// the narrowed negotiation and answers with a coding the gateway cannot decode,
// the response is forwarded verbatim and the request falls back to the estimate,
// visibly (counter + usage source) rather than silently.
func TestTC27_RMBQuotaDeduction_StreamUnsupportedEncodingFailsOpen(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: compressedStreamAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(initialRMBQuota)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialRMBQuota)

	// "compress" is not in the decodable set, so no tap is built and the bytes
	// pass through untouched. The body is an opaque payload (as a real LZW body
	// would be): it must not parse as SSE/JSON, otherwise the LineDecoder
	// pass-through would recover the usage by accident (#1364 parses the
	// usage-bearing line of an ndjson payload).
	opaque := strings.Repeat("\x00\x01\x02\x03compress-opaque\xff", 64)
	backend := e.backends[clusterRMB]
	backend.ResponseHeaders = map[string]string{
		"Content-Type":     "text/event-stream",
		"Content-Encoding": "compress",
	}
	backend.NoContentLength = true
	backend.Body = opaque

	resp, body, err := e.sendRequestWithHeaders(apiHost, apiPath, streamBody, map[string]string{
		"Accept-Encoding": "gzip, deflate, br, zstd",
	})
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	// Forwarding is untouched: same header, same bytes.
	if got := resp.Header.Get("Content-Encoding"); got != "compress" {
		t.Errorf("Content-Encoding = %q, want compress", got)
	}
	if body != opaque {
		t.Errorf("forwarded body mismatch: got %d bytes, want %d", len(body), len(opaque))
	}

	// Degradation is visible: skipped, not decoded, not a mid-stream downgrade.
	if got := e.bodyProcessMetric("ResStreamCompressed"); got != 1 {
		t.Errorf("ResStreamCompressed = %v, want 1", got)
	}
	if got := e.bodyProcessMetric("ResStreamDecodeSkipped"); got != 1 {
		t.Errorf("ResStreamDecodeSkipped = %v, want 1", got)
	}
	if got := e.bodyProcessMetric("ResStreamDecoded"); got != 0 {
		t.Errorf("ResStreamDecoded = %v, want 0", got)
	}
	if got := e.bodyProcessMetric("ResStreamRawOnly"); got != 0 {
		t.Errorf("ResStreamRawOnly = %v, want 0", got)
	}

	// The estimate fallback still bills, but not the parsed amount: this pins
	// that the request did not silently take the parsed path.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	if remaining >= initialRMBQuota {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want less than %d (estimate fallback must still bill)",
			remaining, initialRMBQuota)
	}
	if want := initialRMBQuota - compressedStreamDeduction; remaining == want {
		t.Errorf("remaining quota = %d equals the parsed-usage amount, but the encoding is not decodable", remaining)
	}
}

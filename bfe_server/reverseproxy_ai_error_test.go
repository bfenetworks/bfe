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
	"encoding/base64"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_model_protocol"
	"github.com/bfenetworks/bfe/bfe_route/bfe_cluster"
)

func testClusterWithNormalize(cfg *cluster_conf.UpstreamErrorNormalizeConf) *bfe_cluster.BfeCluster {
	c := bfe_cluster.NewBfeCluster("cluster_ai")
	c.AIConf = &cluster_conf.AIConf{NormalizeUpstreamError: cfg}
	return c
}

func testNormalizeState() *ProxyState {
	return &ProxyState{}
}

func TestRedactSecretMaterial(t *testing.T) {
	key := "sk-test-key"
	b64 := base64.StdEncoding.EncodeToString([]byte(key))
	raw := "Invalid API key provided: " + key + " (base64: " + b64 + ")"
	body := "key=" + key + "&other=" + b64

	if got, changed := redactSecretMaterial("nothing to hide", key); changed || got != "nothing to hide" {
		t.Errorf("no-op redaction changed=%v got=%q", changed, got)
	}
	got, changed := redactSecretMaterial(raw, key)
	if !changed || strings.Contains(got, key) || strings.Contains(got, b64) {
		t.Errorf("raw/base64 key not redacted: %q", got)
	}
	got, changed = redactSecretMaterial(body, key)
	if !changed || strings.Contains(got, b64) {
		t.Errorf("base64 form not redacted: %q", got)
	}
	// empty key is a no-op
	if _, changed := redactSecretMaterial(raw, ""); changed {
		t.Error("empty key must not redact")
	}
}

func TestSecretPatternsDedup(t *testing.T) {
	// an alphanumeric-only key has url-escape == raw; patterns must not dup
	pats := secretPatterns("abcdef123")
	seen := map[string]bool{}
	for _, p := range pats {
		if seen[p] {
			t.Errorf("duplicate pattern %q", p)
		}
		seen[p] = true
	}
	if len(pats) < 2 {
		t.Errorf("expected raw + derived patterns, got %v", pats)
	}
}

func TestNormalizeUpstreamErrorDisabled(t *testing.T) {
	// nil cluster conf: response untouched
	res := &bfe_http.Response{
		StatusCode: 429,
		Header:     bfe_http.Header{"Content-Type": []string{"application/json"}},
		Body:       ioutil.NopCloser(strings.NewReader(`{"error":{"message":"slow"}}`)),
	}
	aiMeta := &bfe_basic.AiBasicInfo{AuthStyle: "openai"}
	normalizeUpstreamError(res, testClusterWithNormalize(nil), aiMeta, testNormalizeState())
	if aiMeta.ErrNormalized || aiMeta.ErrNormalizeMiss {
		t.Error("disabled config must not touch aiMeta")
	}
	if res.StatusCode != 429 {
		t.Errorf("status changed to %d", res.StatusCode)
	}
}

func TestNormalizeUpstreamErrorSkipsGatewayGenerated(t *testing.T) {
	cfg := &cluster_conf.UpstreamErrorNormalizeConf{Enabled: true}
	res := &bfe_http.Response{
		StatusCode: 400,
		Header: bfe_http.Header{
			"Content-Type":             []string{"application/json"},
			bfe_basic.HeaderBfeGwError: []string{"1"},
		},
		Body: ioutil.NopCloser(strings.NewReader(`{"error":{"code":"MODEL_NOT_ALLOWED"}}`)),
	}
	aiMeta := &bfe_basic.AiBasicInfo{AuthStyle: "openai"}
	normalizeUpstreamError(res, testClusterWithNormalize(cfg), aiMeta, testNormalizeState())
	if aiMeta.ErrNormalized || aiMeta.ErrNormalizeMiss {
		t.Error("gateway-generated response must be skipped")
	}
	if res.StatusCode != 400 {
		t.Errorf("status changed to %d", res.StatusCode)
	}
}

func TestNormalizeUpstreamErrorBodyRewrite(t *testing.T) {
	cfg := &cluster_conf.UpstreamErrorNormalizeConf{Enabled: true}
	cluster := testClusterWithNormalize(cfg)
	upstreamBody := `{"error":{"message":"Incorrect API key provided: sk-echoed-key","type":"authentication_error","code":"invalid_api_key"}}`
	res := &bfe_http.Response{
		StatusCode: 401,
		Header:     bfe_http.Header{"Content-Type": []string{"application/json"}},
		Body:       ioutil.NopCloser(strings.NewReader(upstreamBody)),
	}
	aiMeta := &bfe_basic.AiBasicInfo{
		AuthStyle:   "openai",
		ClientModel: "gpt-4",
		UpstreamKey: "sk-echoed-key",
	}
	normalizeUpstreamError(res, cluster, aiMeta, testNormalizeState())

	if !aiMeta.ErrNormalized || aiMeta.ErrNormalizeMiss {
		t.Errorf("aiMeta = %+v, want normalized", aiMeta)
	}
	if aiMeta.UpstreamStatus != 401 || aiMeta.UpstreamErrCode != "invalid_api_key" {
		t.Errorf("upstream recording = %d/%q", aiMeta.UpstreamStatus, aiMeta.UpstreamErrCode)
	}
	if res.StatusCode != 502 {
		t.Errorf("status = %d, want 502 (UPSTREAM_AUTH_ERROR)", res.StatusCode)
	}
	body, _ := ioutil.ReadAll(res.Body)
	s := string(body)
	if !strings.Contains(s, bfe_basic.CodeUpstreamAuthError) {
		t.Errorf("body missing unified code: %s", s)
	}
	if strings.Contains(s, "sk-echoed-key") || !strings.Contains(s, redactMask) {
		t.Errorf("credential not redacted: %s", s)
	}
	if !strings.Contains(s, `"upstream_status":401`) || !strings.Contains(s, `"upstream_code":"invalid_api_key"`) {
		t.Errorf("details missing upstream info: %s", s)
	}
	if res.Header.Get(bfe_basic.HeaderBfeGwError) != "1" {
		t.Error("rewritten response must carry the gateway marker")
	}
}

func TestNormalizeUpstreamErrorBodyPassthroughRedacts(t *testing.T) {
	cfg := &cluster_conf.UpstreamErrorNormalizeConf{Enabled: true}
	// unrecognized HTML error page that echoes the key
	upstreamBody := `<html>proxy error key=sk-secret-abc</html>`
	res := &bfe_http.Response{
		StatusCode: 502,
		Header:     bfe_http.Header{"Content-Type": []string{"text/html"}},
		Body:       ioutil.NopCloser(strings.NewReader(upstreamBody)),
	}
	aiMeta := &bfe_basic.AiBasicInfo{AuthStyle: "openai", UpstreamKey: "sk-secret-abc"}
	normalizeUpstreamError(res, testClusterWithNormalize(cfg), aiMeta, testNormalizeState())

	if !aiMeta.ErrNormalizeMiss || aiMeta.ErrNormalized {
		t.Errorf("aiMeta = %+v, want miss/passthrough", aiMeta)
	}
	if res.StatusCode != 502 {
		t.Errorf("passthrough must keep upstream status, got %d", res.StatusCode)
	}
	body, _ := ioutil.ReadAll(res.Body)
	if strings.Contains(string(body), "sk-secret-abc") {
		t.Errorf("passthrough body not redacted: %s", body)
	}
}

func TestNormalizeUpstreamErrorBodyRewriteGeneric(t *testing.T) {
	cfg := &cluster_conf.UpstreamErrorNormalizeConf{
		Enabled:            true,
		UnrecognizedAction: cluster_conf.NormalizeActionRewriteGeneric,
	}
	res := &bfe_http.Response{
		StatusCode: 500,
		Header:     bfe_http.Header{"Content-Type": []string{"text/html"}},
		Body:       ioutil.NopCloser(strings.NewReader("<html>oops</html>")),
	}
	aiMeta := &bfe_basic.AiBasicInfo{AuthStyle: "openai"}
	normalizeUpstreamError(res, testClusterWithNormalize(cfg), aiMeta, testNormalizeState())

	if !aiMeta.ErrNormalized || !aiMeta.ErrNormalizeMiss {
		t.Errorf("aiMeta = %+v, want generic rewrite with miss", aiMeta)
	}
	if res.StatusCode != 502 {
		t.Errorf("status = %d, want 502 (UPSTREAM_UNKNOWN)", res.StatusCode)
	}
	body, _ := ioutil.ReadAll(res.Body)
	if !strings.Contains(string(body), bfe_basic.CodeUpstreamUnknown) {
		t.Errorf("body missing UPSTREAM_UNKNOWN: %s", body)
	}
}

func TestNormalizeUpstreamErrorSkipsSuccess(t *testing.T) {
	cfg := &cluster_conf.UpstreamErrorNormalizeConf{Enabled: true}
	res := &bfe_http.Response{
		StatusCode: 200,
		Header:     bfe_http.Header{"Content-Type": []string{"application/json"}},
		Body:       ioutil.NopCloser(strings.NewReader(`{"choices":[]}`)),
	}
	aiMeta := &bfe_basic.AiBasicInfo{AuthStyle: "openai"}
	normalizeUpstreamError(res, testClusterWithNormalize(cfg), aiMeta, testNormalizeState())
	if aiMeta.ErrNormalized || aiMeta.ErrNormalizeMiss {
		t.Error("2xx response must not be normalized")
	}
}

func readAllFilter(f *aiErrorStreamFilter) string {
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := f.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}

func TestStreamFilterRewritesErrorEvent(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n" +
		"event: error\n" +
		"data: {\"error\":{\"message\":\"bad key sk-stream-key\",\"type\":\"authentication_error\"}}\n\n" +
		"data: [DONE]\n\n"
	aiMeta := &bfe_basic.AiBasicInfo{AuthStyle: "openai", UpstreamKey: "sk-stream-key"}
	cfg := (&cluster_conf.UpstreamErrorNormalizeConf{StreamEnabled: true}).Effective()
	f := &aiErrorStreamFilter{
		src:     ioutil.NopCloser(strings.NewReader(stream)),
		adapter: bfe_model_protocol.Get("openai"),
		cfg:     cfg,
		aiMeta:  aiMeta,
		state:   testNormalizeState(),
	}
	out := readAllFilter(f)

	if !strings.Contains(out, "\"content\":\"hel\"") {
		t.Errorf("content event not passed through: %s", out)
	}
	if !strings.Contains(out, bfe_basic.CodeUpstreamAuthError) {
		t.Errorf("error event not rewritten: %s", out)
	}
	if strings.Contains(out, "sk-stream-key") {
		t.Errorf("stream error event not redacted: %s", out)
	}
	if !strings.Contains(out, "event: error") {
		t.Errorf("SSE event name not preserved: %s", out)
	}
	if !aiMeta.StreamErrorRewritten || !aiMeta.ErrNormalized {
		t.Errorf("aiMeta = %+v, want rewritten", aiMeta)
	}
	if aiMeta.StreamTruncated {
		t.Error("[DONE] seen; must not be truncated")
	}
}

func TestStreamFilterTruncation(t *testing.T) {
	// no terminal event -> truncation marked
	stream := "data: {\"choices\":[]}\n\n"
	aiMeta := &bfe_basic.AiBasicInfo{AuthStyle: "openai"}
	cfg := (&cluster_conf.UpstreamErrorNormalizeConf{StreamEnabled: true}).Effective()
	f := &aiErrorStreamFilter{
		src:     ioutil.NopCloser(strings.NewReader(stream)),
		adapter: bfe_model_protocol.Get("openai"),
		cfg:     cfg,
		aiMeta:  aiMeta,
		state:   testNormalizeState(),
	}
	readAllFilter(f)
	if !aiMeta.StreamTruncated {
		t.Error("missing terminal event must mark truncation")
	}

	// Gemini-style stream: EOF is normal, never truncated
	aiMeta2 := &bfe_basic.AiBasicInfo{AuthStyle: "gemini"}
	f2 := &aiErrorStreamFilter{
		src:     ioutil.NopCloser(strings.NewReader("data: {\"usageMetadata\":{}}\n\n")),
		adapter: bfe_model_protocol.Get("gemini"),
		cfg:     cfg,
		aiMeta:  aiMeta2,
		state:   testNormalizeState(),
	}
	readAllFilter(f2)
	if aiMeta2.StreamTruncated {
		t.Error("Gemini stream must never be marked truncated")
	}
}

func TestStreamFilterCRLF(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{}}]}\r\n\r\n" +
		"data: [DONE]\r\n\r\n"
	aiMeta := &bfe_basic.AiBasicInfo{AuthStyle: "openai"}
	cfg := (&cluster_conf.UpstreamErrorNormalizeConf{StreamEnabled: true}).Effective()
	f := &aiErrorStreamFilter{
		src:     ioutil.NopCloser(strings.NewReader(stream)),
		adapter: bfe_model_protocol.Get("openai"),
		cfg:     cfg,
		aiMeta:  aiMeta,
		state:   testNormalizeState(),
	}
	out := readAllFilter(f)
	if !strings.Contains(out, "\r\n\r\n") {
		t.Errorf("CRLF line endings not preserved: %q", out)
	}
	if aiMeta.StreamTruncated {
		t.Error("CRLF stream with [DONE] must not be truncated")
	}
}

func TestEffectiveConfDefaults(t *testing.T) {
	eff := (&cluster_conf.UpstreamErrorNormalizeConf{}).Effective()
	if eff.UnrecognizedAction != cluster_conf.NormalizeActionPassthrough {
		t.Errorf("default action = %q", eff.UnrecognizedAction)
	}
	if eff.MaxBodyBytes != 64*1024 {
		t.Errorf("default MaxBodyBytes = %d", eff.MaxBodyBytes)
	}
	if !eff.RedactSecretsEnabled() {
		t.Error("redaction must default to on")
	}
	// explicit disable honored
	dis := false
	eff2 := (&cluster_conf.UpstreamErrorNormalizeConf{RedactSecrets: &dis}).Effective()
	if eff2.RedactSecretsEnabled() {
		t.Error("explicit RedactSecrets=false must disable")
	}
}

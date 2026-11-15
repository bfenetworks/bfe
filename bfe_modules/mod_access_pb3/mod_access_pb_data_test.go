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

package mod_access_pb3

import (
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_http"
)

func writeDataConf(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mod_access_pb.data")
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write data conf: %v", err)
	}
	return path
}

func TestModAccessPbDataLoadBasic(t *testing.T) {
	path := writeDataConf(t, `{
		"Version": "20261115090000",
		"access_header": {
			"req_headers": ["business_type", "X-REQUEST-ID"],
			"res_headers": ["x-trace-id"]
		}
	}`)

	conf, err := ModAccessPbDataLoad(path)
	if err != nil {
		t.Fatalf("ModAccessPbDataLoad() error: %v", err)
	}
	if conf.Version != "20261115090000" {
		t.Errorf("Version error, got: %s", conf.Version)
	}

	// names are canonicalized at load time: matching is case-insensitive
	// by construction and the proto key is stable for aggregation
	wantReq := []string{"Business_type", "X-Request-Id"}
	if len(conf.AccessHeader.ReqHeaders) != len(wantReq) {
		t.Fatalf("ReqHeaders length error, got: %v", conf.AccessHeader.ReqHeaders)
	}
	for i, name := range wantReq {
		if conf.AccessHeader.ReqHeaders[i] != name {
			t.Errorf("ReqHeaders[%d] should be %s, got: %s", i, name, conf.AccessHeader.ReqHeaders[i])
		}
	}
	if len(conf.AccessHeader.ResHeaders) != 1 || conf.AccessHeader.ResHeaders[0] != "X-Trace-Id" {
		t.Errorf("ResHeaders error, got: %v", conf.AccessHeader.ResHeaders)
	}
}

func TestModAccessPbDataLoadSectionMissing(t *testing.T) {
	// no access_header section at all: default behavior (collect nothing)
	path := writeDataConf(t, `{"Version": "20261115090000"}`)
	conf, err := ModAccessPbDataLoad(path)
	if err != nil {
		t.Fatalf("ModAccessPbDataLoad() error: %v", err)
	}
	if len(conf.AccessHeader.ReqHeaders) != 0 || len(conf.AccessHeader.ResHeaders) != 0 {
		t.Error("missing access_header section should yield empty whitelists")
	}

	// empty arrays: same
	path = writeDataConf(t, `{"Version": "v", "access_header": {"req_headers": [], "res_headers": []}}`)
	conf, err = ModAccessPbDataLoad(path)
	if err != nil {
		t.Fatalf("ModAccessPbDataLoad() error: %v", err)
	}
	if len(conf.AccessHeader.ReqHeaders) != 0 || len(conf.AccessHeader.ResHeaders) != 0 {
		t.Error("empty whitelists should stay empty")
	}
}

func TestModAccessPbDataLoadUnknownSectionIgnored(t *testing.T) {
	path := writeDataConf(t, `{
		"Version": "v",
		"access_header": {"req_headers": ["a"]},
		"sampling": {"rate": 10}
	}`)
	conf, err := ModAccessPbDataLoad(path)
	if err != nil {
		t.Fatalf("unknown section must not fail loading: %v", err)
	}
	if len(conf.AccessHeader.ReqHeaders) != 1 {
		t.Error("known section should load normally alongside unknown ones")
	}
}

func TestModAccessPbDataLoadFileNotExist(t *testing.T) {
	_, err := ModAccessPbDataLoad(filepath.Join(t.TempDir(), "not_exist.data"))
	if !os.IsNotExist(err) {
		t.Errorf("should return an IsNotExist error, got: %v", err)
	}
}

func TestModAccessPbDataLoadInvalidJson(t *testing.T) {
	path := writeDataConf(t, `{invalid json`)
	if _, err := ModAccessPbDataLoad(path); err == nil {
		t.Error("invalid json should fail loading")
	}
}

// canonKey centralizes the expected canonical form so the table below
// states intent, not a hand-copied transformation.
func canonKey(s string) string {
	return bfe_http.CanonicalHeaderKey(s)
}

func TestNormalizeHeaderNames(t *testing.T) {
	cases := []struct {
		name    string
		names   []string
		want    []string
		wantErr bool
	}{
		{name: "nil stays nil", names: nil, want: nil},
		{name: "empty list", names: []string{}, want: []string{}},
		{name: "canonicalize", names: []string{"business_type", "X-REQUEST-ID", "content-TYPE"},
			want: []string{"Business_type", "X-Request-Id", "Content-Type"}},
		{name: "trim spaces", names: []string{" x-trace-id "}, want: []string{"X-Trace-Id"}},
		{name: "all token chars are accepted", names: []string{"x-token!#$%&'*+-.^_`|~"},
			want: []string{canonKey("x-token!#$%&'*+-.^_`|~")}},
		{name: "empty name", names: []string{""}, wantErr: true},
		{name: "blank name", names: []string{"   "}, wantErr: true},
		{name: "too long", names: []string{strings.Repeat("a", maxWhitelistedHeaderKeyLen+1)}, wantErr: true},
		{name: "boundary length", names: []string{strings.Repeat("a", maxWhitelistedHeaderKeyLen)},
			want: []string{"A" + strings.Repeat("a", maxWhitelistedHeaderKeyLen-1)}},
		{name: "invalid char space", names: []string{"bad name"}, wantErr: true},
		{name: "invalid char colon", names: []string{"bad:name"}, wantErr: true},
		{name: "duplicate after canonicalization", names: []string{"x-id", "X-ID", "X-Id"}, wantErr: true},
		{name: "sensitive headers are not restricted", names: []string{"Authorization", "Cookie", "Proxy-Authorization"},
			want: []string{"Authorization", "Cookie", "Proxy-Authorization"}},
	}
	for _, c := range cases {
		got, err := normalizeHeaderNames(c.names)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: should return error, got: %v", c.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.name, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			}
		}
	}
}

func TestTruncateHeaderValue(t *testing.T) {
	if got := truncateHeaderValue("short"); got != "short" {
		t.Errorf("short value should be untouched, got: %s", got)
	}
	long := strings.Repeat("a", maxWhitelistedHeaderValueLen+100)
	if got := truncateHeaderValue(long); len(got) != maxWhitelistedHeaderValueLen {
		t.Errorf("long value should be truncated to %d, got: %d", maxWhitelistedHeaderValueLen, len(got))
	}
}

func TestCollectWhitelistedHeaders(t *testing.T) {
	header := bfe_http.Header{}
	header["Business-Type"] = []string{"rag", "chat"}
	header["X-Request-Id"] = []string{""}
	// X-Trace-Id absent on purpose

	got := collectWhitelistedHeaders(header, []string{"Business-Type", "X-Request-Id", "X-Trace-Id"})
	if len(got) != 2 {
		t.Fatalf("absent header should be skipped, got %d entries", len(got))
	}
	if got[0].GetKey() != "Business-Type" || got[0].GetValue() != "rag,chat" {
		t.Errorf("multi-value should be joined with ',': %+v", got[0])
	}
	if got[1].GetKey() != "X-Request-Id" || got[1].GetValue() != "" {
		t.Errorf("present-but-empty header should be kept with empty value: %+v", got[1])
	}

	// empty whitelist: no allocation beyond nil
	if got := collectWhitelistedHeaders(header, nil); len(got) != 0 {
		t.Error("nil whitelist should collect nothing")
	}

	// value truncation
	big := bfe_http.Header{}
	big["X-Big"] = []string{strings.Repeat("v", maxWhitelistedHeaderValueLen+10)}
	got = collectWhitelistedHeaders(big, []string{"X-Big"})
	if len(got[0].GetValue()) != maxWhitelistedHeaderValueLen {
		t.Error("value should be truncated to the downstream column width")
	}
}

func TestHolderSnapshotAndUpdate(t *testing.T) {
	h := NewModAccessPbConfHolder()
	if s := h.Snapshot(); s == nil || len(s.AccessHeader.ReqHeaders) != 0 {
		t.Error("fresh holder should snapshot an empty conf")
	}

	conf := &ModAccessPbConf{Version: "v1", AccessHeader: AccessHeaderConf{ReqHeaders: []string{"A"}}}
	h.Update(conf)
	if s := h.Snapshot(); s.Version != "v1" || len(s.AccessHeader.ReqHeaders) != 1 {
		t.Error("snapshot should reflect Update")
	}

	// failed-load semantics: caller keeps serving the old conf, so Update
	// is simply not called; snapshot must be unaffected
	if s := h.Snapshot(); s.Version != "v1" {
		t.Error("snapshot should keep the previous conf when reload fails")
	}
}

func newTestModuleWithDataConf(t *testing.T, dataConfPath string) *ModuleAccessPb3 {
	t.Helper()
	m := NewModuleAccessPb3()
	m.conf = &ConfModAccessPb2{}
	m.conf.BasicConf.DataConfPath = dataConfPath
	return m
}

func TestLoadModAccessPbConfTolerateMissing(t *testing.T) {
	m := newTestModuleWithDataConf(t, filepath.Join(t.TempDir(), "not_exist.data"))

	// Init path: missing file is tolerated, module runs with defaults
	if err := m.loadModAccessPbConf(nil, true); err != nil {
		t.Errorf("Init load should tolerate missing file, got: %v", err)
	}
	if s := m.dataHolder.Snapshot(); s.Version != "" || len(s.AccessHeader.ReqHeaders) != 0 {
		t.Error("missing file should reset to default conf")
	}

	// Reload path: missing file is an error, old conf keeps serving
	good := writeDataConf(t, `{"Version": "v1", "access_header": {"req_headers": ["a"]}}`)
	m.conf.BasicConf.DataConfPath = good
	if err := m.loadModAccessPbConf(nil, false); err != nil {
		t.Fatalf("reload with good file should succeed: %v", err)
	}
	m.conf.BasicConf.DataConfPath = filepath.Join(t.TempDir(), "gone.data")
	if err := m.loadModAccessPbConf(nil, false); err == nil {
		t.Error("reload with missing file should return error")
	}
	if s := m.dataHolder.Snapshot(); s.Version != "v1" {
		t.Error("failed reload must keep the old conf")
	}
}

func TestLoadModAccessPbConfInvalidKeepsOld(t *testing.T) {
	good := writeDataConf(t, `{"Version": "v1", "access_header": {"req_headers": ["a"]}}`)
	bad := writeDataConf(t, `{"Version": "v2", "access_header": {"req_headers": ["bad name"]}}`)

	m := newTestModuleWithDataConf(t, good)
	if err := m.loadModAccessPbConf(nil, false); err != nil {
		t.Fatalf("load good conf: %v", err)
	}

	m.conf.BasicConf.DataConfPath = bad
	if err := m.loadModAccessPbConf(nil, false); err == nil {
		t.Error("invalid conf should fail reload")
	}
	if s := m.dataHolder.Snapshot(); s.Version != "v1" {
		t.Error("invalid reload must keep the old conf")
	}
}

func TestLoadModAccessPbConfPathOverride(t *testing.T) {
	alt := writeDataConf(t, `{"Version": "alt", "access_header": {"req_headers": ["b"]}}`)
	m := newTestModuleWithDataConf(t, filepath.Join(t.TempDir(), "not_exist.data"))

	query := url.Values{}
	query.Set("path", alt)
	if err := m.loadModAccessPbConf(query, false); err != nil {
		t.Fatalf("path override load: %v", err)
	}
	if s := m.dataHolder.Snapshot(); s.Version != "alt" {
		t.Errorf("path override should load the query-specified file, got version: %s", s.Version)
	}
}

// TestRequestLogGenWhitelistedHeaders exercises the whole collection path
// inside requestLogGen, next to the standalone header fields and the
// credential masking gate.
func TestRequestLogGenWhitelistedHeaders(t *testing.T) {
	m, req, res := makeRequestLogTest(t)
	req.HttpRequest.Header.Set("Business-Type", "rag")
	req.HttpRequest.Header["X-Multi"] = []string{"v1", "v2"}
	res.Header.Set("X-Trace-Id", "trace-1")

	m.dataHolder.Update(&ModAccessPbConf{
		Version: "v1",
		AccessHeader: AccessHeaderConf{
			ReqHeaders: []string{"Business-Type", "X-Multi", "X-Absent"},
			ResHeaders: []string{"X-Trace-Id"},
		},
	})

	reqLog := m.requestLogGen(req, res).GetRequestLog()

	if len(reqLog.ReqHeaders) != 2 {
		t.Fatalf("ReqHeaders should have 2 entries, got: %d", len(reqLog.ReqHeaders))
	}
	if reqLog.ReqHeaders[0].GetKey() != "Business-Type" || reqLog.ReqHeaders[0].GetValue() != "rag" {
		t.Errorf("ReqHeaders[0] error: %+v", reqLog.ReqHeaders[0])
	}
	if reqLog.ReqHeaders[1].GetKey() != "X-Multi" || reqLog.ReqHeaders[1].GetValue() != "v1,v2" {
		t.Errorf("multi-value join error: %+v", reqLog.ReqHeaders[1])
	}
	if len(reqLog.ResHeaders) != 1 || reqLog.ResHeaders[0].GetValue() != "trace-1" {
		t.Errorf("ResHeaders error: %+v", reqLog.ResHeaders)
	}

	// default (empty) whitelist: fields stay unset, behavior unchanged
	m.dataHolder.Update(emptyModAccessPbConf())
	reqLog = m.requestLogGen(req, res).GetRequestLog()
	if len(reqLog.ReqHeaders) != 0 || len(reqLog.ResHeaders) != 0 {
		t.Error("empty whitelist should not collect any headers")
	}

	// res == nil: response whitelist is skipped entirely
	reqLog = m.requestLogGen(req, nil).GetRequestLog()
	if reqLog.ResHeaders != nil {
		t.Error("res_headers should stay unset when res is nil")
	}
}

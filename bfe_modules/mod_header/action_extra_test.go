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

package mod_header

import (
	"net"
	"strings"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_net/textproto"
)

func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func TestActionFileCheckNoCmd(t *testing.T) {
	af := ActionFile{Params: []string{"k", "v"}}
	if err := ActionFileCheck(af); err == nil || !strings.Contains(err.Error(), "no Cmd") {
		t.Errorf("expected no Cmd error, got %v", err)
	}
}

func TestActionFileCheckInvalidCmd(t *testing.T) {
	cmd := "INVALID_CMD"
	af := ActionFile{Cmd: &cmd, Params: []string{"k", "v"}}
	if err := ActionFileCheck(af); err == nil || !strings.Contains(err.Error(), "invalid cmd") {
		t.Errorf("expected invalid cmd error, got %v", err)
	}
}

func TestActionFileCheckParamCount(t *testing.T) {
	tests := []struct {
		cmd    string
		params []string
	}{
		{"REQ_HEADER_SET", []string{"k"}},
		{"REQ_HEADER_ADD", []string{"k", "v", "extra"}},
		{"REQ_HEADER_RENAME", []string{"k"}},
		{"RSP_HEADER_SET", []string{"k"}},
		{"RSP_HEADER_ADD", []string{"k", "v", "extra"}},
		{"RSP_HEADER_RENAME", []string{"k"}},
		{"REQ_HEADER_DEL", []string{}},
		{"RSP_HEADER_DEL", []string{"k", "extra"}},
		{"ReqCookieSet", []string{"k"}},
		{"ReqCookieDel", []string{}},
		{"RspCookieDel", []string{"k"}},
		{"RspCookieSet", []string{"k", "v"}},
	}

	for _, tc := range tests {
		af := ActionFile{Cmd: &tc.cmd, Params: tc.params}
		if err := ActionFileCheck(af); err == nil {
			t.Errorf("ActionFileCheck(%s) should fail", tc.cmd)
		}
	}
}

func TestActionFileCheckEmptyParam(t *testing.T) {
	cmd := "REQ_HEADER_SET"
	af := ActionFile{Cmd: &cmd, Params: []string{"", "value"}}
	if err := ActionFileCheck(af); err == nil || !strings.Contains(err.Error(), "empty Params") {
		t.Errorf("expected empty Params error, got %v", err)
	}
}

func TestActionFileCheckCookieExpires(t *testing.T) {
	cmd := "RSP_COOKIE_SET"
	params := []string{"SET", "v", "example.org", "/", "not-a-date", "100", "true", "true"}
	af := ActionFile{Cmd: &cmd, Params: params}
	if err := ActionFileCheck(af); err == nil || !strings.Contains(err.Error(), "expires") {
		t.Errorf("expected expires format error, got %v", err)
	}
}

func TestActionFileCheckCookieMaxAge(t *testing.T) {
	cmd := "RSP_COOKIE_SET"
	params := []string{"SET", "v", "example.org", "/", "Tue, 10 Nov 2009 23:00:00 UTC", "abc", "true", "true"}
	af := ActionFile{Cmd: &cmd, Params: params}
	if err := ActionFileCheck(af); err == nil || !strings.Contains(err.Error(), "max age") {
		t.Errorf("expected max age error, got %v", err)
	}
}

func TestActionFileCheckCookieHttpOnly(t *testing.T) {
	cmd := "RSP_COOKIE_SET"
	params := []string{"SET", "v", "example.org", "/", "Tue, 10 Nov 2009 23:00:00 UTC", "100", "abc", "true"}
	af := ActionFile{Cmd: &cmd, Params: params}
	if err := ActionFileCheck(af); err == nil || !strings.Contains(err.Error(), "http only") {
		t.Errorf("expected http only error, got %v", err)
	}
}

func TestActionFileCheckCookieSecure(t *testing.T) {
	cmd := "RSP_COOKIE_SET"
	params := []string{"SET", "v", "example.org", "/", "Tue, 10 Nov 2009 23:00:00 UTC", "100", "true", "abc"}
	af := ActionFile{Cmd: &cmd, Params: params}
	if err := ActionFileCheck(af); err == nil || !strings.Contains(err.Error(), "secure") {
		t.Errorf("expected secure error, got %v", err)
	}
}

func TestCheckHeaderModParams(t *testing.T) {
	// valid scheme_set
	if err := checkHeaderModParams([]string{"scheme_set", "referer", "https"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// valid query_add
	if err := checkHeaderModParams([]string{"query_add", "location", "k", "v"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// wrong param count
	if err := checkHeaderModParams([]string{"scheme_set", "referer"}); err == nil {
		t.Error("expected error for wrong param count")
	}

	// invalid cmd
	if err := checkHeaderModParams([]string{"unknown", "referer", "http"}); err == nil {
		t.Error("expected error for invalid cmd")
	}

	// scheme_set with wrong key
	if err := checkHeaderModParams([]string{"scheme_set", "x-foo", "https"}); err == nil {
		t.Error("expected error for wrong key")
	}

	// scheme_set with wrong scheme
	if err := checkHeaderModParams([]string{"scheme_set", "referer", "ftp"}); err == nil {
		t.Error("expected error for wrong scheme")
	}

	// query_add with wrong key
	if err := checkHeaderModParams([]string{"query_add", "x-foo", "k", "v"}); err == nil {
		t.Error("expected error for wrong key")
	}

	// query_add with wrong param count
	if err := checkHeaderModParams([]string{"query_add", "referer", "k"}); err == nil {
		t.Error("expected error for query_add wrong param count")
	}
}

func TestExpectVariableParam(t *testing.T) {
	if got := expectVariableParam("abc_123"); got != 7 {
		t.Errorf("expectVariableParam(abc_123) = %d, want 7", got)
	}
	if got := expectVariableParam("abc-123"); got != 3 {
		t.Errorf("expectVariableParam(abc-123) = %d, want 3", got)
	}
	if got := expectVariableParam(""); got != 0 {
		t.Errorf("expectVariableParam() = %d, want 0", got)
	}
}

func TestSplitParam(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{}},
		{"hello", []string{"hello"}},
		{"%bfe_vip", []string{"%bfe_vip"}},
		{"%%escaped", []string{"%%escaped"}},
		{"pre%bfe_vip", []string{"pre", "%bfe_vip"}},
		{"%bfe_vip;max", []string{"%bfe_vip", ";max"}},
	}

	for _, tc := range cases {
		got := splitParam(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("splitParam(%q) len = %d, want %d", tc.in, len(got), len(tc.want))
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitParam(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestPreProcessParams(t *testing.T) {
	// valid variable
	params, err := preProcessParams("%bfe_vip")
	if err != nil || len(params) != 1 || params[0] != "%bfe_vip" {
		t.Errorf("preProcessParams valid variable failed: %v, %v", params, err)
	}

	// escaped percent
	params, err = preProcessParams("%%bfe_vip")
	if err != nil || len(params) != 1 || params[0] != "%%bfe_vip" {
		t.Errorf("preProcessParams escaped percent failed: %v, %v", params, err)
	}

	// invalid variable
	params, err = preProcessParams("%unknown_var")
	if err == nil {
		t.Error("preProcessParams invalid variable should fail")
	}

	// plain text
	params, err = preProcessParams("plain text")
	if err != nil || len(params) != 1 || params[0] != "plain text" {
		t.Errorf("preProcessParams plain text failed: %v, %v", params, err)
	}
}

func TestActionConvertRspCommands(t *testing.T) {
	tests := []struct {
		cmd    string
		params []string
	}{
		{"RSP_HEADER_SET", []string{"x-foo", "value"}},
		{"RSP_HEADER_ADD", []string{"x-foo", "value"}},
		{"RSP_HEADER_DEL", []string{"x-foo"}},
		{"RSP_HEADER_RENAME", []string{"x-foo", "x-bar"}},
		{"RSP_HEADER_MOD", []string{"scheme_set", "location", "https"}},
		{"RSP_HEADER_MOD", []string{"query_add", "location", "k", "v"}},
		{"REQ_HEADER_RENAME", []string{"x-foo", "x-bar"}},
		{"REQ_HEADER_MOD", []string{"query_add", "referer", "k", "v"}},
	}

	for _, tc := range tests {
		af := ActionFile{Cmd: &tc.cmd, Params: tc.params}
		action, err := actionConvert(af)
		if err != nil {
			t.Errorf("actionConvert(%s) error: %v", tc.cmd, err)
			continue
		}
		if action.Cmd != tc.cmd {
			t.Errorf("actionConvert(%s) cmd = %s, want %s", tc.cmd, action.Cmd, tc.cmd)
		}
	}
}

func TestActionConvertCookieCommands(t *testing.T) {
	cmd := "RSP_COOKIE_SET"
	params := []string{"SET", "2", "example.org", "/unittest", "Tue, 10 Nov 2009 23:00:00 UTC", "100", "true", "true"}
	af := ActionFile{Cmd: &cmd, Params: params}
	action, err := actionConvert(af)
	if err != nil {
		t.Fatalf("actionConvert(RSP_COOKIE_SET) error: %v", err)
	}
	if len(action.Params) != 8 {
		t.Errorf("actionConvert(RSP_COOKIE_SET) params len = %d, want 8", len(action.Params))
	}

	delCmd := "RSP_COOKIE_DEL"
	delParams := []string{"DEL", "example.org", "/unittest"}
	af = ActionFile{Cmd: &delCmd, Params: delParams}
	action, err = actionConvert(af)
	if err != nil {
		t.Fatalf("actionConvert(RSP_COOKIE_DEL) error: %v", err)
	}
	if action.Params[0] != "DEL" {
		t.Errorf("actionConvert(RSP_COOKIE_DEL) first param = %s, want DEL", action.Params[0])
	}
}

func TestActionConvertInvalid(t *testing.T) {
	cmd := "UNKNOWN"
	af := ActionFile{Cmd: &cmd, Params: []string{"k", "v"}}
	if _, err := actionConvert(af); err == nil {
		t.Error("actionConvert invalid cmd should fail")
	}
}

func TestSetSchemeEdgeCases(t *testing.T) {
	// no scheme prefix
	if got := setScheme("example.org", "https"); got != "example.org" {
		t.Errorf("setScheme(no prefix) = %s, want example.org", got)
	}

	// no colon
	if got := setScheme("httpsexample.org", "http"); got != "httpsexample.org" {
		t.Errorf("setScheme(no colon) = %s, want httpsexample.org", got)
	}
}

func TestAddQueryParseError(t *testing.T) {
	if got := addQuery("://invalid", "k", "v"); got != "://invalid" {
		t.Errorf("addQuery parse error = %s, want ://invalid", got)
	}
}

func TestGetHeaderValue(t *testing.T) {
	req := makeBasicRequest("http://www.example.org")
	req.ClientAddr = nil

	action := Action{Cmd: "REQ_HEADER_SET", Params: []string{"X-Client-Ip", "%bfe_client_ip"}}
	if got := getHeaderValue(req, action); got != "" {
		t.Errorf("getHeaderValue(nil ClientAddr) = %s, want empty", got)
	}

	action = Action{Cmd: "REQ_HEADER_SET", Params: []string{"X-Unknown", "%unknown"}}
	if got := getHeaderValue(req, action); got != "unknown" {
		t.Errorf("getHeaderValue(unknown var) = %s, want unknown", got)
	}

	action = Action{Cmd: "REQ_HEADER_SET", Params: []string{"X-Key"}}
	if got := getHeaderValue(req, action); got != "" {
		t.Errorf("getHeaderValue(no value) = %s, want empty", got)
	}
}

func TestProcessHeaderModNoHeader(t *testing.T) {
	req := makeBasicRequest("http://www.example.org")
	action := Action{Cmd: "REQ_HEADER_MOD", Params: []string{"SCHEME_SET", "Referer", "https"}}
	processHeader(req, ReqHeader, action)
	if req.HttpRequest.Header.Get("Referer") != "" {
		t.Error("processHeader should not add missing header")
	}
}

func TestProcessHeaderRenameCases(t *testing.T) {
	req := makeBasicRequest("http://www.example.org")
	req.HttpRequest.Header.Set("Old-Key", "value")

	// successful rename
	action := Action{Cmd: "REQ_HEADER_RENAME", Params: []string{"Old-Key", "New-Key"}}
	processHeader(req, ReqHeader, action)
	if req.HttpRequest.Header.Get("New-Key") != "value" {
		t.Error("rename failed")
	}

	// rename when original key missing
	req2 := makeBasicRequest("http://www.example.org")
	processHeader(req2, ReqHeader, action)
	if req2.HttpRequest.Header.Get("New-Key") != "" {
		t.Error("rename should not add missing header")
	}

	// rename when target key exists
	req3 := makeBasicRequest("http://www.example.org")
	req3.HttpRequest.Header.Set("Old-Key", "value")
	req3.HttpRequest.Header.Set("New-Key", "exists")
	processHeader(req3, ReqHeader, action)
	if req3.HttpRequest.Header.Get("Old-Key") == "" {
		t.Error("rename should not overwrite existing target")
	}
}

func TestHeaderActionDo(t *testing.T) {
	h := make(bfe_http.Header)

	HeaderActionDo(&h, "HEADER_SET", "X-Set", "1")
	if h.Get("X-Set") != "1" {
		t.Error("HEADER_SET failed")
	}

	HeaderActionDo(&h, "HEADER_ADD", "X-Add", "1")
	HeaderActionDo(&h, "HEADER_ADD", "X-Add", "2")
	if len(h["X-Add"]) != 2 {
		t.Errorf("HEADER_ADD failed, got %v", h["X-Add"])
	}

	HeaderActionDo(&h, "HEADER_DEL", "X-Set", "")
	if h.Get("X-Set") != "" {
		t.Error("HEADER_DEL failed")
	}

	h.Set("X-Old", "value")
	HeaderActionDo(&h, "HEADER_RENAME", "X-Old", "X-New")
	if h.Get("X-New") != "value" || h.Get("X-Old") != "" {
		t.Error("HEADER_RENAME failed")
	}

	// unknown cmd should be ignored
	HeaderActionDo(&h, "UNKNOWN", "X-Unknown", "v")
}

func TestActionFileListCheckErrorIndex(t *testing.T) {
	cmd := "REQ_HEADER_SET"
	list := ActionFileList{
		{Cmd: &cmd, Params: []string{"k", "v"}},
		{Cmd: strPtr("INVALID"), Params: []string{"k", "v"}},
	}
	err := ActionFileListCheck(&list)
	if err == nil || !strings.Contains(err.Error(), "ActionFileList:1") {
		t.Errorf("expected ActionFileList index error, got %v", err)
	}
}

func TestActionsConvertError(t *testing.T) {
	cmd := "REQ_HEADER_SET"
	list := ActionFileList{
		{Cmd: &cmd, Params: []string{"k", "v"}},
		{Cmd: strPtr("INVALID"), Params: []string{"k", "v"}},
	}
	actions, err := actionsConvert(list)
	if err == nil {
		t.Error("actionsConvert should fail")
	}
	if len(actions) != 1 {
		t.Errorf("actionsConvert returned %d actions, want 1", len(actions))
	}
}

func TestPreProcessParamsCanonicalKey(t *testing.T) {
	// actionConvert for REQ_HEADER_SET should canonicalize header key
	cmd := "REQ_HEADER_SET"
	af := ActionFile{Cmd: &cmd, Params: []string{"x-lower-case", "value"}}
	action, err := actionConvert(af)
	if err != nil {
		t.Fatalf("actionConvert error: %v", err)
	}
	if action.Params[0] != textproto.CanonicalMIMEHeaderKey("x-lower-case") {
		t.Errorf("key not canonicalized: %s", action.Params[0])
	}
}

func TestGetCookieValuePlain(t *testing.T) {
	req := makeBasicRequest("http://www.example.org")
	if got := getCookieValue(req, "plain"); got != "plain" {
		t.Errorf("getCookieValue(plain) = %s, want plain", got)
	}
}

func TestGetCookieValueVariable(t *testing.T) {
	req := makeBasicRequest("http://www.example.org")
	req.ClientAddr = &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 12345}
	if got := getCookieValue(req, "%bfe_client_ip"); got != "10.0.0.1" {
		t.Errorf("getCookieValue(variable) = %s, want 10.0.0.1", got)
	}
}

func TestReqAddCookieNilCookieMap(t *testing.T) {
	req := makeBasicRequest("http://www.example.org")
	req.CookieMap = nil
	cookie := &bfe_http.Cookie{Name: "ADD", Value: "1"}
	reqAddCookie(req, cookie)
	if c, err := req.HttpRequest.Cookie("ADD"); err != nil || c.Value != "1" {
		t.Error("reqAddCookie with nil CookieMap failed")
	}
}

func TestActionFileCheckRspCookieDelValid(t *testing.T) {
	cmd := "RSP_COOKIE_DEL"
	af := ActionFile{Cmd: &cmd, Params: []string{"DEL", "example.org", "/"}}
	if err := ActionFileCheck(af); err != nil {
		t.Errorf("RSP_COOKIE_DEL valid failed: %v", err)
	}
}

func TestActionFileCheckCookieSetValid(t *testing.T) {
	cmd := "RSP_COOKIE_SET"
	params := []string{"SET", "v", "example.org", "/", "Tue, 10 Nov 2009 23:00:00 UTC", "100", "true", "true"}
	af := ActionFile{Cmd: &cmd, Params: params}
	if err := ActionFileCheck(af); err != nil {
		t.Errorf("RSP_COOKIE_SET valid failed: %v", err)
	}

	cmd = "REQ_COOKIE_SET"
	af = ActionFile{Cmd: &cmd, Params: []string{"SET", "1"}}
	if err := ActionFileCheck(af); err != nil {
		t.Errorf("REQ_COOKIE_SET valid failed: %v", err)
	}

	cmd = "REQ_COOKIE_DEL"
	af = ActionFile{Cmd: &cmd, Params: []string{"DEL"}}
	if err := ActionFileCheck(af); err != nil {
		t.Errorf("REQ_COOKIE_DEL valid failed: %v", err)
	}
}

func TestReqDelCookieNotExist(t *testing.T) {
	req := makeBasicRequest("http://www.example.org")
	req.CookieMap = make(bfe_http.CookieMap)
	cookie := &bfe_http.Cookie{Name: "NOTEXIST"}
	reqDelCookie(req, cookie)
	if _, err := req.HttpRequest.Cookie("NOTEXIST"); err == nil {
		t.Error("reqDelCookie should not add missing cookie")
	}
}

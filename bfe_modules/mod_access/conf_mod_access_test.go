// Copyright (c) 2019 The BFE Authors.
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

package mod_access

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestConfLoad(t *testing.T) {
	config, err := ConfLoad("./testdata/mod_access/mod_access.conf", "")
	if err != nil {
		t.Errorf("BfeConfigLoad error: %v", err)
		return
	}

	if config.Log.LogPrefix != "access" {
		t.Errorf("Log.Prefix should be access")
	}
}

func TestConfLoadNotExist(t *testing.T) {
	_, err := ConfLoad("./testdata/mod_access/not_exist.conf", "")
	if err == nil {
		t.Error("ConfLoad() should return error for non-existent file")
	}
}

func TestConfLoadInvalidLog(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_access_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	confPath := filepath.Join(dir, "mod_access.conf")
	content := `[Log]
LogFile = "/dev/null"
LogPrefix = "access"

[Template]
RequestTemplate = "$time"
SessionTemplate = "$ses_clientip"
`
	if err := ioutil.WriteFile(confPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	_, err = ConfLoad(confPath, "")
	if err == nil {
		t.Error("ConfLoad() should return error when LogFile and LogPrefix both set")
	}
}

func TestConfCheckEmptyRequestTemplate(t *testing.T) {
	cfg := &ConfModAccess{}
	cfg.Template.SessionTemplate = "$ses_clientip"

	err := cfg.Check("")
	if err == nil {
		t.Error("Check() should return error when RequestTemplate is empty")
	}
}

func TestConfCheckEmptySessionTemplate(t *testing.T) {
	cfg := &ConfModAccess{}
	cfg.Template.RequestTemplate = "$time"

	err := cfg.Check("")
	if err == nil {
		t.Error("Check() should return error when SessionTemplate is empty")
	}
}

func TestConfConvertCommon(t *testing.T) {
	cfg := &ConfModAccess{}
	cfg.Template.RequestTemplate = "COMMON"
	cfg.Convert()

	want := "$host - - $request_time \"$request_line\" $status_code $res_len"
	if cfg.Template.RequestTemplate != want {
		t.Errorf("Convert COMMON error, got: %s, want: %s", cfg.Template.RequestTemplate, want)
	}
}

func TestConfConvertCombined(t *testing.T) {
	cfg := &ConfModAccess{}
	cfg.Template.RequestTemplate = "COMBINED"
	cfg.Convert()

	want := "$host - - $request_time \"$request_line\" $status_code $res_len \"${Referer}req_header\" \"${User-Agent}req_header\""
	if cfg.Template.RequestTemplate != want {
		t.Errorf("Convert COMBINED error, got: %s, want: %s", cfg.Template.RequestTemplate, want)
	}
}

func TestCheckLogFmtInvalidLogFmtType(t *testing.T) {
	item := LogFmtItem{Key: "status_code", Type: FormatStatusCode}
	err := checkLogFmt(item, "Invalid")
	if err == nil {
		t.Error("checkLogFmt() should return error for invalid logFmtType")
	}
}

func TestCheckLogFmtTypeNotFound(t *testing.T) {
	item := LogFmtItem{Key: "unknown", Type: 99999}
	err := checkLogFmt(item, Request)
	if err == nil {
		t.Error("checkLogFmt() should return error for type not in domain table")
	}
}

func TestCheckLogFmtDomainMismatch(t *testing.T) {
	item := LogFmtItem{Key: "ses_clientip", Type: FormatSesClientIP}
	err := checkLogFmt(item, Request)
	if err == nil {
		t.Error("checkLogFmt() should return error when session item used in request log")
	}
}

func TestTokenTypeGet(t *testing.T) {
	template := "123$status_code$res_header"

	logType, end, err := tokenTypeGet(&template, 4)
	if err != nil {
		t.Errorf("tokenTypeGet() error: %v", err)
	}
	if logType != fmtTable["status_code"] {
		t.Errorf("logType error, logType: %d", logType)
	}
	if end != 14 {
		t.Errorf("end error, end: %d", end)
	}

	logType, end, err = tokenTypeGet(&template, 16)
	if err != nil {
		t.Errorf("tokenTypeGet() error: %v", err)
	}
	if logType != fmtTable["res_header"] {
		t.Errorf("logType error, logType: %d", logType)
	}
	if end != 25 {
		t.Errorf("end error, end: %d", end)
	}
}

func TestTokenTypeGetNotFound(t *testing.T) {
	template := "$unknown"
	_, _, err := tokenTypeGet(&template, 1)
	if err == nil {
		t.Error("tokenTypeGet() should return error for unknown token")
	}
}

func TestParseBracketToken(t *testing.T) {
	template := "{CLIENTIP}res_cookie, log"

	item, end, err := parseBracketToken(&template, 0)
	if err != nil {
		t.Errorf("parseBracketToken() error: %v", err)
	}

	if end != 19 {
		t.Errorf("end error, end: %d", end)
	}

	if item.Key != "CLIENTIP" || item.Type != fmtTable["res_cookie"] {
		t.Errorf("item error, item: %v", item)
	}
}

func TestParseBracketTokenNoClose(t *testing.T) {
	template := "{CLIENTIPres_cookie"
	_, _, err := parseBracketToken(&template, 0)
	if err == nil {
		t.Error("parseBracketToken() should return error when } is missing")
	}
}

func TestParseBracketTokenAtEnd(t *testing.T) {
	template := "{CLIENTIP}"
	_, _, err := parseBracketToken(&template, 0)
	if err == nil {
		t.Error("parseBracketToken() should return error when } is at end")
	}
}

func TestParseBracketTokenInvalidItem(t *testing.T) {
	template := "{CLIENTIP}unknown"
	_, _, err := parseBracketToken(&template, 0)
	if err == nil {
		t.Error("parseBracketToken() should return error for invalid item type")
	}
}

func TestParseLogTemplate(t *testing.T) {
	template := "REQ $time $status_code ${User-Agent}req_header end"
	items, err := parseLogTemplate(template)
	if err != nil {
		t.Errorf("parseLogTemplate() error: %v", err)
		return
	}

	if len(items) != 7 {
		t.Errorf("items length error, got: %d, want: 7", len(items))
	}

	if items[0].Type != FormatString || items[0].Key != "REQ " {
		t.Errorf("first item error: %v", items[0])
	}
	if items[1].Type != FormatTime {
		t.Errorf("second item error: %v", items[1])
	}
	if items[2].Type != FormatString || items[2].Key != " " {
		t.Errorf("third item error: %v", items[2])
	}
	if items[3].Type != FormatStatusCode {
		t.Errorf("fourth item error: %v", items[3])
	}
	if items[4].Type != FormatString || items[4].Key != " " {
		t.Errorf("fifth item error: %v", items[4])
	}
	if items[5].Key != "User-Agent" || items[5].Type != FormatReqHeader {
		t.Errorf("sixth item error: %v", items[5])
	}
	if items[6].Type != FormatString || items[6].Key != " end" {
		t.Errorf("seventh item error: %v", items[6])
	}
}

func TestParseLogTemplateDollarAtEnd(t *testing.T) {
	_, err := parseLogTemplate("$")
	if err == nil {
		t.Error("parseLogTemplate() should return error when $ is at end")
	}
}

func TestParseLogTemplateNoCloseBracket(t *testing.T) {
	_, err := parseLogTemplate("${User-Agent")
	if err == nil {
		t.Error("parseLogTemplate() should return error when { is not closed")
	}
}

func TestParseLogTemplateBracketAtEnd(t *testing.T) {
	_, err := parseLogTemplate("${User-Agent}")
	if err == nil {
		t.Error("parseLogTemplate() should return error when } is at end")
	}
}

func TestParseLogTemplateUnknownToken(t *testing.T) {
	_, err := parseLogTemplate("$unknown")
	if err == nil {
		t.Error("parseLogTemplate() should return error for unknown token")
	}
}

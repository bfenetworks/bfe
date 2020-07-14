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

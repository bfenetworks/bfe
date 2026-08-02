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

package mod_markdown

import (
	"errors"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, r.err
}

type emptyReader struct{}

func (r *emptyReader) Read(p []byte) (int, error) {
	return 0, io.EOF
}

func TestModuleMarkdown_Name(t *testing.T) {
	m := NewModuleMarkdown()
	if got := m.Name(); got != ModMarkdown {
		t.Errorf("ModuleMarkdown.Name() = %v, want %v", got, ModMarkdown)
	}
}

func TestModuleMarkdown_loadProductRuleConf(t *testing.T) {
	m := NewModuleMarkdown()
	m.conf = &ConfModMarkdown{}
	m.conf.Basic.ProductRulePath = "./testdata/mod_markdown.data"

	// load with explicit path in query values
	query := url.Values{}
	query.Set("path", "./testdata/mod_markdown.data")
	if err := m.loadProductRuleConf(query); err != nil {
		t.Errorf("ModuleMarkdown.loadProductRuleConf() error = %v", err)
	}

	// load with empty query values, fallback to configured path
	if err := m.loadProductRuleConf(nil); err != nil {
		t.Errorf("ModuleMarkdown.loadProductRuleConf() error = %v", err)
	}

	// load with non-existent path
	query = url.Values{}
	query.Set("path", "./testdata/not_exist.data")
	if err := m.loadProductRuleConf(query); err == nil {
		t.Errorf("ModuleMarkdown.loadProductRuleConf() expected error, got nil")
	}
}

func TestModuleMarkdown_renderMarkDownHandler_noRuleMatch(t *testing.T) {
	m := prepareModule()

	// product exists but no rule matches the request path
	req := prepareRequest("unittest", "/nomatch")
	res := prepareResponse("./testdata/testcase0.md")

	code := m.renderMarkDownHandler(req, res)
	if code != bfe_module.BfeHandlerGoOn {
		t.Errorf("renderMarkDownHandler() code = %v, want %v", code, bfe_module.BfeHandlerGoOn)
	}
}

func TestModuleMarkdown_renderMarkDownHandler_checkResponseFail(t *testing.T) {
	m := prepareModule()

	req := prepareRequest("unittest", "/default")
	res := prepareResponse("./testdata/testcase0.md")
	// wrong content type causes checkResponse to fail
	res.Header.Set("Content-Type", "text/html")

	code := m.renderMarkDownHandler(req, res)
	if code != bfe_module.BfeHandlerGoOn {
		t.Errorf("renderMarkDownHandler() code = %v, want %v", code, bfe_module.BfeHandlerGoOn)
	}
}

func TestModuleMarkdown_renderMarkDownHandler_renderError(t *testing.T) {
	m := prepareModule()

	req := prepareRequest("unittest", "/default")
	res := prepareResponse("./testdata/testcase0.md")

	// force renderMarkDown to fail by passing a non-empty ContentLength with an empty body
	res.ContentLength = 1
	res.Header.Set("Content-Length", "1")
	res.Body = ioutil.NopCloser(&emptyReader{})

	code := m.renderMarkDownHandler(req, res)
	if code != bfe_module.BfeHandlerGoOn {
		t.Errorf("renderMarkDownHandler() code = %v, want %v", code, bfe_module.BfeHandlerGoOn)
	}
	if res.StatusCode != bfe_http.StatusInternalServerError {
		t.Errorf("renderMarkDownHandler() status = %v, want %v", res.StatusCode, bfe_http.StatusInternalServerError)
	}
}

func TestModuleMarkdown_renderMarkDownHandler_multipleRules(t *testing.T) {
	m := prepareModule()

	// first rule does not match, second rule matches
	req := prepareRequest("unittest", "/github")
	res := prepareResponse("./testdata/testcase0.md")

	code := m.renderMarkDownHandler(req, res)
	if code != bfe_module.BfeHandlerGoOn {
		t.Errorf("renderMarkDownHandler() code = %v, want %v", code, bfe_module.BfeHandlerGoOn)
	}
	if got := res.Header.Get("Content-Type"); got != ConvertContentType {
		t.Errorf("renderMarkDownHandler() content-type = %v, want %v", got, ConvertContentType)
	}
}

func TestModuleMarkdown_renderMarkDown_readError(t *testing.T) {
	m := prepareModule()

	res := &bfe_http.Response{
		StatusCode: 200,
		Header:     make(bfe_http.Header),
		Body:       ioutil.NopCloser(&errorReader{err: errors.New("read error")}),
	}
	res.Header.Set("Content-Type", MarkdownContentType)
	res.ContentLength = 100

	err := m.renderMarkDown(res)
	if err == nil {
		t.Errorf("renderMarkDown() expected error, got nil")
	}
}

func TestModuleMarkdown_renderMarkDown_renderError(t *testing.T) {
	m := prepareModule()

	res := &bfe_http.Response{
		StatusCode: 200,
		Header:     make(bfe_http.Header),
		Body:       ioutil.NopCloser(&emptyReader{}),
	}
	res.Header.Set("Content-Type", MarkdownContentType)
	res.ContentLength = 1
	res.Header.Set("Content-Length", "1")

	err := m.renderMarkDown(res)
	if err == nil {
		t.Errorf("renderMarkDown() expected error, got nil")
	}
}

func TestModuleMarkdown_getState(t *testing.T) {
	m := prepareModule()

	// trigger metrics changes to get non-zero state
	req := prepareRequest("unittest", "/default")
	res := prepareResponse("./testdata/testcase0.md")
	m.renderMarkDownHandler(req, res)

	state, err := m.getState(nil)
	if err != nil {
		t.Errorf("getState() error = %v", err)
	}
	if len(state) == 0 {
		t.Errorf("getState() returned empty state")
	}
}

func TestModuleMarkdown_getStateDiff(t *testing.T) {
	m := prepareModule()

	state, err := m.getStateDiff(nil)
	if err != nil {
		t.Errorf("getStateDiff() error = %v", err)
	}
	if len(state) == 0 {
		t.Errorf("getStateDiff() returned empty state")
	}
}

func TestModuleMarkdown_monitorHandlers(t *testing.T) {
	m := NewModuleMarkdown()
	handlers := m.monitorHandlers()
	if _, ok := handlers[ModMarkdown]; !ok {
		t.Errorf("monitorHandlers() missing %s handler", ModMarkdown)
	}
	if _, ok := handlers[ModMarkdown+".diff"]; !ok {
		t.Errorf("monitorHandlers() missing %s.diff handler", ModMarkdown)
	}
}

func TestModuleMarkdown_reloadHandlers(t *testing.T) {
	m := NewModuleMarkdown()
	handlers := m.reloadHandlers()
	if _, ok := handlers[ModMarkdown]; !ok {
		t.Errorf("reloadHandlers() missing %s handler", ModMarkdown)
	}
}

func TestProductRuleConfLoad_FileNotFound(t *testing.T) {
	_, err := ProductRuleConfLoad("./testdata/not_exist.data")
	if err == nil {
		t.Errorf("ProductRuleConfLoad() expected error for missing file")
	}
}

func TestProductRuleConfLoad_InvalidJSON(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "mod_markdown_invalid_*.data")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString("not valid json"); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	_, err = ProductRuleConfLoad(tmpFile.Name())
	if err == nil {
		t.Errorf("ProductRuleConfLoad() expected error for invalid JSON")
	}
}

func Test_ruleConvert_invalidCond(t *testing.T) {
	_, err := ruleConvert(&markdownRuleFile{Cond: "???"})
	if err == nil {
		t.Errorf("ruleConvert() expected error for invalid condition")
	}
}

func Test_rulesConvert_invalidRule(t *testing.T) {
	ruleFiles := &markdownRuleFiles{
		&markdownRuleFile{Cond: "req_host_in(\"www.example.com\")"},
		&markdownRuleFile{Cond: "???"},
	}
	_, err := rulesConvert(ruleFiles)
	if err == nil {
		t.Errorf("rulesConvert() expected error when a rule fails to convert")
	}
}

func TestModuleMarkdown_checkResponse(t *testing.T) {
	m := NewModuleMarkdown()

	cases := []struct {
		name    string
		res     *bfe_http.Response
		wantErr bool
	}{
		{
			name: "valid response",
			res: &bfe_http.Response{
				Header:           make(bfe_http.Header),
				ContentLength:    100,
				Body:             ioutil.NopCloser(strings.NewReader("body")),
				TransferEncoding: []string{},
			},
			wantErr: false,
		},
		{
			name: "zero content length",
			res: &bfe_http.Response{
				Header:        make(bfe_http.Header),
				ContentLength: 0,
				Body:          ioutil.NopCloser(strings.NewReader("")),
			},
			wantErr: true,
		},
		{
			name: "oversized content length",
			res: &bfe_http.Response{
				Header:        make(bfe_http.Header),
				ContentLength: MaxBodyBytes + 1,
				Body:          ioutil.NopCloser(strings.NewReader("body")),
			},
			wantErr: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			tt.res.Header.Set("Content-Type", MarkdownContentType)
			err := m.checkResponse(tt.res)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestModuleMarkdown_renderMarkDownHandler_productNotFound(t *testing.T) {
	m := prepareModule()

	req := prepareRequest("unknown_product", "/default")
	res := prepareResponse("./testdata/testcase0.md")

	code := m.renderMarkDownHandler(req, res)
	if code != bfe_module.BfeHandlerGoOn {
		t.Errorf("renderMarkDownHandler() code = %v, want %v", code, bfe_module.BfeHandlerGoOn)
	}
	if got := res.Header.Get("Content-Type"); got != MarkdownContentType {
		t.Errorf("renderMarkDownHandler() content-type unchanged = %v", got)
	}
}

func TestModuleMarkdown_Init_DefaultRulePath(t *testing.T) {
	// create a temp config root with only the module config and no explicit rule path
	tmpRoot, err := ioutil.TempDir("", "mod_markdown_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpRoot)

	confDir := tmpRoot + "/mod_markdown"
	if err := os.MkdirAll(confDir, 0755); err != nil {
		t.Fatalf("failed to create conf dir: %v", err)
	}

	confContent := `[basic]
ProductRulePath = markdown_rule.data

[log]
OpenDebug = false
`
	if err := ioutil.WriteFile(confDir+"/mod_markdown.conf", []byte(confContent), 0644); err != nil {
		t.Fatalf("failed to write conf: %v", err)
	}

	ruleContent := `{
	"Version": "1",
	"Config": {}
}
`
	if err := ioutil.WriteFile(tmpRoot+"/markdown_rule.data", []byte(ruleContent), 0644); err != nil {
		t.Fatalf("failed to write rule: %v", err)
	}

	m := NewModuleMarkdown()
	if err := m.Init(bfe_module.NewBfeCallbacks(), web_monitor.NewWebHandlers(), tmpRoot); err != nil {
		t.Errorf("ModuleMarkdown.Init() error = %v", err)
	}
}

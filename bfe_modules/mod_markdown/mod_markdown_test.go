// Copyright 2021 The BFE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package mod_markdown

import (
	"bytes"
	"io/ioutil"
	"net/url"
	"reflect"
	"strconv"
	"testing"

	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func TestModuleMarkdown_Init(t *testing.T) {
	type args struct {
		cbs     *bfe_module.BfeCallbacks
		whs     *web_monitor.WebHandlers
		cr      string
		wantErr bool
	}

	// normal test case
	m := NewModuleMarkdown()
	case0 := args{
		cbs:     bfe_module.NewBfeCallbacks(),
		whs:     web_monitor.NewWebHandlers(),
		cr:      "./testdata",
		wantErr: false,
	}
	if err := m.Init(case0.cbs, case0.whs, case0.cr); (err != nil) != case0.wantErr {
		t.Errorf("ModuleMarkdown.Init() error = %v, wantErr %v", err, case0.wantErr)
	}

	// not exist path test case
	m = NewModuleMarkdown()
	case0 = args{
		cbs:     bfe_module.NewBfeCallbacks(),
		whs:     web_monitor.NewWebHandlers(),
		cr:      "./notexist",
		wantErr: true,
	}
	if err := m.Init(case0.cbs, case0.whs, case0.cr); (err != nil) != case0.wantErr {
		t.Errorf("ModuleMarkdown.Init() error = %v, wantErr %v", err, case0.wantErr)
	}

	// normal case
	m = NewModuleMarkdown()
	case0 = args{
		cbs:     bfe_module.NewBfeCallbacks(),
		whs:     nil,
		cr:      "./testdata",
		wantErr: true,
	}
	if err := m.Init(case0.cbs, case0.whs, case0.cr); (err != nil) != case0.wantErr {
		t.Errorf("ModuleMarkdown.Init() error = %v, wantErr %v", err, case0.wantErr)
	}

	// no register pointer case
	m = NewModuleMarkdown()
	case0 = args{
		cbs:     &bfe_module.BfeCallbacks{},
		whs:     web_monitor.NewWebHandlers(),
		cr:      "./testdata",
		wantErr: true,
	}
	if err := m.Init(case0.cbs, case0.whs, case0.cr); (err != nil) != case0.wantErr {
		t.Errorf("ModuleMarkdown.Init() error = %v, wantErr %v", err, case0.wantErr)
	}

	// no data case
	m = NewModuleMarkdown()
	case0 = args{
		cbs:     &bfe_module.BfeCallbacks{},
		whs:     web_monitor.NewWebHandlers(),
		cr:      "./testdata/case0",
		wantErr: true,
	}
	if err := m.Init(case0.cbs, case0.whs, case0.cr); (err != nil) != case0.wantErr {
		t.Errorf("ModuleMarkdown.Init() error = %v, wantErr %v", err, case0.wantErr)
	}

	// no data case
	m = NewModuleMarkdown()
	case0 = args{
		cbs:     &bfe_module.BfeCallbacks{},
		whs:     web_monitor.NewWebHandlers(),
		cr:      "./testdata/case1",
		wantErr: true,
	}
	if err := m.Init(case0.cbs, case0.whs, case0.cr); (err != nil) != case0.wantErr {
		t.Errorf("ModuleMarkdown.Init() error = %v, wantErr %v", err, case0.wantErr)
	}

}

func TestModuleMarkdown_renderMarkDownHandler(t *testing.T) {
	m := prepareModule()
	reqStrs := []string{"testcase0", "testcase1"}
	typeStrs := []string{"default"}

	for _, str := range reqStrs {
		for _, typeStr := range typeStrs {
			mdPath := "./testdata/" + str + ".md"
			targetPath := "./testdata/" + str + "_" + typeStr + ".output"
			urlPath := "/" + typeStr

			req := prepareRequest("unittest", urlPath)
			res := prepareResponse(mdPath)

			m.renderMarkDownHandler(req, res)
			got, err := ioutil.ReadAll(res.Body)
			if err != nil {
				t.Errorf("ModuleMarkdown.TestModuleMarkdown_renderMarkDownHandler() error = %v", err)
			}
			want, err := ioutil.ReadFile(targetPath)
			if err != nil {
				t.Errorf("ModuleMarkdown.TestModuleMarkdown_renderMarkDownHandler() error = %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("ModuleMarkdown.TestModuleMarkdown_renderMarkDownHandler(), got[%s], want[%s]", string(got), string(want))
			}
			if int64(len(want)) != res.ContentLength {
				t.Errorf("ModuleMarkdown.TestModuleMarkdown_renderMarkDownHandler() got[%d], want[%d]", res.ContentLength, len(want))
			}
		}
	}

	// test invalid response
	mdPath := "./testdata/testcase0.md"
	responses := prepareabnormalResponse(mdPath)
	for _, res := range responses {
		err := m.checkResponse(res)
		if err == nil {
			t.Errorf("ModuleMarkdown.TestModuleMarkdown_checkResponse() got[nil], want[%s]", err)
		}
	}
	// check not exist product
	urlPath := "/default"
	req := prepareRequest("not_exists", urlPath)
	res := prepareResponse(mdPath)
	code := m.renderMarkDownHandler(req, res)
	if code != bfe_module.BfeHandlerGoOn {
		t.Errorf("ModuleMarkdown.TestModuleMarkdown_checkResponse() got[%d], want[%d]", code, bfe_module.BfeHandlerGoOn)
	}
}

func prepareModule() *ModuleMarkdown {
	m := NewModuleMarkdown()
	m.Init(bfe_module.NewBfeCallbacks(), web_monitor.NewWebHandlers(), "./testdata")
	return m
}

func prepareRequest(product, path string) *bfe_basic.Request {
	req := new(bfe_basic.Request)
	req.HttpRequest = new(bfe_http.Request)
	req.HttpRequest.Header = make(bfe_http.Header)
	req.Route.Product = product
	req.Session = new(bfe_basic.Session)
	req.Context = make(map[interface{}]interface{})
	req.HttpRequest.URL = &url.URL{}
	req.HttpRequest.URL.Path = path
	return req
}

func prepareResponse(filename string) *bfe_http.Response {
	res := new(bfe_http.Response)
	res.StatusCode = 200
	res.Header = make(bfe_http.Header)
	content, _ := ioutil.ReadFile(filename)
	res.ContentLength = int64(len(content))

	res.Body = ioutil.NopCloser(bytes.NewReader(content))
	res.Header.Set("Content-Type", "text/markdown")
	res.Header.Set("Content-length", strconv.FormatInt(res.ContentLength, 10))

	return res
}

func prepareabnormalResponse(filename string) []*bfe_http.Response {
	var responses []*bfe_http.Response

	res := new(bfe_http.Response)
	res.StatusCode = 200
	res.Header = make(bfe_http.Header)
	content, _ := ioutil.ReadFile(filename)
	res.ContentLength = int64(len(content))

	res.Body = ioutil.NopCloser(bytes.NewReader(content))
	res.Header.Set("Content-Type", "text/html")
	res.Header.Set("Content-length", strconv.FormatInt(res.ContentLength, 10))
	responses = append(responses, res)

	res = new(bfe_http.Response)
	res.StatusCode = 200
	res.Header = make(bfe_http.Header)
	res.ContentLength = -1
	res.Header.Set("Content-Type", "text/markdown")
	res.Header.Set("Content-length", strconv.FormatInt(res.ContentLength, 10))
	responses = append(responses, res)

	res = new(bfe_http.Response)
	res.StatusCode = 200
	res.Header = make(bfe_http.Header)
	res.ContentLength = -1
	res.TransferEncoding = []string{"chunked"}
	res.Header.Set("Content-Type", "text/markdown")
	responses = append(responses, res)

	return responses
}

// Copyright (c) 2020 The BFE Authors.
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
package mod_waf

import (
	"net/url"
	"os"
	"testing"
)

import (
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func getModWaf() *ModuleWaf {
	mw := NewModuleWaf()
	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()
	cr := "./testdata"
	if err := mw.Init(cbs, whs, cr); err != nil {
		return nil
	}
	return mw
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

func TestModuleWafHandleWaf(t *testing.T) {
	mw := getModWaf()
	defer os.RemoveAll(mw.conf.Log.LogDir)
	req := prepareRequest("unittest", "/md")
	ret, _ := mw.handleWaf(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("handleWaf(), got=%v, want=%v", ret, bfe_module.BfeHandlerGoOn)
	}

	reqBashcmd := prepareRequest("unittest", "/md")
	reqBashcmd.HttpRequest.Header["user-agent"] = []string{"() { :; }; echo; echo; rm -rf ./*"}
	ret, _ = mw.handleWaf(reqBashcmd)
	if ret != bfe_module.BfeHandlerFinish {
		t.Errorf("handleWaf(), got=%v, want=%v", ret, bfe_module.BfeHandlerFinish)
	}

	queryV := map[string][]string{"path": {"./testdata/mod_waf/waf_rule_check.data"}}
	err := mw.loadProductRuleConf(queryV)
	if err != nil {
		t.Errorf("reload waf rule err=%s", err)
		t.FailNow()
	}

	ret, _ = mw.handleWaf(reqBashcmd)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("handleWaf(), got=%v, want=%v", ret, bfe_module.BfeHandlerGoOn)
	}

}

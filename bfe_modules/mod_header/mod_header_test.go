// Copyright (c) 2019 Baidu, Inc.
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
	"testing"
)

import (
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/baidu/bfe/bfe_basic"
	"github.com/baidu/bfe/bfe_module"
)

func TestGetModuleName(t *testing.T) {
	m := NewModuleHeader()
	if m.Name() != "mod_header" {
		t.Error("module name is wrong, Expect \"mod_header\"")
	}
}

func initModHeader() (*ModuleHeader, error) {
	m := NewModuleHeader()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, "./testdata"); err != nil {
		return nil, err
	}
	return m, nil
}

func TestModHeaderSetGlobal(t *testing.T) {
	m, err := initModHeader()
	if err != nil {
		t.Errorf("Test_mod_header(): %s", err)
		return
	}

	req := makeBasicRequest()
	req.Session = new(bfe_basic.Session)
	req.Session.IsSecure = true
	req.Session.Proto = "https"
	req.Route.Product = "pb"

	handler, _ := m.reqHeaderHandler(req)
	if handler != bfe_module.BfeHandlerGoOn {
		t.Error("reqHeaderHandler works abnormal")
	}

	headerValue := req.HttpRequest.Header["X-Ssl-Header"]
	if len(headerValue) == 0 || headerValue[0] != "2" {
		t.Error("header set failed for https, Expect header \"X-Ssl-Header:2\"")
	}
}

func TestModHeaderSet(t *testing.T) {
	m, err := initModHeader()
	if err != nil {
		t.Errorf("Test_mod_header(): %s", err)
		return
	}

	req := makeBasicRequest()
	req.Session = new(bfe_basic.Session)
	req.Session.IsSecure = true
	req.Session.Proto = "https"
	req.Route.Product = "pn"

	handler, _ := m.reqHeaderHandler(req)
	if handler != bfe_module.BfeHandlerGoOn {
		t.Error("reqHeaderHandler works abnormal")
	}

	headerValue := req.HttpRequest.Header["X-Ssl-Header"]
	if len(headerValue) == 0 || headerValue[0] != "1" {
		t.Error("header set failed for https, Expect header \"X-Ssl-Header:1\"")
	}

	req.Session.IsSecure = false
	handler, _ = m.reqHeaderHandler(req)
	if handler != bfe_module.BfeHandlerGoOn {
		t.Error("reqHeaderHandler works abnormal")
	}

	if len(req.HttpRequest.Header["X-Ssl-Header"]) != 0 {
		t.Error("header set failed for http, Expect empty header")
	}
}

func TestModHeaderAdd(t *testing.T) {
	m, err := initModHeader()
	if err != nil {
		t.Errorf("Test_mod_header(): %s", err)
		return
	}

	req := makeBasicRequest()
	req.HttpRequest.Header["Header_Add_Test"] = []string{"Header_Add_Value_app"}
	req.Session = new(bfe_basic.Session)
	req.Session.IsSecure = false
	req.Route.Product = "pb"

	handler, _ := m.reqHeaderHandler(req)
	if handler != bfe_module.BfeHandlerGoOn {
		t.Error("reqHeaderHandler works abnormal")
	}

	headerValue := req.HttpRequest.Header["Header_Add_Test"]

	if len(headerValue) < 1 {
		t.Errorf("header add failed, headerValue is: %s", headerValue)
	}
}

//NOTE: HEADER_DEL can't delete user defined headers.
func TestModHeaderDel(t *testing.T) {
	m, err := initModHeader()
	if err != nil {
		t.Errorf("Test_mod_header(): %s", err)
		return
	}

	req := makeBasicRequest()
	req.HttpRequest.Header["Host"] = []string{"www.example.org"}

	req.Session = new(bfe_basic.Session)
	req.Session.IsSecure = false
	req.Route.Product = "pb"

	handler, _ := m.reqHeaderHandler(req)
	if handler != bfe_module.BfeHandlerGoOn {
		t.Error("reqHeaderHandler works abnormal")
	}

	headerValue := req.HttpRequest.Header["Header_Del_Test"]

	if len(headerValue) > 0 {
		t.Error("header delete failed, headerValue is:", headerValue)
	}
}

func TestModHeaderTrustIP(t *testing.T) {
	m, err := initModHeader()
	if err != nil {
		t.Errorf("Test_mod_header(): %s", err)
		return
	}

	req := makeBasicRequest()
	req.HttpRequest.Header["Host"] = []string{"www.example.org"}

	req.Session = new(bfe_basic.Session)
	req.Session.IsTrustIP = false
	req.Route.Product = "pb"

	handler, _ := m.reqHeaderHandler(req)
	if handler != bfe_module.BfeHandlerGoOn {
		t.Error("reqHeaderHandler works abnormal")
	}

	headerValue := req.HttpRequest.Header["Trustip"]
	if len(headerValue) == 0 || headerValue[0] != "False" {
		t.Error("header set failed for trust ip, Expect header \"False\"")
	}

	req.Session.IsTrustIP = true
	handler, _ = m.reqHeaderHandler(req)
	if handler != bfe_module.BfeHandlerGoOn {
		t.Error("reqHeaderHandler works abnormal")
	}

	headerValue = req.HttpRequest.Header["Trustip"]
	if len(headerValue) == 0 || headerValue[0] != "True" {
		t.Error("header set failed for trust ip, Expect header \"True\"")
	}
}

func TestDelXsslHeaderNotTrustIPAndHttp(t *testing.T) {
	m, err := initModHeader()
	if err != nil {
		t.Errorf("Test_mod_header(): %s", err)
		return
	}

	req := makeBasicRequest()
	req.HttpRequest.Header["X-Ssl-Header"] = []string{"1"}
	req.Session = new(bfe_basic.Session)
	req.Session.IsTrustIP = false
	req.Session.IsSecure = false
	req.Route.Product = "pb"

	handler, _ := m.reqHeaderHandler(req)
	if handler != bfe_module.BfeHandlerGoOn {
		t.Error("reqHeaderHandler works abnormal")
	}

	headerValue := req.HttpRequest.Header["X-Ssl-Header"]
	if len(headerValue) != 0 {
		t.Error("header delete failed for not trust ip when in http")
	}
}

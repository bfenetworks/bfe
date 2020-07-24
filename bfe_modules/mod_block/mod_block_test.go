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

package mod_block

import (
	"net"
	"net/url"
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

func prepareModule() *ModuleBlock {
	m := NewModuleBlock()
	m.Init(bfe_module.NewBfeCallbacks(), web_monitor.NewWebHandlers(), "./testdata")
	return m
}

func prepareRequest() *bfe_basic.Request {
	request := new(bfe_basic.Request)
	request.HttpRequest = new(bfe_http.Request)
	request.Session = new(bfe_basic.Session)
	request.Context = make(map[interface{}]interface{})
	return request
}

func TestGlobalBlock(t *testing.T) {
	m := prepareModule()
	s := new(bfe_basic.Session)

	// case 1
	s.RemoteAddr, _ = net.ResolveTCPAddr("tcp", "192.168.1.10:8098")
	status := m.globalBlockHandler(s)
	if status != bfe_module.BfeHandlerGoOn {
		t.Errorf("Should not block session")
	}

	// case 2
	s.RemoteAddr, _ = net.ResolveTCPAddr("tcp", "10.1.1.200:8098")
	status = m.globalBlockHandler(s)
	if status != bfe_module.BfeHandlerClose {
		t.Errorf("Should block session")
	}

	// case 3
	s.RemoteAddr, _ = net.ResolveTCPAddr("tcp", "[1::2]:8098")
	status = m.globalBlockHandler(s)
	if status != bfe_module.BfeHandlerGoOn {
		t.Errorf("Should not block session")
	}

	// case 4
	s.RemoteAddr, _ = net.ResolveTCPAddr("tcp", "[1::1]:8098")
	status = m.globalBlockHandler(s)
	if status != bfe_module.BfeHandlerClose {
		t.Errorf("Should block session")
	}
}

func TestProductBlock(t *testing.T) {
	m := prepareModule()

	// case 1
	req := prepareRequest()
	status, _ := m.productBlockHandler(req)
	if status != bfe_module.BfeHandlerGoOn {
		t.Errorf("Should not block request")
	}

	// case 2
	req = prepareRequest()
	req.HttpRequest = &bfe_http.Request{
		Host: "n.example.org",
		URL:  &url.URL{},
	}
	req.Route = bfe_basic.RequestRoute{Product: "pn"}
	status, _ = m.productBlockHandler(req)
	if status != bfe_module.BfeHandlerClose {
		t.Errorf("Should block request")
	}

	val := req.GetContext(CtxBlockInfo)
	if val == nil {
		t.Errorf("ruleName should be pn_block_rule")
	}
	blockInfo, ok := val.(*BlockInfo)
	if !ok {
		t.Errorf("ruleName should be pn_block_rule")
	}
	if blockInfo.BlockRuleName != "pn_block_rule" {
		t.Errorf("ruleName should be pn_block_rule")
	}
}

func TestModuleMisc(t *testing.T) {
	m := prepareModule()
	if s, _ := m.getState(nil); s == nil {
		t.Errorf("Should return valid state")
	}
	if m.monitorHandlers() == nil {
		t.Errorf("Should return valid monitor handlers")
	}
	if m.reloadHandlers() == nil {
		t.Errorf("Should return valid reload handlers")
	}
}

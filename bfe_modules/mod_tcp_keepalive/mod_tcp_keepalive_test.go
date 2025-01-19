// Copyright (c) 2021 The BFE Authors.
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

package mod_tcp_keepalive

import (
	"testing"

	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func prepareModule() (*ModuleTcpKeepAlive, error) {
	m := NewModuleTcpKeepAlive()
	err := m.Init(bfe_module.NewBfeCallbacks(), web_monitor.NewWebHandlers(), "./testdata")
	return m, err
}

func prepareRequest() *bfe_basic.Request {
	request := new(bfe_basic.Request)
	request.HttpRequest = new(bfe_http.Request)
	request.Session = new(bfe_basic.Session)
	request.Context = make(map[interface{}]interface{})
	return request
}

func TestModuleMisc(t *testing.T) {
	m, err := prepareModule()
	if err != nil {
		t.Errorf("prepareModule() error: %v", err)
		return
	}
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

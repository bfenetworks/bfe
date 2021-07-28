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

// module for bfe unified log id generation

package mod_logid

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

import (
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	ModLogId = "mod_logid"
)

type ModuleLogIdState struct {
	NoLogidFromUpperBfe *metrics.Counter // counter for no logid cases when requests come from trusted ip
}

type ModuleLogId struct {
	name    string           // name of module
	state   ModuleLogIdState // module state
	metrics metrics.Metrics
}

func NewModuleLogId() *ModuleLogId {
	m := new(ModuleLogId)
	m.name = ModLogId
	m.metrics.Init(&m.state, ModLogId, 0)

	return m
}

func (m *ModuleLogId) Name() string {
	return m.name
}

func (m *ModuleLogId) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	// register handler
	err := cbs.AddFilter(bfe_module.HandleAccept, m.sessionIdHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.sessionIdHandler): %s", m.name, err.Error())
	}

	err = cbs.AddFilter(bfe_module.HandleBeforeLocation, m.requestIdHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.requestIdHandler): %s", m.name, err.Error())
	}

	// register web handler
	err = whs.RegisterHandler(web_monitor.WebHandleMonitor, m.name, m.getState)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.getState): %s", m.name, err.Error())
	}

	return nil
}

func (m *ModuleLogId) sessionIdHandler(session *bfe_basic.Session) int {
	session.SessionId = genLogId()

	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleLogId) requestIdHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	// check if request comes from trusted ip
	if req.Session.TrustSource() {
		logId := req.HttpRequest.Header.Get(bfe_basic.HeaderBfeLogId)
		if logId != "" {
			return bfe_module.BfeHandlerGoOn, nil
		} else {
			// trust ip, should has a logid
			m.state.NoLogidFromUpperBfe.Inc(1)
		}
	}

	// generate a new log id
	req.LogId = genLogId()
	req.HttpRequest.Header.Set(bfe_basic.HeaderBfeLogId, req.LogId)
	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleLogId) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func genLogId() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

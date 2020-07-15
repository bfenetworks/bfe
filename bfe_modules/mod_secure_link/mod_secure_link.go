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

package mod_secure_link

import (
	"fmt"
	"net/url"
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

var (
	openDebug = false
)

type ModuleSecureLinkState struct {
	ReqTotal  *metrics.Counter // all request in
	ReqAccept *metrics.Counter // request accept

	ReqWithoutExpiresKey   *metrics.Counter
	ReqInvalidExpiresValue *metrics.Counter
	ReqWithoutChecksumKey  *metrics.Counter
	ReqInvalidChecksum     *metrics.Counter
	ReqExpired             *metrics.Counter
}

// ModuleSecureLink mean secure link module
type ModuleSecureLink struct {
	name       string // name of module
	configPath string // path of config file
	state      ModuleSecureLinkState
	metrics    metrics.Metrics
	ruleTable  *SecureLinkTable // table of header rules
}

// NewModuleSecureLink create module
func NewModuleSecureLink() *ModuleSecureLink {
	m := &ModuleSecureLink{
		name:      "mod_secure_link",
		ruleTable: NewSecureLinkTable(),
	}
	m.metrics.Init(&m.state, m.name, 0)
	return m
}

// Name return module name
func (m *ModuleSecureLink) Name() string {
	return m.name
}

// Init init mode, will be invoked by framework
func (m *ModuleSecureLink) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	confPath := bfe_module.ModConfPath(cr, m.name)
	cfg, err := ConfLoad(confPath, cr)
	if err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}

	openDebug = cfg.Log.OpenDebug

	m.configPath = cfg.Basic.DataPath
	// load from config file to rule table
	if err := m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %s", err.Error())
	}

	// register handler
	err = cbs.AddFilter(bfe_module.HandleAfterLocation, m.validateHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.headerHandler): %s", m.name, err.Error())
	}

	// register web handler for reload
	err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.loadConfData): %s", m.name, err.Error())
	}

	return nil
}

func (m *ModuleSecureLink) validateHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
	rules, ok := m.ruleTable.Search(request.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn, nil
	}

	for _, rule := range rules {
		if !rule.Cond.Match(request) {
			continue
		}

		m.state.ReqTotal.Inc(1)
		err := rule.Checker.Check(request)
		if err == nil {
			m.state.ReqAccept.Inc(1)
			return bfe_module.BfeHandlerGoOn, nil
		}

		switch err {
		case ErrReqWithoutExpiresKey:
			m.state.ReqWithoutExpiresKey.Inc(1)
		case ErrReqInvalidExpiresValue:
			m.state.ReqInvalidExpiresValue.Inc(1)
		case ErrReqWithoutChecksumKey:
			m.state.ReqWithoutChecksumKey.Inc(1)
		case ErrReqInvalidChecksum:
			m.state.ReqInvalidChecksum.Inc(1)
		case ErrReqExpired:
			m.state.ReqExpired.Inc(1)
		}
		return bfe_module.BfeHandlerResponse, &bfe_http.Response{
			StatusCode: 403,
		}
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleSecureLink) loadConfData(query url.Values) error {
	// get file path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.configPath
	}

	// load from config file
	conf, err := DataLoad(path)
	if err != nil {
		return fmt.Errorf("err in SecureLinkConfLoad(%s): %s", path, err.Error())
	}

	// update to rule table
	m.ruleTable.Update(conf)

	return nil
}

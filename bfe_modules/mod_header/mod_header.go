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

package mod_header

import (
	"fmt"
	"net/url"
	"strconv"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	ReqHeader = iota
	RspHeader
	TotalType
)

const (
	GlobalProduct = "global"
)

var (
	openDebug = false
)

type ModuleHeader struct {
	name                 string       // name of module
	configPath           string       // path of config file
	disableDefaultHeader bool         // disable add default header
	ruleTable            *HeaderTable // table of header rules
}

func NewModuleHeader() *ModuleHeader {
	m := new(ModuleHeader)
	m.name = "mod_header"
	m.ruleTable = NewHeaderTable()
	return m
}

func (m *ModuleHeader) Name() string {
	return m.name
}

func (m *ModuleHeader) loadConfData(query url.Values) error {
	// get file path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.configPath
	}

	// load from config file
	conf, err := HeaderConfLoad(path)
	if err != nil {
		return fmt.Errorf("err in HeaderConfLoad(%s):%s", path, err.Error())
	}

	// update to rule table
	m.ruleTable.Update(conf)

	return nil
}

func DoHeader(req *bfe_basic.Request, headerType int, ruleList *RuleList) {
	for _, rule := range *ruleList {
		// rule condition is satisfied ?
		if rule.Cond.Match(req) {
			// do actions of the rule
			HeaderActionsDo(req, headerType, rule.Actions)

			// flag of last is true?
			if rule.Last {
				break
			}
		}
	}
}

func (m *ModuleHeader) applyProductRule(request *bfe_basic.Request, headerType int, product string) {
	// find rules for given product
	rules, ok := m.ruleTable.Search(product)
	if ok {
		h := getHeader(request, headerType)
		if openDebug {
			log.Logger.Debug("%s:before:headers=%s", m.name, *h)
		}

		DoHeader(request, headerType, rules[headerType])

		if openDebug {
			log.Logger.Debug("%s:after:headers=%s", m.name, *h)
		}
	}
}

func (m *ModuleHeader) setDefaultHeader(request *bfe_basic.Request) {
	if openDebug {
		log.Logger.Debug("setDefaultHeader():src ip=%s, isTrustIP=%t",
			request.RemoteAddr.String(), request.Session.TrustSource())
	}

	// set client addr
	modHeaderForwardedAddr(request)
	if request.ClientAddr != nil {
		setHeaderRealAddr(request, request.ClientAddr.IP.String(), strconv.Itoa(request.ClientAddr.Port))
	}

	// set bfe ip
	setHeaderBfeIP(request)
}

func (m *ModuleHeader) reqHeaderHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
	// if disableDefaultHeader is true
	// do not Set default header
	if !m.disableDefaultHeader {
		m.setDefaultHeader(request)
	}

	// apply global rule first
	m.applyProductRule(request, ReqHeader, GlobalProduct)

	// product specific rule will overwrite global rule for HEADER_SET action
	m.applyProductRule(request, ReqHeader, request.Route.Product)

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleHeader) rspHeaderHandler(request *bfe_basic.Request, res *bfe_http.Response) int {
	// apply global rule first
	m.applyProductRule(request, RspHeader, GlobalProduct)

	// product specific rule will overwrite global rule for HEADER_SET action
	m.applyProductRule(request, RspHeader, request.Route.Product)

	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleHeader) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error
	var conf *ConfModHeader

	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}

	return m.init(conf, cbs, whs)
}

func (m *ModuleHeader) init(cfg *ConfModHeader, cbs *bfe_module.BfeCallbacks,
	whs *web_monitor.WebHandlers) error {
	openDebug = cfg.Log.OpenDebug

	m.configPath = cfg.Basic.DataPath
	// set add default header or not
	m.disableDefaultHeader = cfg.Basic.DisableDefaultHeader

	// load from config file to rule table
	if err := m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %s", err.Error())
	}

	// register handler
	err := cbs.AddFilter(bfe_module.HandleAfterLocation, m.reqHeaderHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.headerHandler): %s", m.name, err.Error())
	}

	// register handler
	err = cbs.AddFilter(bfe_module.HandleReadResponse, m.rspHeaderHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.respHeaderHandler): %s", m.name, err.Error())
	}

	// register web handler for reload
	err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.loadConfData): %s", m.name, err.Error())
	}

	return nil
}

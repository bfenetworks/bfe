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

package mod_redirect

import (
	"fmt"
	"net/url"
	"path/filepath"
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

var (
	openDebug = false
)

type ModuleRedirect struct {
	name       string         // name of module
	configPath string         // path of config file
	ruleTable  *RedirectTable // table of redirect rules
}

func NewModuleRedirect() *ModuleRedirect {
	m := new(ModuleRedirect)
	m.name = "mod_redirect"
	m.ruleTable = NewRedirectTable()
	return m
}

func (m *ModuleRedirect) Name() string {
	return m.name
}

func (m *ModuleRedirect) loadConfData(query url.Values) (string, error) {
	// get file path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.configPath
	}

	// load from config file
	conf, err := redirectConfLoad(path)

	if err != nil {
		return "", fmt.Errorf("err in redirectConfLoad(%s):%s", path, err.Error())
	}

	// update to rule table
	m.ruleTable.Update(conf)

	_, fileName := filepath.Split(path)
	return fmt.Sprintf("%s=%s", fileName, conf.Version), nil
}

func redirectCodeSet(req *bfe_basic.Request, code int) {
	req.Redirect.Code = code
}

// PrepareReqRedirect do redirect to http request, with given redirect rules.
func PrepareReqRedirect(req *bfe_basic.Request, rules *RuleList) bool {
	for _, rule := range *rules {
		// rule condition is satisfied ?
		if rule.Cond.Match(req) {
			// do actions of the rule
			redirectActionsDo(req, rule.Actions)
			redirectCodeSet(req, rule.Status)

			// finish redirect rules process
			return true
		}
	}
	return false
}

// redirectHandler is a handler for doing redirect.
func (m *ModuleRedirect) redirectHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
	// find redirect rules for given request
	rules, ok := m.ruleTable.Search(request.Route.Product)

	if ok {
		if openDebug {
			log.Logger.Debug("%s:before:host=%s, path=%s, query=%s, rules=%v",
				m.name,
				request.HttpRequest.Host, request.HttpRequest.URL.Path,
				request.HttpRequest.URL.RawQuery, rules)
		}

		// redirect rules process
		needRedirect := PrepareReqRedirect(request, rules)

		if openDebug {
			if needRedirect {
				log.Logger.Debug("%s:after:redirectUrl=%s, redirectCode=%d",
					m.name, request.Redirect.Url, request.Redirect.Code)
			} else {
				log.Logger.Debug("%s:after:not need redirect", m.name)
			}
		}

		if needRedirect {
			return bfe_module.BfeHandlerRedirect, nil
		}
	}
	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleRedirect) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error
	var conf *ConfModRedirect

	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: cond load err %s", m.name, err.Error())
	}

	return m.init(conf, cbs, whs)
}

func (m *ModuleRedirect) init(cfg *ConfModRedirect, cbs *bfe_module.BfeCallbacks,
	whs *web_monitor.WebHandlers) error {
	openDebug = cfg.Log.OpenDebug

	m.configPath = cfg.Basic.DataPath

	// load from config file to rule table
	if _, err := m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %s", err.Error())
	}

	// register handler
	err := cbs.AddFilter(bfe_module.HandleFoundProduct, m.redirectHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.redirectHandler): %s", m.name, err.Error())
	}

	// register web handler for reload
	err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.loadConfData): %s", m.name, err.Error())
	}

	return nil
}

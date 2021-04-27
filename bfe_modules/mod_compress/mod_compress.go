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

package mod_compress

import (
	"errors"
	"fmt"
	"net/url"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	// support encode type of Accept-Encoding header
	EncodeGzip   = "gzip"
	EncodeBrotli = "br"

	// support encode type of Content-Encoding header
	EncodeIdentity = "identity"

	ModCompress = "mod_compress"
)

var (
	openDebug = false
)

type ModuleCompressState struct {
	ReqTotal              *metrics.Counter
	ReqSupportCompress    *metrics.Counter
	ReqMatchCompressRule  *metrics.Counter
	ResEncodeCompress     *metrics.Counter
	ResEncodeGzipCompress *metrics.Counter
	ResEncodeBrCompress   *metrics.Counter
}

type ModuleCompress struct {
	name      string
	conf      *ConfModCompress
	ruleTable *CompressRuleTable
	state     ModuleCompressState
	metrics   metrics.Metrics
}

func NewModuleCompress() *ModuleCompress {
	m := new(ModuleCompress)
	m.name = ModCompress
	m.metrics.Init(&m.state, ModCompress, 0)
	m.ruleTable = NewCompressRuleTable()
	return m
}

func (m *ModuleCompress) Name() string {
	return m.name
}

func (m *ModuleCompress) loadProductRuleConf(query url.Values) error {
	path := query.Get("path")
	if path == "" {
		path = m.conf.Basic.ProductRulePath
	}

	conf, err := ProductRuleConfLoad(path)
	if err != nil {
		return fmt.Errorf("err in ProductRuleConfLoad(%s): %s", path, err)
	}

	m.ruleTable.Update(conf)
	return nil
}

func checkSupportCompress(acceptEncoding string) bool {
	return checkSupportGzipCompress(acceptEncoding) || checkSupportBrotliCompress(acceptEncoding)
}

func checkSupportGzipCompress(acceptEncoding string) bool {
	return bfe_http.HasToken(acceptEncoding, EncodeGzip)
}

func checkSupportBrotliCompress(acceptEncoding string) bool {
	return bfe_http.HasToken(acceptEncoding, EncodeBrotli)
}

func (m *ModuleCompress) getCompressRule(req *bfe_basic.Request) (*compressRule, error) {
	if openDebug {
		log.Logger.Debug("%s check request", m.name)
	}
	m.state.ReqTotal.Inc(1)

	rules, ok := m.ruleTable.Search(req.Route.Product)
	if !ok {
		if openDebug {
			log.Logger.Debug("%s product %s not found, just pass", m.name, req.Route.Product)
		}
		return nil, errors.New("no compress rule")
	}

	for _, rule := range *rules {
		if openDebug {
			log.Logger.Debug("%s process rule: %v", m.name, rule)
		}

		if rule.Cond.Match(req) {
			m.state.ReqMatchCompressRule.Inc(1)
			return &rule, nil
		}
	}

	return nil, errors.New("no matched rule")
}

func (m *ModuleCompress) compressHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	acceptEncoding := req.HttpRequest.Header.GetDirect("Accept-Encoding")
	if !checkSupportCompress(acceptEncoding) {
		return bfe_module.BfeHandlerGoOn
	}
	m.state.ReqSupportCompress.Inc(1)

	contentEncoding := res.Header.GetDirect("Content-Encoding")
	if len(contentEncoding) != 0 && contentEncoding != EncodeIdentity {
		return bfe_module.BfeHandlerGoOn
	}

	rule, err := m.getCompressRule(req)
	if err != nil {
		return bfe_module.BfeHandlerGoOn
	}

	switch rule.Action.Cmd {
	case ActionGzip:
		if !checkSupportGzipCompress(acceptEncoding) {
			return bfe_module.BfeHandlerGoOn
		}

		res.Body, err = NewGzipFilter(res.Body, rule.Action.Quality, rule.Action.FlushSize)
		if err != nil {
			return bfe_module.BfeHandlerGoOn
		}

		res.Header.Set("Content-Encoding", EncodeGzip)
		m.state.ResEncodeGzipCompress.Inc(1)
	case ActionBrotli:
		if !checkSupportBrotliCompress(acceptEncoding) {
			return bfe_module.BfeHandlerGoOn
		}

		res.Body, err = NewBrotliFilter(res.Body, rule.Action.Quality, rule.Action.FlushSize)
		if err != nil {
			return bfe_module.BfeHandlerGoOn
		}

		res.Header.Set("Content-Encoding", EncodeBrotli)
		m.state.ResEncodeBrCompress.Inc(1)
	default:
		return bfe_module.BfeHandlerGoOn
	}

	res.Header.Del("Content-Length")
	m.state.ResEncodeCompress.Inc(1)

	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleCompress) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleCompress) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleCompress) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

func (m *ModuleCompress) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadProductRuleConf,
	}
	return handlers
}

func (m *ModuleCompress) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error

	confPath := bfe_module.ModConfPath(cr, m.name)
	if m.conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %v", m.name, err)
	}
	openDebug = m.conf.Log.OpenDebug

	if err = m.loadProductRuleConf(nil); err != nil {
		return fmt.Errorf("%s: loadProductRuleConf() err %v", m.name, err)
	}

	err = cbs.AddFilter(bfe_module.HandleReadResponse, m.compressHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.compressHandler): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.monitorHandlers): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.reloadHandlerr): %v", m.name, err)
	}

	return nil
}

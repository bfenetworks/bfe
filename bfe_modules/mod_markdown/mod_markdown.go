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
// package mod mark_down, deal with the markdown`s response

package mod_markdown

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/url"
	"strconv"
	"strings"
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
	ModMarkdown         = "mod_markdown"
	MaxBodyBytes        = 32 * 1024
	MarkdownContentType = "text/markdown"
	ConvertContentType  = "text/html; charset=UTF-8"
)

var openDebug = false

type ModuleMarkdownState struct {
	ReqTotal           *metrics.Counter
	ReqMarkDownRuleHit *metrics.Counter
	RspRenderSuccess   *metrics.Counter
	RspRenderFailure   *metrics.Counter
	RspRenderIgnore    *metrics.Counter
	// detail err reason
	ErrCountReadFail   *metrics.Counter
	ErrCountRenderFail *metrics.Counter
}

type ModuleMarkdown struct {
	name      string              //module name
	conf      *ConfModMarkdown    //module conf
	ruleTable *MarkdownRuleTable  // module rule table
	state     ModuleMarkdownState // module state
	metrics   metrics.Metrics     //module metrics
}

func NewModuleMarkdown() *ModuleMarkdown {
	m := new(ModuleMarkdown)
	m.name = ModMarkdown
	m.metrics.Init(&m.state, ModMarkdown, 0)
	m.ruleTable = NewMarkdownRuleTable()
	return m
}

func (m *ModuleMarkdown) Name() string {
	return m.name
}

func (m *ModuleMarkdown) loadProductRuleConf(query url.Values) error {
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

func (m *ModuleMarkdown) checkResponse(res *bfe_http.Response) error {
	contentType := res.Header.Get("Content-Type")
	if !strings.Contains(contentType, MarkdownContentType) {
		return fmt.Errorf("ModuleMarkdown.checkResponse(): content type don`t contain %s ", MarkdownContentType)
	}
	if len(res.TransferEncoding) > 0 && res.TransferEncoding[0] == "chunked" {
		return fmt.Errorf("ModuleMarkdown.checkResponse(): can not process chunked body")
	}
	if res.ContentLength <= 0 || res.ContentLength > MaxBodyBytes {
		return fmt.Errorf("ModuleMarkdown.checkResponse(): content len:%d", res.ContentLength)
	}
	return nil
}

func (m *ModuleMarkdown) renderMarkDownHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	rules, ok := m.ruleTable.Search(req.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn
	}
	m.state.ReqTotal.Inc(1)

	for _, rule := range *rules {
		if !(*rule.Cond).Match(req) {
			continue
		}
		m.state.ReqMarkDownRuleHit.Inc(1)

		err := m.checkResponse(res)
		if err != nil {
			m.state.RspRenderFailure.Inc(1)
			if openDebug {
				log.Logger.Debug("err in ModuleMarkdown.render.checkResponse(): %s", err)
			}
			return bfe_module.BfeHandlerGoOn
		}
		err = m.renderMarkDown(res)
		if err != nil {
			if openDebug {
				log.Logger.Debug("err in ModuleMarkdown.renderMarkDown(): %s", err)
			}

			res.Body.Close()
			*res = *bfe_basic.CreateInternalSrvErrResp(req)

			m.state.RspRenderFailure.Inc(1)
			return bfe_module.BfeHandlerGoOn
		}

		m.state.RspRenderSuccess.Inc(1)
		return bfe_module.BfeHandlerGoOn
	}
	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleMarkdown) renderMarkDown(res *bfe_http.Response) error {
	src, err := ioutil.ReadAll(res.Body)
	if err != nil {
		m.state.ErrCountReadFail.Inc(1)
		return err
	}

	dst, err := Render(src)
	if err != nil {
		m.state.ErrCountRenderFail.Inc(1)
		return err
	}
	res.Body.Close()
	res.Body = ioutil.NopCloser(bytes.NewReader(dst))

	res.ContentLength = int64(len(dst))
	res.Header.Set("Content-Length", strconv.FormatInt(res.ContentLength, 10))
	res.Header.Set("Content-Type", ConvertContentType)
	return nil
}

func (m *ModuleMarkdown) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleMarkdown) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleMarkdown) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

func (m *ModuleMarkdown) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadProductRuleConf,
	}
	return handlers
}

func (m *ModuleMarkdown) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	var err error

	confPath := bfe_module.ModConfPath(cr, m.Name())
	if m.conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %v", m.name, err)
	}
	openDebug = m.conf.Log.OpenDebug

	if err = m.loadProductRuleConf(nil); err != nil {
		return fmt.Errorf("%s: loadProductRuleConf() err %v", m.Name(), err)
	}

	err = cbs.AddFilter(bfe_module.HandleReadResponse, m.renderMarkDownHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.renderMarkDownHandler): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.monitorHandlers): %v", m.Name(), err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.reloadHandlerr): %v", m.Name(), err)
	}
	return nil
}

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

package mod_trace

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/opentracing/opentracing-go"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_modules/mod_trace/trace"
)

const (
	ModTrace = "mod_trace"
	CtxSpan  = "mod_trace.span"
)

var (
	openDebug = false
)

var (
	globalTrace *trace.Trace
)

type ModuleTrace struct {
	name      string
	conf      *ConfModTrace
	ruleTable *TraceRuleTable

	// metrics
	state   ModuleTraceState
	metrics metrics.Metrics
}

type ModuleTraceState struct {
	StartSpanCount  *metrics.Counter
	FinishSpanCount *metrics.Counter
}

func NewModuleTrace() *ModuleTrace {
	m := new(ModuleTrace)
	m.name = ModTrace
	m.metrics.Init(&m.state, ModTrace, 0)
	m.ruleTable = NewTraceRuleTable()
	return m
}

func (m *ModuleTrace) Name() string {
	return m.name
}

func (m *ModuleTrace) loadRuleData(query url.Values) (string, error) {
	// get file path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.conf.Basic.DataPath
	}

	// load from config file
	conf, err := TraceRuleFileLoad(path)
	if err != nil {
		return "", fmt.Errorf("%s: TraceRuleFileLoad(%s) error: %v", m.name, path, err)
	}

	// update to rule table
	m.ruleTable.Update(conf)

	_, fileName := filepath.Split(path)
	return fmt.Sprintf("%s=%s", fileName, conf.Version), nil
}

func (m *ModuleTrace) startTrace(request *bfe_basic.Request) (int, *bfe_http.Response) {
	rules, ok := m.ruleTable.Search(request.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn, nil
	}

	for _, rule := range rules {
		if rule.Cond.Match(request) {
			if !rule.Enable {
				continue
			}

			// count start span
			m.state.StartSpanCount.Inc(1)

			// start span
			span := StartSpan(request.HttpRequest)

			// log request with span
			trace.LogRequest(span, request.HttpRequest)

			// inject request header
			InjectRequestHeader(span, request.HttpRequest)

			// set context, used by finishTrace
			request.SetContext(CtxSpan, span)
			if openDebug {
				log.Logger.Info("%s%s start span", request.HttpRequest.Host, request.HttpRequest.URL.Path)
			}

			// if hit one rule, no need to match next
			break
		}
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleTrace) finishTrace(req *bfe_basic.Request, res *bfe_http.Response) int {
	value := req.GetContext(CtxSpan)
	if value == nil {
		return bfe_module.BfeHandlerGoOn
	}

	span := value.(opentracing.Span)

	// set http code
	if req.HttpResponse != nil {
		trace.LogResponseCode(span, req.HttpResponse.StatusCode)
	}

	// set error msg
	if len(req.ErrMsg) > 0 {
		trace.SetErrorWithEvent(span, req.ErrMsg)
	}

	// set backend info
	trace.LogBackend(span, req)

	// count finish span
	m.state.FinishSpanCount.Inc(1)

	// finish span
	span.Finish()

	if openDebug {
		log.Logger.Info("%s%s finish span, err msg:[%s]", req.HttpRequest.Host, req.HttpRequest.URL.Path, req.ErrMsg)
	}

	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleTrace) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleTrace) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleTrace) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

func (m *ModuleTrace) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadRuleData,
	}
	return handlers
}

func (m *ModuleTrace) init(conf *ConfModTrace, cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers) error {
	var err error

	globalTrace, err = trace.NewTrace(conf.Basic.ServiceName, conf.GetTraceConfig())
	if err != nil {
		return err
	}

	_, err = m.loadRuleData(nil)
	if err != nil {
		return err
	}

	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.startTrace)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.startTrace): %v", m.name, err)
	}

	err = cbs.AddFilter(bfe_module.HandleRequestFinish, m.finishTrace)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.finishTrace): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.reloadHandlers): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.reloadHandlers): %v", m.name, err)
	}

	return nil
}

func (m *ModuleTrace) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	var err error
	var conf *ConfModTrace

	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}

	m.conf = conf
	openDebug = conf.Log.OpenDebug
	return m.init(conf, cbs, whs)
}

// InjectRequestHeaders used to inject opentracing headers into the request.
func InjectRequestHeader(span opentracing.Span, r *bfe_http.Request) {
	if globalTrace != nil && span != nil && r != nil {
		globalTrace.Inject(
			span.Context(),
			opentracing.HTTPHeaders,
			opentracing.HTTPHeadersCarrier(r.Header))
	}
}

// StartSpan starts a new span
func StartSpan(r *bfe_http.Request) opentracing.Span {
	var span opentracing.Span
	if globalTrace != nil && r != nil {
		// set span name
		spName := spanName(r)

		// If headers contain trace data, create child span from parent; else, create root span
		spanCtx, err := globalTrace.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(r.Header))
		if err != nil {
			span = globalTrace.StartSpan(spName)
		} else {
			span = globalTrace.StartSpan(spName, opentracing.ChildOf(spanCtx))
		}
	} else {
		// if trace is nil or request is nil, create noop span
		span = opentracing.NoopTracer{}.StartSpan("")
	}

	return span // caller must defer span.Finish()
}

// spanName returns the rendered span name by request
func spanName(r *bfe_http.Request) string {
	host := strings.SplitN(r.Host, ":", 2)[0]
	return host + r.URL.Path
}

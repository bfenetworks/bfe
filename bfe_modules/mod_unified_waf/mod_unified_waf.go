// Copyright (c) 2025 The BFE Authors.
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

package mod_unified_waf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/delay_counter"
	"github.com/baidu/go-lib/web-monitor/module_state2"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

// delay_counter.DelayRecent parameters
const (
	DELAY_STAT_INTERVAL = 20 // delay stat interval
	DELAY_BUCKET_SIZE   = 1  // delay bucket size
	DELAY_BUCKET_NUM    = 20 // delay bucket num
)

const (
	DIFF_COUNTER_INTERVAL = 20
)

const NoneWafName = "None"

const (
	ModChaitinWaf = "mod_unified_waf"

	NOAH_SD_MOD_WAF         = "waf_client"
	NOAH_SD_MOD_WAF_DIFF    = "waf_client_diff"
	NOAH_MOD_WAF_DELAY      = "waf_client_delay"
	NOAH_MOD_WAF_PEEK_DELAY = "waf_client_delay_peek_body"
	NOAH_MOD_WAF_COMP_DELAY = "waf_client_delay_call_competition"

	TO_DELETE_CLIENTS = "waf_client.to_delete_clients"
	ACTIVE_CLIENTS    = "waf_client.active_clients"
	DELETED_CLIENTS   = "waf_client.deleted_clients"
	ADDED_CLIENTS     = "waf_client.added_clients"
)

var COUNTER_KEYS = []string{
	bfe_basic.REQ_NO_CHECK,
	bfe_basic.REQ_FORBIDDEN,
	bfe_basic.REQ_OK,
	bfe_basic.REQ_TIMEOUT,
	bfe_basic.REQ_OTHER,
	bfe_basic.NET_ERR,
}

var (
	openDebug = false
)

type ModuleWaf struct {
	name          string // name of module
	conf          *ConfModWaf
	wafClientPool *WafClientPool
	prodParams    *ProductParamTable
	wafData       *GlobalParamConf

	modWafDataPath      string // path for mod_unified_waf.data
	productParamPath    string // path for product_param.data
	albWafInstancesPath string // path for alb_waf_instances.data

	monitor *MonitorStates // monitor states

	isNoneWaf bool
}

func NewModuleWaf() *ModuleWaf {
	m := new(ModuleWaf)
	m.name = ModChaitinWaf

	m.monitor = NewMonitorStates()
	m.wafClientPool = NewWafClientPool(m.monitor)
	m.prodParams = NewProductParamTable()

	return m
}

func (m *ModuleWaf) Name() string {
	return m.name
}

func (m *ModuleWaf) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	var err error

	// parse config
	confPath := bfe_module.ModConfPath(cr, m.name)
	if err = m.LoadConfig(confPath, cr); err != nil {
		return fmt.Errorf("%s.Init(): ParseConfig %s", m.name, err.Error())
	}

	m.monitor.state.Set("WafProductName", m.conf.Basic.WafProductName)
	if m.conf.Basic.WafProductName == NoneWafName {
		m.isNoneWaf = true
	} else {
		m.isNoneWaf = false
	}
	if m.isNoneWaf {
		log.Logger.Info("WafProductName is None.")
	}

	// set debug switch
	openDebug = m.conf.Log.OpenDebug
	if openDebug {
		log.Logger.Debug("mod_unified_waf openDebug")
	}

	if !m.isNoneWaf {
		err = m.wafClientPool.SetConfBasic(m.conf.Basic)
		if err != nil {
			// log.Logger.Error("failed to SetConfBasic: %s", err.Error())
			return err
		}
	}

	// load configs
	err = m.loadWafData(nil)
	if err != nil {
		return fmt.Errorf("%s.Init(): loadWafData(): %s", m.name, err.Error())
	}

	err = m.loadWafInstances(nil)
	if err != nil {
		return fmt.Errorf("%s.Init(): loadWafInstances(): %s", m.name, err.Error())
	}

	err = m.loadProductParam(nil)
	if err != nil {
		return fmt.Errorf("%s.Init(): loadProductParam(): %s", m.name, err.Error())
	}

	if !m.isNoneWaf {
		// register handler
		err = cbs.AddFilter(bfe_module.HandleAfterLocation, m.wafHandler) // for after location
		if err != nil {
			return fmt.Errorf("%s.Init(): AddFilter(m.wafHandler): %s", m.name, err.Error())
		}
	}

	// register web handlers for reload
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.reloadHandlers): %s", m.name, err.Error())
	}

	// register web handlers for monitor
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.monitorHandlers): %s", m.name, err.Error())
	}

	return nil
}

func (m *ModuleWaf) getState() *module_state2.StateData {

	res := m.monitor.state.GetAll()

	return res
}

func (m *ModuleWaf) getStateDiff() *module_state2.CounterDiff {
	stateDiff := m.monitor.stateDiff.Get()
	return &stateDiff
}

func (m *ModuleWaf) getMetricsState(params map[string][]string) ([]byte, error) {
	s := m.monitor.metrics.GetAll()
	return s.Format(params)
}

// register web monitor handlers
func (m *ModuleWaf) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:                             web_monitor.CreateStateDataHandler(m.getState),
		m.name + ".diff":                   web_monitor.CreateCounterDiffHandler(m.getStateDiff),
		m.name + ".delay":                  m.monitor.delay.FormatOutput,
		m.name + ".delay_peek_body":        m.monitor.delayPeekBody.FormatOutput,
		m.name + ".delay_call_competition": m.monitor.delayCallComp.FormatOutput,
		m.name + ".mstate":                 m.getMetricsState,
	}

	return handlers
}

// register web reload handlers
func (m *ModuleWaf) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name + ".product_parameter": m.loadProductParam,
		m.name + ".waf_data":          m.loadWafData,
		m.name + ".waf_instances":     m.loadWafInstances,
	}

	return handlers
}

// for mod_unified_waf.data
func (m *ModuleWaf) WafClientDataLoad(path string) error {
	data, err := WafDataParamLoadAndCheck(path)
	if err != nil {
		return err
	}
	m.wafData = data

	if !m.isNoneWaf {
		m.wafClientPool.UpdateWafParam(data)
	}

	ver := data.Version
	param := &data.Config
	bdata, _ := json.Marshal(param)
	m.monitor.state.Set("GlobalParam", string(bdata))
	m.monitor.state.Set("GlobalParam.Version", ver)

	return nil
}

// for alb_waf_instances.data
func (m *ModuleWaf) WafInstancesLoad(path string) error {
	data, err := AlbWafInstancesLoadAndCheck(path)
	if err != nil {
		return err
	}

	var wafInstances []WafInstance
	wafInstances = data.WafCluster

	if !m.isNoneWaf {
		m.wafClientPool.Update(wafInstances, data.Version)
	}

	instData, _ := json.Marshal(wafInstances)
	m.monitor.state.Set("WafInstances", string(instData))
	m.monitor.state.Set("WafInstances.Version", data.Version)

	return nil
}

// for product_param.data
func (m *ModuleWaf) ProductParamLoad(path string) error {
	data, err := ProductParamLoadAndCheck(path)
	if err != nil {
		return err
	}

	m.prodParams.Update(data.Config, data.Version)

	conf, _ := json.Marshal(data.Config)
	m.monitor.state.Set("ProductParam", string(conf))
	m.monitor.state.Set("ProductParam.Version", data.Version)

	return nil
}

// loadWafData is a registered reload callback
// params:
//   - query: url query, query["path"] is the file need to load
//     if query["path"] is not set, use default path
func (m *ModuleWaf) loadWafData(query url.Values) error {
	// get file path
	path := query.Get("path")
	if path == "" {
		//use default
		path = m.modWafDataPath
	}
	err := m.WafClientDataLoad(path)
	return err
}

// loadWafInstances is a registered reload callback
// params:
//   - query: url query, query["path"] is the file need to load
//     if query["path"] is not set, use default path
func (m *ModuleWaf) loadWafInstances(query url.Values) error {
	// get file path
	path := query.Get("path")
	if path == "" {
		//use default
		path = m.albWafInstancesPath
	}
	err := m.WafInstancesLoad(path)
	if err != nil {
		log.Logger.Warn("loadWafInstances(): %s", err.Error())
	}
	return err
}

// loadProductParam is a registered reload callback
// params:
//   - query: url query, query["path"] is the file need to load
//     if query["path"] is not set, use default path
func (m *ModuleWaf) loadProductParam(query url.Values) error {
	// get file path
	path := query.Get("path")
	if path == "" {
		//use default
		path = m.productParamPath
	}
	err := m.ProductParamLoad(path)
	return err
}

// load configure from conf file
func (m *ModuleWaf) LoadConfig(confPath string, confRoot string) error {
	conf, err := ConfLoad(confPath, confRoot)
	if err != nil {
		return fmt.Errorf("%s conf load error %s", m.name, err.Error())
	}
	m.conf = conf

	m.modWafDataPath = conf.ConfigPath.ModWafDataPath
	m.productParamPath = conf.ConfigPath.ProductParamPath
	m.albWafInstancesPath = conf.ConfigPath.AlbWafInstancesPath

	return nil
}

func (m *ModuleWaf) getRequestWafParam(req *bfe_basic.Request) *WafParam {
	return m.prodParams.GetRequestWafParam(req)
}

// module call backs
// handler for finish http request
func (m *ModuleWaf) wafHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	conf := m.getRequestWafParam(req)
	// no waf check
	if conf == nil {
		if openDebug {
			log.Logger.Debug("product %s has no waf config", req.Route.Product)
		}

		m.monitor.state.Inc(bfe_basic.REQ_NO_CHECK, 1)
		setWafStatus(req, int(bfe_basic.WAF_NO_CHECK))
		return bfe_module.BfeHandlerGoOn, nil
	}
	// if wafClientPool is not created. this should never happen.
	if m.wafClientPool == nil {
		log.Logger.Warn("wafClientPool is nil")
		m.monitor.state.Inc(bfe_basic.REQ_NO_CHECK, 1)
		setWafStatus(req, int(bfe_basic.WAF_NO_CHECK))
		return bfe_module.BfeHandlerGoOn, nil
	}

	// convert request
	wafReq, err := m.genWafRequest(req, conf, &m.monitor.delayPeekBody)
	if err != nil {
		log.Logger.Error("genWafRequest(): %s", err.Error())
		m.monitor.state.Inc(bfe_basic.REQ_NO_CHECK, 1)
		setWafStatus(req, int(bfe_basic.WAF_NO_CHECK))
		return bfe_module.BfeHandlerGoOn, nil
	}

	// get a waf client object
	wafClient, err := m.wafClientPool.Alloc()
	if err != nil {
		// only if all waf-instance is not usable
		log.Logger.Warn("m.wafClientPool.Alloc() failed: %s", err.Error())
		m.monitor.state.Inc(bfe_basic.REQ_NO_CHECK, 1)
		setWafStatus(req, int(bfe_basic.WAF_NO_CHECK))
		return bfe_module.BfeHandlerGoOn, nil
	}
	defer m.wafClientPool.Release(wafClient)

	// call waf-server
	block, eventId := wafClient.Detect(req, wafReq, conf)
	if block {
		return bfe_module.BfeHandlerFinish, GenForbiddenHttpResponse(req, eventId)
	}

	return bfe_module.BfeHandlerGoOn, nil
}

// generate request for remote call
func (m *ModuleWaf) genWafRequest(req *bfe_basic.Request, param *WafParam, delayPeekBody *delay_counter.DelayRecent) (*http.Request, error) {
	httpRequest := req.HttpRequest
	wafRequest, err := http.NewRequest(req.HttpRequest.Method, httpRequest.URL.String(), nil)
	if err != nil {
		return nil, err
	}

	// copy request data
	wafRequest.Method = httpRequest.Method
	wafRequest.URL = httpRequest.URL
	wafRequest.Proto = httpRequest.Proto
	wafRequest.ProtoMajor = httpRequest.ProtoMajor
	wafRequest.ProtoMinor = httpRequest.ProtoMinor
	//copy httpRequest.Header
	wafRequest.Header = generateHeaders(httpRequest.Header)
	wafRequest.TransferEncoding = httpRequest.TransferEncoding
	wafRequest.Host = httpRequest.Host
	wafRequest.Form = httpRequest.Form
	wafRequest.PostForm = httpRequest.PostForm
	wafRequest.MultipartForm = httpRequest.MultipartForm
	//copy httpRequest.Trailer
	wafRequest.Trailer = generateHeaders(httpRequest.Trailer)
	wafRequest.RemoteAddr = httpRequest.RemoteAddr
	wafRequest.RequestURI = httpRequest.RequestURI

	// make empty body
	wafRequest.Body = ioutil.NopCloser(bytes.NewReader([]byte{}))
	wafRequest.ContentLength = 0
	wafRequest.Header.Set("Content-Length", fmt.Sprintf("%d", wafRequest.ContentLength))

	// copy body if needed
	var peekN int64 = 0
	if param.SendBody && checkBodyWithHttpMethod(httpRequest.Method) && httpRequest.ContentLength > 0 {
		// set when request is not chunk (ContentLength > 0) and method is POST/PUT/PATCH
		peekN = httpRequest.ContentLength
		if peekN > int64(param.SendBodySize) {
			peekN = int64(param.SendBodySize)
		}
	}

	if peekN <= 0 {
		return wafRequest, nil
	}

	var wafBodySize int64
	if p, ok := httpRequest.Body.(Peeker); ok {
		t := time.Now()
		b, err := p.Peek(int(peekN))
		if err == nil {
			// set body
			wafRequest.Body = ioutil.NopCloser(bytes.NewReader(b))
			wafBodySize = int64(len(b))
			wafRequest.ContentLength = wafBodySize
			if openDebug {
				log.Logger.Info("mod_unified_waf Peek succ, %d, contentlen:%d", peekN, wafBodySize)
			}
		} else {
			log.Logger.Info("mod_unified_waf genWafRequest():peekN:%d, contentlen:%d, peek body err %s", peekN, httpRequest.ContentLength, err)
		}

		delayPeekBody.AddBySub(t, time.Now())
	} else {
		log.Logger.Info("mod_unified_waf genWafRequest(): do not have Peeker")
	}

	wafRequest.Header.Set("Content-Length", fmt.Sprintf("%d", wafRequest.ContentLength))

	return wafRequest, nil
}

type wafForbiddenInfo struct {
	EventId string `json:"event_id"`
}

func GenForbiddenHttpResponse(req *bfe_basic.Request, eventId string) *bfe_http.Response {
	tmp := &wafForbiddenInfo{}
	tmp.EventId = eventId
	bodystr := ""
	if bodybytes, err := json.Marshal(tmp); err == nil {
		bodystr = string(bodybytes)
	}

	ret := bfe_basic.CreateSpecifiedContentResp(req, bfe_http.StatusOK, "application/json", bodystr)

	return ret
}

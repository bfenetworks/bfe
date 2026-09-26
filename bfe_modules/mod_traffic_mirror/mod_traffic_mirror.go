// Copyright (c) 2026 The BFE Authors.
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

// Package mod_traffic_mirror implements traffic mirroring (shadow traffic):
// while the gateway forwards a request to its production upstream, an
// asynchronous copy is sent to a mirror target cluster. The mirror response
// is drained to completion (SSE streams to [DONE]) and discarded; only
// lightweight semantics (status, usage, finish_reason, error, latency) are
// recorded for verification and statistics. The main request path is never
// blocked, failed or canceled by anything the mirror does.
package mod_traffic_mirror

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"path/filepath"
	"sync"

	"github.com/bfenetworks/go-lib/log"
	"github.com/bfenetworks/go-lib/web-monitor/module_state2"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_module"
)

var (
	openDebug = false
)

const (
	DiffCounterInterval = 20 // interval for diff counter (in seconds)
)

const (
	ModTrafficMirror     = "mod_traffic_mirror"
	ModTrafficMirrorDiff = "mod_traffic_mirror_diff"

	// CtxMirrored marks that a request has already been mirrored; the AI
	// retry loop re-enters clusterInvoke (and thus HandleForward) per
	// attempt, and each request must be mirrored at most once
	CtxMirrored = "mod_traffic_mirror.ctx"
)

// key for counter of mod_traffic_mirror
var CounterKeys = []string{
	"REQ_TOTAL",
	"SUBMIT_DROP",
	"SKIP_SAMPLE",
	"SKIP_BODY_LIMIT",
	"SKIP_REWRITE",
	"SEND_FAIL",
	"CIRCUIT_OPEN",
	"RESP_TRUNCATED",
}

// skip reasons for the labeled skip metric and debug logs
const (
	skipReasonSample    = "sample"
	skipReasonBodyLimit = "body_limit"
	skipReasonRewrite   = "rewrite"
)

type ModuleTrafficMirror struct {
	name      string                     // name of module
	state     *module_state2.State       // module state
	stateDiff module_state2.CounterSlice // diff counter for module state

	conf      *ConfModTrafficMirror
	ruleTable *mirrorRuleTable
	sender    *mirrorSender
	breaker   *mirrorCircuitBreaker

	pmsStates *PrometheusStates
	lock      sync.RWMutex
}

func NewModuleTrafficMirror() *ModuleTrafficMirror {
	m := new(ModuleTrafficMirror)
	m.name = ModTrafficMirror
	m.state = new(module_state2.State)

	// init module state
	m.state.Init()
	m.state.CountersInit(CounterKeys)
	m.state.SetKeyPrefix(ModTrafficMirror)

	// init module state diff
	m.stateDiff.Init(m.state, DiffCounterInterval)
	m.stateDiff.SetKeyPrefix(ModTrafficMirrorDiff)

	// create rule table
	m.ruleTable = newMirrorRuleTable()

	m.pmsStates = newPrometheusStates()

	return m
}

func (m *ModuleTrafficMirror) Name() string {
	return m.name
}

func (m *ModuleTrafficMirror) getState() *module_state2.StateData {
	return m.state.GetAll()
}

func (m *ModuleTrafficMirror) getStateDiff() *module_state2.CounterDiff {
	stateDiff := m.stateDiff.Get()
	return &stateDiff
}

func (m *ModuleTrafficMirror) getPrometheus() ([]byte, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	snap := m.ruleTable.snapshotCounters()

	m.pmsStates.reqTotal.Set(float64(snap.reqTotal))
	m.pmsStates.submitDrop.Set(float64(snap.submitDrop))
	m.pmsStates.skipSample.Set(float64(snap.skipSample))
	m.pmsStates.skipBodyLimit.Set(float64(snap.skipBodyLimit))
	m.pmsStates.skipRewrite.Set(float64(snap.skipRewrite))
	m.pmsStates.sendFail.Set(float64(snap.sendFail))

	return m.pmsStates.toString()
}

// load product rule table
func (m *ModuleTrafficMirror) loadProductRuleTable(query url.Values) (string, error) {
	// get file path
	path := query.Get("path")
	if path == "" {
		path = m.conf.Basic.ProductRulePath // use default
	}
	log.Logger.Info("%s: begin load ProductRuleTable, path:%s", m.name, path)

	// load productRule conf
	productConf, err := MirrorRuleConfLoad(path)
	if err != nil {
		return "", fmt.Errorf("%s: product conf load err %s", m.name, err.Error())
	}

	if err = m.ruleTable.load(productConf); err != nil {
		return "", fmt.Errorf("%s: build mirror rule err %s", m.name, err.Error())
	}

	// set module version
	version := *productConf.Version
	m.state.Set("Version", version)

	log.Logger.Info("%s: load ProductRuleTable done, version[%s]", m.name, version)

	confBytes, _ := json.Marshal(productConf)
	m.state.Set("ProductRuleTable", string(confBytes))

	_, fileName := filepath.Split(path)
	return fmt.Sprintf("%s=%s", fileName, version), nil
}

// all monitor handlers
func (m *ModuleTrafficMirror) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:                 web_monitor.CreateStateDataHandler(m.getState),
		m.name + ".diff":       web_monitor.CreateCounterDiffHandler(m.getStateDiff),
		m.name + ".prometheus": m.getPrometheus,
	}
	return handlers
}

// all reload handlers
func (m *ModuleTrafficMirror) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadProductRuleTable,
	}

	return handlers
}

// module Init
func (m *ModuleTrafficMirror) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	// load module config
	confPath := bfe_module.ModConfPath(cr, m.name)
	conf, err := ConfLoad(confPath, cr)
	if err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}
	m.conf = conf
	openDebug = conf.Log.OpenDebug

	// create circuit breaker, sender and worker pool
	m.breaker = newMirrorCircuitBreaker(
		conf.Basic.CircuitBreakerFailThreshold, conf.Basic.CircuitBreakerCooldownSec)
	m.sender = newMirrorSender(conf, m.ruleTable, m.breaker, m.pmsStates)
	m.sender.start(conf.Basic.MaxConcurrent)

	// load product rule table
	if _, err := m.loadProductRuleTable(nil); err != nil {
		return fmt.Errorf("%s.Init(): loadProductRuleTable(): %s", m.name, err.Error())
	}

	// register filter at HandleForward: at this point the cluster/backend is
	// selected, the route/auth result is resolved and the request body is
	// buffered, so mirror rules can match on the full AI context. The mirror
	// task is submitted asynchronously and the main request continues without
	// waiting.
	err = cbs.AddFilter(bfe_module.HandleForward, m.mirrorHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.mirrorHandler): %s", m.name, err.Error())
	}

	// register web handler for monitor
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.monitorHandlers): %s", m.name, err.Error())
	}

	// register web handler for reload
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.reloadHandlers): %s", m.name, err.Error())
	}

	return nil
}

// sampleHit decides per-request whether to mirror under the rule percentage
func sampleHit(percentage int) bool {
	if percentage >= 100 {
		return true
	}
	if percentage <= 0 {
		return false
	}
	return rand.Intn(100) < percentage
}

// mirrorHandler is the HandleForward callback. It only matches rules,
// snapshots the request and submits the mirror task; everything that can
// fail is counted and swallowed so the main request is never affected.
func (m *ModuleTrafficMirror) mirrorHandler(req *bfe_basic.Request) int {
	// only process requests that went through the AI pipeline
	rules, ok := m.ruleTable.Search(req.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn
	}
	rule := m.ruleTable.Match(req, rules)
	if rule == nil {
		return bfe_module.BfeHandlerGoOn
	}

	// each request is mirrored at most once (AI fallback retries re-enter
	// clusterInvoke and thus HandleForward per attempt)
	if req.GetContext(CtxMirrored) != nil {
		return bfe_module.BfeHandlerGoOn
	}

	// percentage sampling (FR-3)
	if !sampleHit(rule.Percentage) {
		m.ruleTable.incSkipSample()
		m.pmsStates.reportSkip(skipReasonSample)
		return bfe_module.BfeHandlerGoOn
	}

	// snapshot everything the async side needs; SvrDataConf is reset to nil
	// after clusterInvoke and the async goroutine must not touch the request
	task, rewriteFailed, skipReason := buildMirrorTask(req, rule, m.conf.Basic.MaxMirrorBodyBytes)
	if task == nil {
		m.ruleTable.incSkipBodyLimit()
		m.pmsStates.reportSkip(skipReason)
		return bfe_module.BfeHandlerGoOn
	}
	if rewriteFailed {
		m.ruleTable.incSkipRewrite()
		m.pmsStates.reportSkip(skipReasonRewrite)
	}

	// mark before submit: even a dropped attempt must not be mirrored again
	// by a subsequent retry of the same request
	req.SetContext(CtxMirrored, true)

	if !m.sender.Submit(task) {
		m.ruleTable.incSubmitDrop()
		return bfe_module.BfeHandlerGoOn
	}

	// synchronous fields for the access log; async results are reported via
	// Prometheus metrics and are not written back (they usually finish after
	// the access log is written)
	if aiInfo := req.GetAiBasicInfo(); aiInfo != nil {
		aiInfo.MirrorHit = true
		aiInfo.MirrorCluster = rule.MirrorCluster
	}

	if openDebug {
		log.Logger.Info("%s: mirrored req product[%s] cluster[%s] model[%s]",
			m.name, task.Product, task.Cluster, task.Model)
	}

	return bfe_module.BfeHandlerGoOn
}

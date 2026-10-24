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

package mod_ai_context

import (
	"fmt"

	"github.com/bfenetworks/go-lib/web-monitor/module_state2"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	DiffCounterInterval = 20 // interval for diff counter (in seconds)
)

const (
	ModAiContext     = "mod_ai_context"
	ModAiContextDiff = "mod_ai_context_diff"
)

// key for counter of mod_ai_context (see design-changes.md 10.2)
var CounterKeys = []string{
	"CTX_TOTAL",
	"CTX_SKIP_NO_RULE",
	"CTX_SKIP_PROTOCOL",
	"CTX_SKIP_BODY_INCOMPLETE",
	"CTX_SKIP_PARSE_ERR",
	"CTX_SKIP_UNDER_THRESHOLD",
	"CTX_TRIGGERED",
	"CTX_TRIGGERED_CONSERVATIVE",
	"CTX_TRIGGERED_BALANCED",
	"CTX_TRIGGERED_AGGRESSIVE",
	"CTX_DONE_TRIM",
	"CTX_DONE_REWRITE",
	"CTX_GATE_FALLBACK",
	"CTX_REPAIR_ROLLBACK",
	"CTX_WINDOW_DEFAULTED",
	"CTX_CFG_UNKNOWN_FIELD",
	"CTX_LATENCY_MS",
}

type ModuleAiContext struct {
	name      string                     // name of module
	state     *module_state2.State       // module state
	stateDiff module_state2.CounterSlice // diff counter for module state

	productConfPath string // path for product rule

	ruleTable *contextRuleTable // product rule table
	estimator TokenEstimator    // token estimator (phase 1: heuristic)
	pipeline  *Pipeline         // compress pipeline
}

func NewModuleAiContext() *ModuleAiContext {
	m := new(ModuleAiContext)
	m.name = ModAiContext
	m.state = new(module_state2.State)

	// init module state
	m.state.Init()
	m.state.CountersInit(CounterKeys)
	m.state.SetKeyPrefix(ModAiContext)

	// init module state diff
	m.stateDiff.Init(m.state, DiffCounterInterval)
	m.stateDiff.SetKeyPrefix(ModAiContextDiff)

	// create rule table
	m.ruleTable = newContextRuleTable()
	m.estimator = NewHeuristicEstimator()
	m.pipeline = NewPipeline(m.estimator)

	return m
}

func (m *ModuleAiContext) Name() string {
	return m.name
}

// module Init
func (m *ModuleAiContext) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	// load module config
	confPath := bfe_module.ModConfPath(cr, m.name)
	conf, err := ConfLoad(confPath, cr)
	if err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}
	m.productConfPath = conf.Basic.ProductRulePath
	openDebug = conf.Log.OpenDebug

	// load product rule table
	if _, err := m.loadProductRuleTable(nil); err != nil {
		return fmt.Errorf("%s.Init(): loadProductRuleTable(): %s", m.name, err.Error())
	}

	// register filter for context compression at HandleAfterAITargetModel:
	// the callback fires after the target model is resolved (needed for the
	// token budget) and before the request is forwarded to the backend; it
	// fires per cluster attempt, so the handler must enforce per-request-once
	// semantics itself (AiBasicInfo.ContextCompressStatus is the guard).
	err = cbs.AddFilter(bfe_module.HandleAfterAITargetModel, m.contextCompressHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.contextCompressHandler): %s", m.name, err.Error())
	}

	// register filter for the response annotation at HandleReadResponse: the
	// response headers are annotated (x-ai-context-compression) when the
	// request was actually compressed; body streaming semantics are unchanged.
	err = cbs.AddFilter(bfe_module.HandleReadResponse, m.responseAnnotationHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.responseAnnotationHandler): %s", m.name, err.Error())
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

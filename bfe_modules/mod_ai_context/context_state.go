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
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/bfenetworks/go-lib/log"
	"github.com/bfenetworks/go-lib/web-monitor/module_state2"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

// context compress status values recorded in AiBasicInfo.ContextCompressStatus
// and in the access log (ai_context_compress_status). The empty string means
// the request was not processed by the module; the non-empty value also acts
// as the per-request idempotency guard for the per-cluster-attempt callback.
const (
	StatusSkipNoRule         = "skip_no_rule"         // no rule matched or mode=off
	StatusSkipProtocol       = "skip_protocol"        // not an OpenAI chat completions request
	StatusSkipBodyIncomplete = "skip_body_incomplete" // request body not fully buffered
	StatusSkipParseErr       = "skip_parse_err"       // messages parsing failed
	StatusSkipUnderThreshold = "skip_under_threshold" // estimate below budget*triggerRatio
	StatusTrim               = "trim"                 // pipeline finished on the lossless trim layers
	StatusRewrite            = "rewrite"              // pipeline finished with the P2 rule rewrite
	StatusRepairRollback     = "repair_rollback"      // repair/serialize failed, original body forwarded
)

// isCompressDone reports whether the status means the request was actually
// compressed (trim or rewrite); skip_* / rollback statuses are not.
func isCompressDone(status string) bool {
	return status == StatusTrim || status == StatusRewrite
}

var (
	openDebug = false
)

func (m *ModuleAiContext) getState() *module_state2.StateData {
	return m.state.GetAll()
}

func (m *ModuleAiContext) getStateDiff() *module_state2.CounterDiff {
	stateDiff := m.stateDiff.Get()
	return &stateDiff
}

// load product rule table
func (m *ModuleAiContext) loadProductRuleTable(query url.Values) (string, error) {
	// get file path
	path := ""
	if query != nil {
		path = query.Get("path")
	}
	if path == "" {
		path = m.productConfPath // use default
	}
	log.Logger.Info("%s: begin load ProductRuleTable, path:%s", m.name, path)

	// load productRule conf
	productConf, err := ContextRuleConfLoad(path)
	if err != nil {
		return "", fmt.Errorf("%s: product conf load err %s", m.name, err.Error())
	}

	if err = m.ruleTable.load(productConf); err != nil {
		return "", fmt.Errorf("%s: build ai context rule err %s", m.name, err.Error())
	}

	// forward-compat alert: count ignored unknown config fields
	if productConf.UnknownFields > 0 {
		m.state.Inc("CTX_CFG_UNKNOWN_FIELD", int(productConf.UnknownFields))
		log.Logger.Warn("%s: %d unknown config field(s) ignored (forward compat)", m.name, productConf.UnknownFields)
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
func (m *ModuleAiContext) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           web_monitor.CreateStateDataHandler(m.getState),
		m.name + ".diff": web_monitor.CreateCounterDiffHandler(m.getStateDiff),
	}
	return handlers
}

// all reload handlers
func (m *ModuleAiContext) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadProductRuleTable,
	}

	return handlers
}

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

package mod_ai_intent

import (
	"bytes"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/bfenetworks/go-lib/web-monitor/metrics"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_module"
)

const ModAiIntent = "mod_ai_intent"

var openDebug = false

type ModuleAiIntentState struct {
	ReqTotal         *metrics.Counter // total resolve triggered by intent conditions
	ReqResolved      *metrics.Counter // resolves with usable answers (header/cache/model)
	ReqHeader        *metrics.Counter // resolves using explicit intent header entries
	ReqCacheHit      *metrics.Counter // resolves served from in-process cache
	ReqUnknown       *metrics.Counter // resolves degraded to unknown (non-AI request, extract failure)
	ReqErr           *metrics.Counter // resolves failed on decision service (error / breaker open)
	BreakerOpen      *metrics.Counter // times the circuit breaker opened
	QuestionsVersion *metrics.State   // current version of the questions conf
}

type ModuleAiIntent struct {
	name      string
	conf      *ConfModAiIntent
	questions *QuestionsConf
	cache     *IntentCache
	resolver  *intentResolver
	state     ModuleAiIntentState
	metrics   metrics.Metrics
	latency   *latencyHistogram
}

func NewModuleAiIntent() *ModuleAiIntent {
	m := new(ModuleAiIntent)
	m.name = ModAiIntent
	m.metrics.Init(&m.state, ModAiIntent, 0)
	m.latency = newLatencyHistogram()
	return m
}

func (m *ModuleAiIntent) Name() string {
	return m.name
}

func (m *ModuleAiIntent) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	confPath := bfe_module.ModConfPath(cr, m.name)
	var err error
	if m.conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %v", m.name, err)
	}
	openDebug = m.conf.Log.OpenDebug

	m.questions = NewQuestionsConf(m.conf.Basic.QuestionsPath)
	if err := m.questions.Load(); err != nil {
		return fmt.Errorf("%s: questions load err %v", m.name, err)
	}
	if cur := m.questions.Current(); cur != nil {
		m.state.QuestionsVersion.Set(cur.Version())
	}

	m.cache = NewIntentCache(m.conf.Basic.CacheSize, time.Duration(m.conf.Basic.CacheTTLSeconds)*time.Second)
	client := NewDecisionClient(m.conf.Basic.DecisionServiceAddr, m.conf.Basic.TimeoutMs)
	m.resolver = newIntentResolver(m.conf, m.questions, m.cache, client, &m.state, m.latency)

	// Inject the lazy resolver and the confidence gate into bfe_basic, so
	// the req_ai_intent_in condition primitive can trigger classification
	// on demand. bfe_basic only holds these function pointers and does not
	// depend on this module package, hence module registration order is not
	// sensitive.
	bfe_basic.SetAiIntentResolver(m.resolver.Resolve)
	bfe_basic.SetAiIntentThreshold(m.questions.Threshold)

	monitorHandlers := map[string]interface{}{
		m.name:                m.getState,
		m.name + "_histogram": m.getLatency,
	}
	if err := web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, monitorHandlers); err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(monitor): %v", m.name, err)
	}

	reloadHandlers := map[string]interface{}{
		m.name: m.loadQuestionsConf,
	}
	if err := web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, reloadHandlers); err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(reload): %v", m.name, err)
	}

	return nil
}

// loadQuestionsConf hot reloads the questions data file. A reload is
// rejected (and the old version kept) when the file is unreadable or
// invalid; a reload with an unchanged Version is a no-op.
func (m *ModuleAiIntent) loadQuestionsConf(query url.Values) error {
	path := query.Get("path")
	if path == "" {
		path = m.conf.Basic.QuestionsPath
	}

	if err := m.questions.Load(path); err != nil {
		return fmt.Errorf("%s: questions load(%s) err: %s", m.name, path, err)
	}
	if cur := m.questions.Current(); cur != nil {
		m.state.QuestionsVersion.Set(cur.Version())
	}
	return nil
}

func (m *ModuleAiIntent) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleAiIntent) getLatency(params map[string][]string) ([]byte, error) {
	return m.latency.format(), nil
}

// latencyHistogram is a fixed-bucket histogram of classification latency
// (milliseconds), rendered on the module monitor page.
var latencyBucketsMs = []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}

type latencyHistogram struct {
	mu     sync.Mutex
	counts []int64 // len = len(latencyBucketsMs)+1, last bucket is +Inf
	total  int64
	sumMs  int64
}

func newLatencyHistogram() *latencyHistogram {
	return &latencyHistogram{counts: make([]int64, len(latencyBucketsMs)+1)}
}

func (h *latencyHistogram) observe(ms int64) {
	if ms < 0 {
		ms = 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.total++
	h.sumMs += ms
	for i, b := range latencyBucketsMs {
		if ms <= b {
			h.counts[i]++
			return
		}
	}
	h.counts[len(h.counts)-1]++
}

func (h *latencyHistogram) format() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s_classify_latency_ms_total: %d\n", ModAiIntent, h.total)
	if h.total > 0 {
		fmt.Fprintf(&buf, "%s_classify_latency_ms_avg: %d\n", ModAiIntent, h.sumMs/h.total)
	}
	for i, b := range latencyBucketsMs {
		fmt.Fprintf(&buf, "%s_classify_latency_ms_le_%d: %d\n", ModAiIntent, b, h.counts[i])
	}
	fmt.Fprintf(&buf, "%s_classify_latency_ms_inf: %d\n", ModAiIntent, h.counts[len(h.counts)-1])
	return buf.Bytes()
}

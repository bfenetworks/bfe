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
	"github.com/baidu/go-lib/web-monitor/delay_counter"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/module_state2"
)

type ModuleWafState struct {
}

type MonitorStates struct {
	delay         delay_counter.DelayRecent  // delay counter for request of wait response type
	delayPeekBody delay_counter.DelayRecent  // delay counter for peek http body
	delayCallComp delay_counter.DelayRecent  // delay counter for concurrency call competition
	state         *module_state2.State       // module state
	stateDiff     module_state2.CounterSlice // diff counter of module state

	underlyingState ModuleWafState
	metrics         metrics.Metrics //moudle state with prometheus format

}

func NewMonitorStates() *MonitorStates {
	m := MonitorStates{}
	m.delay.Init(DELAY_STAT_INTERVAL, DELAY_BUCKET_SIZE, DELAY_BUCKET_NUM)
	m.delayPeekBody.Init(DELAY_STAT_INTERVAL, DELAY_BUCKET_SIZE, DELAY_BUCKET_NUM)
	m.delayCallComp.Init(DELAY_STAT_INTERVAL, DELAY_BUCKET_SIZE, DELAY_BUCKET_NUM)

	m.state = new(module_state2.State)
	m.state.Init()
	m.state.CountersInit(COUNTER_KEYS)
	m.stateDiff.Init(m.state, DIFF_COUNTER_INTERVAL)

	m.delay.SetKeyPrefix(KP_MOD_WAF_DELAY)
	m.delayPeekBody.SetKeyPrefix(KP_MOD_WAF_PEEK_DELAY)
	m.delayCallComp.SetKeyPrefix(KP_MOD_WAF_COMP_DELAY)

	m.state.SetKeyPrefix(KP_SD_MOD_WAF)
	m.stateDiff.SetKeyPrefix(KP_SD_MOD_WAF_DIFF)

	m.metrics.Init(&m.underlyingState, ModUnifiedWaf, 0)

	return &m
}

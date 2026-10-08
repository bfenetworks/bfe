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

// Package mod_ai_batch implements OpenAI Batch API (/v1/files, /v1/batches)
// passthrough support: operation classification, upload line/byte counting,
// response field extraction, batch-level key affinity bindings, and the
// reserve/settle/release orchestration of post-paid batch quota. Request and
// response bodies are never rewritten; batch traffic streams end to end
// without retry buffering (see bfe_server.isBatchPassthroughMode).
package mod_ai_batch

import (
	"fmt"
	"sync"

	"github.com/bfenetworks/go-lib/log"
	"github.com/bfenetworks/go-lib/web-monitor/module_state2"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

var (
	openDebug = false

	// defaultBatchModule backs the exported cross-module helpers
	// (BatchActiveCount/BatchGlobalFileCeilings) used by mod_ai_rate_limit.
	// The module is a singleton in practice: bfe_modules.go constructs it once.
	defaultBatchModule *ModuleAiBatch
)

const (
	ModAiBatch = "mod_ai_batch"

	// ctxBatchKey holds the per-request *batchContext between this module's
	// callbacks (classify at HandleAfterAITargetModel, consume later).
	ctxBatchKey = "mod_ai_batch.ctx"

	// defaultBatchStateTTL is the fallback TTL (seconds) for BATCH_FILE /
	// BATCH_TASK hash keys and the batch affinity binding: 24h completion
	// window plus a 24h download grace.
	defaultBatchStateTTL = 172800

	// smallJsonCap bounds the buffered response body for upload/create/get/
	// cancel operations (their response bodies are KB-level JSON).
	smallJsonCap = 1 << 20
)

// key for counter of mod_ai_batch
var counterKeys = []string{
	"REQ_TOTAL",             // classified batch requests
	"FILE_BIND_WRITE",       // BATCH_FILE bindings written
	"FILE_BIND_WRITE_FAIL",  // BATCH_FILE write failed (fail-open)
	"TASK_WRITE",            // BATCH_TASK writes
	"TASK_WRITE_FAIL",       // BATCH_TASK write failed (fail-open)
	"UPLOAD_REJECT_SIZE",    // upload rejected by effective file limits
	"UPLOAD_ABORT_STREAM",   // chunked upload aborted mid-stream on limit
	"OWNER_CHECK_REJECT",    // download rejected by ownership check
	"OWNER_CHECK_MISS",      // ownership binding missing (allow_log policy)
	"RESERVE_OK",            // reserve written at batch create
	"RESERVE_PRECHECK_MISS", // balance pre-check failed, create rejected
	"RELEASE_OK",            // reserve released (terminal status/cancel)
	"SETTLE_USAGE_PARSE",    // download usage parsed (jsonl lines)
	"SETTLE_USAGE_PARSE_ERR",
	"REDIS_ERR",
}

type ModuleAiBatch struct {
	name      string
	state     *module_state2.State
	stateDiff module_state2.CounterSlice

	conf     *ConfModAiBatch
	data     *BatchDataConf // hot reloadable
	dataLock sync.RWMutex

	redisClient redis_client.Client // nil when Redis is not configured
}

func NewModuleAiBatch() *ModuleAiBatch {
	m := new(ModuleAiBatch)
	m.name = ModAiBatch
	m.state = new(module_state2.State)
	m.state.Init()
	m.state.CountersInit(counterKeys)
	m.state.SetKeyPrefix(ModAiBatch)

	m.stateDiff.Init(m.state, 20)
	m.stateDiff.SetKeyPrefix(ModAiBatch + "_diff")

	m.data = defaultBatchDataConf()
	defaultBatchModule = m
	return m
}

func (m *ModuleAiBatch) Name() string {
	return m.name
}

func (m *ModuleAiBatch) getState(params map[string][]string) ([]byte, error) {
	return m.state.GetAll().FormatOutput(params)
}

func (m *ModuleAiBatch) getStateDiff() *module_state2.CounterDiff {
	stateDiff := m.stateDiff.Get()
	return &stateDiff
}

func (m *ModuleAiBatch) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": web_monitor.CreateCounterDiffHandler(m.getStateDiff),
	}
	return handlers
}

func (m *ModuleAiBatch) reloadHandlers() map[string]interface{} {
	return map[string]interface{}{
		m.name: m.loadBatchData,
	}
}

// getData returns the current hot-reloadable data conf.
func (m *ModuleAiBatch) getData() *BatchDataConf {
	m.dataLock.RLock()
	defer m.dataLock.RUnlock()
	return m.data
}

func (m *ModuleAiBatch) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	confPath := bfe_module.ModConfPath(cr, m.name)
	conf, err := ConfLoad(confPath, cr)
	if err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}
	m.conf = conf
	openDebug = conf.Log.OpenDebug

	if conf.Redis.Bns != "" {
		m.redisClient = redis_client.NewRedisClient(&redis_client.Options{
			ServiceConf:    conf.Redis.Bns,
			MaxIdle:        conf.Redis.MaxIdle,
			MaxActive:      conf.Redis.MaxActive,
			Wait:           false,
			ConnTimeoutMs:  conf.Redis.ConnectTimeout,
			ReadTimeoutMs:  conf.Redis.ReadTimeout,
			WriteTimeoutMs: conf.Redis.WriteTimeout,
			Password:       conf.Redis.Password,
		})
	} else {
		log.Logger.Warn("%s.Init(): redis not configured, batch state/affinity disabled (fail-open)", m.name)
	}

	if _, err := m.loadBatchData(nil); err != nil {
		return fmt.Errorf("%s.Init(): loadBatchData(): %s", m.name, err)
	}

	// Register callbacks. The module is registered after mod_ai_rate_limit in
	// bfe_modules.go so that its HandleAfterAITargetModel handler runs after
	// rate limiting: by then the batch limits are already resolved into
	// AiBasicInfo and a local-limit rejection has already happened, so the
	// pre-checks here (balance reserve pre-check, ownership read, upload size
	// gate) see the final decision context. Its HandleReadResponse handler
	// also runs after mod_body_process, which stays a pass-through for
	// file/batch responses (see the batch guards there), so the wrapper sees
	// the exact response bytes.
	if err = cbs.AddFilter(bfe_module.HandleAfterAITargetModel, m.targetModelHandler); err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(targetModelHandler): %v", m.name, err)
	}
	if err = cbs.AddFilter(bfe_module.HandleReadResponse, m.readResponseHandler); err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(readResponseHandler): %v", m.name, err)
	}
	if err = cbs.AddFilter(bfe_module.HandleRequestFinish, m.requestFinishHandler); err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(requestFinishHandler): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(monitor): %v", m.name, err)
	}
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(reload): %v", m.name, err)
	}
	return nil
}

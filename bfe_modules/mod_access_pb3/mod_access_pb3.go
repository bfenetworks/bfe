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

package mod_access_pb3

import (
	"fmt"

	"github.com/bfenetworks/go-lib/log"
	"github.com/bfenetworks/go-lib/log/log4go"
	"github.com/bfenetworks/go-lib/web-monitor/metrics"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_util/access_log"
)

const (
	BFE_MOD_ACCESS_PB3 = "mod_access_pb3"
)

var (
	openDebug = false
)

type ModuleAccessLogState struct {
	AllReqLogCount *metrics.Counter
	AllSesLogCount *metrics.Counter
}

type ModuleAccessPb3 struct {
	name    string
	logger  log4go.Logger
	conf    *ConfModAccessPb2
	state   ModuleAccessLogState
	metrics metrics.Metrics
}

func NewModuleAccessPb3() *ModuleAccessPb3 {
	m := new(ModuleAccessPb3)
	m.name = "mod_access_pb3"
	m.metrics.Init(&m.state, BFE_MOD_ACCESS_PB3, 0)
	return m
}

func (m *ModuleAccessPb3) Name() string {
	return m.name
}

func (m *ModuleAccessPb3) Close() error {
	if m.logger != nil {
		m.logger.Close()
	}
	return nil
}

func (m *ModuleAccessPb3) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error
	var conf *ConfModAccessPb2

	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath); err != nil {
		return fmt.Errorf("%s: cond load err %s", m.name, err.Error())
	}
	m.conf = conf

	m.logger, err = access_log.LoggerInit(access_log.LogConfig{
		LogPrefix:   conf.Log.LogPrefix,
		LogDir:      conf.Log.LogDir,
		RotateWhen:  conf.Log.RotateWhen,
		BackupCount: conf.Log.BackupCount,
	})
	if err != nil {
		return fmt.Errorf("%s.Init():create logger:%s", m.name, err.Error())
	}

	openDebug = m.conf.BasicConf.OpenDebug

	err = cbs.AddFilter(bfe_module.HandleRequestFinish, m.requestFinishHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.requestFinishHandler): %s", m.name, err.Error())
	}

	err = cbs.AddFilter(bfe_module.HandleFinish, m.sessionFinishHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.sessionFinishHandler): %s", m.name, err.Error())
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.monitorHandlers): %s", m.name, err.Error())
	}

	return nil
}

func (m *ModuleAccessPb3) requestFinishHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	bfeLog := m.requestLogGen(req, res)

	if openDebug {
		log.Logger.Debug("%s.requestFinishHandler(), bfeLog: %s", m.name, bfeLog.String())
	}

	b2logMsg, err := b2logMsgGen(bfeLog)
	if err != nil {
		if openDebug {
			log.Logger.Debug("%s.requestFinishHandler() b2logMsgGen err: %s", m.name, err.Error())
		}
		return bfe_module.BfeHandlerGoOn
	}

	m.state.AllReqLogCount.Inc(1)
	m.logger.Info(b2logMsg)

	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleAccessPb3) sessionFinishHandler(session *bfe_basic.Session) int {
	bfeLog := sessionLogGen(session)

	if openDebug {
		log.Logger.Debug("%s.sessionFinishHandler:%s", m.name, bfeLog.String())
	}

	b2logMsg, err := b2logMsgGen(bfeLog)
	if err != nil {
		if openDebug {
			log.Logger.Debug("%s.sessionFinishHandler() b2logMsgGen err: %s", m.name, err.Error())
		}
		return bfe_module.BfeHandlerGoOn
	}

	m.state.AllSesLogCount.Inc(1)
	m.logger.Info(b2logMsg)

	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleAccessPb3) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleAccessPb3) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleAccessPb3) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

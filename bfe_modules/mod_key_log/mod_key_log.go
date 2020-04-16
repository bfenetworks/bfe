// Copyright (c) 2019 Baidu, Inc.
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

package mod_key_log

import (
	"encoding/hex"
	"fmt"
)

import (
	"github.com/baidu/go-lib/log/log4go"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/baidu/bfe/bfe_basic"
	"github.com/baidu/bfe/bfe_module"
	"github.com/baidu/bfe/bfe_util/access_log"
)

// ModuleKeyLog writes key logs in NSS key log format so that external
// programs(eg. wireshark) can decrypt TLS connections for trouble shooting.
//
// For more information about NSS key log format, see:
// https://developer.mozilla.org/en-US/docs/Mozilla/Projects/NSS/Key_Log_Format
type ModuleKeyLog struct {
	name   string         // module name
	conf   *ConfModKeyLog // module config
	logger log4go.Logger  // key logger
}

func NewModuleKeyLog() *ModuleKeyLog {
	m := new(ModuleKeyLog)
	m.name = "mod_key_log"
	return m
}

func (m *ModuleKeyLog) Name() string {
	return m.name
}

func (m *ModuleKeyLog) logTlsKey(session *bfe_basic.Session) int {
	tlsState := session.TlsState
	if tlsState == nil {
		return bfe_module.BfeHandlerGoOn
	}

	// key log format: <label> <ClientRandom> <MasterSecret>
	keyLog := fmt.Sprintf("CLIENT_RANDOM %s %s",
		hex.EncodeToString(tlsState.ClientRandom), // connection id
		hex.EncodeToString(tlsState.MasterSecret)) // connection master secret
	m.logger.Info(keyLog)

	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleKeyLog) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var conf *ConfModKeyLog
	var err error

	// load config
	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}
	m.conf = conf

	// init logger
	m.logger, err = access_log.LoggerInit(m.conf.Log.LogPrefix, m.conf.Log.LogDir,
		m.conf.Log.RotateWhen, m.conf.Log.BackupCount)
	if err != nil {
		return fmt.Errorf("%s.Init():create logger:%s", m.name, err.Error())
	}

	// register handler
	err = cbs.AddFilter(bfe_module.HandleHandshake, m.logTlsKey)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.logTlsKey): %s", m.name, err.Error())
	}

	return nil
}

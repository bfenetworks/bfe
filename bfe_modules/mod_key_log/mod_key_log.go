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

package mod_key_log

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
)

import (
	"github.com/baidu/go-lib/log/log4go"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_util/access_log"
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

	dataConfigPath string       // path of data config file
	ruleTable      *KeyLogTable // table of key_log rules

}

func NewModuleKeyLog() *ModuleKeyLog {
	m := new(ModuleKeyLog)
	m.name = "mod_key_log"
	m.ruleTable = NewKeyLogTable()
	return m
}

func (m *ModuleKeyLog) Name() string {
	return m.name
}

func (m *ModuleKeyLog) logTlsKey(session *bfe_basic.Session) int {
	if m.isNeedKeyLog(session) {
		tlsState := session.TlsState
		if tlsState == nil {
			return bfe_module.BfeHandlerGoOn
		}
		// key log format: <label> <ClientRandom> <MasterSecret>
		keyLog := fmt.Sprintf("CLIENT_RANDOM %s %s",
			hex.EncodeToString(tlsState.ClientRandom), // connection id
			hex.EncodeToString(tlsState.MasterSecret)) // connection master secret
		m.logger.Info(keyLog)
	}
	return bfe_module.BfeHandlerGoOn
}

// isNeedKeyLog Determine if you need to print the key log
func (m *ModuleKeyLog) isNeedKeyLog(session *bfe_basic.Session) bool {
	rules, ok := m.ruleTable.Search(session.Product)
	if !ok {
		rules, ok = m.ruleTable.Search(bfe_basic.GlobalProduct)
	}
	if !ok {
		return false
	}
	req := &bfe_basic.Request{
		Session: session,
	}
	for _, rule := range *rules {
		// rule condition is satisfied ?
		if rule.Cond.Match(req) {
			// do actions of the rule
			// todo

			// finish key_log rules process
			return true
		}
	}
	return false
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
	m.dataConfigPath = conf.Basic.DataPath

	// load from data config file to rule table
	if _, err := m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %s", err.Error())
	}

	// init logger
	m.logger, err = access_log.LoggerInit(m.conf.Log)
	if err != nil {
		return fmt.Errorf("%s.Init():create logger:%s", m.name, err.Error())
	}

	// register handler
	err = cbs.AddFilter(bfe_module.HandleHandshake, m.logTlsKey)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.logTlsKey): %s", m.name, err.Error())
	}

	// register web handler for reload
	err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.loadConfData): %s", m.name, err.Error())
	}

	return nil
}

func (m *ModuleKeyLog) loadConfData(query url.Values) (string, error) {
	// get file path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.dataConfigPath
	}

	// load from config file
	conf, err := keyLogConfLoad(path)
	if err != nil {
		return "", fmt.Errorf("err in keyLogConfLoad(%s):%s", path, err.Error())
	}

	// update to rule table
	m.ruleTable.Update(conf)

	_, fileName := filepath.Split(path)
	return fmt.Sprintf("%s=%s", fileName, conf.Version), nil
}

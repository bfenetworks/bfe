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
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
)

import (
	"github.com/bfenetworks/go-lib/log/log4go"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_util/access_log"
)

// ModuleKeyLog provides a writer for TLS key logs in NSS key log format so
// that external programs (eg. wireshark) can decrypt TLS connections for
// trouble shooting.
//
// The writer is registered (via bfe_module.TlsKeyLogProvider) as
// tls.Config.KeyLogWriter, and the standard library writes key log lines
// during the TLS handshake.
//
// For more information about NSS key log format, see:
// https://developer.mozilla.org/en-US/docs/Mozilla/Projects/NSS/Key_Log_Format
//
// Deprecated: product based key log filtering (rules in the data config) is
// no longer supported, because the key log is written during the handshake
// while the product is only known after routing. The rules in the data
// config are still parsed for backward compatibility but ignored.
type ModuleKeyLog struct {
	name   string         // module name
	conf   *ConfModKeyLog // module config
	writer io.Writer      // writer for TLS key log (NSS keylog format)
	file   *os.File       // key log file

	dataConfigPath string       // path of data config file
	ruleTable      *KeyLogTable // table of key_log rules (deprecated)
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

// TlsKeyLogWriter returns the writer for TLS key log (NSS keylog format).
// Returning nil disables tls key logging.
func (m *ModuleKeyLog) TlsKeyLogWriter() io.Writer {
	return m.writer
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

	// open key log file
	if m.file, err = openKeyLogFile(conf.Log); err != nil {
		log4go.Warn("%s.Init(): open key log file failed, tls key log disabled: %s",
			m.name, err.Error())
	} else {
		m.writer = m.file
	}

	// register web handler for reload
	err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.loadConfData): %s", m.name, err.Error())
	}

	return nil
}

func openKeyLogFile(cfg access_log.LogConfig) (*os.File, error) {
	if cfg.LogFile != "" {
		return os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}

	// log with rotation config: write to LogDir/LogPrefix.log
	if err := os.MkdirAll(cfg.LogDir, 0777); err != nil {
		return nil, err
	}
	path := filepath.Join(cfg.LogDir, cfg.LogPrefix+".log")
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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

	if len(conf.Config) > 0 {
		log4go.Warn("%s: product based key log filtering is deprecated, "+
			"rules in %s are ignored; key log applies to all TLS connections",
			m.name, path)
	}

	// update to rule table (deprecated, kept for config compatibility)
	m.ruleTable.Update(conf)

	_, fileName := filepath.Split(path)
	return fmt.Sprintf("%s=%s", fileName, conf.Version), nil
}

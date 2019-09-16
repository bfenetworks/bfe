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

package mod_access

import (
	"bytes"
	"fmt"
)

import (
	"github.com/baidu/bfe/bfe_basic"
	"github.com/baidu/bfe/bfe_http"
	"github.com/baidu/bfe/bfe_module"
	"github.com/baidu/bfe/bfe_util/access_log"
)

import (
	"github.com/baidu/go-lib/log/log4go"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

type ModuleAccess struct {
	name   string
	logger log4go.Logger
	conf   *ConfModAccess

	reqFmts     []LogFmtItem // log formate meta data, parsed from LogTemplate
	sessionFmts []LogFmtItem // session finish log formate meta data, parsed from FinishTemplate
}

func NewModuleAccess() *ModuleAccess {
	m := new(ModuleAccess)
	m.name = "mod_access"
	return m
}

func (m *ModuleAccess) Name() string {
	return m.name
}

func (m *ModuleAccess) ParseConfig(conf *ConfModAccess) error {
	var err error

	// parse request finish template
	m.reqFmts, err = parseLogTemplate(conf.Template.RequestTemplate)
	if err != nil {
		return fmt.Errorf("%s.Init(): RequestTemplate %s", m.name, err.Error())
	}

	// parse session finish template
	m.sessionFmts, err = parseLogTemplate(conf.Template.SessionTemplate)
	if err != nil {
		return fmt.Errorf("%s.Init(): SessionTemplate %s", m.name, err.Error())
	}

	return nil
}

func (m *ModuleAccess) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error
	var conf *ConfModAccess

	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath); err != nil {
		return fmt.Errorf("%s: cond load err %s", m.name, err.Error())
	}

	return m.init(conf, cbs, whs)
}

func (m *ModuleAccess) init(conf *ConfModAccess, cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers) error {
	var err error

	// parse config
	if err = m.ParseConfig(conf); err != nil {
		return fmt.Errorf("%s.Init(): ParseConfig %s", m.name, err.Error())
	}

	// save conf
	m.conf = conf

	// check all log items in templates
	if err = m.CheckLogFormat(); err != nil {
		return fmt.Errorf("%s.Init(): CheckLogFormat %s", m.name, err.Error())
	}

	// init logger agent
	m.logger, err = access_log.LoggerInit(conf.Log.LogPrefix, conf.Log.LogDir,
		conf.Log.RotateWhen, conf.Log.BackupCount)
	if err != nil {
		return fmt.Errorf("%s.Init(): create logger", m.name)
	}

	// register handler
	// for finish request
	err = cbs.AddFilter(bfe_module.HANDLE_REQUEST_FINISH, m.requestFinish)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.requestFinish): %s", m.name, err.Error())
	}
	// for finish connection
	err = cbs.AddFilter(bfe_module.HANDLE_FINISH, m.sessionFinish)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.sessionFinish): %s", m.name, err.Error())
	}

	return nil
}

// Check log format template.
func (m *ModuleAccess) CheckLogFormat() error {
	for _, item := range m.reqFmts {
		err := checkLogFmt(item, Request)
		if err != nil {
			return err
		}
	}

	for _, item := range m.sessionFmts {
		err := checkLogFmt(item, Session)
		if err != nil {
			return err
		}
	}

	return nil
}

// Handler for finish http request.
func (m *ModuleAccess) requestFinish(req *bfe_basic.Request, res *bfe_http.Response) int {
	byteStr := bytes.NewBuffer(nil)

	// write log
	for _, item := range m.reqFmts {
		// literal string
		if item.Type == FormatString {
			byteStr.WriteString(item.Key)
		} else if item.Type == FormatTime {
			onLogFmtTime(m, byteStr)
		} else {
			handler, found := fmtHandlerTable[item.Type]
			if found {
				h := handler.(func(*ModuleAccess, *LogFmtItem, *bytes.Buffer,
					*bfe_basic.Request, *bfe_http.Response) error)
				h(m, &item, byteStr, req, res)
			}
		}
	}

	byteStr.WriteString("\n")
	m.logger.Info(byteStr.Bytes())

	return bfe_module.BFE_HANDLER_GOON
}

// Handler for finish http connection.
func (m *ModuleAccess) sessionFinish(session *bfe_basic.Session) int {
	byteStr := bytes.NewBuffer(nil)

	// write log
	for _, item := range m.sessionFmts {
		// literal string
		if item.Type == FormatString {
			byteStr.WriteString(item.Key)
		} else if item.Type == FormatTime {
			onLogFmtTime(m, byteStr)
		} else {
			handler, found := fmtHandlerTable[item.Type]
			if found {
				h := handler.(func(*ModuleAccess, *LogFmtItem, *bytes.Buffer,
					*bfe_basic.Session) error)
				h(m, &item, byteStr, session)
			}
		}
	}

	byteStr.WriteString("\n")
	m.logger.Info(byteStr.Bytes())

	return bfe_module.BFE_HANDLER_GOON
}

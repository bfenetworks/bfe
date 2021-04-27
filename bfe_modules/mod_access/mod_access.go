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

package mod_access

import (
	"bytes"
	"fmt"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_util/access_log"
)

import (
	"github.com/baidu/go-lib/log/log4go"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

type ModuleAccess struct {
	name   string
	logger log4go.Logger
	conf   *ConfModAccess

	reqFmts     []LogFmtItem
	sessionFmts []LogFmtItem
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

	m.reqFmts, err = parseLogTemplate(conf.Template.RequestTemplate)
	if err != nil {
		return fmt.Errorf("%s.Init(): RequestTemplate %s", m.name, err.Error())
	}

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
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: cond load err %s", m.name, err.Error())
	}

	return m.init(conf, cbs, whs)
}

func (m *ModuleAccess) init(conf *ConfModAccess, cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers) error {
	var err error

	if err = m.ParseConfig(conf); err != nil {
		return fmt.Errorf("%s.Init(): ParseConfig %s", m.name, err.Error())
	}

	m.conf = conf

	if err = m.CheckLogFormat(); err != nil {
		return fmt.Errorf("%s.Init(): CheckLogFormat %s", m.name, err.Error())
	}

	m.logger, err = access_log.LoggerInit(conf.Log)
	if err != nil {
		return fmt.Errorf("%s.Init(): create logger", m.name)
	}

	err = cbs.AddFilter(bfe_module.HandleRequestFinish, m.requestLogHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m. requestLogHandler): %s", m.name, err.Error())
	}

	err = cbs.AddFilter(bfe_module.HandleFinish, m.sessionLogHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.sessionLogHandler): %s", m.name, err.Error())
	}

	return nil
}

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

func (m *ModuleAccess) requestLogHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	byteStr := bytes.NewBuffer(nil)

	for _, item := range m.reqFmts {
		switch item.Type {
		case FormatString:
			byteStr.WriteString(item.Key)
		case FormatTime:
			onLogFmtTime(m, byteStr)
		default:
			handler, found := fmtHandlerTable[item.Type]
			if found {
				h := handler.(func(*ModuleAccess, *LogFmtItem, *bytes.Buffer,
					*bfe_basic.Request, *bfe_http.Response) error)
				h(m, &item, byteStr, req, res)
			}
		}
	}

	m.logger.Info(byteStr.String())

	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleAccess) sessionLogHandler(session *bfe_basic.Session) int {
	byteStr := bytes.NewBuffer(nil)

	for _, item := range m.sessionFmts {
		switch item.Type {
		case FormatString:
			byteStr.WriteString(item.Key)
		case FormatTime:
			onLogFmtTime(m, byteStr)
		default:
			handler, found := fmtHandlerTable[item.Type]
			if found {
				h := handler.(func(*ModuleAccess, *LogFmtItem, *bytes.Buffer,
					*bfe_basic.Session) error)
				h(m, &item, byteStr, session)
			}
		}
	}

	m.logger.Info(byteStr.String())

	return bfe_module.BfeHandlerGoOn
}

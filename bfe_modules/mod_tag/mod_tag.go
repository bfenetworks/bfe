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

package mod_tag

import (
	"fmt"
	"net/url"
	"path/filepath"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	ModTag = "mod_tag"
)

var (
	openDebug = false
)

type ModuleTag struct {
	name      string
	conf      *ConfModTag
	ruleTable *TagRuleTable
}

func NewModuleTag() *ModuleTag {
	m := new(ModuleTag)
	m.name = ModTag
	m.ruleTable = NewTagRuleTable()
	return m
}

func (m *ModuleTag) Name() string {
	return m.name
}

func (m *ModuleTag) loadRuleData(query url.Values) (string, error) {
	// get file path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.conf.Basic.DataPath
	}

	// load from config file
	conf, err := TagRuleFileLoad(path)
	if err != nil {
		return "", fmt.Errorf("%s: TagRuleFileLoad(%s) error: %v", m.name, path, err)
	}

	// update to rule table
	m.ruleTable.Update(conf)

	_, fileName := filepath.Split(path)
	return fmt.Sprintf("%s=%s", fileName, conf.Version), nil
}

func (m *ModuleTag) tagHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
	rules, ok := m.ruleTable.Search(request.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn, nil
	}

	for _, rule := range rules {
		if rule.Cond.Match(request) {
			if openDebug {
				log.Logger.Info("%s add tag: %s:%s", request.Route.Product, rule.Param.TagName, rule.Param.TagValue)
			}
			request.AddTags(rule.Param.TagName, []string{rule.Param.TagValue})
			if rule.Last {
				break
			}
		}
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleTag) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadRuleData,
	}
	return handlers
}

func (m *ModuleTag) init(conf *ConfModTag, cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers) error {
	var err error

	_, err = m.loadRuleData(nil)
	if err != nil {
		return err
	}

	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.tagHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.tagHandler): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.reloadHandlers): %v", m.name, err)
	}

	return nil
}

func (m *ModuleTag) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	var err error
	var conf *ConfModTag

	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}

	m.conf = conf
	openDebug = conf.Log.OpenDebug
	return m.init(conf, cbs, whs)
}

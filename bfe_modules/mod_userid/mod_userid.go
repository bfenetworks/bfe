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

// Package mod_userid generate user identity to trace one user in different request
// this mod will auto set user id for request if user id not exited in cookie to cookie
package mod_userid

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/url"
	"path/filepath"
	"sync"
	"time"
)

import (
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

var openDebug = false

const (
	ModName   = "mod_userid"
	UidCtxKey = "mod_userid.uid_cookie"
)

type ModuleUserID struct {
	name         string
	confFile     string
	config       *Config
	configLocker sync.RWMutex
}

func NewModuleUserID() *ModuleUserID {
	return &ModuleUserID{
		name: ModName,
	}
}

func (m *ModuleUserID) Name() string {
	return m.name
}

func (m *ModuleUserID) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	// load config
	cfg, err := ConfLoad(bfe_module.ModConfPath(cr, m.name), cr)
	if err != nil {
		return fmt.Errorf("%s: conf load err %v", m.name, err)
	}

	openDebug = cfg.Log.OpenDebug
	m.confFile = cfg.Basic.DataPath
	if _, err := m.loadConfData(nil); err != nil {
		return fmt.Errorf("%s: conf load err %v", m.name, err)
	}

	// register handlers
	if err := whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData); err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.loadConfData): %s", m.name, err.Error())
	}

	if err := cbs.AddFilter(bfe_module.HandleFoundProduct, m.reqSetUid); err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.reqSetUid): %s", m.name, err.Error())
	}

	if err := cbs.AddFilter(bfe_module.HandleReadResponse, m.rspSetUid); err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.rspSetUid): %s", m.name, err.Error())
	}

	return nil
}

func (m *ModuleUserID) loadConfData(query url.Values) (string, error) {
	path := m.confFile
	if q := query.Get("path"); q != "" {
		path = q
	}

	config, err := NewConfigFromFile(path)
	if err != nil {
		return "", err
	}

	m.setConfig(config)

	_, fileName := filepath.Split(path)
	return fmt.Sprintf("%s=%s", fileName, config.Version), nil
}

func (m *ModuleUserID) setConfig(config *Config) {
	m.configLocker.Lock()
	m.config = config
	m.configLocker.Unlock()
}

func (m *ModuleUserID) getConfig() *Config {
	m.configLocker.RLock()
	config := m.config
	m.configLocker.RUnlock()

	return config
}

func (m *ModuleUserID) reqSetUid(request *bfe_basic.Request) (int, *bfe_http.Response) {
	conf := m.getConfig()
	if conf == nil {
		return bfe_module.BfeHandlerGoOn, nil
	}

	productRules := conf.FindProductRules(request.Route.Product)
	if len(productRules) == 0 {
		productRules = conf.FindProductRules(bfe_basic.GlobalProduct)
	}
	for _, rule := range productRules {
		if !rule.Cond.Match(request) {
			continue
		}

		params := rule.Params
		if _, ok := request.Cookie(params.Name); ok {
			return bfe_module.BfeHandlerGoOn, nil
		}

		cookie := &bfe_http.Cookie{
			Name:    params.Name,
			Value:   genUid(),
			Path:    params.Path,
			Domain:  params.Domain,
			Expires: time.Now().Add(params.MaxAge),
			MaxAge:  int(params.MaxAge.Seconds()),
		}
		request.CookieMap[cookie.Name] = cookie
		request.HttpRequest.AddCookie(cookie)
		request.SetContext(UidCtxKey, cookie)
		break
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func genUid() string {
	id := fmt.Sprintf("%d_%d", time.Now().UnixNano(), rand.Intn(1<<31-1))
	return hex.EncodeToString([]byte(id))
}

func (m *ModuleUserID) rspSetUid(request *bfe_basic.Request, res *bfe_http.Response) int {
	data := request.GetContext(UidCtxKey)
	if data == nil {
		return bfe_module.BfeHandlerGoOn
	}

	uidCookie, ok := data.(*bfe_http.Cookie)
	if !ok {
		return bfe_module.BfeHandlerGoOn
	}

	res.Header.Add("Set-Cookie", uidCookie.String())
	return bfe_module.BfeHandlerGoOn
}

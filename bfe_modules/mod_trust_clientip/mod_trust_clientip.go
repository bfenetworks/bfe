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

// module for marking trust-client-ip in session

package mod_trust_clientip

import (
	"bytes"
	"fmt"
	"net/url"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_util/ipdict"
)

const (
	ModTrustClientIP = "mod_trust_clientip"
)

var (
	openDebug = false
)

type ModuleTrustClientIPState struct {
	ConnTotal                *metrics.Counter // all connnetion checked
	ConnTrustClientip        *metrics.Counter // connection from trust addr
	ConnAddrInternal         *metrics.Counter // connection from internal
	ConnAddrInternalNotTrust *metrics.Counter // connection from internal and not trust
}

type ModuleTrustClientIP struct {
	name       string                   // name of module
	configPath string                   // path of config file
	state      ModuleTrustClientIPState // module state
	metrics    metrics.Metrics          // diff counter of module state
	trustTable *ipdict.IPTable          // table for storing trust-ip
}

func NewModuleTrustClientIP() *ModuleTrustClientIP {
	m := new(ModuleTrustClientIP)
	m.name = ModTrustClientIP
	m.metrics.Init(&m.state, ModTrustClientIP, 0)

	return m
}

func (m *ModuleTrustClientIP) Name() string {
	return m.name
}

func ipItemsMake(conf TrustIPConf) (*ipdict.IPItems, error) {
	// calculate singleIPNum and pairIPNum
	singleIPNum, pairIPNum := 0, 0
	for _, addrScopeList := range conf.Config {
		for _, AddrScope := range *addrScopeList {
			// Insert start & end ip into ipItems
			ret := bytes.Compare(AddrScope.Begin, AddrScope.End)
			if ret == 0 {
				// startip == endip
				singleIPNum += 1
			} else {
				// startip != endip
				pairIPNum += 1
			}
		}
	}

	// create ipItems
	ipItems, err := ipdict.NewIPItems(singleIPNum, pairIPNum)
	if err != nil {
		return nil, err
	}

	// insert
	for src, addrScopeList := range conf.Config {
		for index, AddrScope := range *addrScopeList {
			// Insert start & end ip into ipItems
			ret := bytes.Compare(AddrScope.Begin, AddrScope.End)
			if ret == 0 {
				// startip == endip
				err = ipItems.InsertSingle(AddrScope.Begin)
			} else {
				// startip != endip
				err = ipItems.InsertPair(AddrScope.Begin, AddrScope.End)
			}

			if err != nil {
				return nil, fmt.Errorf("ipItemsMake():[%s, %d], err:[%s]", src, index, err.Error())
			}
		}
	}

	// Load succ, sort dict
	ipItems.Sort()
	ipItems.Version = conf.Version

	return ipItems, nil
}

func (m *ModuleTrustClientIP) loadConfData(query url.Values) error {
	// get file path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.configPath
	}

	// load from config file
	conf, err := TrustIPConfLoad(path)
	if err != nil {
		return fmt.Errorf("err in TrustIPConfLoad(%s):%s", path, err.Error())
	}

	items, err := ipItemsMake(conf)
	if err != nil {
		return fmt.Errorf("err in ipItemsMake():%s", err.Error())
	}

	// update to trust-table
	m.trustTable.Update(items)

	return nil
}

func (m *ModuleTrustClientIP) acceptHandler(session *bfe_basic.Session) int {
	m.state.ConnTotal.Inc(1)

	trusted := m.trustTable.Search(session.RemoteAddr.IP)
	if trusted {
		m.state.ConnTrustClientip.Inc(1)
	}
	session.SetTrustSource(trusted)

	// state for internal remote ip
	if session.RemoteAddr.IP.IsPrivate() {
		m.state.ConnAddrInternal.Inc(1)
		if !trusted {
			m.state.ConnAddrInternalNotTrust.Inc(1)
		}
	}

	if openDebug {
		log.Logger.Debug("mod_trust_clientip:src ip = %s, trusted = %t",
			session.RemoteAddr.IP, session.TrustSource())
	}

	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleTrustClientIP) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleTrustClientIP) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleTrustClientIP) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

func (m *ModuleTrustClientIP) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error
	var conf *ConfModTrustClientIP

	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}

	return m.init(conf, cbs, whs)
}

func (m *ModuleTrustClientIP) init(cfg *ConfModTrustClientIP, cbs *bfe_module.BfeCallbacks,
	whs *web_monitor.WebHandlers) error {
	m.configPath = cfg.Basic.DataPath

	// set debug switch
	openDebug = cfg.Log.OpenDebug

	// initialize trust-table
	m.trustTable = ipdict.NewIPTable()

	// load from config file to trust-table
	if err := m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %s", err.Error())
	}

	// register handler
	// for accept
	err := cbs.AddFilter(bfe_module.HandleAccept, m.acceptHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.acceptHandler): %s", m.name, err.Error())
	}

	// register web handler for monitor
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.monitorHandlers): %s", m.name, err.Error())
	}

	// register web handler for reload
	err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.loadConfData): %s", m.name, err.Error())
	}

	return nil
}

// Copyright (c) 2021 The BFE Authors.
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

package mod_tcp_keepalive

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"reflect"

	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_util"
)

const (
	ModTcpKeepAlive = "mod_tcp_keepalive"
)

var (
	openDebug = false
)

type ModuleTcpKeepAliveState struct {
	ConnToSet                 *metrics.Counter // connection hit rule, to set or disable keeplaive
	ConnSetKeepIdle           *metrics.Counter // connection set keepalive idle
	ConnSetKeepIdleError      *metrics.Counter // connection set keepalive idle error
	ConnSetKeepIntvl          *metrics.Counter // connection set keepalive interval
	ConnSetKeepIntvlError     *metrics.Counter // connection set keepalive interval error
	ConnSetKeepCnt            *metrics.Counter // connection set keepalive retry count
	ConnSetKeepCntError       *metrics.Counter // connection set keepalive retry count error
	ConnDisableKeepAlive      *metrics.Counter // connection disable keepalive message
	ConnDisableKeepAliveError *metrics.Counter // connection disable keepalive error
	ConnConvertToTcpConnError *metrics.Counter // connection convert to TCPConn error
}

type ModuleTcpKeepAlive struct {
	name string // name of module

	state   ModuleTcpKeepAliveState // module state
	metrics metrics.Metrics         // module metrics

	dataPath  string          // path of module data file
	ruleTable *KeepAliveTable // table of keepalive rules
}

func NewModuleTcpKeepAlive() *ModuleTcpKeepAlive {
	m := new(ModuleTcpKeepAlive)
	m.name = ModTcpKeepAlive
	m.metrics.Init(&m.state, ModTcpKeepAlive, 0)
	m.ruleTable = NewKeepAliveTable()

	return m
}

func (m *ModuleTcpKeepAlive) Name() string {
	return m.name
}

func (m *ModuleTcpKeepAlive) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	var conf *ConfModTcpKeepAlive
	var err error

	// load module config
	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}

	m.dataPath = conf.Basic.DataPath
	openDebug = conf.Log.OpenDebug

	// load conf data
	if err = m.loadConfData(nil); err != nil {
		return fmt.Errorf("%s: loadConfData() err %s", m.name, err.Error())
	}

	// register handler
	err = cbs.AddFilter(bfe_module.HandleAccept, m.HandleAccept)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.HandleAccept): %s", m.name, err.Error())
	}

	// register web handler for monitor
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.monitorHandlers): %s", m.name, err.Error())
	}

	// register web handler for reload
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.reloadHandlers): %s", m.name, err.Error())
	}

	return nil
}

func (m *ModuleTcpKeepAlive) loadConfData(query url.Values) error {
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.dataPath
	}

	// load file
	data, err := KeepAliveDataLoad(path)
	if err != nil {
		return fmt.Errorf("err in KeepAliveDataLoad(%s): %s", path, err.Error())
	}

	m.ruleTable.Update(data)

	return nil
}

func (m *ModuleTcpKeepAlive) disableKeepAlive(tcpConn *net.TCPConn) error {
	err := tcpConn.SetKeepAlive(false)
	if err != nil {
		m.state.ConnDisableKeepAliveError.Inc(1)
		return fmt.Errorf("SetKeepAlive(false) failed: %v", err)
	}

	m.state.ConnDisableKeepAlive.Inc(1)
	return nil
}

func (m *ModuleTcpKeepAlive) setKeepAliveParam(conn *net.TCPConn, p KeepAliveParam) error {
	var err error
	var f *os.File

	if f, err = conn.File(); err != nil {
		return fmt.Errorf("get conn.File() error: %v", err)
	}
	defer f.Close()
	fd := int(f.Fd())

	if p.KeepIdle > 0 {
		err = setIdle(fd, p.KeepIdle)
		if err != nil {
			m.state.ConnSetKeepIdleError.Inc(1)
			return fmt.Errorf("set tcp keepIdle error: %v", err)
		}
		m.state.ConnSetKeepIdle.Inc(1)
	}

	if p.KeepIntvl > 0 {
		err = setInterval(fd, p.KeepIntvl)
		if err != nil {
			m.state.ConnSetKeepIntvlError.Inc(1)
			return fmt.Errorf("set tcp keepIntvl error: %v", err)
		}
		m.state.ConnSetKeepIntvl.Inc(1)
	}

	if p.KeepCnt > 0 {
		err = setCount(fd, p.KeepCnt)
		if err != nil {
			m.state.ConnSetKeepCntError.Inc(1)
			return fmt.Errorf("set tcp KeepCnt error: %v", err)
		}
		m.state.ConnSetKeepCnt.Inc(1)
	}

	err = setNonblock(fd)
	if err != nil {
		return fmt.Errorf("setNonblock error: %v", err)
	}

	return err
}

func (m *ModuleTcpKeepAlive) handleTcpKeepAlive(conn *net.TCPConn, p KeepAliveParam) error {
	if p.Disable {
		return m.disableKeepAlive(conn)
	}

	return m.setKeepAliveParam(conn, p)
}

func (m *ModuleTcpKeepAlive) getTcpConn(conn net.Conn) (*net.TCPConn, error) {
	if c, ok := conn.(bfe_util.ConnFetcher); ok {
		conn = c.GetNetConn()
		return m.getTcpConn(conn)
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		m.state.ConnConvertToTcpConnError.Inc(1)
		if openDebug {
			log.Logger.Debug("conn type[%s] connvert to TCPConn failed", reflect.TypeOf(conn))
		}

		return nil, fmt.Errorf("conn connvert to TCPConn failed")
	}

	return tcpConn, nil
}

func (m *ModuleTcpKeepAlive) HandleAccept(session *bfe_basic.Session) int {
	vip := session.Vip.String()
	if openDebug {
		log.Logger.Debug("mod[%s] get connection, remote: %v, vip: %s", m.name, session.RemoteAddr, vip)
	}

	rules, ok := m.ruleTable.Search(session.Product)
	if !ok {
		if openDebug {
			log.Logger.Debug("mod[%s] product[%s] not found, just pass", m.name, session.Product)
		}
		return bfe_module.BfeHandlerGoOn
	}

	if param, ok := rules[vip]; ok {
		m.state.ConnToSet.Inc(1)
		conn, err := m.getTcpConn(session.Connection)
		if err != nil {
			log.Logger.Error("mod[%s] vip[%s] getTcpConn error: %v", err)
			return bfe_module.BfeHandlerGoOn
		}

		err = m.handleTcpKeepAlive(conn, param)
		if err != nil {
			log.Logger.Error("mod[%s] vip[%s] remote[%v] handleTcpKeepAlive error: %v", m.name, vip, session.RemoteAddr, err)
		}

		if openDebug {
			log.Logger.Debug("mod[%s] vip[%s] product[%s] found, set keepalive success, param[%+v]", m.name, vip, session.Product, param)
		}

		return bfe_module.BfeHandlerGoOn
	}

	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleTcpKeepAlive) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleTcpKeepAlive) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleTcpKeepAlive) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}

	return handlers
}

func (m *ModuleTcpKeepAlive) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadConfData,
	}

	return handlers
}

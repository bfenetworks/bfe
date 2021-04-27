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

// embedded web server in bfe

package bfe_server

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

type BfeMonitor struct {
	WebServer   *web_monitor.MonitorServer
	WebHandlers *web_monitor.WebHandlers
	srv         *BfeServer
}

func newBfeMonitor(srv *BfeServer, monitorPort int) (*BfeMonitor, error) {
	m := &BfeMonitor{nil, nil, srv}

	// initialize web handlers
	m.WebHandlers = web_monitor.NewWebHandlers()
	if err := m.WebHandlersInit(m.srv); err != nil {
		log.Logger.Error("newBfeMonitor(): in WebHandlersInit(): ", err.Error())
		return nil, err
	}

	// initialize web server
	m.WebServer = web_monitor.NewMonitorServer("bfe", srv.Version, monitorPort)
	m.WebServer.HandlersSet(m.WebHandlers)

	return m, nil
}

// monitorHandlers holds all monitor handlers.
func (m *BfeMonitor) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		// for host-table
		"host_table_status":  m.srv.HostTableStatusGet,
		"host_table_version": m.srv.HostTableVersionGet,

		// for cluster-table: only contain cluster_conf version
		"cluster_table_version": m.srv.ClusterTableVersionGet,

		// for bal-table
		"bal_table_status":  m.srv.BalTableStatusGet,
		"bal_table_version": m.srv.BalTableVersionGet,

		// for proxy_state
		"proxy_state":      m.srv.proxyStateGetAll,
		"proxy_state_diff": m.srv.proxyStateGetDiff,

		// for balance
		"bal_state":      m.srv.balStateGetAll,
		"bal_state_diff": m.srv.balStateGetDiff,

		// for proxy protocol
		"proxy_protocol_state":      m.srv.proxyProtocolStateGetAll,
		"proxy_protocol_state_diff": m.srv.proxyProtocolStateGetDiff,

		// for tls
		"tls_state":      m.srv.tlsStateGetAll,
		"tls_state_diff": m.srv.tlsStateGetDiff,

		// for spdy
		"spdy_state":      m.srv.spdyStateGetAll,
		"spdy_state_diff": m.srv.spdyStateGetDiff,

		// for http2
		"http2_state":      m.srv.http2StateGetAll,
		"http2_state_diff": m.srv.http2StateGetDiff,

		// for http
		"http_state":      m.srv.httpStateGetAll,
		"http_state_diff": m.srv.httpStateGetDiff,

		// for stream
		"stream_state":      m.srv.streamStateGetAll,
		"stream_state_diff": m.srv.streamStateGetDiff,

		// for websocket
		"websocket_state":      m.srv.websocketStateGetAll,
		"websocket_state_diff": m.srv.websocketStateGetDiff,

		// for proxy delay
		"proxy_delay":      m.srv.proxyDelayGet,
		"proxy_post_delay": m.srv.proxyPostDelayGet,

		// for handshake dely
		"proxy_handshake_delay":        m.srv.proxyHandshakeDelayGet,
		"proxy_handshake_full_delay":   m.srv.proxyHandshakeFullDelayGet,
		"proxy_handshake_resume_delay": m.srv.proxyHandshakeResumeDelayGet,

		// for module status
		"module_status":   m.srv.ModuleStatusGetJSON,
		"module_handlers": m.srv.ModuleHandlersGetJSON,

		// for proxy memory stat
		"proxy_mem_stat": web_monitor.CreateMemStatsHandler("proxy_mem_stat"),
	}
	return handlers
}

// reloadHandlers holds all reload handlers.
func (m *BfeMonitor) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		// for server data conf
		"server_data_conf": m.srv.ServerDataConfReload,

		// for gslb data conf
		"gslb_data_conf": m.srv.GslbDataConfReload,

		// for name conf
		"name_conf": m.srv.NameConfReload,

		// for tls
		"tls_conf":               m.srv.TLSConfReload,
		"tls_session_ticket_key": m.srv.SessionTicketKeyReload,
	}
	return handlers
}

func (m *BfeMonitor) WebHandlersInit(srv *BfeServer) error {
	// register handlers for monitor
	err := web_monitor.RegisterHandlers(m.WebHandlers, web_monitor.WebHandleMonitor,
		m.monitorHandlers())
	if err != nil {
		return err
	}

	// register handlers for for reload
	err = web_monitor.RegisterHandlers(m.WebHandlers, web_monitor.WebHandleReload,
		m.reloadHandlers())
	if err != nil {
		return err
	}

	return nil
}

func (m *BfeMonitor) Start() {
	go m.WebServer.Start()
}

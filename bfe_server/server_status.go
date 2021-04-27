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

// server internal status

package bfe_server

import (
	"github.com/baidu/go-lib/web-monitor/delay_counter"
	"github.com/baidu/go-lib/web-monitor/metrics"
)

import (
	bal "github.com/bfenetworks/bfe/bfe_balance/bal_gslb"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_http2"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_proxy"
	"github.com/bfenetworks/bfe/bfe_spdy"
	"github.com/bfenetworks/bfe/bfe_stream"
	"github.com/bfenetworks/bfe/bfe_tls"
	"github.com/bfenetworks/bfe/bfe_websocket"
)

// setting for delay
const (
	DC_INTERVAL          = 60  // interval for moving current to past (in s)
	DC_BUCKET_SIZE_PROXY = 1   // size of delay counter bucket for forwarding (in ms)
	DC_BUCKET_SIZE_HS    = 100 // size of delay counter bucket for handshake (in ms)
	DC_BUCKET_NUM        = 10  // number of delay counter bucket
)

// key prefix
const (
	KP_PROXY_DELAY                  = "proxy_delay"
	KP_PROXY_POST_DEALY             = "proxy_post_delay"
	KP_PROXY_HANDSHAKE_DELAY        = "proxy_handshake_delay"
	KP_PROXY_HANDSHAKE_FULL_DELAY   = "proxy_handshake_full_delay"
	KP_PROXY_HANDSHAKE_RESUME_DELAY = "proxy_handshake_resume_delay"
	KP_PROXY_STATE                  = "proxy_state"
)

type ServerStatus struct {
	// for proxy protocol
	ProxyProtocolState   *bfe_proxy.ProxyState
	ProxyProtocolMetrics metrics.Metrics

	// for tls protocol
	TlsState   *bfe_tls.TlsState
	TlsMetrics metrics.Metrics

	// for spdy protocol
	SpdyState   *bfe_spdy.SpdyState
	SpdyMetrics metrics.Metrics

	// for http2 protocol
	Http2State   *bfe_http2.Http2State
	Http2Metrics metrics.Metrics

	// for http protocol
	HttpState   *bfe_http.HttpState
	HttpMetrics metrics.Metrics

	// for stream protocol (tls proxy)
	StreamState   *bfe_stream.StreamState
	StreamMetrics metrics.Metrics

	// for websocket protocol (websocket proxy)
	WebSocketState   *bfe_websocket.WebSocketState
	WebSocketMetrics metrics.Metrics

	// for balance
	BalState   *bal.BalErrState
	BalMetrics metrics.Metrics

	// for proxy
	ProxyState   *ProxyState
	ProxyMetrics metrics.Metrics

	// for monitor "internal delay"
	ProxyDelay *delay_counter.DelayRecent

	// for monitor "internal delay" of post/put request
	ProxyPostDelay *delay_counter.DelayRecent

	// for monitor "internal delay" of tls handshake
	ProxyHandshakeDelay       *delay_counter.DelayRecent
	ProxyHandshakeFullDelay   *delay_counter.DelayRecent
	ProxyHandshakeResumeDelay *delay_counter.DelayRecent
}

func NewServerStatus() *ServerStatus {
	m := new(ServerStatus)

	// initialize counter state
	m.ProxyProtocolState = bfe_proxy.GetProxyState()
	m.TlsState = bfe_tls.GetTlsState()
	m.SpdyState = bfe_spdy.GetSpdyState()
	m.Http2State = bfe_http2.GetHttp2State()
	m.HttpState = bfe_http.GetHttpState()
	m.StreamState = bfe_stream.GetStreamState()
	m.WebSocketState = bfe_websocket.GetWebSocketState()
	m.ProxyState = new(ProxyState)
	m.BalState = bal.GetBalErrState()

	// initialize metrics
	m.ProxyProtocolMetrics.Init(m.ProxyProtocolState, KP_PROXY_STATE, 0)
	m.TlsMetrics.Init(m.TlsState, KP_PROXY_STATE, 0)
	m.SpdyMetrics.Init(m.SpdyState, KP_PROXY_STATE, 0)
	m.Http2Metrics.Init(m.Http2State, KP_PROXY_STATE, 0)
	m.HttpMetrics.Init(m.HttpState, KP_PROXY_STATE, 0)
	m.StreamMetrics.Init(m.StreamState, KP_PROXY_STATE, 0)
	m.WebSocketMetrics.Init(m.WebSocketState, KP_PROXY_STATE, 0)
	m.ProxyMetrics.Init(m.ProxyState, KP_PROXY_STATE, 0)
	m.BalMetrics.Init(m.BalState, KP_PROXY_STATE, 0)

	// initialize delay counter
	m.ProxyDelay = new(delay_counter.DelayRecent)
	m.ProxyPostDelay = new(delay_counter.DelayRecent)
	m.ProxyHandshakeDelay = new(delay_counter.DelayRecent)
	m.ProxyHandshakeFullDelay = new(delay_counter.DelayRecent)
	m.ProxyHandshakeResumeDelay = new(delay_counter.DelayRecent)

	m.ProxyDelay.Init(DC_INTERVAL, DC_BUCKET_SIZE_PROXY, DC_BUCKET_NUM)
	m.ProxyPostDelay.Init(DC_INTERVAL, DC_BUCKET_SIZE_PROXY, DC_BUCKET_NUM)
	m.ProxyHandshakeDelay.Init(DC_INTERVAL, DC_BUCKET_SIZE_HS, DC_BUCKET_NUM)
	m.ProxyHandshakeFullDelay.Init(DC_INTERVAL, DC_BUCKET_SIZE_HS, DC_BUCKET_NUM)
	m.ProxyHandshakeResumeDelay.Init(DC_INTERVAL, DC_BUCKET_SIZE_HS, DC_BUCKET_NUM)

	m.ProxyDelay.SetKeyPrefix(KP_PROXY_DELAY)
	m.ProxyPostDelay.SetKeyPrefix(KP_PROXY_POST_DEALY)
	m.ProxyHandshakeDelay.SetKeyPrefix(KP_PROXY_HANDSHAKE_DELAY)
	m.ProxyHandshakeFullDelay.SetKeyPrefix(KP_PROXY_HANDSHAKE_FULL_DELAY)
	m.ProxyHandshakeResumeDelay.SetKeyPrefix(KP_PROXY_HANDSHAKE_RESUME_DELAY)

	return m
}

func (srv *BfeServer) proxyProtocolStateGetAll(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.ProxyProtocolMetrics.GetAll()
	return s.Format(params)
}

func (srv *BfeServer) proxyProtocolStateGetDiff(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.ProxyProtocolMetrics.GetDiff()
	return s.Format(params)
}

func (srv *BfeServer) tlsStateGetAll(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.TlsMetrics.GetAll()
	return s.Format(params)
}

func (srv *BfeServer) tlsStateGetDiff(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.TlsMetrics.GetDiff()
	return s.Format(params)
}

func (srv *BfeServer) spdyStateGetAll(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.SpdyMetrics.GetAll()
	return s.Format(params)
}

func (srv *BfeServer) spdyStateGetDiff(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.SpdyMetrics.GetDiff()
	return s.Format(params)
}

func (srv *BfeServer) http2StateGetAll(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.Http2Metrics.GetAll()
	return s.Format(params)
}

func (srv *BfeServer) http2StateGetDiff(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.Http2Metrics.GetDiff()
	return s.Format(params)
}

func (srv *BfeServer) httpStateGetAll(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.HttpMetrics.GetAll()
	return s.Format(params)
}

func (srv *BfeServer) httpStateGetDiff(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.HttpMetrics.GetDiff()
	return s.Format(params)
}

func (srv *BfeServer) streamStateGetAll(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.StreamMetrics.GetAll()
	return s.Format(params)
}

func (srv *BfeServer) streamStateGetDiff(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.StreamMetrics.GetDiff()
	return s.Format(params)
}

func (srv *BfeServer) websocketStateGetAll(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.WebSocketMetrics.GetAll()
	return s.Format(params)
}

func (srv *BfeServer) websocketStateGetDiff(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.WebSocketMetrics.GetDiff()
	return s.Format(params)
}

func (srv *BfeServer) proxyStateGetAll(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.ProxyMetrics.GetAll()
	return s.Format(params)
}

func (srv *BfeServer) proxyStateGetDiff(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.ProxyMetrics.GetDiff()
	return s.Format(params)
}

func (srv *BfeServer) balStateGetAll(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.BalMetrics.GetAll()
	return s.Format(params)
}

func (srv *BfeServer) balStateGetDiff(params map[string][]string) ([]byte, error) {
	s := srv.serverStatus.BalMetrics.GetDiff()
	return s.Format(params)
}

func (srv *BfeServer) proxyDelayGet(params map[string][]string) ([]byte, error) {
	d := srv.serverStatus.ProxyDelay
	return d.FormatOutput(params)
}

func (srv *BfeServer) proxyPostDelayGet(params map[string][]string) ([]byte, error) {
	d := srv.serverStatus.ProxyPostDelay
	return d.FormatOutput(params)
}

func (srv *BfeServer) proxyHandshakeDelayGet(params map[string][]string) ([]byte, error) {
	d := srv.serverStatus.ProxyHandshakeDelay
	return d.FormatOutput(params)
}

func (srv *BfeServer) proxyHandshakeFullDelayGet(params map[string][]string) ([]byte, error) {
	d := srv.serverStatus.ProxyHandshakeFullDelay
	return d.FormatOutput(params)
}

func (srv *BfeServer) proxyHandshakeResumeDelayGet(params map[string][]string) ([]byte, error) {
	d := srv.serverStatus.ProxyHandshakeResumeDelay
	return d.FormatOutput(params)
}

func (srv *BfeServer) ModuleStatusGetJSON() ([]byte, error) {
	return bfe_module.ModuleStatusGetJSON()
}

func (srv *BfeServer) ModuleHandlersGetJSON() ([]byte, error) {
	return srv.CallBacks.ModuleHandlersGetJSON()
}

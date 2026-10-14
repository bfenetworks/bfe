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

// the main structure of bfe-server

package bfe_server

import (
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/bfenetworks/bfe/bfe_balance"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_tls_conf/session_ticket_key_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_tls_conf/tls_rule_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_http2"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_route"
	"github.com/bfenetworks/bfe/bfe_spdy"
	"github.com/bfenetworks/bfe/bfe_stream"
	"github.com/bfenetworks/bfe/bfe_util"
	"github.com/bfenetworks/bfe/bfe_util/signal_table"
	"github.com/bfenetworks/bfe/bfe_websocket"
	"github.com/bfenetworks/go-lib/log"
)

type BfeServer struct {
	bfe_http.Server

	// service listeners
	listenerMap   map[string]net.Listener // all listeners
	HttpListener  net.Listener            // listener for http
	HttpsListener *HttpsListener          // listener for https

	connWaitGroup sync.WaitGroup // waits for server conns to finish

	// for http server
	ReverseProxy *ReverseProxy // reverse proxy

	// TLS certificates
	MultiCert *MultiCertMap

	// TLS server rule
	TLSServerRule *TLSServerRuleMap

	// server config
	Config   bfe_conf.BfeConfig
	ConfRoot string

	// module and callback
	CallBacks *bfe_module.BfeCallbacks // call back functions
	Modules   *bfe_module.BfeModules   // bfe modules

	// web server for bfe monitor and reload
	Monitor *BfeMonitor

	// bufio cache
	BufioCache *BufioCache

	// signal table
	SignalTable *signal_table.SignalTable

	// server status
	serverStatus *ServerStatus

	confLock   sync.RWMutex              // mutex when reload data conf
	ServerConf *bfe_route.ServerDataConf // cluster_conf and host table conf
	balTable   *bfe_balance.BalTable     // for balance

	Version string // version of bfe server
}

// NewBfeServer create a new instance of BfeServer.
func NewBfeServer(cfg bfe_conf.BfeConfig, confRoot string,
	version string) *BfeServer {

	s := new(BfeServer)

	// bfe config
	s.Config = cfg
	s.ConfRoot = confRoot
	s.InitConfig()

	// initialize counters, proxyState
	s.serverStatus = NewServerStatus()

	// initialize bufioCache
	s.BufioCache = NewBufioCache()

	// create reverse proxy
	s.ReverseProxy = NewReverseProxy(s, s.serverStatus.ProxyState)

	// initialize callbacks
	s.CallBacks = bfe_module.NewBfeCallbacks()
	// create modules
	s.Modules = bfe_module.NewBfeModules()

	// initialize balTable
	s.balTable = bfe_balance.NewBalTable(s.GetCheckConf)

	// set keep-alive
	s.SetKeepAlivesEnabled(cfg.Server.KeepAliveEnabled)

	s.CloseNotifyCh = make(chan bool)

	s.Version = version

	return s
}

// InitConfig set some parameter based on config.
func (srv *BfeServer) InitConfig() {
	// set service port, according to config
	srv.Addr = fmt.Sprintf(":%d", srv.Config.Server.HttpPort)

	// set TlsHandshakeTimeout
	if srv.Config.Server.TlsHandshakeTimeout != 0 {
		srv.TlsHandshakeTimeout = time.Duration(srv.Config.Server.TlsHandshakeTimeout) * time.Second
	}

	// set ReadTimeout
	if srv.Config.Server.ClientReadTimeout != 0 {
		srv.ReadTimeout = time.Duration(srv.Config.Server.ClientReadTimeout) * time.Second
	}

	// set MaxHeaderBytes
	if srv.Config.Server.MaxHeaderBytes != 0 {
		srv.MaxHeaderBytes = srv.Config.Server.MaxHeaderBytes
	} else {
		srv.MaxHeaderBytes = bfe_http.DefaultMaxHeaderBytes
	}

	// set GracefulShutdownTimeout
	srv.GracefulShutdownTimeout = time.Duration(srv.Config.Server.GracefulShutdownTimeout) * time.Second

	// set MaxHeaderUriBytes
	if srv.Config.Server.MaxHeaderUriBytes != 0 {
		srv.MaxHeaderUriBytes = srv.Config.Server.MaxHeaderUriBytes
	} else {
		srv.MaxHeaderUriBytes = bfe_http.DefaultMaxHeaderUriBytes
	}

	// set AccessibleBodySize
	bfe_http.SetAccessibleBodySize(srv.Config.Server.AccessibleBodySize)

	// set TotalBodyBufferSize limit
	bfe_http.SetTotalBodyBufferSizeLimit(srv.Config.Server.TotalBodyBufferSize)
}

func (srv *BfeServer) InitHttp() (err error) {
	// disable cookie value sanitize
	bfe_http.SetDisableSanitize(true)

	// initialize http next proto handlers
	httpNextProto := make(map[string]func(*bfe_http.Server, bfe_http.ResponseWriter, *bfe_http.Request))
	httpNextProto[bfe_websocket.WebSocket] = bfe_websocket.NewProtoHandler(&bfe_websocket.Server{
		BalanceHandler: srv.Balance})
	srv.HTTPNextProto = httpNextProto

	return nil
}

func (srv *BfeServer) InitHttps() (err error) {
	// initialize tls config
	if err := srv.initTLSConfig(); err != nil {
		return err
	}

	// init tls next proto handlers
	srv.initTLSNextProtoHandler()

	return nil
}

func (srv *BfeServer) initTLSConfig() (err error) {
	srv.TLSConfig = new(tls.Config)
	httpsConf := srv.Config.HttpsBasic

	// set max and min TLS version
	srv.TLSConfig.MaxVersion, srv.TLSConfig.MinVersion = bfe_conf.GetTlsVersion(&httpsConf)

	// initialize ciphersuites preference
	srv.TLSConfig.PreferServerCipherSuites = true
	cipherSuites, err := bfe_conf.GetCipherSuites(httpsConf.CipherSuites)
	if err != nil {
		return fmt.Errorf("in ServerCertConfLoad() :%s", err.Error())
	}
	srv.TLSConfig.CipherSuites = cipherSuites

	// initialize elliptic curves preference
	srv.TLSConfig.CurvePreferences, err = bfe_conf.GetCurvePreferences(httpsConf.CurvePreferences)
	if err != nil {
		return fmt.Errorf("in ServerCertConfLoad() :%s", err.Error())
	}

	// initialize session ticket
	if err = srv.initTLSSessionTicket(); err != nil {
		return err
	}

	// initialize tls rule
	if err = srv.initTLSRule(httpsConf); err != nil {
		return err
	}

	return nil
}

func (srv *BfeServer) initTLSSessionTicket() error {
	sessionTicketConf := srv.Config.SessionTicket

	// initialize session ticket key
	if !sessionTicketConf.SessionTicketsDisabled {
		srv.TLSConfig.SessionTicketsDisabled = false
		keyFile := sessionTicketConf.SessionTicketKeyFile
		keyConf, err := session_ticket_key_conf.SessionTicketKeyConfLoad(keyFile)
		if err != nil {
			return err
		}
		key, err := hex.DecodeString(keyConf.SessionTicketKey)
		if err != nil {
			return fmt.Errorf("wrong session ticket key %s (%s)", err, key)
		}

		copy(srv.TLSConfig.SessionTicketKey[:], key)
	} else {
		srv.TLSConfig.SessionTicketsDisabled = true
	}

	return nil
}

func (srv *BfeServer) initTLSRule(httpsConf bfe_conf.ConfigHttpsBasic) error {
	srv.MultiCert = NewMultiCertMap(srv.serverStatus.ProxyState)
	srv.TLSServerRule = NewTLSServerRuleMap(srv.serverStatus.ProxyState)
	if err := srv.tlsConfLoad(httpsConf.ServerCertConf, httpsConf.TlsRuleConf,
		httpsConf.ClientCABaseDir, httpsConf.ClientCRLBaseDir); err != nil {
		return err
	}

	cert := srv.MultiCert.GetDefault()
	if cert == nil {
		return fmt.Errorf("createTlsConfig get default Cert error")
	}

	// Note: the default certificate is used only when GetCertificate
	// (wired to MultiCert) returns nil, which should not happen since
	// MultiCert always falls back to the default certificate.
	srv.TLSConfig.Certificates = make([]tls.Certificate, 1)
	srv.TLSConfig.Certificates[0] = *cert

	// select server certificate by vip and sni
	srv.TLSConfig.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		vip := bfe_util.GetVip(hello.Conn)
		return srv.MultiCert.GetByVipSni(vip, hello.ServerName), nil
	}

	// apply tls server rule (next protos gray release, client auth, etc)
	srv.TLSConfig.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		vip := bfe_util.GetVip(hello.Conn)
		rule := srv.TLSServerRule.GetByVipSni(vip, hello.ServerName)

		// Clone the listener's current config (instead of srv.TLSConfig) so
		// that updates applied at the listener level, e.g. a rotated session
		// ticket key after /reload/tls_session_ticket_key, take effect on
		// new connections. srv.TLSConfig is only the startup-time base.
		var config *tls.Config
		if srv.HttpsListener != nil {
			config = srv.HttpsListener.GetConfigClone()
		} else {
			config = srv.TLSConfig.Clone()
		}

		// select next protos for current conn
		config.NextProtos = rule.NextProtosConf.Get(hello.Conn)

		// set client auth policy
		config.ClientAuth = rule.ClientAuth
		config.ClientCAs = rule.ClientCAs

		// check client cert revocation if CRL pool configured
		if rule.ClientCRLPool != nil {
			clientCRLPool := rule.ClientCRLPool
			config.VerifyConnection = func(cs tls.ConnectionState) error {
				for _, cert := range cs.PeerCertificates {
					if clientCRLPool.CheckCertRevoked(cert) {
						return fmt.Errorf("client certificate (serial %s) is revoked",
							cert.SerialNumber)
					}
				}
				return nil
			}
		}

		return config, nil
	}
	return nil
}

func (srv *BfeServer) initTLSNextProtoHandler() {
	// init next protocol handler
	tlsNextProto := make(map[string]func(*bfe_http.Server, *tls.Conn, bfe_http.Handler))
	tlsNextProto[tls_rule_conf.SPDY31] = bfe_spdy.NewProtoHandler(nil)
	tlsNextProto[tls_rule_conf.HTTP2] = bfe_http2.NewProtoHandler(nil)
	tlsNextProto[tls_rule_conf.STREAM] = bfe_stream.NewProtoHandler(&bfe_stream.Server{
		BalanceHandler: srv.Balance})
	srv.TLSNextProto = tlsNextProto

	// init params for http2
	bfe_http2.DisableConnHeaderCheck()
	bfe_http2.SetServerRule(srv.TLSServerRule)
	bfe_http2.EnableLargeConnRecvWindow()

	// init params for stream
	bfe_stream.SetServerRule(srv.TLSServerRule)
}

func (srv *BfeServer) InitModules() error {
	if err := srv.Modules.Init(srv.CallBacks, srv.Monitor.WebHandlers, srv.ConfRoot); err != nil {
		return err
	}

	// set tls key log writer from modules (eg. mod_key_log) that
	// implement bfe_module.TlsKeyLogProvider
	for _, module := range srv.Modules.GetModules() {
		if provider, ok := module.(bfe_module.TlsKeyLogProvider); ok {
			if w := provider.TlsKeyLogWriter(); w != nil {
				srv.TLSConfig.KeyLogWriter = w
				log.Logger.Info("InitModules(): set TLS KeyLogWriter from %s", module.Name())
			}
		}
	}

	return nil
}

func (srv *BfeServer) InitSignalTable() {
	/* create signal table */
	srv.SignalTable = signal_table.NewSignalTable()

	/* register signal handlers */
	srv.SignalTable.Register(syscall.SIGQUIT, srv.ShutdownHandler)
	srv.SignalTable.Register(syscall.SIGTERM, signal_table.TermHandler)
	srv.SignalTable.Register(syscall.SIGHUP, signal_table.IgnoreHandler)
	srv.SignalTable.Register(syscall.SIGILL, signal_table.IgnoreHandler)
	srv.SignalTable.Register(syscall.SIGTRAP, signal_table.IgnoreHandler)
	srv.SignalTable.Register(syscall.SIGABRT, signal_table.IgnoreHandler)

	/* start signal handler routine */
	srv.SignalTable.StartSignalHandle()
}

func (srv *BfeServer) InitWebMonitor(port int, addr string) error {
	var err error
	srv.Monitor, err = newBfeMonitor(srv, port, addr)
	return err
}

// ShutdownHandler is signal handler for QUIT
func (srv *BfeServer) ShutdownHandler(sig os.Signal) {
	shutdownTimeout := srv.Config.Server.GracefulShutdownTimeout
	log.Logger.Info("get signal %s, graceful shutdown in %ds", sig, shutdownTimeout)

	// notify that server is in graceful shutdown state
	close(srv.CloseNotifyCh)

	// close server listeners
	srv.closeListeners()

	// waits server conns to finish
	connFinCh := make(chan bool)
	go func() {
		srv.connWaitGroup.Wait()
		connFinCh <- true
	}()

	shutdownTimer := time.After(time.Duration(shutdownTimeout) * time.Second)

Loop:
	for {
		select {
		// waits server conns to finish
		case <-connFinCh:
			log.Logger.Info("graceful shutdown success.")
			break Loop

		// wait for shutdown timeout
		case <-shutdownTimer:
			log.Logger.Info("graceful shutdown timeout.")
			break Loop
		}
	}

	// shutdown server
	log.Logger.Close()
	os.Exit(0)
}

// CheckGracefulShutdown check whether the server is in graceful shutdown state.
func (srv *BfeServer) CheckGracefulShutdown() bool {
	select {
	case <-srv.CloseNotifyCh:
		return true
	default:
		return false
	}
}

func (srv *BfeServer) GetServerConf() *bfe_route.ServerDataConf {
	srv.confLock.RLock()
	sf := srv.ServerConf
	srv.confLock.RUnlock()

	return sf
}

// GetCheckConf implements CheckConfFetcher and return current
// health check configuration.
func (s *BfeServer) GetCheckConf(clusterName string) (*cluster_conf.BackendCheck, *cluster_conf.BackendHTTPS) {
	sf := s.GetServerConf()
	cluster, err := sf.ClusterTable.Lookup(clusterName)
	if err != nil {
		return nil, nil
	}
	return cluster.BackendCheckConf(), cluster.BackendHTTPSConf()
}

func (srv *BfeServer) InitListeners(config bfe_conf.BfeConfig) error {
	listenerMap := make(map[string]net.Listener)
	lnConf := map[string]struct {
		port int
		addr string
	}{
		"HTTP":  {config.Server.HttpPort, config.Server.HttpAddr},
		"HTTPS": {config.Server.HttpsPort, config.Server.HttpsAddr},
	}

	for proto, c := range lnConf {
		addr := c.addr
		if addr == "" {
			addr = ""
		}
		listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", addr, c.port))
		if err != nil {
			return err
		}

		// wrap underlying listener according to balancer type
		listener = NewBfeListener(listener, config)
		listenerMap[proto] = listener
		log.Logger.Info("InitListeners(): begin to listen [%s:%d]", addr, c.port)
	}

	srv.listenerMap = listenerMap
	srv.HttpListener = listenerMap["HTTP"]
	srv.HttpsListener = NewHttpsListener(srv.listenerMap["HTTPS"], srv.TLSConfig)

	return nil
}

func (srv *BfeServer) closeListeners() {
	for _, ln := range srv.listenerMap {
		if err := ln.Close(); err != nil {
			log.Logger.Error("closeListeners(): %s, %s", err, ln.Addr())
		}
	}
}

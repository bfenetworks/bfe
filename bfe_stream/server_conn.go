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

package bfe_stream

import (
	"fmt"
	"io"
	"net"
	"time"
)

import (
	"github.com/baidu/go-lib/gotrack"
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_balance/backend"
	http "github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_proxy"
	tls "github.com/bfenetworks/bfe/bfe_tls"
	"github.com/bfenetworks/bfe/bfe_util"
)

type serverConn struct {
	// Immutable:
	srv           *Server              // server config for tls proxy
	hs            *http.Server         // server config for http
	conn          net.Conn             // underlying conn
	tlsState      *tls.ConnectionState // tls conn state
	closeNotifyCh chan bool            // from outside -> serve
	copyErrCh     chan error           // from copy goroutine -> serve

	// Everything following is owned by the serve loop
	serveG          gotrack.GoroutineLock // to verify funcs are on serve()
	shutdownTimerCh <-chan time.Time      // nil until used
	shutdownTimer   *time.Timer           // nil until used
	rule            *Rule
}

func (sc *serverConn) serve() {
	sc.serveG.Check()
	defer sc.notePanic()
	defer sc.conn.Close()

	log.Logger.Debug("bfe_stream: process stream connection from %v", sc.conn.RemoteAddr())
	var zero time.Time
	sc.conn.SetDeadline(zero)

	// connect start time
	start := time.Now()

	// select and connect to backend
	bc, back, err := sc.findBackend()
	if err != nil {
		log.Logger.Info("bfe_stream: findBackend() fail: %v", err)
		return
	}

	defer bc.Close()
	defer back.DecConnNum()
	log.Logger.Debug("bfe_stream: proxy connection %v to %v", sc.conn.RemoteAddr(), bc.RemoteAddr())

	err = sc.processProxyProtocol(bc)
	if err != nil {
		log.Logger.Info("bfe_stream: processProxyProtocol() fail: %v", err)
		return
	}

	// copy data between client conn and backend conn
	fn := sc.srv.proxyHandler()
	fn(sc.srv, sc.conn, bc, sc.copyErrCh)

	// wait for finish
	for {
		select {
		case err := <-sc.copyErrCh:
			if err != nil {
				state.StreamErrTransfer.Inc(1)
				duration := time.Since(start)
				tlsConn := sc.conn.(*tls.Conn)
				log.Logger.Info("bfe_stream: stream conn finish: vip:[%s], sni:[%s], clientip:[%v], backend:[%s], "+
					"duration:%fs, error:[%v]", tlsConn.GetVip().String(), sc.tlsState.ServerName, sc.conn.RemoteAddr(),
					back.AddrInfo, duration.Seconds(), err)
			}
			sc.shutDownIn(250 * time.Millisecond)

		case <-sc.closeNotifyCh:
			log.Logger.Debug("bfe_stream: closing conn from %v", sc.conn.RemoteAddr())
			sc.shutDownIn(sc.hs.GracefulShutdownTimeout)
			sc.closeNotifyCh = nil

		case <-sc.shutdownTimerCh:
			return
		}
	}
}

func (sc *serverConn) processProxyProtocol(bc net.Conn) error {
	if sc.rule == nil || sc.rule.ProxyProtocol == 0 {
		return nil
	}

	var virtualAddr net.Addr
	var destinationIP, sourceIP net.IP
	var destinationPort, sourcePort int
	var err error

	netConn := sc.conn.(*tls.Conn).GetNetConn()

	if addressFetcher, ok := netConn.(bfe_util.AddressFetcher); ok {
		virtualAddr = addressFetcher.VirtualAddr()
	}
	if virtualAddr == nil {
		virtualAddr = sc.conn.LocalAddr()
		log.Logger.Debug("bfe_stream: get nil virtual addr: %v, using local address: %v", err, sc.conn.LocalAddr())
	}
	destinationIP, destinationPort, err = bfe_util.ParseIpAndPort(virtualAddr.String())
	if err != nil {
		return fmt.Errorf("parse virtual address error: %v", err)
	}

	sourceIP, sourcePort, err = bfe_util.ParseIpAndPort(sc.conn.RemoteAddr().String())
	if err != nil {
		return fmt.Errorf("parse remote address error: %v", err)
	}

	proxyHeader := &bfe_proxy.Header{
		Version:            byte(sc.rule.ProxyProtocol),
		Command:            bfe_proxy.PROXY,
		SourceAddress:      sourceIP,
		DestinationAddress: destinationIP,
		SourcePort:         uint16(sourcePort),
		DestinationPort:    uint16(destinationPort),
	}

	if sourceIP.To4() != nil {
		proxyHeader.TransportProtocol = bfe_proxy.TCPv4
	} else {
		proxyHeader.TransportProtocol = bfe_proxy.TCPv6
	}

	log.Logger.Debug("bfe_stream: write proxy header[%v] to backend connection", proxyHeader)
	_, err = proxyHeader.WriteTo(bc)
	return err
}

func (sc *serverConn) findBackend() (net.Conn, *backend.BfeBackend, error) {
	balanceHandler := sc.srv.balanceHandler()
	if balanceHandler == nil {
		return nil, nil, errBalanceHandler
	}

	for i := 0; i < sc.srv.connectRetryMax(); i++ {
		// balance backend for current client
		backend, err := balanceHandler(sc.conn)
		if err != nil {
			state.StreamErrBalance.Inc(1)
			log.Logger.Debug("bfe_stream: balance error: %s ", err)
			continue
		}
		backend.IncConnNum()

		// establish tcp conn to backend
		timeout := time.Duration(sc.srv.connectTimeout()) * time.Millisecond
		bAddr := backend.GetAddrInfo()
		bc, err := net.DialTimeout("tcp", bAddr, timeout)
		if err != nil {
			// connect backend failed, desc connection num
			backend.DecConnNum()
			state.StreamErrConnect.Inc(1)
			log.Logger.Debug("bfe_stream: connect %s error: %s", bAddr, err)
			continue
		}

		return bc, backend, nil
	}

	state.StreamErrProxy.Inc(1)
	return nil, nil, errRetryTooMany
}

func (sc *serverConn) shutDownIn(d time.Duration) {
	sc.serveG.Check()
	if sc.shutdownTimer != nil {
		return
	}
	sc.shutdownTimer = time.NewTimer(d)
	sc.shutdownTimerCh = sc.shutdownTimer.C
}

func (sc *serverConn) notePanic() {
	if e := recover(); e != nil {
		log.Logger.Warn("bfe_stream: panic serving %v:%v\n%s", sc.conn.RemoteAddr(),
			e, gotrack.CurrentStackTrace(0))
		state.StreamPanicConn.Inc(1)
	}
}

// TLSProxyHandler copy data between client conn and backend conn.
func TLSProxyHandler(s *Server, c net.Conn, b net.Conn, errCh chan error) {
	// TODO: add read/write timeout
	go func() {
		n, err := io.Copy(b, c)
		state.StreamBytesRecv.Inc(uint(n))
		errCh <- err
	}()

	go func() {
		n, err := io.Copy(c, b)
		state.StreamBytesSent.Inc(uint(n))
		errCh <- err
	}()
}

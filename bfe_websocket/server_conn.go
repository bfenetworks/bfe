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

// websocket connection for server side

package bfe_websocket

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
	bufio "github.com/bfenetworks/bfe/bfe_bufio"
	http "github.com/bfenetworks/bfe/bfe_http"
)

type serverConn struct {
	// Immutable:
	srv           *Server             // server config for websocket proxy
	hs            *http.Server        // server config for http
	req           *http.Request       // handshake request
	rw            http.ResponseWriter // for handshake response
	cconn         net.Conn            // underlying conn to client
	bconn         net.Conn            // underlying conn to backend
	bbr           *bufio.Reader       // buffer reader to backend
	closeNotifyCh chan bool           // from outside -> serve
	errCh         chan error          // from copy goroutine -> serve

	// Everything following is owned by the serve loop
	serveG          gotrack.GoroutineLock // to verify funcs are on serve()
	shutdownTimerCh <-chan time.Time      // nil until used
	shutdownTimer   *time.Timer           // nil until used
}

func (sc *serverConn) serve() {
	var err error
	var back *backend.BfeBackend

	sc.serveG.Check()
	defer sc.notePanic()
	defer func() {
		if sc.cconn != nil {
			sc.cconn.Close()
		}
		if sc.bconn != nil {
			sc.bconn.Close()
		}
	}()

	// select and connect to backend
	sc.bconn, back, err = sc.findBackend(sc.req)
	if err != nil {
		log.Logger.Info("bfe_websocket: findBackend() select backend failed: %s (%s)", err, sc.req.Host)
		return
	}
	log.Logger.Debug("bfe_websocket: proxy websocket connection to %v", sc.bconn.RemoteAddr())
	defer back.DecConnNum()

	// websocket handshake
	if err := sc.websocketHandshake(); err != nil {
		log.Logger.Info("bfe_websocket: websocket handshake fail: %v (%v)", err, sc.bconn.RemoteAddr())
		state.WebSocketErrHandshake.Inc(1)
		return
	}

	// websocket data transfer
	sc.websocketDataTransfer()

	// wait for finish
	for {
		select {
		case err := <-sc.errCh:
			log.Logger.Debug("bfe_websocket: websocket conn finish %v: %v", sc.cconn.RemoteAddr(), err)
			if err != nil {
				state.WebSocketErrTransfer.Inc(1)
			}
			sc.shutDownIn(250 * time.Millisecond)

		case <-sc.closeNotifyCh:
			log.Logger.Debug("bfe_websocket: closing conn from %v", sc.cconn.RemoteAddr())
			sc.shutDownIn(sc.hs.GracefulShutdownTimeout)
			sc.closeNotifyCh = nil

		case <-sc.shutdownTimerCh:
			return
		}
	}
}

func (sc *serverConn) findBackend(req *http.Request) (net.Conn, *backend.BfeBackend, error) {
	balanceHandler := sc.srv.balanceHandler()
	if balanceHandler == nil {
		return nil, nil, errBalanceHandler
	}

	for i := 0; i < sc.srv.connectRetryMax(); i++ {
		// balance backend for current client
		backend, err := balanceHandler(req)
		if err != nil {
			state.WebSocketErrBalance.Inc(1)
			log.Logger.Debug("bfe_websocket: balance error: %s ", err)
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
			state.WebSocketErrConnect.Inc(1)
			log.Logger.Debug("bfe_websocket: connect %s error: %s", bAddr, err)
			continue
		}

		return bc, backend, nil
	}

	state.WebSocketErrProxy.Inc(1)
	return nil, nil, errRetryTooMany
}

func (sc *serverConn) websocketHandshake() error {
	rw, req := sc.rw, sc.req

	// write client request to backend
	if err := req.Write(sc.bconn); err != nil {
		return err
	}

	// read response from backend
	sc.bbr = bufio.NewReader(sc.bconn)
	rsp, err := http.ReadResponse(sc.bbr, req)
	if err != nil {
		return err
	}

	// check whether backend accept websocket upgrade
	if !CheckAcceptWebSocket(rsp) {
		state.WebSocketErrBackendReject.Inc(1)
		// write response anyway and finish conn
		sendResponse(rw, rsp)
		return fmt.Errorf("server reject upgrade to websocket (%v)", rsp.Status)
	}

	// write 101 response
	return sendResponse(rw, rsp)
}

func (sc *serverConn) websocketDataTransfer() {
	var cbr *bufio.ReadWriter
	var err error
	errCh := sc.errCh

	// take over the underlying connection from client
	sc.cconn, cbr, err = sc.rw.(http.Hijacker).Hijack()
	if err != nil {
		/* never come here */
		errCh <- fmt.Errorf("Hijack failed: " + err.Error())
		return
	}

	// if underlying buffer contain unprocessed data
	cbuf, err := peekBufferedData(cbr.Reader)
	if err != nil {
		errCh <- err
		return
	}
	if len(cbuf) > 0 {
		if _, err := sc.bconn.Write(cbuf); err != nil {
			errCh <- err
			return
		}
	}

	bbuf, err := peekBufferedData(sc.bbr)
	if err != nil {
		errCh <- err
		return
	}
	if len(bbuf) > 0 {
		if _, err := sc.cconn.Write(bbuf); err != nil {
			errCh <- err
			return
		}
	}

	// proxy data from client to backend
	go func() {
		n, err := io.Copy(sc.bconn, sc.cconn)
		state.WebSocketBytesRecv.Inc(uint(n))
		errCh <- err
	}()

	// proxy data from backend to client
	go func() {
		n, err := io.Copy(sc.cconn, sc.bconn)
		state.WebSocketBytesSent.Inc(uint(n))
		errCh <- err
	}()
}

func sendResponse(rw http.ResponseWriter, rsp *http.Response) error {
	http.CopyHeader(rw.Header(), rsp.Header)
	rw.WriteHeader(rsp.StatusCode)

	if _, err := io.Copy(rw, rsp.Body); err != nil {
		return err
	}

	if f, ok := rw.(http.Flusher); ok {
		if err := f.Flush(); err != nil {
			return err
		}
	}
	return nil
}

func peekBufferedData(r *bufio.Reader) ([]byte, error) {
	n := r.Buffered()
	if n <= 0 {
		return nil, nil
	}

	b, err := r.Peek(n)
	if err != nil {
		return nil, err
	}

	return b, nil
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
		log.Logger.Warn("bfe_websocket: panic serving :%v\n%s",
			e, gotrack.CurrentStackTrace(0))
		state.WebSocketPanicConn.Inc(1)
	}
}

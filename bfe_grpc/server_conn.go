package bfe_grpc

import (
	"time"
	"net"
	"sync"
	"context"
	"math"
	"strings"
	"fmt"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/bfe/bfe_balance/backend"
	"github.com/baidu/go-lib/gotrack"
	http "github.com/baidu/bfe/bfe_http"
	"github.com/baidu/bfe/bfe_grpc/transport"
)

import (
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
)

const (
	defaultServerMaxReceiveMessageSize = 1024 * 1024 * 4
	defaultServerMaxSendMessageSize    = math.MaxInt32
)

type serverConn struct {
	srv           *Server             // server config for grpc proxy
	hs            *http.Server        // server config for http
	conn         net.Conn            // underlying conn to client
	remoteAddrStr    string
	//tlsState      *tls.ConnectionState // tls conn state
	closeNotifyCh chan bool           // from outside -> serve
	errCh         chan error          // from copy goroutine -> serve

	// Everything following is owned by the serve loop
	serveG          gotrack.GoroutineLock // to verify funcs are on serve()
	shutdownTimerCh <-chan time.Time      // nil until used
	shutdownTimer   *time.Timer           // nil until used
}

func (sc *serverConn) serve() {
	sc.serveG.Check()
	defer sc.notePanic()
	defer func() {
		if sc.conn != nil {
			sc.conn.Close()
		}
	}()

	//// select and connect to backend
	//sc.bconn, back, err = sc.findBackend(sc.req)
	//if err != nil {
	//	log.Logger.Info("bfe_grpc: findBackend() select backend failed: %s (%s)", err, sc.req.Host)
	//	return
	//}
	//log.Logger.Debug("bfe_grpc: proxy grpc connection to %v", sc.bconn.RemoteAddr())
	//defer back.DecConnNum()

	// grpc handshake
	if err := sc.grpcHandshake(); err != nil {
		log.Logger.Info("bfe_grpc: grpc handshake fail: %v (%v)", err, sc.conn.RemoteAddr())
		//state.WebSocketErrHandshake.Inc(1)
		return
	}

	// wait for finish
	for {
		select {
		case err := <-sc.errCh:
			log.Logger.Debug("bfe_grpc: grpc conn finish %v: %v", sc.conn.RemoteAddr(), err)
			if err != nil {
				//state.WebSocketErrTransfer.Inc(1)
			}
			sc.shutDownIn(250 * time.Millisecond)

		case <-sc.closeNotifyCh:
			log.Logger.Debug("bfe_grpc: closing conn from %v", sc.conn.RemoteAddr())
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
		//backend, err := balanceHandler(req)
		//if err != nil {
		//	//state.WebSocketErrBalance.Inc(1)
		//	log.Logger.Debug("bfe_grpc: balance error: %s ", err)
		//	continue
		//}
		// TODO: backend must started
		backend := backend.NewBfeBackend()
		backend.Addr = "localhost"
		backend.Port = 50000
		backend.AddrInfo = "localhost:50000"

		backend.AddConnNum()

		// establish tcp conn to backend
		timeout := time.Duration(sc.srv.connectTimeout()) * time.Millisecond
		bAddr := backend.GetAddrInfo()
		bc, err := net.DialTimeout("tcp", bAddr, timeout)
		if err != nil {
			// connect backend failed, desc connection num
			backend.DecConnNum()
			//state.WebSocketErrConnect.Inc(1)
			log.Logger.Debug("bfe_grpc: connect %s error: %s", bAddr, err)
			continue
		}

		return bc, backend, nil
	}

	//state.WebSocketErrProxy.Inc(1)
	return nil, nil, errRetryTooMany
}

func (sc *serverConn) grpcHandshake() error {
	config := &transport.ServerConfig{
		MaxStreams: 0,
	}
	st, err := transport.NewServerTransport("http2", sc.conn, config)
	if err != nil {
		return err
	}

	go func() {
		sc.serveStreams(st)
		// TODO: st去重
	}()

	return nil
}

func (sc *serverConn) serveStreams(st transport.ServerTransport) {
	defer st.Close()
	var wg sync.WaitGroup
	st.HandleStreams(func(stream *transport.Stream) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc.handleStream(st, stream)
		}()
	}, func(ctx context.Context, method string) context.Context {
		return ctx
	})
	wg.Wait()
}

func (sc *serverConn) handleStream(t transport.ServerTransport, stream *transport.Stream) {
	sm := stream.Method()
	if sm != "" && sm[0] == '/' {
		sm = sm[1:]
	}
	pos := strings.LastIndex(sm, "/")
	if pos == -1 {
		errDesc := fmt.Sprintf("malformed method name: %q", stream.Method())
		if err := t.WriteStatus(stream, status.New(codes.ResourceExhausted, errDesc)); err != nil {
			grpclog.Warningf("bfe_grpc: Server.handleStream failed to write status: %v", err)
		}
		return
	}
	service := sm[:pos]
	method := sm[pos+1:]

	// Unknown service, or known server unknown method.
	if unknownDesc := sc.srv.getUnknownServiceHandler(); unknownDesc != nil {
		sc.processStreamingRPC(t, stream, unknownDesc)
		return
	}

	var errDesc string
	errDesc = fmt.Sprintf("unknown method %v for service %v", method, service)

	if err := t.WriteStatus(stream, status.New(codes.Unimplemented, errDesc)); err != nil {
		grpclog.Warningf("bfe_grpc: Server.handleStream failed to write status: %v", err)
	}
}


func (sc *serverConn) processStreamingRPC(t transport.ServerTransport, stream *transport.Stream, sd *grpc.StreamDesc) (err error) {
	ctx := grpc.NewContextWithServerTransportStream(stream.Context(), stream)
	ss := &serverStream{
		ctx:                   ctx,
		t:                     t,
		s:                     stream,
		p:                     &parser{r: stream},
		codec:                 sc.srv.getCustomCodec(), // TODO: we should using contentSubtype, default using proto codec
		maxReceiveMessageSize: defaultServerMaxReceiveMessageSize,
		maxSendMessageSize:    defaultServerMaxSendMessageSize,
	}

	// If dc is set and matches the stream's compression, use it.  Otherwise, try
	// to find a matching registered compressor for decomp.
	if rc := stream.RecvCompress(); sc.srv.dc != nil && sc.srv.dc.Type() == rc {
		ss.dc = sc.srv.dc
	} else if rc != "" && rc != encoding.Identity {
		ss.decomp = encoding.GetCompressor(rc)
		if ss.decomp == nil {
			st := status.Newf(codes.Unimplemented, "bfe_grpc: Decompressor is not installed for grpc-encoding %q", rc)
			t.WriteStatus(ss.s, st)
			return st.Err()
		}
	}

	// If cp is set, use it.  Otherwise, attempt to compress the response using
	// the incoming message compression method.
	//
	// NOTE: this needs to be ahead of all handling, https://github.com/grpc/grpc-go/issues/686.
	if sc.srv.cp != nil {
		ss.cp = sc.srv.cp
		stream.SetSendCompress(sc.srv.cp.Type())
	} else if rc := stream.RecvCompress(); rc != "" && rc != encoding.Identity {
		// Legacy compressor not specified; attempt to respond with same encoding.
		ss.comp = encoding.GetCompressor(rc)
		if ss.comp != nil {
			stream.SetSendCompress(rc)
		}
	}

	var appErr error
	var server interface{}
	appErr = sd.Handler(server, ss)
	if appErr != nil {
		appStatus, ok := status.FromError(appErr)
		if !ok {
			appStatus = status.New(codes.Unknown, appErr.Error())
			appErr = appStatus.Err()
		}
		t.WriteStatus(ss.s, appStatus)
		// TODO: Should we log an error from WriteStatus here and below?
		return appErr
	}
	err = t.WriteStatus(ss.s, status.New(codes.OK, ""))
	return err
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
		log.Logger.Warn("bfe_grpc: panic serving :%v\n%s",
			e, gotrack.CurrentStackTrace(0))
		//state.WebSocketPanicConn.Inc(1)
	}
}
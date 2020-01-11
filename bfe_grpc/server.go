package bfe_grpc

import (
	"net"
)

import (
	http "github.com/baidu/bfe/bfe_http"
	tls "github.com/baidu/bfe/bfe_tls"
	"github.com/baidu/go-lib/gotrack"
)

import (
	"google.golang.org/grpc"
)

const (
	defaultConnectTimeout  = 1000 // ms
	defaultConnectRetryMax = 3

	// NextProtoTLS is the NPN/ALPN protocol negotiated during
	// GRPC's TLS setup.
	NextProtoTLS = GRPC
)

type Server struct {
	//TODO: args
	backendIsInsecure bool

	// ConnectTimeout optionally specifies the timeout value (ms) to
	// connect backend. If zero, a default value is used.
	ConnectTimeout int

	// ConnectRetryMax optionally specifies the upper limit of connect
	// retris. If zero, a default value is used
	ConnectRetryMax int

	// BalanceHandler optionally specifies the handler for backends balance
	// BalanceHandler should not be nil.
	BalanceHandler BalanceHandler

	// ProxyHandler optionally specifies the handler for process client conn
	// and backend conn. If nil, a default value is used.
	ProxyHandler StreamDirector

	unknownStreamDesc *grpc.StreamDesc
	codec                 baseCodec

	cp                    Compressor
	dc                    Decompressor
}

func (s *Server) SetUnknownServiceHandler(streamHandler grpc.StreamHandler) {
	s.unknownStreamDesc = &grpc.StreamDesc{
		StreamName: "unknown_service_handler",
		Handler:    streamHandler,
		ClientStreams: true,
		ServerStreams: true,
	}
}

func (s *Server) getUnknownServiceHandler() *grpc.StreamDesc {
	return s.unknownStreamDesc
}

func (s *Server) CustomCodec(codec baseCodec) {
	s.codec = codec
}

func (s *Server) getCustomCodec() baseCodec {
	return s.codec
}

func (s *Server) connectTimeout() int {
	if v := s.ConnectTimeout; v > 0 {
		return v
	}
	return defaultConnectTimeout
}

func (s *Server) connectRetryMax() int {
	if v := s.ConnectRetryMax; v > 0 {
		return v
	}
	return defaultConnectRetryMax
}

func (s *Server) balanceHandler() BalanceHandler {
	return s.BalanceHandler
}

func (s *Server) proxyHandler() StreamDirector {
	if v := s.ProxyHandler; v != nil {
		return v
	}
	//return TransparentHandler(Director())
	return Director()
}

func (s *Server) ServeConn(c net.Conn, opts *ServeConnOpts) {
	sc := &serverConn{
		srv:              s,
		hs:               opts.baseConfig(),
		conn:             c,
		remoteAddrStr:    c.RemoteAddr().String(),

		closeNotifyCh:    opts.BaseConfig.CloseNotifyCh,
		errCh :make(chan error, 2),

		serveG:            gotrack.NewGoroutineLock(),

	}
	sc.serve()
}

func NewProtoHandler(conf *Server) func(*http.Server, *tls.Conn, http.Handler) {
	if conf == nil {
		conf = new(Server)
	}

	// set default unknown service handler
	conf.SetUnknownServiceHandler(TransparentHandler(Director()))

	// set default custom codec
	conf.CustomCodec(CustomCodec())

	protoHandler := func(hs *http.Server, c *tls.Conn, h http.Handler) {
		connOpts := &ServeConnOpts{hs, h}
		conf.ServeConn(c, connOpts)
	}
	return protoHandler
}

// ServeConnOpts are options for the Server.ServeConn method.
type ServeConnOpts struct {
	// BaseConfig optionally sets the base configuration
	// for values. If nil, defaults are used.
	BaseConfig *http.Server

	// Handler specifies which handler to use for processing
	// requests. If nil, BaseConfig.Handler is used.
	Handler http.Handler
}

func (o *ServeConnOpts) baseConfig() *http.Server {
	if o != nil && o.BaseConfig != nil {
		return o.BaseConfig
	}
	return new(http.Server)
}

func (o *ServeConnOpts) handler() http.Handler {
	if o != nil {
		if o.Handler != nil {
			return o.Handler
		}
		if o.BaseConfig != nil && o.BaseConfig.Handler != nil {
			return o.BaseConfig.Handler
		}
	}
	return nil
}


package mod_sdk_nsfocus_waf

import (
	"net"
	"net/http"

	"github.com/bfenetworks/bwi/bwi"
)

// Server
type Server struct {
	// createFactory create socket
	createFactory func() (net.Conn, error)
	// poolSize
	poolSize int
	// pool
	pool chan *connection
}

// NewWafServerWithPoolSize new serve with pool size
func NewWafServerWithPoolSize(socketFactory func() (net.Conn, error), poolSize int) bwi.WafServer {
	server := &Server{
		createFactory: socketFactory,
		poolSize:      poolSize,
		pool:          make(chan *connection, poolSize),
	}
	server.init()
	return server
}

// init
func (m *Server) init() {
	for index := 0; index < m.poolSize; index++ {
		conn, err := newConn(m)
		if err != nil {
			continue
		}
		// add to pool
		select {
		case m.pool <- conn:
		default:
		}
	}
}

// DetectRequest detect if request is valid
func (m *Server) DetectRequest(req *http.Request, logId string) (bwi.WafResult, error) {
	// get http request
	conn, err := m.getConn()
	if err != nil {
		return nil, err
	}
	defer m.putConn(conn)
	return conn.detectRequst(req, logId)
}

// getConn get connection from pool
func (m *Server) getConn() (*connection, error) {
	select {
	case conn := <-m.pool:
		return conn, nil
	default:
		conn, err := newConn(m)
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
}

// putConn put connection back to pool
func (m *Server) putConn(conn *connection) {
	// check if connection is close
	err := conn.keepAlive()
	if err != nil {
		return
	}
	// put back to pool
	select {
	case m.pool <- conn:
	default:
		conn.Close()
	}
}

// UpdateSockFactory update socket factory
func (m *Server) UpdateSockFactory(socketFactory func() (net.Conn, error)) {
	m.createFactory = socketFactory
}

// Close close the server
func (m *Server) Close() {
}

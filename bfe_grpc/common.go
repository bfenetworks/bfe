package bfe_grpc

import (
	"errors"
	"net"
	http "github.com/baidu/bfe/bfe_http"
)

import (
	"github.com/baidu/bfe/bfe_balance/backend"
	"crypto/tls"
)

const (
	GRPC = "grpc"
)

var (
	errBalanceHandler = errors.New("bfe_grpc: balanceHandler uninitial")
	errRetryTooMany   = errors.New("bfe_grpc: proxy retry too many")
)

// CheckUpgradeGRPC checks whether client request for GRPC protocol.
func CheckUpgradeGRPC(req *http.Request) bool {
	if req.ProtoMajor != 2 {
		return false
	}
	return true
}

// Scheme returns scheme of current grpc conn.
func Scheme(c net.Conn) string {
	if _, ok := c.(*tls.Conn); ok {
		return "grpcs://"
	}
	return "grpc://"
}

// BalanceHandler selects backend for current conn.
type BalanceHandler func(c interface{}) (*backend.BfeBackend, error)


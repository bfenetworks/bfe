/* mod_tcp_keepalive_test.go - test for mod_tcp_keepalive.go */
/*
modification history
--------------------
2021/9/16, by Yu Hui, create
*/
/*
DESCRIPTION
*/

package mod_tcp_keepalive

import (
	"net"
	"testing"
)

import (
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func prepareModule() (*ModuleTcpKeepAlive, error) {
	m := NewModuleTcpKeepAlive()
	err := m.Init(bfe_module.NewBfeCallbacks(), web_monitor.NewWebHandlers(), "./testdata")
	return m, err
}

func prepareRequest() *bfe_basic.Request {
	request := new(bfe_basic.Request)
	request.HttpRequest = new(bfe_http.Request)
	request.Session = new(bfe_basic.Session)
	request.Context = make(map[interface{}]interface{})
	return request
}

func TestSetKeepAlive(t *testing.T) {
	m, err := prepareModule()
	if err != nil {
		t.Errorf("prepareModule() error: %v", err)
		return
	}
	s := new(bfe_basic.Session)
	ip := "180.97.93.196"
	address := "180.97.93.196:80"

	s.Product = "product1"
	s.Vip = net.ParseIP(ip)
	if s.Vip == nil {
		t.Errorf("net.ParseIP(%s) == nil", ip)
	}

	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Errorf("net.Dial(tcp, %s) error: %v", address, err)
		return
	}
	s.Connection = conn

	m.HandleAccept(s)
	metrics := m.metrics.GetAll()
	if metrics.CounterData["CONN_TO_SET"] != 1 ||
		metrics.CounterData["CONN_SET_KEEP_IDLE"] != 1 ||
		metrics.CounterData["CONN_SET_KEEP_INTVL"] != 1 {

		t.Errorf("CONN_TO_SET and CONN_SET_KEEP_IDLE and CONN_SET_KEEP_INTVL should be 1")
		return
	}
}

func TestModuleMisc(t *testing.T) {
	m, err := prepareModule()
	if err != nil {
		t.Errorf("prepareModule() error: %v", err)
		return
	}
	if s, _ := m.getState(nil); s == nil {
		t.Errorf("Should return valid state")
	}
	if m.monitorHandlers() == nil {
		t.Errorf("Should return valid monitor handlers")
	}
	if m.reloadHandlers() == nil {
		t.Errorf("Should return valid reload handlers")
	}
}

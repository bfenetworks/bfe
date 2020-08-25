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

// find cluster name for incoming request

package bfe_server

import (
	"fmt"
	"net"
	"net/url"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_balance/backend"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_route"
	"github.com/bfenetworks/bfe/bfe_tls"
	"github.com/bfenetworks/bfe/bfe_util"
)

// findProduct finds product name for given request.
func (srv *BfeServer) findProduct(req *bfe_basic.Request) error {
	req.Stat.FindProStart = time.Now()
	defer func() {
		req.Stat.FindProEnd = time.Now()
	}()

	serverConf := req.SvrDataConf.(*bfe_route.ServerDataConf)

	// look up hostTag and product in host table
	return serverConf.HostTable.LookupHostTagAndProduct(req)
}

// findCluster finds clusterName for given request.
func (srv *BfeServer) findCluster(req *bfe_basic.Request) error {
	req.Stat.LocateStart = time.Now()
	defer func() {
		req.Stat.LocateEnd = time.Now()
	}()

	serverConf := req.SvrDataConf.(*bfe_route.ServerDataConf)

	// look up clusterName
	return serverConf.HostTable.LookupCluster(req)
}

// FindLocation finds product and cluster for given request
func (srv *BfeServer) FindLocation(request *bfe_basic.Request) (string, error) {
	var clusterName string
	// find product
	if err := srv.findProduct(request); err != nil {
		return "", err
	}

	// find cluster
	if err := srv.findCluster(request); err != nil {
		return "", err
	}

	clusterName = request.Route.ClusterName
	return clusterName, nil
}

// FindProduct finds product for proxied conn (under tls proxy mode).
func (srv *BfeServer) FindProduct(conn net.Conn) string {
	sc := srv.GetServerConf()

	// get vip from connection
	vip := bfe_util.GetVip(conn)
	if vip == nil {
		return ""
	}

	// find product from vip
	product, err := sc.HostTable.LookupProductByVip(vip.String())
	if err != nil {
		return ""
	}

	return product
}

// Balance finds backend for proxied conn (under tls proxy mode).
func (srv *BfeServer) Balance(e interface{}) (*backend.BfeBackend, error) {
	serverDataConf := srv.GetServerConf()

	var conn net.Conn
	var req *bfe_http.Request
	switch v := e.(type) {
	case net.Conn:
		conn = v
		req = &bfe_http.Request{URL: new(url.URL)}
	case *bfe_http.Request:
		req = v
		conn = req.State.Conn
	default:
		return nil, fmt.Errorf("invalid type for Balance:%T", v)
	}

	// create pesudo request
	session := bfe_basic.NewSession(conn)
	vip, vport, err := bfe_util.GetVipPort(conn)
	if err == nil {
		session.Vip = vip
		session.Vport = vport
	}

	if _, ok := conn.(*bfe_tls.Conn); ok {
		session.IsSecure = true
	}

	reqStat := bfe_basic.NewRequestStat(time.Now())
	reqBasic := bfe_basic.NewRequest(req, conn, reqStat, session, serverDataConf)

	// find cluster
	clusterName, err := srv.FindLocation(reqBasic)
	if err != nil {
		return nil, err
	}

	// find backend
	bal, err := srv.balTable.Lookup(clusterName)
	if err != nil {
		return nil, err
	}
	backend, err := bal.Balance(reqBasic)
	if err != nil {
		return nil, err
	}

	return backend, nil
}

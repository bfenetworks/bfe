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

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// HTTP reverse proxy handler

package bfe_server

import (
	"crypto/tls"
	"io"
	"net"
	"reflect"
	"sync"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
	"golang.org/x/net/http2"
)

import (
	bfe_cluster_backend "github.com/bfenetworks/bfe/bfe_balance/backend"
	bal_gslb "github.com/bfenetworks/bfe/bfe_balance/bal_gslb"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_debug"
	"github.com/bfenetworks/bfe/bfe_fcgi"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_http2"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_route"
	"github.com/bfenetworks/bfe/bfe_route/bfe_cluster"
	"github.com/bfenetworks/bfe/bfe_spdy"
	"github.com/bfenetworks/bfe/bfe_util"
)

// TrailerPrefix is a magic prefix for ResponseWriter.Header map keys
// that, if present, signals that the map entry is actually for
// the response trailers, and not the response headers. The prefix
// is stripped after the ServeHTTP call finishes and the values are
// sent in the trailers.
//
// This mechanism is intended only for trailers that are not known
// prior to the headers being written. If the set of trailers is fixed
// or known before the header is written, the normal Go trailers mechanism
// is preferred:
//    https://golang.org/pkg/net/http/#ResponseWriter
//    https://golang.org/pkg/net/http/#example_ResponseWriter_trailers
const TrailerPrefix = "Trailer:"

// RoundTripperMap holds mappings from cluster-name to RoundTripper.
type RoundTripperMap map[string]bfe_http.RoundTripper

// ReverseProxy takes an incoming request and sends it to another server,
// proxying the response back to the client.
type ReverseProxy struct {
	// The transport used to perform proxy requests.
	// If no transport from clustername->transport map, create one.
	tsMu       sync.RWMutex
	transports RoundTripperMap
	bufferPool *bfe_util.FixedPool

	server     *BfeServer  // link to bfe server
	proxyState *ProxyState // state of proxy
}

// NewReverseProxy returns a new ReverseProxy.
func NewReverseProxy(server *BfeServer, state *ProxyState) *ReverseProxy {
	rp := new(ReverseProxy)
	rp.transports = make(RoundTripperMap)
	rp.server = server
	rp.proxyState = state
	rp.bufferPool = bfe_util.NewFixedPool(32 * 1024)
	return rp
}

// httpProtoSet set http proto for out request.
func httpProtoSet(outreq *bfe_http.Request) {
	outreq.Proto = "HTTP/1.1"
	outreq.ProtoMajor = 1
	outreq.ProtoMinor = 1
	outreq.Close = false
}

// hopByHopHeaderRemove remove hop-by-hop headers.
func hopByHopHeaderRemove(outreq, req *bfe_http.Request) {
	// Remove hop-by-hop headers to the backend.  Especially
	// important is "Connection" because we want a persistent
	// connection, regardless of what the client sent to us.  This
	// is modifying the same underlying map from req (shallow
	// copied above) so we only copy it if necessary.
	copiedHeaders := false
	for _, h := range bfe_basic.HopHeaders {
		hv := outreq.Header.Get(h)
		if hv == "" {
			continue
		}

		if h == "Te" && hv == "trailers" {
			// Issue 21096: tell backend applications that
			// care about trailer support that we support
			// trailers. (We do, but we don't go out of
			// our way to advertise that unless the
			// incoming client request thought it was
			// worth mentioning)
			continue
		}

		if !copiedHeaders {
			outreq.Header = make(bfe_http.Header, len(req.Header))
			bfe_http.CopyHeader(outreq.Header, req.Header)
			copiedHeaders = true
		}
		outreq.Header.Del(h)
	}
}

// setBackendAddr set backend addr to host of request url.
func setBackendAddr(req *bfe_http.Request, backend *bfe_cluster_backend.BfeBackend) {
	req.URL.Scheme = "http"
	req.URL.Host = backend.GetAddrInfo()
}

func (p *ReverseProxy) setTransports(clusterMap bfe_route.ClusterMap) {
	p.tsMu.Lock()
	defer p.tsMu.Unlock()

	newTransports := make(RoundTripperMap)
	for cluster, conf := range clusterMap {
		transport, ok := p.transports[cluster]
		if !ok {
			transport = createTransport(conf)
			newTransports[cluster] = transport
			continue
		}

		switch t := transport.(type) {
		case *bfe_http.Transport:
			// get transport, check if transport needs update
			backendConf := conf.BackendConf()
			if (t.MaxIdleConnsPerHost != *backendConf.MaxIdleConnsPerHost) ||
				(t.ResponseHeaderTimeout != time.Millisecond*time.Duration(*backendConf.TimeoutResponseHeader)) ||
				(t.ReqWriteBufferSize != conf.ReqWriteBufferSize()) ||
				(t.ReqFlushInterval != conf.ReqFlushInterval()) {
				// create new transport with newConf instead of update transport
				// update transport needs lock
				transport = createTransport(conf)
				newTransports[cluster] = transport
				continue
			}

			newTransports[cluster] = transport
		default:
			transport = createTransport(conf)
			newTransports[cluster] = transport
		}
	}

	p.transports = newTransports
}

// getTransport return transport from map, if not exist, create a transport.
func (p *ReverseProxy) getTransport(cluster *bfe_cluster.BfeCluster) bfe_http.RoundTripper {
	p.tsMu.RLock()
	transport, ok := p.transports[cluster.Name]
	p.tsMu.RUnlock()

	if !ok {
		transport = createTransport(cluster)
		p.tsMu.Lock()
		p.transports[cluster.Name] = transport
		p.tsMu.Unlock()
	}

	return transport
}

func createTransport(cluster *bfe_cluster.BfeCluster) bfe_http.RoundTripper {
	backendConf := cluster.BackendConf()
	protocol := *backendConf.Protocol

	log.Logger.Debug("create a new transport for %s, timeout %d", cluster.Name, *backendConf.TimeoutResponseHeader)

	switch protocol {
	case "http":
		// cluster has its own Connect Server Timeout.
		// so each cluster has a different transport
		// once cluster's timeout updated, dailer use new value
		dailer := func(network, add string) (net.Conn, error) {
			timeout := time.Duration(cluster.TimeoutConnSrv()) * time.Millisecond
			return net.DialTimeout(network, add, timeout)
		}

		return &bfe_http.Transport{
			Dial:                  dailer,
			DisableKeepAlives:     (*backendConf.MaxIdleConnsPerHost) == 0,
			MaxIdleConnsPerHost:   *backendConf.MaxIdleConnsPerHost,
			ResponseHeaderTimeout: time.Millisecond * time.Duration(*backendConf.TimeoutResponseHeader),
			ReqWriteBufferSize:    cluster.ReqWriteBufferSize(),
			ReqFlushInterval:      cluster.ReqFlushInterval(),
			DisableCompression:    true,
		}
	case "fcgi":
		return &bfe_fcgi.Transport{
			Root:    backendConf.FCGIConf.Root,
			EnvVars: backendConf.FCGIConf.EnvVars,
		}
	case "h2c":
		return &bfe_http2.Transport{
			T: &http2.Transport{
				AllowHTTP: true,
				DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
					timeout := time.Duration(cluster.TimeoutConnSrv()) * time.Millisecond
					return net.DialTimeout(network, addr, timeout)
				},
			},
		}
	default:
		/* never come here */
		log.Logger.Warn("unknown cluster protocol %s", protocol)
		return nil
	}
}

// clusterInvoke invoke cluster to get response.
func (p *ReverseProxy) clusterInvoke(srv *BfeServer, cluster *bfe_cluster.BfeCluster,
	request *bfe_basic.Request, rw bfe_http.ResponseWriter) (
	res *bfe_http.Response, action int, err error) {
	var clusterBackend *bfe_cluster_backend.BfeBackend
	var bal *bal_gslb.BalanceGslb
	var outreq *bfe_http.Request = request.OutRequest

	// mark start/end of cluster invoke
	request.Stat.ClusterStart = time.Now()
	defer func() {
		request.Stat.ClusterEnd = time.Now()
	}()

	clusterTransport := p.getTransport(cluster)

	// look up for balance
	bal, err = srv.balTable.Lookup(cluster.Name)
	if err != nil {
		log.Logger.Warn("no balance for %s", cluster.Name)
		request.Stat.ResponseStart = time.Now()
		request.ErrCode = bfe_basic.ErrBkNoCluster
		request.ErrMsg = err.Error()
		p.proxyState.ErrBkNoBalance.Inc(1)
		action = closeAfterReply
		return
	}

	// When request.RetryTime exceeds some value, srv.clusterTable.Lookup()
	// will return error. Here set a limit of 20, to avoid endless loop
	for i := 0; i < 20; i++ {
		// get backend with cluster-name and request
		clusterBackend, err = bal.Balance(request)
		if err == bfe_basic.ErrBkCrossRetryBalance {
			request.RetryTime += 1
			continue
		}

		if err != nil {
			// p.proxystate counter is set by bal.Balance(), only log
			log.Logger.Warn("cluster [%s] select backend failed, err[%s]", cluster.Name,
				err.Error())
			break
		}

		// err == nil if and only if we choose a new backend,
		// decr old backend connection num
		if request.Trans.Backend != nil {
			request.Trans.Backend.DecConnNum()
			request.Trans.Backend = nil
		}
		request.SetRequestTransport(clusterBackend, clusterTransport)

		log.Logger.Debug("ReverseProxy.Invoke(): before HandleForward backend %s:%d",
			request.Trans.Backend.Addr, request.Trans.Backend.Port)

		// Callback for HandleForward
		hl := srv.CallBacks.GetHandlerList(bfe_module.HandleForward)
		if hl != nil {
			retVal := hl.FilterForward(request)
			switch retVal {
			case bfe_module.BfeHandlerFinish:
				// close the connection after response
				action = closeAfterReply
				return
			}
		}

		log.Logger.Debug("ReverseProxy.Invoke(): after HandleForward backend %s:%d",
			request.Trans.Backend.Addr, request.Trans.Backend.Port)

		// set backend addr to out request
		backend := request.Trans.Backend
		backend.IncConnNum()
		setBackendAddr(outreq, backend)

		// invoke backend
		request.Stat.BackendStart = time.Now()
		if i == 0 {
			// record start time of the first try
			request.Stat.BackendFirst = request.Stat.BackendStart
		}

		transport := request.Trans.Transport

		res, err = transport.RoundTrip(outreq)

		request.Stat.BackendEnd = time.Now()

		// record backend info to request, no matter succeed or fail
		request.Backend.SubclusterName = backend.SubCluster
		request.Backend.BackendName = backend.Name
		request.Backend.BackendAddr = backend.Addr
		request.Backend.BackendPort = uint32(backend.Port)

		if err == nil {
			if checkBackendStatus(cluster.OutlierDetectionLevel(), res.StatusCode) {
				backend.OnFail(cluster.Name)
			} else {
				backend.OnSuccess()
			}

			// clear err msg in req.
			// this step is required, if finally succeed after retry
			request.ErrCode = nil
			request.ErrMsg = ""

			// record body size of request after forward
			request.Stat.BodyLenIn = int(outreq.State.BodySize)

			if bfe_debug.DebugServHTTP {
				log.Logger.Debug("ReverseProxy.ServeHTTP(): get response from %s", backend.Name)
			}
			break
		}

		// fail in invoking backend
		log.Logger.Info("[%s] [%s:%d] roundtrip %s", cluster.Name, backend.Addr, backend.Port, err)
		p.proxyState.ErrBkRequestBackend.Inc(1)

		// deal with errors here, possible error type:
		//  1. connect backend error
		//  2. read client request body error(POST/PUT)
		//  3. write backend error
		//     a. haven't write any byte
		//     b. aleady write part of data
		//  4. read backend error
		//  5. other error
		allowRetry := false
		switch err.(type) {
		case bfe_http.ConnectError, bfe_fcgi.ConnectError:
			// if error happens in dial phrase, we can retry
			request.ErrCode = bfe_basic.ErrBkConnectBackend
			request.ErrMsg = err.Error()
			p.proxyState.ErrBkConnectBackend.Inc(1)
			allowRetry = true
			backend.OnFail(cluster.Name)

		case bfe_http.WriteRequestError, bfe_fcgi.WriteRequestError:
			request.ErrCode = bfe_basic.ErrBkWriteRequest
			request.ErrMsg = err.Error()
			p.proxyState.ErrBkWriteRequest.Inc(1)
			allowRetry = checkAllowRetry(cluster.RetryLevel(), outreq)

			// if error is caused by backend server
			rerr := err.(bfe_http.WriteRequestError)
			if !rerr.CheckTargetError(request.RemoteAddr) {
				backend.OnFail(cluster.Name)
			}

		case bfe_http.ReadRespHeaderError, bfe_fcgi.ReadRespHeaderError:
			request.ErrCode = bfe_basic.ErrBkReadRespHeader
			request.ErrMsg = err.Error()
			p.proxyState.ErrBkReadRespHeader.Inc(1)
			allowRetry = checkAllowRetry(cluster.RetryLevel(), outreq)
			backend.OnFail(cluster.Name)

		case bfe_http.RespHeaderTimeoutError:
			request.ErrCode = bfe_basic.ErrBkRespHeaderTimeout
			request.ErrMsg = err.Error()
			p.proxyState.ErrBkRespHeaderTimeout.Inc(1)
			allowRetry = checkAllowRetry(cluster.RetryLevel(), outreq)
			backend.OnFail(cluster.Name)

		case bfe_http.TransportBrokenError:
			request.ErrCode = bfe_basic.ErrBkTransportBroken
			request.ErrMsg = err.Error()
			p.proxyState.ErrBkTransportBroken.Inc(1)
			allowRetry = checkAllowRetry(cluster.RetryLevel(), outreq)

		default:
			// never go here
			log.Logger.Info("roundtrip %s %s", reflect.TypeOf(err), err)
		}

		if !allowRetry {
			log.Logger.Debug("request fail, not retry now")
			p.proxyState.ClientReqFailWithNoRetry.Inc(1)
			break
		}

		request.RetryTime += 1
	}

	// have retry?
	if request.RetryTime > 0 {
		p.proxyState.ClientReqWithRetry.Inc(1)
	}
	// have cross-cluster retry?
	if request.Stat.IsCrossCluster {
		p.proxyState.ClientReqWithCrossRetry.Inc(1)
	}

	log.Logger.Debug("clusterInvoke %v %v", res, err)
	return
}

// sendResponse send http response to client.
func (p *ReverseProxy) sendResponse(rw bfe_http.ResponseWriter, res *bfe_http.Response,
	flushInterval time.Duration, cancelOnClientClose bool) error {
	// prepare SignCalculater for response
	p.prepareSigner(rw, res)

	bfe_http.CopyHeader(rw.Header(), res.Header)

	// note: writeheader don't guarantee send header
	rw.WriteHeader(res.StatusCode)

	err := p.copyResponse(rw, res.Body, flushInterval, cancelOnClientClose)
	res.Body.Close() // close now, instead of defer, to populate res.Trailer
	if err != nil {
		return err
	}

	if res.H2Trailer == nil {
		return nil
	}

	if len(*res.H2Trailer) > 0 {
		// Force chunking if we saw a response trailer.
		// This prevents net/http from calculating the length for short
		// bodies and adding a Content-Length.
		if fl, ok := rw.(bfe_http.Flusher); ok {
			fl.Flush()
		}
	}

	for k, vv := range *res.H2Trailer {
		k = TrailerPrefix + k
		for _, v := range vv {
			rw.Header().Add(k, v)
		}
	}
	return nil
}

// prepareSigner prepare SignCalculater for response.
func (p *ReverseProxy) prepareSigner(rw bfe_http.ResponseWriter, res *bfe_http.Response) {
	// not need to add signature for respsone
	if res.Signer == nil {
		return
	}

	// prepare Singer for signature
	if resp, ok := rw.(*response); ok {
		resp.SetSigner(res.Signer)
	}
}

// FinishReq should be invoked after quit ServHTTP().
func (p *ReverseProxy) FinishReq(rw bfe_http.ResponseWriter, request *bfe_basic.Request) (action int) {
	// get instance of BfeServer
	srv := p.server

	// desc connection num after request finish
	defer func() {
		// desc backend connection counter
		if request.Trans.Backend != nil {
			request.Trans.Backend.DecConnNum()
		}
	}()

	// Callback for HandleRequestFinish
	hl := srv.CallBacks.GetHandlerList(bfe_module.HandleRequestFinish)
	if hl != nil {
		retVal := hl.FilterResponse(request, request.HttpResponse)
		switch retVal {
		case bfe_module.BfeHandlerFinish:
			// close the connection after response
			action = closeAfterReply
			return
		}
	}

	return
}

func (p *ReverseProxy) setTimeout(stage bfe_basic.OperationStage,
	conn net.Conn, req *bfe_http.Request, d time.Duration) {
	switch b := req.Body.(type) {
	case *bfe_http2.RequestBody: // http2
		if stage == bfe_basic.StageReadReqBody {
			bfe_http2.SetReadStreamTimeout(b, d)
		}
		if stage == bfe_basic.StageWriteClient {
			bfe_http2.SetWriteStreamTimeout(b, d)
		}
		if stage == bfe_basic.StageEndRequest {
			bfe_http2.SetConnTimeout(b, d)
		}
	case *bfe_spdy.RequestBody: // spdy
		if stage == bfe_basic.StageReadReqBody {
			bfe_spdy.SetReadStreamTimeout(b, d)
		}
		if stage == bfe_basic.StageWriteClient {
			bfe_spdy.SetWriteStreamTimeout(b, d)
		}
		if stage == bfe_basic.StageEndRequest {
			bfe_spdy.SetConnTimeout(b, d)
		}
	default: // http
		if stage == bfe_basic.StageReadReqBody || stage == bfe_basic.StageEndRequest {
			conn.SetReadDeadline(time.Now().Add(d))
		}
		if stage == bfe_basic.StageWriteClient {
			conn.SetWriteDeadline(time.Now().Add(d))
		}
	}
}

func (p *ReverseProxy) setReadClientAgainTimeout(cluster *bfe_cluster.BfeCluster, conn net.Conn) {
	// for idle time + read next header time
	conn.SetReadDeadline(time.Now().Add(cluster.TimeoutReadClientAgain()))
}

// ServeHTTP processes http request and send http response.
//
// Params:
//    - rw : context for sending response
//    - request: context for request
//
// Return:
//    - action: action to do after ServeHTTP
func (p *ReverseProxy) ServeHTTP(rw bfe_http.ResponseWriter, basicReq *bfe_basic.Request) (action int) {
	var err error
	var res *bfe_http.Response
	var hl *bfe_module.HandlerList
	var retVal int
	var clusterName string
	var cluster *bfe_cluster.BfeCluster
	var outreq *bfe_http.Request
	var serverConf *bfe_route.ServerDataConf
	var writeTimer *time.Timer

	req := basicReq.HttpRequest
	isRedirect := false
	resFlushInterval := time.Duration(0)
	cancelOnClientClose := false

	timeoutWriteClient := time.Duration(cluster_conf.DefaultWriteClientTimeout) * time.Millisecond
	timeoutReadClientAgain := time.Duration(cluster_conf.DefaultReadClientAgainTimeout) * time.Millisecond

	// get instance of BfeServer
	srv := p.server

	// set clientip of original user for request
	setClientAddr(basicReq)

	// Callback for HandleBeforeLocation
	hl = srv.CallBacks.GetHandlerList(bfe_module.HandleBeforeLocation)
	if hl != nil {
		retVal, res = hl.FilterRequest(basicReq)
		basicReq.HttpResponse = res
		switch retVal {
		case bfe_module.BfeHandlerClose:
			// close the connection directly (with no response)
			action = closeDirectly
			return
		case bfe_module.BfeHandlerFinish:
			// close the connection after response
			action = closeAfterReply
			basicReq.BfeStatusCode = bfe_http.StatusInternalServerError
			return
		case bfe_module.BfeHandlerRedirect:
			// make redirect
			Redirect(rw, req, basicReq.Redirect.Url, basicReq.Redirect.Code, basicReq.Redirect.Header)
			isRedirect = true
			basicReq.BfeStatusCode = basicReq.Redirect.Code
			goto send_response
		case bfe_module.BfeHandlerResponse:
			goto response_got
		}
	}

	// find product
	if err := srv.findProduct(basicReq); err != nil {
		basicReq.ErrCode = bfe_basic.ErrBkFindProduct
		basicReq.ErrMsg = err.Error()
		p.proxyState.ErrBkFindProduct.Inc(1)
		log.Logger.Info("FindProduct error[%s] host[%s] vip[%s] clientip[%s]", err.Error(),
			basicReq.HttpRequest.Host, basicReq.Session.Vip, basicReq.ClientAddr)

		// close connection
		res = bfe_basic.CreateInternalSrvErrResp(basicReq)
		action = closeAfterReply
		goto response_got
	}

	// Callback for HandleFoundProduct
	hl = srv.CallBacks.GetHandlerList(bfe_module.HandleFoundProduct)
	if hl != nil {
		retVal, res = hl.FilterRequest(basicReq)
		basicReq.HttpResponse = res
		switch retVal {
		case bfe_module.BfeHandlerClose:
			// close the connection directly (with no response)
			action = closeDirectly
			return
		case bfe_module.BfeHandlerFinish:
			// close the connection after response
			action = closeAfterReply
			basicReq.BfeStatusCode = bfe_http.StatusInternalServerError
			return
		case bfe_module.BfeHandlerRedirect:
			// make redirect
			Redirect(rw, req, basicReq.Redirect.Url, basicReq.Redirect.Code, basicReq.Redirect.Header)
			isRedirect = true
			basicReq.BfeStatusCode = basicReq.Redirect.Code
			goto send_response
		case bfe_module.BfeHandlerResponse:
			goto response_got
		}
	}

	// find cluster
	if err = srv.findCluster(basicReq); err != nil {
		basicReq.ErrCode = bfe_basic.ErrBkFindLocation
		basicReq.ErrMsg = err.Error()
		p.proxyState.ErrBkFindLocation.Inc(1)
		log.Logger.Info("FindLocation error[%s] host[%s]", err, basicReq.HttpRequest.Host)

		// close connection
		res = bfe_basic.CreateInternalSrvErrResp(basicReq)
		action = closeAfterReply
		goto response_got
	}
	clusterName = basicReq.Route.ClusterName

	// look up for cluster
	serverConf = basicReq.SvrDataConf.(*bfe_route.ServerDataConf)
	cluster, err = serverConf.ClusterTable.Lookup(clusterName)
	if err != nil {
		log.Logger.Warn("no cluster for %s", clusterName)
		basicReq.Stat.ResponseStart = time.Now()
		basicReq.ErrCode = bfe_basic.ErrBkNoCluster
		basicReq.ErrMsg = err.Error()
		p.proxyState.ErrBkNoCluster.Inc(1)

		res = bfe_basic.CreateInternalSrvErrResp(basicReq)
		action = closeAfterReply
		goto response_got
	}

	basicReq.Backend.ClusterName = clusterName

	// set deadline to finish read client request body
	p.setTimeout(bfe_basic.StageReadReqBody, basicReq.Connection, req, cluster.TimeoutReadClient())
	resFlushInterval = cluster.ResFlushInterval()
	cancelOnClientClose = cluster.CancelOnClientClose()
	timeoutWriteClient = cluster.TimeoutWriteClient()
	timeoutReadClientAgain = cluster.TimeoutReadClientAgain()

	// Callback for HandleAfterLocation
	hl = srv.CallBacks.GetHandlerList(bfe_module.HandleAfterLocation)
	if hl != nil {
		retVal, res = hl.FilterRequest(basicReq)
		basicReq.HttpResponse = res
		switch retVal {
		case bfe_module.BfeHandlerClose:
			// close the connection directly (with no response)
			action = closeDirectly
			return
		case bfe_module.BfeHandlerFinish:
			// close the connection after response
			action = closeAfterReply
			basicReq.BfeStatusCode = bfe_http.StatusInternalServerError
			return
		case bfe_module.BfeHandlerRedirect:
			// make redirect
			Redirect(rw, req, basicReq.Redirect.Url, basicReq.Redirect.Code, basicReq.Redirect.Header)

			isRedirect = true

			basicReq.BfeStatusCode = basicReq.Redirect.Code
			goto send_response
		case bfe_module.BfeHandlerResponse:
			goto response_got
		}
	}

	if bfe_debug.DebugServHTTP {
		log.Logger.Debug("ReverseProxy.ServeHTTP(): cluster name = %s", clusterName)
	}

	// prepare out request to downstream RS backend
	outreq = new(bfe_http.Request)
	*outreq = *req // includes shallow copies of maps, but okay
	basicReq.OutRequest = outreq

	// set http proto for out request
	httpProtoSet(outreq)
	// remove hop-by-hop headers
	hopByHopHeaderRemove(outreq, req)

	// invoke cluster to get response
	res, action, err = p.clusterInvoke(srv, cluster, basicReq, rw)
	basicReq.HttpResponse = res

	// Note: The runtime will not GC the objects referenced by basicReq.SvrDataConf until the request
	// has been processed. But the request may last a long time. It's better to remove the reference
	// to objects which are not used any more.
	basicReq.SvrDataConf = nil

	if err != nil || res == nil {
		basicReq.Stat.ResponseStart = time.Now()
		basicReq.BfeStatusCode = bfe_http.StatusInternalServerError
		res = bfe_basic.CreateInternalSrvErrResp(basicReq)
		goto response_got
	}
	if resFlushInterval == 0 && basicReq.HttpRequest.Header.Get("Accept") == "text/event-stream" {
		resFlushInterval = cluster.DefaultSSEFlushInterval()
	}

response_got:
	// timeout for write response to client
	// Note: we use io.Copy() to read from backend and write to client.
	// For avoid from blocking on client conn or backend conn forever,
	// we must timeout both conns after specified duration.
	p.setTimeout(bfe_basic.StageWriteClient, basicReq.Connection, req, timeoutWriteClient)
	writeTimer = time.AfterFunc(timeoutWriteClient, func() {
		transport := basicReq.Trans.Transport.(*bfe_http.Transport)
		transport.CancelRequest(basicReq.OutRequest) // force close connection to backend
	})
	defer writeTimer.Stop()

	// for read next request
	defer p.setTimeout(bfe_basic.StageEndRequest, basicReq.Connection, req, timeoutReadClientAgain)

	defer res.Body.Close()

	// Callback for HandleReadResponse
	hl = srv.CallBacks.GetHandlerList(bfe_module.HandleReadResponse)
	if hl != nil {
		retVal = hl.FilterResponse(basicReq, res)
		switch retVal {
		case bfe_module.BfeHandlerFinish:
			// close the connection after response
			action = closeAfterReply
			basicReq.BfeStatusCode = bfe_http.StatusInternalServerError
			return
		case bfe_module.BfeHandlerRedirect:
			// make redirect
			Redirect(rw, req, basicReq.Redirect.Url, basicReq.Redirect.Code, basicReq.Redirect.Header)
			isRedirect = true
			basicReq.BfeStatusCode = basicReq.Redirect.Code
			goto send_response
		}
	}

send_response:
	// send http response to client
	basicReq.Stat.ResponseStart = time.Now()

	if !isRedirect && res != nil {
		err = p.sendResponse(rw, res, resFlushInterval, cancelOnClientClose)
		if err != nil {
			// Note: for h2/spdy protocol, not close client conn when send
			// response error. h2/spdy module will close conn/stream properly
			if !CheckSupportMultiplex(basicReq.Session.Proto) {
				action = closeAfterReply
			}
			basicReq.ErrCode = bfe_basic.ErrClientWrite
			basicReq.ErrMsg = err.Error()

			p.proxyState.ErrClientWrite.Inc(1)
		}
	}
	return
}

func (p *ReverseProxy) copyResponse(dst io.Writer, src io.ReadCloser,
	flushInterval time.Duration, cancelOnClientClose bool) error {

	// Note: When server is blocking on read from backend (eg. io.Copy(dst, src)),
	// if the client has disconnected, cancel the block operation immediately.
	//
	// Note: cancelOnClientClose feature must be enabled for AVS client (over http2)
	if cancelOnClientClose {
		if cn, ok := dst.(bfe_http.CloseNotifier); ok {
			cw := bfe_http.NewCloseWatcher(cn, func() {
				// Note: src is type of bfe_http.bodyEofSignal. Close() on src will
				// close the underlying connection if response not ready.
				// Duplicated Close() will be ignore.
				src.Close()
			})
			go cw.WatchLoop()
			defer cw.Stop()
		}
	}

	if flushInterval < 0 {
		if wf, ok := dst.(bfe_http.WriteFlusher); ok {
			// Note: Flush response header immediately
			if err := wf.Flush(); err != nil {
				return err
			}
			_, err := bfe_util.CopyWithoutBuffer(wf, src)
			return err
		}
	}

	if flushInterval > 0 {
		if wf, ok := dst.(bfe_http.WriteFlusher); ok {
			mlw := bfe_http.NewMaxLatencyWriter(wf, flushInterval, nil)
			go mlw.FlushLoop()
			defer mlw.Stop()
			dst = mlw
		}
	}

	buf := p.bufferPool.GetBlock()
	defer p.bufferPool.PutBlock(buf)

	_, err := io.CopyBuffer(dst, src, buf)
	return err
}

func checkAllowRetry(retryLevel int, outreq *bfe_http.Request) bool {
	if retryLevel == cluster_conf.RetryGet {
		// if forward GET request error (eg. backend restart)
		if outreq.Method == "GET" && checkRequestWithoutBody(outreq) {
			return true
		}
	}
	return false
}

// checkRequestWithoutBody check whether request without entity body.
func checkRequestWithoutBody(req *bfe_http.Request) bool {
	// Note: RFC 2616 doesn't explicitly permit nor forbid an
	// entity-body on a GET request
	if req.Body == nil || req.Body == bfe_http.EofReader {
		return true
	}
	if body, ok := req.Body.(*bfe_spdy.RequestBody); ok {
		return body.Eof()
	}
	return false
}

func checkBackendStatus(outlierDetectionLevel int, statusCode int) bool {
	return outlierDetectionLevel == cluster_conf.OutlierDetection5XX && statusCode/100 == 5
}

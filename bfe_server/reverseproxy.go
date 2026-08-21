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
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"math/rand"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"

	"github.com/bfenetworks/go-lib/log"

	bfe_cluster_backend "github.com/bfenetworks/bfe/bfe_balance/backend"
	"github.com/bfenetworks/bfe/bfe_balance/bal_gslb"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_debug"
	"github.com/bfenetworks/bfe/bfe_fcgi"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_http2"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_modules/mod_ai_token_auth"
	"github.com/bfenetworks/bfe/bfe_modules/mod_body_process"
	"github.com/bfenetworks/bfe/bfe_route"
	"github.com/bfenetworks/bfe/bfe_route/bfe_cluster"
	"github.com/bfenetworks/bfe/bfe_spdy"
	"github.com/bfenetworks/bfe/bfe_util"
	"github.com/bfenetworks/bfe/bfe_util/epp"
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
//
//	https://golang.org/pkg/net/http/#ResponseWriter
//	https://golang.org/pkg/net/http/#example_ResponseWriter_trailers
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

// compareHttpsConf compares two BackendHTTPS configurations and determines whether they are identical.
// Return:
// - bool: Returns true if all fields in src and dst are identical, otherwise false.
func compareHttpsConf(src, dst *cluster_conf.BackendHTTPS) bool {
	// Check if either src or dst is nil
	if src == nil || dst == nil {
		return src == dst // Both must be nil to be equal
	}

	// Compare RSHost
	if (src.RSHost == nil) != (dst.RSHost == nil) || (src.RSHost != nil && *src.RSHost != *dst.RSHost) {
		return false
	}

	// Compare RSInsecureSkipVerify
	if (src.RSInsecureSkipVerify == nil) != (dst.RSInsecureSkipVerify == nil) || (src.RSInsecureSkipVerify != nil && *src.RSInsecureSkipVerify != *dst.RSInsecureSkipVerify) {
		return false
	}

	// Compare RSCAList
	if (src.RSCAList == nil) != (dst.RSCAList == nil) {
		return false
	}
	if src.RSCAList != nil && dst.RSCAList != nil {
		if len(*src.RSCAList) != len(*dst.RSCAList) {
			return false
		}
		for i := range *src.RSCAList {
			if (*src.RSCAList)[i] != (*dst.RSCAList)[i] {
				return false
			}
		}
	}

	// Compare BFECertFile
	if (src.BFECertFile == nil) != (dst.BFECertFile == nil) || (src.BFECertFile != nil && *src.BFECertFile != *dst.BFECertFile) {
		return false
	}

	// Compare BFEKeyFile
	if (src.BFEKeyFile == nil) != (dst.BFEKeyFile == nil) || (src.BFEKeyFile != nil && *src.BFEKeyFile != *dst.BFEKeyFile) {
		return false
	}

	return true
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

			proto := "http"
			if t.HttpsConf != nil {
				proto = "https"
			}

			if (proto != *backendConf.Protocol) ||
				!compareHttpsConf(t.HttpsConf, conf.BackendHTTPSConf()) ||
				(t.MaxIdleConnsPerHost != *backendConf.MaxIdleConnsPerHost) ||
				(t.MaxConnsPerHost != *backendConf.MaxConnsPerHost) ||
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
	case "http", "https":
		// cluster has its own Connect Server Timeout.
		// so each cluster has a different transport
		// once cluster's timeout updated, dailer use new value
		dailer := func(network, add string) (net.Conn, error) {
			timeout := time.Duration(cluster.TimeoutConnSrv()) * time.Millisecond
			return net.DialTimeout(network, add, timeout)
		}

		transport := &bfe_http.Transport{
			Dial:                  dailer,
			DisableKeepAlives:     (*backendConf.MaxIdleConnsPerHost) == 0,
			MaxIdleConnsPerHost:   *backendConf.MaxIdleConnsPerHost,
			ResponseHeaderTimeout: time.Millisecond * time.Duration(*backendConf.TimeoutResponseHeader),
			ReqWriteBufferSize:    cluster.ReqWriteBufferSize(),
			ReqFlushInterval:      cluster.ReqFlushInterval(),
			DisableCompression:    true,
			MaxConnsPerHost:       *backendConf.MaxConnsPerHost,
		}
		if protocol == "https" {
			transport.SetHttpsConf(cluster.BackendHTTPSConf())
		}
		return transport
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
		clusterBackend = nil
		err = nil
		if bal.BalanceMode == cluster_conf.BalanceModeEPP {
			eppClient := request.GetContext(bal_gslb.REQ_CTX_EPP)
			if eppClient != nil {
				eppClient.(*epp.EppClient).Close()
				request.SetContext(bal_gslb.REQ_CTX_EPP, nil)
			}
			clusterBackend, err = bal.BalanceEpp(request)
		}
		if clusterBackend == nil {
			// get backend with cluster-name and request
			clusterBackend, err = bal.Balance(request)
			if err == bfe_basic.ErrBkCrossRetryBalance {
				request.RetryTime += 1
				continue
			}
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
			if checkBackendStatus(cluster.OutlierDetectionHttpCode(), res.StatusCode) {
				backend.OnFailByCluster(cluster)
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
		//     b. already write part of data
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
			backend.OnFailByCluster(cluster)

		case bfe_http.WriteRequestError, bfe_fcgi.WriteRequestError:
			var be *mod_body_process.BPError
			if errors.As(err, &be) {
				// body process error, no retry
				request.ErrCode = bfe_basic.ErrBkBodyProcess
				request.ErrMsg = err.Error()
				p.proxyState.ErrBkBodyProcess.Inc(1)
				allowRetry = false
				action = closeAfterReply
				break
			}

			request.ErrCode = bfe_basic.ErrBkWriteRequest
			request.ErrMsg = err.Error()
			p.proxyState.ErrBkWriteRequest.Inc(1)
			allowRetry = checkAllowRetry(cluster.RetryLevel(), outreq)

			// if error is caused by backend server
			rerr := err.(bfe_http.WriteRequestError)
			if !rerr.CheckTargetError(request.RemoteAddr) {
				backend.OnFailByCluster(cluster)
			}

		case bfe_http.ReadRespHeaderError, bfe_fcgi.ReadRespHeaderError:
			request.ErrCode = bfe_basic.ErrBkReadRespHeader
			request.ErrMsg = err.Error()
			p.proxyState.ErrBkReadRespHeader.Inc(1)
			allowRetry = checkAllowRetry(cluster.RetryLevel(), outreq)
			backend.OnFailByCluster(cluster)

		case bfe_http.RespHeaderTimeoutError:
			request.ErrCode = bfe_basic.ErrBkRespHeaderTimeout
			request.ErrMsg = err.Error()
			p.proxyState.ErrBkRespHeaderTimeout.Inc(1)
			allowRetry = checkAllowRetry(cluster.RetryLevel(), outreq)
			backend.OnFailByCluster(cluster)

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
	// prepare SignCalculator for response
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

// prepareSigner prepare SignCalculator for response.
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
		if d >= 0 {
			if stage == bfe_basic.StageReadReqBody {
				bfe_http2.SetReadStreamTimeout(b, d)
			}
			if stage == bfe_basic.StageWriteClient {
				bfe_http2.SetWriteStreamTimeout(b, d)
			}
			if stage == bfe_basic.StageEndRequest {
				bfe_http2.SetConnTimeout(b, d)
			}
		} else {
			//skip timeout setingg
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
		timeout := time.Time{} //no timeout
		if d >= 0 {
			timeout = time.Now().Add(d)
		} else {
			//skip timeout setingg
		}

		if stage == bfe_basic.StageReadReqBody || stage == bfe_basic.StageEndRequest {
			conn.SetReadDeadline(timeout)
		}
		if stage == bfe_basic.StageWriteClient {
			conn.SetWriteDeadline(timeout)
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
//   - rw : context for sending response
//   - request: context for request
//
// Return:
//   - action: action to do after ServeHTTP
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
	var ok bool
	var eppClient *epp.EppClient

	req := basicReq.HttpRequest
	isRedirect := false
	resFlushInterval := time.Duration(0)
	cancelOnClientClose := false

	timeoutReadClient := time.Duration(cluster_conf.DefaultReadClientTimeout) * time.Millisecond
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
			goto send_response
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
			goto send_response
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
	timeoutReadClient = cluster.TimeoutReadClient()
	resFlushInterval = cluster.ResFlushInterval()
	cancelOnClientClose = cluster.CancelOnClientClose()
	timeoutWriteClient = cluster.TimeoutWriteClient()
	timeoutReadClientAgain = cluster.TimeoutReadClientAgain()

	if basicReq.IsSse {
		timeoutReadClient = -1
		timeoutWriteClient = -1
		cancelOnClientClose = true
	}

	p.setTimeout(bfe_basic.StageReadReqBody, basicReq.Connection, req, timeoutReadClient)

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
			goto send_response
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

	if cluster.DisableHostHeader {
		// if cluster.DisableHostHeader is true, del outreq.Host
		outreq.Host = ""
	}

	/*
		// do body process before forwarding
		bf, ok = outreq.Body.(BufferFiller)
		if ok {
			// if body is BufferFiller, call FillBuffer to process body before forwarding
			for err == nil {
				err = bf.FillBuffer()
			}
			if err != io.EOF {
				basicReq.ErrCode = bfe_basic.ErrBkBodyProcess
				basicReq.ErrMsg = err.Error()

				p.proxyState.ErrBkBodyProcess.Inc(1)

				// close connection
				res = bfe_basic.CreateSpecifiedContentResp(basicReq, bfe_http.StatusBadRequest, "text/plain",
					fmt.Sprintf("Error %s: %s", basicReq.ErrCode.Error(), basicReq.ErrMsg))
				action = closeAfterReply
				goto send_response
			}
		}
	*/
	// invoke cluster to get response
	res, action, err = p.clusterInvoke(srv, cluster, basicReq, rw)
	basicReq.HttpResponse = res

	// Note: The runtime will not GC the objects referenced by basicReq.SvrDataConf until the request
	// has been processed. But the request may last a long time. It's better to remove the reference
	// to objects which are not used any more.
	basicReq.SvrDataConf = nil

	if err != nil || res == nil {
		eppclient := basicReq.GetContext(bal_gslb.REQ_CTX_EPP)
		if eppclient != nil {
			eppclient.(*epp.EppClient).Close()
			basicReq.SetContext(bal_gslb.REQ_CTX_EPP, nil)
		}

		basicReq.Stat.ResponseStart = time.Now()
		basicReq.BfeStatusCode = bfe_http.StatusInternalServerError
		res = bfe_basic.CreateInternalSrvErrResp(basicReq)
		goto response_got
	}
	if resFlushInterval == 0 && basicReq.HttpRequest.Header.Get("Accept") == "text/event-stream" {
		resFlushInterval = cluster.DefaultSSEFlushInterval()
	}

response_got:
	if res != nil && res.IsSse {
		if !basicReq.IsSse {
			timeoutReadClient = -1
			p.setTimeout(bfe_basic.StageReadReqBody, basicReq.Connection, req, timeoutReadClient)

			timeoutWriteClient = -1
			cancelOnClientClose = true
			basicReq.IsSse = true
		}
	}

	eppClient, ok = basicReq.GetContext(bal_gslb.REQ_CTX_EPP).(*epp.EppClient)
	if ok {
		basicReq.SetContext(bal_gslb.REQ_CTX_EPP, nil)
		eppClient.ProcRespHeader(res.Header, false)
		b := epp.NewEppResponseBodyFilter(res.Body, eppClient)
		res.Body = b
	}

	// timeout for write response to client
	// Note: we use io.Copy() to read from backend and write to client.
	// For avoid from blocking on client conn or backend conn forever,
	// we must timeout both conns after specified duration.
	p.setTimeout(bfe_basic.StageWriteClient, basicReq.Connection, req, timeoutWriteClient)
	writeTimer = time.AfterFunc(timeoutWriteClient, func() {
		if basicReq.Trans.Transport != nil {
			// TODO: process bfe_fcgi.Transport & bfe_http2.Transport
			switch t := basicReq.Trans.Transport.(type) {
			case *bfe_http.Transport:
				t.CancelRequest(req)
			default:
				// do nothing
			}
		}

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
			goto send_response
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

func checkBackendStatus(outlierDetectionHttpCodeStr string, statusCode int) bool {
	if outlierDetectionHttpCodeStr == "" {
		return false
	}
	for _, code := range strings.Split(outlierDetectionHttpCodeStr, "|") {
		switch code {
		case "3xx", "4xx", "5xx":
			if strconv.Itoa(statusCode/100) == code[0:1] {
				return true
			}
		default:
			codeInt, err := strconv.Atoi(code)
			if err != nil {
				continue
			}
			if codeInt == statusCode {
				return true
			}
		}
	}
	return false
}

type BufferFiller interface {
	FillBuffer() error
}

// aiTargetRand is used for weighted random target selection in AI gateway mode.
var aiTargetRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// SelectTarget selects a target based on weight.
func SelectTarget(targets []bfe_basic.AiRouteTarget) bfe_basic.AiRouteTarget {
	if len(targets) == 1 {
		return targets[0]
	}

	r := aiTargetRand.Intn(100)
	sum := 0
	for _, target := range targets {
		sum += target.Weight
		if r < sum {
			return target
		}
	}
	return targets[len(targets)-1]
}

type aiForwardAttempt struct {
	ClusterName string
	Model       string
	IsFallback  bool
}

// ServeHTTPForAI processes AI gateway http request and sends http response.
func (p *ReverseProxy) ServeHTTPForAI(rw bfe_http.ResponseWriter, basicReq *bfe_basic.Request) (action int) {
	var err error
	var res *bfe_http.Response
	var hl *bfe_module.HandlerList
	var retVal int
	var req *bfe_http.Request = basicReq.HttpRequest
	var serverConf *bfe_route.ServerDataConf
	var writeTimer *time.Timer
	var eppClient *epp.EppClient
	var ok bool

	// declare ai-related vars at top to avoid goto jumping over declarations
	var aiResult *bfe_basic.AiRouteResult
	var aiMeta *bfe_basic.AiBasicInfo
	var selectedTarget bfe_basic.AiRouteTarget
	var attempts []aiForwardAttempt
	var lastCluster *bfe_cluster.BfeCluster
	var invokeErr error

	isRedirect := false
	resFlushInterval := time.Duration(0)
	cancelOnClientClose := false

	timeoutReadClient := time.Duration(cluster_conf.DefaultReadClientTimeout) * time.Millisecond
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
			goto send_response
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
			goto send_response
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

	// AI Route Result Check
	aiResult = basicReq.GetAiRouteResult()
	if aiResult == nil {
		// AI gateway mode: no route hit, return 404
		basicReq.ErrCode = bfe_basic.ErrBkFindLocation
		basicReq.ErrMsg = "no ai route found"
		p.proxyState.ErrBkFindLocation.Inc(1)
		res = bfe_basic.CreateSpecifiedContentResp(basicReq, bfe_http.StatusNotFound,
			"text/plain", "AI route not found")
		action = closeAfterReply
		goto response_got
	}

	aiMeta = basicReq.GetAiBasicInfo()

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
			goto send_response
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

	// AI Forward Loop
	serverConf = basicReq.SvrDataConf.(*bfe_route.ServerDataConf)

	// weighted random select target
	if len(aiResult.Targets) > 0 {
		selectedTarget = SelectTarget(aiResult.Targets)
	}

	// build attempt list: selected target + fallbacks
	attempts = make([]aiForwardAttempt, 0, 1+len(aiResult.Fallbacks))
	if selectedTarget.ClusterName != "" {
		attempts = append(attempts, aiForwardAttempt{
			ClusterName: selectedTarget.ClusterName,
			Model:       selectedTarget.Model,
			IsFallback:  false,
		})
	}
	for _, fb := range aiResult.Fallbacks {
		attempts = append(attempts, aiForwardAttempt{
			ClusterName: fb.ClusterName,
			Model:       fb.Model,
			IsFallback:  true,
		})
	}

	// ensure request body is rewindable before attempting fallbacks
	if len(attempts) > 1 && basicReq.HttpRequest.Body != nil {
		if !prepareRequestBodyForRetry(basicReq.HttpRequest) {
			log.Logger.Warn("ServeHTTPForAI: request body is not rewindable, disable fallback")
			attempts = attempts[:1]
		}
	}

	for i, attempt := range attempts {
		if i > 0 {
			// fallback attempt: reset request state
			if !p.resetRequestForRetry(basicReq) {
				log.Logger.Warn("ServeHTTPForAI: fallback aborted, request body cannot be rewound")
				break
			}
		}

		res, action, lastCluster, invokeErr = p.aiClusterInvoke(srv, serverConf, basicReq, rw, attempt, aiMeta)
		if invokeErr == nil && res != nil && res.StatusCode < 400 {
			// success: 2xx/3xx, stop fallback loop
			break
		}

		// decide whether to try next fallback
		if i == len(attempts)-1 {
			// last attempt
			break
		}
		if !shouldTriggerFallback(res, invokeErr) {
			break
		}

		// log fallback
		log.Logger.Info("ServeHTTPForAI: fallback triggered, cluster[%s] err[%v] status[%d]",
			attempt.ClusterName, invokeErr, getResponseStatus(res))

		if res != nil {
			res.Body.Close()
		}
	}

	basicReq.HttpResponse = res

	// Note: The runtime will not GC the objects referenced by basicReq.SvrDataConf until the request
	// has been processed. But the request may last a long time. It's better to remove the reference
	// to objects which are not used any more.
	basicReq.SvrDataConf = nil

	if err != nil || res == nil {
		eppclient := basicReq.GetContext(bal_gslb.REQ_CTX_EPP)
		if eppclient != nil {
			eppclient.(*epp.EppClient).Close()
			basicReq.SetContext(bal_gslb.REQ_CTX_EPP, nil)
		}

		basicReq.Stat.ResponseStart = time.Now()
		basicReq.BfeStatusCode = bfe_http.StatusInternalServerError
		res = bfe_basic.CreateInternalSrvErrResp(basicReq)
		goto response_got
	}

	// set response-phase timeouts based on the last cluster used
	if lastCluster != nil {
		resFlushInterval = lastCluster.ResFlushInterval()
		cancelOnClientClose = lastCluster.CancelOnClientClose()
		timeoutWriteClient = lastCluster.TimeoutWriteClient()
		timeoutReadClientAgain = lastCluster.TimeoutReadClientAgain()
	}
	if resFlushInterval == 0 && basicReq.HttpRequest.Header.Get("Accept") == "text/event-stream" {
		if lastCluster != nil {
			resFlushInterval = lastCluster.DefaultSSEFlushInterval()
		}
	}

response_got:
	if res != nil && res.IsSse {
		timeoutReadClient = -1
		p.setTimeout(bfe_basic.StageReadReqBody, basicReq.Connection, req, timeoutReadClient)

		timeoutWriteClient = -1
		cancelOnClientClose = true
		basicReq.IsSse = true
	}

	eppClient, ok = basicReq.GetContext(bal_gslb.REQ_CTX_EPP).(*epp.EppClient)
	if ok {
		basicReq.SetContext(bal_gslb.REQ_CTX_EPP, nil)
		eppClient.ProcRespHeader(res.Header, false)
		b := epp.NewEppResponseBodyFilter(res.Body, eppClient)
		res.Body = b
	}

	// timeout for write response to client
	// Note: we use io.Copy() to read from backend and write to client.
	// For avoid from blocking on client conn or backend conn forever,
	// we must timeout both conns after specified duration.
	p.setTimeout(bfe_basic.StageWriteClient, basicReq.Connection, req, timeoutWriteClient)
	writeTimer = time.AfterFunc(timeoutWriteClient, func() {
		if basicReq.Trans.Transport != nil {
			// TODO: process bfe_fcgi.Transport & bfe_http2.Transport
			switch t := basicReq.Trans.Transport.(type) {
			case *bfe_http.Transport:
				t.CancelRequest(req)
			default:
				// do nothing
			}
		}

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
			goto send_response
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

// stripProviderPrefix strips the configured provider/model prefix from model.
// It returns the stripped model and true when stripping succeeds; otherwise it
// returns the original model and false.
func stripProviderPrefix(model string, matchPrefix string) (string, bool) {
	if model == "" || !strings.HasPrefix(model, matchPrefix) {
		return model, false
	}

	stripped := strings.TrimPrefix(model, matchPrefix)
	if stripped == "" {
		log.Logger.Warn("Model %s stripped by prefix %s results in empty model, skip stripping",
			model, matchPrefix)
		return model, false
	}

	return stripped, true
}

// doSingleAIForward performs a single AI forward attempt with the given key.
func (p *ReverseProxy) doSingleAIForward(srv *BfeServer, cluster *bfe_cluster.BfeCluster,
	basicReq *bfe_basic.Request, rw bfe_http.ResponseWriter,
	attempt aiForwardAttempt, aiMeta *bfe_basic.AiBasicInfo,
	selectedKey cluster_conf.AIKey) (
	res *bfe_http.Response, action int, err error) {

	req := basicReq.HttpRequest

	// prepare out request to downstream RS backend
	outreq := new(bfe_http.Request)
	*outreq = *req // includes shallow copies of maps, but okay
	basicReq.OutRequest = outreq

	// set http proto for out request
	httpProtoSet(outreq)
	// remove hop-by-hop headers
	hopByHopHeaderRemove(outreq, req)

	if cluster.DisableHostHeader {
		// if cluster.DisableHostHeader is true, del outreq.Host
		outreq.Host = ""
	}

	// Calculate the final model in order: route target/fallback override ->
	// provider/model prefix stripping -> cluster model mapping. Then write it
	// to the request body at most once to avoid repeated JSON parsing/serialization.
	// Always start from ClientModel so that each cluster attempt is independent;
	// otherwise the previous attempt's TargetModel would leak into the next
	// fallback/retry when the next cluster has no model override/mapping of its own.
	model := aiMeta.ClientModel

	// apply model override from ai route target/fallback
	if attempt.Model != "" {
		model = attempt.Model
	}

	// strip provider/model prefix according to cluster AIConf
	if cluster.AIConf != nil && cluster.AIConf.StripPrefix && cluster.AIConf.MatchPrefix != "" {
		if stripped, ok := stripProviderPrefix(model, cluster.AIConf.MatchPrefix); ok {
			model = stripped
		}
	}

	// apply cluster model mapping
	if cluster.AIConf != nil && cluster.AIConf.ModelMapping != nil && model != "" {
		if newModel, ok := (*cluster.AIConf.ModelMapping)[model]; ok {
			model = newModel
		}
	}

	if model != aiMeta.ClientModel {
		// Need to rewrite the body. Isolate outreq.Body from req.Body so that
		// the rewrite does not leak into the next fallback/retry attempt.
		// bytes_body.Rewind() only resets the read position and does not restore
		// the original content modified by ReqBodyJsonSet.
		if req.Body != nil {
			if bodyAccessor, err := req.GetBodyAccessor(); err != nil {
				log.Logger.Warn("doSingleAIForward: failed to get body accessor: %s", err)
			} else if bodyAccessor != nil {
				bodyBytes, all := bodyAccessor.GetBytes()
				if !all {
					log.Logger.Warn("doSingleAIForward: request body not fully buffered, model rewrite may leak between attempts")
				} else {
					newBody, err := bfe_http.NewBytesBody(io.NopCloser(bytes.NewReader(bodyBytes)), int64(len(bodyBytes)))
					if err != nil {
						log.Logger.Warn("doSingleAIForward: failed to copy request body: %s", err)
					} else {
						outreq.Body = newBody
					}
				}
			}
		}

		if err := condition.ReqBodyJsonSet(basicReq, "model", model); err != nil {
			log.Logger.Warn("Failed to set model in request body: %s", err)
		} else {
			// outreq body already changed, need reset Content-Length
			if outreq.ContentLength >= 0 {
				outreq.ContentLength = -1
				outreq.Header.Del("Content-Length")
			}
			// Also reset the original request's Content-Length so that fallback/retry
			// creates a new outreq with consistent body length.
			if basicReq.HttpRequest != nil && basicReq.HttpRequest.ContentLength >= 0 {
				basicReq.HttpRequest.ContentLength = -1
				basicReq.HttpRequest.Header.Del("Content-Length")
			}
			aiMeta.TargetModel = model
		}
	}

	// apply cluster.AIConf (api key, provider, cost currency)
	if cluster.AIConf != nil {
		if cluster.AIConf.Provider != "" {
			aiMeta.Provider = cluster.AIConf.Provider
		}
		if cluster.AIConf.ModelTable != nil && cluster.AIConf.ModelTable.Currency != "" {
			aiMeta.CostCurrency = cluster.AIConf.ModelTable.Currency
		}
		aiMeta.AppendClusterKeyName(cluster.Name, selectedKey.Name)
		if selectedKey.Key != "" {
			mod_ai_token_auth.SetApiKey(outreq, selectedKey.Key)
		}
	}

	// invoke cluster to get response
	return p.clusterInvoke(srv, cluster, basicReq, rw)
}

func (p *ReverseProxy) aiClusterInvoke(srv *BfeServer, serverConf *bfe_route.ServerDataConf,
	basicReq *bfe_basic.Request, rw bfe_http.ResponseWriter,
	attempt aiForwardAttempt, aiMeta *bfe_basic.AiBasicInfo) (
	res *bfe_http.Response, action int, cluster *bfe_cluster.BfeCluster, err error) {

	req := basicReq.HttpRequest

	// update route info
	basicReq.Route.ClusterName = attempt.ClusterName
	basicReq.Backend.ClusterName = attempt.ClusterName

	// look up for cluster
	cluster, err = serverConf.ClusterTable.Lookup(attempt.ClusterName)
	if err != nil {
		log.Logger.Warn("no cluster for %s", attempt.ClusterName)
		basicReq.Stat.ResponseStart = time.Now()
		basicReq.ErrCode = bfe_basic.ErrBkNoCluster
		basicReq.ErrMsg = err.Error()
		p.proxyState.ErrBkNoCluster.Inc(1)
		return nil, closeAfterReply, nil, err
	}

	// set deadline to finish read client request body
	timeoutReadClient := cluster.TimeoutReadClient()

	if basicReq.IsSse {
		timeoutReadClient = -1
	}

	p.setTimeout(bfe_basic.StageReadReqBody, basicReq.Connection, req, timeoutReadClient)

	// no api keys configured, skip key injection
	if cluster.AIConf == nil || len(cluster.AIConf.Keys) == 0 {
		res, action, err = p.doSingleAIForward(srv, cluster, basicReq, rw, attempt, aiMeta, cluster_conf.AIKey{})
		return res, action, cluster, err
	}

	policy := defaultAIKeyPolicy()
	if cluster.AIConf.KeyPolicy != nil {
		policy = *cluster.AIConf.KeyPolicy
	}

	keys := cluster.AIConf.Keys

	// ensure request body is rewindable when key-level retry is possible
	keyRetryEnabled := policy.MaxRetries > 0
	if keyRetryEnabled {
		if !prepareRequestBodyForRetry(basicReq.HttpRequest) {
			log.Logger.Warn("aiClusterInvoke: request body is not rewindable, disable key-level retry for cluster[%s]",
				attempt.ClusterName)
			keyRetryEnabled = false
			policy.MaxRetries = 0
		}
	}

	state := newAIKeyAttemptState()

	var lastErr error
	var idx int
	var key cluster_conf.AIKey
	var ok bool
	keepKey := false
	for retry := 0; retry <= policy.MaxRetries; retry++ {
		if retry > 0 {
			if aiMeta != nil {
				aiMeta.IncrementRetryCount()
			}
			// rewind body before retrying with another key
			if !rewindRequestBody(basicReq.HttpRequest) {
				log.Logger.Warn("aiClusterInvoke: failed to rewind request body, abort key-level retry for cluster[%s]",
					attempt.ClusterName)
				break
			}
			backoff := calcBackoff(policy.RetryBackoffInitial, policy.RetryBackoffMax, retry)
			time.Sleep(backoff)
		}

		if !keepKey {
			idx, key, ok = chooseNextAIKey(keys, state)
			if !ok {
				log.Logger.Warn("aiClusterInvoke: all ai keys exhausted for cluster[%s]", attempt.ClusterName)
				break
			}

			log.Logger.Info("aiClusterInvoke: select ai key [name=%s weight=%d] for cluster[%s]",
				key.Name, key.Weight, attempt.ClusterName)
		}
		keepKey = false

		res, action, err = p.doSingleAIForward(srv, cluster, basicReq, rw, attempt, aiMeta, key)

		lastErr = err
		statusCode := 0
		if res != nil {
			statusCode = res.StatusCode
		}

		// success: stop key-level retry
		if err == nil && statusCode < 400 {
			return res, action, cluster, nil
		}

		// classify failure
		switch {
		case statusCode == 429:
			// rate limit: mark key as used and rotate to another key
			state.usedSet[idx] = struct{}{}
			log.Logger.Info("aiClusterInvoke: ai key [name=%s] rate limited (429), rotate", key.Name)
		case statusCode == 401 || statusCode == 402 || statusCode == 403:
			// auth failure: mark key as dead
			state.deadSet[idx] = struct{}{}
			log.Logger.Info("aiClusterInvoke: ai key [name=%s] auth failed (%d), dead", key.Name, statusCode)
		case statusCode >= 500 || err != nil:
			// transient server failure or connection error:
			// keep current key selected for next retry (with backoff)
			keepKey = true
			log.Logger.Info("aiClusterInvoke: ai key [name=%s] transient failure [status=%d err=%v], retry same key",
				key.Name, statusCode, err)
		default:
			// other 4xx client errors (e.g. 400, 404): stop key-level retry
			return res, action, cluster, nil
		}
	}

	return res, action, cluster, lastErr
}

// aiFallbackStatusCodes defines the 4xx status codes that should trigger
// cluster-level fallback by default. 5xx is handled uniformly by code >= 500.
// This matches DeepSeek issue #1317 requirements (400/401/402/422/429) and
// aligns with Bifrost's built-in status code classification.
var aiFallbackStatusCodes = map[int]struct{}{
	400: {},
	401: {},
	402: {},
	403: {},
	422: {},
	429: {},
}

func shouldTriggerFallback(res *bfe_http.Response, err error) bool {
	if err != nil {
		return true
	}
	code := getResponseStatus(res)
	if code >= 500 {
		return true
	}
	if _, ok := aiFallbackStatusCodes[code]; ok {
		return true
	}
	return false
}

func getResponseStatus(res *bfe_http.Response) int {
	if res == nil {
		return 0
	}
	return res.StatusCode
}

func (p *ReverseProxy) resetRequestForRetry(basicReq *bfe_basic.Request) bool {
	// desc backend connection counter
	if basicReq.Trans.Backend != nil {
		basicReq.Trans.Backend.DecConnNum()
		basicReq.Trans.Backend = nil
	}
	basicReq.Trans.Transport = nil
	basicReq.RetryTime = 0

	// reset out request so body can be re-read
	basicReq.OutRequest = nil

	// rewind request body for next fallback attempt
	if !rewindRequestBody(basicReq.HttpRequest) {
		return false
	}

	// reset Content-Length so that the next outreq is created with a length
	// consistent with the current (possibly modified) body.
	if basicReq.HttpRequest.ContentLength >= 0 {
		basicReq.HttpRequest.ContentLength = -1
		basicReq.HttpRequest.Header.Del("Content-Length")
	}

	// clear error info from previous attempt
	basicReq.ErrCode = nil
	basicReq.ErrMsg = ""
	return true
}

// prepareRequestBodyForRetry makes the request body rewindable for fallback.
// If the body already implements Rewindable, it returns true directly.
// Otherwise, it tries to convert the body to bytes_body via GetBodyAccessor.
// It rejects wrapping when the total bytes_body buffer size reaches the limit.
func prepareRequestBodyForRetry(req *bfe_http.Request) bool {
	// if total buffer size already reaches the limit, do not wrap (no retry)
	if limit := bfe_http.TotalBodyBufferSizeLimit(); limit > 0 {
		if bfe_http.TotalBytesBodyBuffer() >= limit {
			return false
		}
	}
	if req.Body == nil {
		return true
	}
	if _, ok := req.Body.(bfe_http.Rewindable); ok {
		return true
	}
	if _, err := req.GetBodyAccessor(); err != nil {
		return false
	}
	_, ok := req.Body.(bfe_http.Rewindable)
	return ok
}

// rewindRequestBody rewinds the request body to the beginning.
// It assumes the body already implements Rewindable.
func rewindRequestBody(req *bfe_http.Request) bool {
	if req.Body == nil {
		return true
	}
	rewindable, ok := req.Body.(bfe_http.Rewindable)
	if !ok {
		return false
	}
	return rewindable.Rewind()
}

// aiKeyAttemptState tracks key usage within one aiClusterInvoke call.
type aiKeyAttemptState struct {
	usedSet map[int]struct{} // index of keys used for 429 in this request
	deadSet map[int]struct{} // index of keys dead for 401/402/403 in this request
}

func newAIKeyAttemptState() *aiKeyAttemptState {
	return &aiKeyAttemptState{
		usedSet: make(map[int]struct{}),
		deadSet: make(map[int]struct{}),
	}
}

// aiKeyRand is used for weighted random AI key selection.
var aiKeyRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// selectAIKey selects one key by weighted random.
// keys should have weight > 0 and total weight > 0.
func selectAIKey(keys []cluster_conf.AIKey) (cluster_conf.AIKey, int) {
	if len(keys) == 1 {
		return keys[0], 0
	}

	total := 0
	for _, k := range keys {
		total += k.Weight
	}
	if total <= 0 {
		return cluster_conf.AIKey{}, -1
	}

	r := aiKeyRand.Intn(total)
	sum := 0
	for i, k := range keys {
		sum += k.Weight
		if r < sum {
			return k, i
		}
	}
	return keys[len(keys)-1], len(keys) - 1
}

// chooseNextAIKey returns next eligible key and its index.
// If all keys are dead, returns (-1, empty key, false).
// If all alive keys are in used_set, clears used_set and retries.
func chooseNextAIKey(keys []cluster_conf.AIKey, state *aiKeyAttemptState) (int, cluster_conf.AIKey, bool) {
	var eligible []cluster_conf.AIKey
	var indices []int

	for i, k := range keys {
		if k.Weight == 0 {
			continue
		}
		if _, dead := state.deadSet[i]; dead {
			continue
		}
		eligible = append(eligible, k)
		indices = append(indices, i)
	}

	if len(eligible) == 0 {
		return -1, cluster_conf.AIKey{}, false
	}

	// filter out used_set keys
	var filteredKeys []cluster_conf.AIKey
	var filteredIdx []int
	for j, k := range eligible {
		idx := indices[j]
		if _, used := state.usedSet[idx]; used {
			continue
		}
		filteredKeys = append(filteredKeys, k)
		filteredIdx = append(filteredIdx, idx)
	}

	if len(filteredKeys) == 0 {
		// all alive keys used (429 only), reset used_set and try again
		state.usedSet = make(map[int]struct{})
		filteredKeys = eligible
		filteredIdx = indices
	}

	_, selectedIdx := selectAIKey(filteredKeys)
	if selectedIdx < 0 {
		return -1, cluster_conf.AIKey{}, false
	}
	return filteredIdx[selectedIdx], filteredKeys[selectedIdx], true
}

// calcBackoff calculates exponential backoff with jitter.
func calcBackoff(initial, max, attempt int) time.Duration {
	backoff := initial
	for i := 1; i < attempt; i++ {
		backoff *= 2
		if backoff > max {
			backoff = max
			break
		}
	}
	// add jitter (±20%)
	jitter := backoff / 5
	if jitter > 0 {
		backoff = backoff - jitter/2 + aiKeyRand.Intn(jitter)
	}
	return time.Duration(backoff) * time.Millisecond
}

// defaultAIKeyPolicy returns the default key policy.
func defaultAIKeyPolicy() cluster_conf.AIKeyPolicy {
	return cluster_conf.AIKeyPolicy{
		Strategy:            "weighted_random",
		MaxRetries:          0,
		RetryBackoffInitial: 500,
		RetryBackoffMax:     5000,
	}
}

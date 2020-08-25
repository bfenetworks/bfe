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

// proxy internal status

package bfe_server

import (
	"github.com/baidu/go-lib/web-monitor/metrics"
)

type ProxyState struct {
	// panic
	PanicClientConnServe *metrics.Counter // panic when accept from client
	PanicBackendWrite    *metrics.Counter // panic when write to backend
	PanicBackendRead     *metrics.Counter // panic when read from backend

	// client side errors
	ErrClientLongUrl        *metrics.Counter
	ErrClientLongHeader     *metrics.Counter
	ErrClientClose          *metrics.Counter
	ErrClientTimeout        *metrics.Counter
	ErrClientBadRequest     *metrics.Counter
	ErrClientZeroContentlen *metrics.Counter
	ErrClientExpectFail     *metrics.Counter
	ErrClientConnAccept     *metrics.Counter
	ErrClientWrite          *metrics.Counter
	ErrClientReset          *metrics.Counter

	// route config errors
	ErrBkFindProduct  *metrics.Counter
	ErrBkFindLocation *metrics.Counter
	ErrBkNoBalance    *metrics.Counter
	ErrBkNoCluster    *metrics.Counter

	// backend side errors
	ErrBkConnectBackend    *metrics.Counter
	ErrBkRequestBackend    *metrics.Counter
	ErrBkWriteRequest      *metrics.Counter
	ErrBkReadRespHeader    *metrics.Counter
	ErrBkRespHeaderTimeout *metrics.Counter
	ErrBkTransportBroken   *metrics.Counter

	// tls handshake
	TlsHandshakeAll  *metrics.Counter
	TlsHandshakeSucc *metrics.Counter

	// tls session cache
	SessionCacheConn         *metrics.Counter
	SessionCacheConnFail     *metrics.Counter
	SessionCacheSet          *metrics.Counter
	SessionCacheSetFail      *metrics.Counter
	SessionCacheGet          *metrics.Counter
	SessionCacheGetFail      *metrics.Counter
	SessionCacheTypeNotBytes *metrics.Counter
	SessionCacheMiss         *metrics.Counter
	SessionCacheHit          *metrics.Counter
	SessionCacheNoInstance   *metrics.Counter

	// tls multiply certificates
	TlsMultiCertGet            *metrics.Counter
	TlsMultiCertConnWithoutVip *metrics.Counter
	TlsMultiCertConnVipUnknown *metrics.Counter
	TlsMultiCertConnWithoutSni *metrics.Counter
	TlsMultiCertUseDefault     *metrics.Counter
	TlsMultiCertUpdate         *metrics.Counter
	TlsMultiCertUpdateErr      *metrics.Counter

	// client side
	ClientReqWithRetry       *metrics.Counter // req served with retry
	ClientReqWithCrossRetry  *metrics.Counter // req served with cross cluster retry
	ClientReqFail            *metrics.Counter // req with ErrCode != nil
	ClientReqFailWithNoRetry *metrics.Counter // req fail with no retry
	ClientConnUse100Continue *metrics.Counter // connection used Expect 100 Continue
	ClientConnUnfinishedReq  *metrics.Counter // connection closed with unfinished request

	// request successful received
	ClientReqServed      *metrics.Counter
	HttpClientReqServed  *metrics.Counter
	HttpsClientReqServed *metrics.Counter
	Http2ClientReqServed *metrics.Counter
	SpdyClientReqServed  *metrics.Counter

	// active request
	ClientReqActive      *metrics.Gauge
	HttpClientReqActive  *metrics.Gauge
	HttpsClientReqActive *metrics.Gauge
	Http2ClientReqActive *metrics.Gauge
	SpdyClientReqActive  *metrics.Gauge

	// connection successful accepted
	ClientConnServed       *metrics.Counter
	HttpClientConnServed   *metrics.Counter
	HttpsClientConnServed  *metrics.Counter
	Http2ClientConnServed  *metrics.Counter
	SpdyClientConnServed   *metrics.Counter
	StreamClientConnServed *metrics.Counter
	WsClientConnServed     *metrics.Counter
	WssClientConnServed    *metrics.Counter

	// active connection
	ClientConnActive       *metrics.Gauge
	HttpClientConnActive   *metrics.Gauge
	HttpsClientConnActive  *metrics.Gauge
	Http2ClientConnActive  *metrics.Gauge
	SpdyClientConnActive   *metrics.Gauge
	StreamClientConnActive *metrics.Gauge
	WsClientConnActive     *metrics.Gauge
	WssClientConnActive    *metrics.Gauge
}

func (s *ProxyState) ClientConnServedInc(proto string, value uint) {
	switch proto {
	case "http":
		s.HttpClientConnServed.Inc(value)
	case "https":
		s.HttpsClientConnServed.Inc(value)
	case "h2":
		s.Http2ClientConnServed.Inc(value)
	case "spdy/3.1":
		s.SpdyClientConnServed.Inc(value)
	case "ws":
		s.WsClientConnServed.Inc(value)
	case "wss":
		s.WssClientConnServed.Inc(value)
	case "stream":
		s.StreamClientConnServed.Inc(value)
	}
	s.ClientConnServed.Inc(value)
}

func (s *ProxyState) ClientConnActiveInc(proto string, value uint) {
	switch proto {
	case "http":
		s.HttpClientConnActive.Inc(value)
	case "https":
		s.HttpsClientConnActive.Inc(value)
	case "h2":
		s.Http2ClientConnActive.Inc(value)
	case "spdy/3.1":
		s.SpdyClientConnActive.Inc(value)
	case "ws":
		s.WsClientConnActive.Inc(value)
	case "wss":
		s.WssClientConnActive.Inc(value)
	case "stream":
		s.StreamClientConnActive.Inc(value)
	}
	s.ClientConnActive.Inc(value)
}

func (s *ProxyState) ClientConnActiveDec(proto string, value uint) {
	switch proto {
	case "http":
		s.HttpClientConnActive.Dec(value)
	case "https":
		s.HttpsClientConnActive.Dec(value)
	case "h2":
		s.Http2ClientConnActive.Dec(value)
	case "spdy/3.1":
		s.SpdyClientConnActive.Dec(value)
	case "ws":
		s.WsClientConnActive.Dec(value)
	case "wss":
		s.WssClientConnActive.Dec(value)
	case "stream":
		s.StreamClientConnActive.Dec(value)
	}
	s.ClientConnActive.Dec(value)
}

func (s *ProxyState) ClientReqServedInc(proto string, value uint) {
	switch proto {
	case "http":
		s.HttpClientReqServed.Inc(value)
	case "https":
		s.HttpsClientReqServed.Inc(value)
	case "h2":
		s.Http2ClientReqServed.Inc(value)
	case "spdy/3.1":
		s.SpdyClientReqServed.Inc(value)
	}
	s.ClientReqServed.Inc(value)
}

func (s *ProxyState) ClientReqActiveInc(proto string, value uint) {
	switch proto {
	case "http":
		s.HttpClientReqActive.Inc(value)
	case "https":
		s.HttpsClientReqActive.Inc(value)
	case "h2":
		s.Http2ClientReqActive.Inc(value)
	case "spdy/3.1":
		s.SpdyClientReqActive.Inc(value)
	}
	s.ClientReqActive.Inc(value)
}

func (s *ProxyState) ClientReqActiveDec(proto string, value uint) {
	switch proto {
	case "http":
		s.HttpClientReqActive.Dec(value)
	case "https":
		s.HttpsClientReqActive.Dec(value)
	case "h2":
		s.Http2ClientReqActive.Dec(value)
	case "spdy/3.1":
		s.SpdyClientReqActive.Dec(value)
	}
	s.ClientReqActive.Dec(value)
}

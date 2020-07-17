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

package bfe_basic

import (
	"errors"
)

var (
	// client error
	ErrClientTlsHandshake   = errors.New("CLIENT_TLS_HANDSHAKE")   // tls handshake error
	ErrClientWrite          = errors.New("CLIENT_WRITE")           // write client error
	ErrClientClose          = errors.New("CLIENT_CLOSE")           // close by peer
	ErrClientLongHeader     = errors.New("CLIENT_LONG_HEADER")     // req too long header
	ErrClientLongUrl        = errors.New("CLIENT_LONG_URL")        // req too long url
	ErrClientTimeout        = errors.New("CLIENT_TIMEOUT")         // timeout
	ErrClientBadRequest     = errors.New("CLIENT_BAD_REQUEST")     // bad request
	ErrClientZeroContentlen = errors.New("CLIENT_ZERO_CONTENTLEN") // zero content length
	ErrClientExpectFail     = errors.New("CLIENT_EXPECT_FAIL")     // expect fail
	ErrClientReset          = errors.New("CLIENT_RESET")           // client reset connection
	ErrClientFrame          = errors.New("CLIENT_LONG_FRAME")      // only used for spdy/http2

	// backend error
	ErrBkFindProduct       = errors.New("BK_FIND_PRODUCT")         // fail to find product
	ErrBkFindLocation      = errors.New("BK_FIND_LOCATION")        // fail to find location
	ErrBkNoCluster         = errors.New("BK_NO_CLUSTER")           // no cluster found
	ErrBkNoSubCluster      = errors.New("BK_NO_SUB_CLUSTER")       // no sub-cluster found
	ErrBkNoBackend         = errors.New("BK_NO_BACKEND")           // no backend found
	ErrBkRequestBackend    = errors.New("BK_REQUEST_BACKEND")      // forward request to backend error
	ErrBkConnectBackend    = errors.New("BK_CONNECT_BACKEND")      // connect backend error
	ErrBkWriteRequest      = errors.New("BK_WRITE_REQUEST")        // write request error (caused by bk or client)
	ErrBkReadRespHeader    = errors.New("BK_READ_RESP_HEADER")     // read response error
	ErrBkRespHeaderTimeout = errors.New("BK_RESP_HEADER_TIMEOUT")  // read response timeout
	ErrBkTransportBroken   = errors.New("BK_TRANSPORT_BROKEN")     // conn broken
	ErrBkRetryTooMany      = errors.New("BK_RETRY_TOOMANY")        // reach retry max
	ErrBkNoSubClusterCross = errors.New("BK_NO_SUB_CLUSTER_CROSS") // no sub-cluster found
	ErrBkCrossRetryBalance = errors.New("BK_CROSS_RETRY_BALANCE")  // cross retry balance failed

	// GSLB error
	ErrGslbBlackhole = errors.New("GSLB_BLACKHOLE") // deny by blackhole
)

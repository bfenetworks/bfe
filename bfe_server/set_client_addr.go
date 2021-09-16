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

package bfe_server

import (
	"net"
	"strconv"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
)

// setClientAddr set real client addr from headers.
func setClientAddr(req *bfe_basic.Request) {
	// use remote addr
	if !req.Session.TrustSource() { // request not from upstream bfe server
		req.ClientAddr = req.RemoteAddr
		return
	}

	req.ClientAddr = nil
	clientip := req.HttpRequest.Header.Get(bfe_basic.HeaderRealIP)
	clientport := req.HttpRequest.Header.Get(bfe_basic.HeaderRealPort)
	if clientip == "" {
		clientip = getFirstSplitFromHeader(req, bfe_basic.HeaderForwardedFor, ",")
		clientport = getFirstSplitFromHeader(req, bfe_basic.HeaderForwardedPort, ",")
	}
	if clientip != "" {
		parseClientAddr(req, clientip, clientport)
	}
}

func getFirstSplitFromHeader(req *bfe_basic.Request, header string, sep string) string {
	ret := ""
	if str := req.HttpRequest.Header.Get(header); str != "" {
		l := strings.SplitN(str, sep, 2)
		ret = strings.TrimSpace(l[0]) // get first split from header
	}
	return ret
}

func parseClientAddr(req *bfe_basic.Request, clientip string, clientport string) {
	if ip := net.ParseIP(clientip); ip != nil { // valid clientip
		req.ClientAddr = new(net.TCPAddr)
		req.ClientAddr.IP = ip
		if port, err := strconv.Atoi(clientport); err == nil { // valid port
			req.ClientAddr.Port = port
		}
	}
}

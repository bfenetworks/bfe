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

package mod_header

import (
	"net"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
)

func modHeaderForwardedAddr(req *bfe_basic.Request) {
	if clientHost := req.HttpRequest.Host; clientHost != "" {
		if prior, existHost := req.HttpRequest.Header[bfe_basic.HeaderForwardedHost]; existHost {
			clientHost = strings.Join(prior, ", ") + ", " + clientHost
		}
		req.HttpRequest.Header.Set(bfe_basic.HeaderForwardedHost, clientHost)
	}

	if clientIP, clientPort, err := net.SplitHostPort(req.HttpRequest.RemoteAddr); err == nil {
		if prior, existIP := req.HttpRequest.Header[bfe_basic.HeaderForwardedFor]; existIP {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		req.HttpRequest.Header.Set(bfe_basic.HeaderForwardedFor, clientIP)

		if priorPort, existPort := req.HttpRequest.Header[bfe_basic.HeaderForwardedPort]; existPort {
			clientPort = strings.Join(priorPort, ", ") + ", " + clientPort
		}
		req.HttpRequest.Header.Set(bfe_basic.HeaderForwardedPort, clientPort)
	}
}

func setHeaderRealAddr(req *bfe_basic.Request, clientIP string, clientPort string) {
	req.HttpRequest.Header.Set(bfe_basic.HeaderRealIP, clientIP)
	req.HttpRequest.Header.Set(bfe_basic.HeaderRealPort, clientPort)
}

func setHeaderBfeIP(req *bfe_basic.Request) {
	localip := req.Connection.LocalAddr().(*net.TCPAddr).IP.String()
	req.HttpRequest.Header.Set(bfe_basic.HeaderBfeIP, localip)
}

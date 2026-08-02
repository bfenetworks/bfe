// Copyright (c) 2026 The BFE Authors.
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

package mod_doh

import (
	"net"
	"strings"
	"testing"
)

import (
	"github.com/miekg/dns"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_util/net_util"
)

func buildBasicRequest(method, urlStr string, t *testing.T) *bfe_basic.Request {
	req := new(bfe_basic.Request)
	httpReq, err := bfe_http.NewRequest(method, urlStr, nil)
	if err != nil {
		t.Fatalf("NewRequest() error: %v", err)
	}
	req.HttpRequest = httpReq
	req.RemoteAddr = &net.TCPAddr{IP: net_util.ParseIPv4("127.0.0.1")}
	return req
}

func TestRequestToDnsMsgUnsupportedMethod(t *testing.T) {
	req := buildBasicRequest("PUT", "https://example.org", t)
	_, err := RequestToDnsMsg(req)
	if err == nil || !strings.Contains(err.Error(), "unsupported method") {
		t.Fatalf("expected unsupported method error, got %v", err)
	}
}

func TestRequestToDnsMsgGetMissingDnsQuery(t *testing.T) {
	req := buildBasicRequest("GET", "https://example.org", t)
	_, err := RequestToDnsMsg(req)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected dns query not found error, got %v", err)
	}
}

func TestRequestToDnsMsgGetMultipleDnsQuery(t *testing.T) {
	req := buildBasicRequest("GET", "https://example.org?dns=AAAA&dns=BBBB", t)
	_, err := RequestToDnsMsg(req)
	if err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("expected multiple dns query error, got %v", err)
	}
}

func TestRequestToDnsMsgGetInvalidBase64(t *testing.T) {
	req := buildBasicRequest("GET", "https://example.org?dns=!!!", t)
	_, err := RequestToDnsMsg(req)
	if err == nil {
		t.Fatal("expected error for invalid base64 dns query")
	}
}

func TestRequestToDnsMsgNilRemoteAddr(t *testing.T) {
	req := buildDohRequest("GET", t)
	req.RemoteAddr = nil
	_, err := RequestToDnsMsg(req)
	if err != nil {
		t.Fatalf("RequestToDnsMsg() error: %v", err)
	}
}

func TestRequestToDnsMsgWithClientSubnet(t *testing.T) {
	req := buildDohRequest("GET", t)
	req.ClientAddr = &net.TCPAddr{IP: net.ParseIP("2001:db8::1")}

	dnsMsg, err := RequestToDnsMsg(req)
	if err != nil {
		t.Fatalf("RequestToDnsMsg() error: %v", err)
	}
	if len(dnsMsg.Extra) == 0 {
		t.Fatal("expected OPT record in Extra")
	}

	opt, ok := dnsMsg.Extra[0].(*dns.OPT)
	if !ok {
		t.Fatalf("expected *dns.OPT, got %T", dnsMsg.Extra[0])
	}
	if len(opt.Option) == 0 {
		t.Fatal("expected EDNS0 option")
	}

	subnet, ok := opt.Option[0].(*dns.EDNS0_SUBNET)
	if !ok {
		t.Fatalf("expected *dns.EDNS0_SUBNET, got %T", opt.Option[0])
	}
	if subnet.SourceNetmask != 128 {
		t.Errorf("SourceNetmask = %d, want 128", subnet.SourceNetmask)
	}
}

func TestGetTTLEmptyAnswer(t *testing.T) {
	msg := &dns.Msg{}
	if got := getTTL(msg); got != 0 {
		t.Errorf("getTTL() = %d, want 0", got)
	}
}

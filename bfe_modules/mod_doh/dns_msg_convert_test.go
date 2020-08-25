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

package mod_doh

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
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

func buildPostRequest(data []byte, t *testing.T) *bfe_http.Request {
	body := bytes.NewBuffer(data)
	req, err := bfe_http.NewRequest("POST", "https://example.org", body)
	if err != nil {
		t.Fatalf("bfe_http.NewRequest error: %v", err)
	}
	return req
}

func buildGetRequest(data []byte, t *testing.T) *bfe_http.Request {
	url := fmt.Sprintf("https://example.org?dns=%s", base64.RawURLEncoding.EncodeToString(data))
	req, err := bfe_http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("bfe_http.NewRequest error: %v", err)
	}
	return req
}

func buildDohRequest(method string, t *testing.T) *bfe_basic.Request {
	msg := dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:               uint16(59713),
			RecursionDesired: true,
			CheckingDisabled: true,
		},
		Question: []dns.Question{
			{
				Name:   "example.org.",
				Qtype:  dns.TypeA,
				Qclass: dns.ClassINET,
			},
		},
	}
	data, err := msg.Pack()
	if err != nil {
		t.Fatalf("msg Pack error: %v", err)
	}

	var httpRequest *bfe_http.Request
	switch method {
	case "POST":
		httpRequest = buildPostRequest(data, t)
	case "GET":
		httpRequest = buildGetRequest(data, t)
	default:
		t.Fatalf("unsupported method %s", method)
		return nil
	}

	req := new(bfe_basic.Request)
	req.HttpRequest = httpRequest
	req.RemoteAddr = new(net.TCPAddr)
	req.RemoteAddr.IP = net_util.ParseIPv4("127.0.0.1")

	return req
}

func TestRequestToDnsMsgPOST(t *testing.T) {
	req := buildDohRequest("POST", t)
	_, err := RequestToDnsMsg(req)
	if err != nil {
		t.Errorf("RequestToDnsMsg error: %v", err)
	}
}

func TestRequestToDnsMsgPOSTBodyExceed(t *testing.T) {
	maxPostMsgLength = 1
	req := buildDohRequest("POST", t)
	_, err := RequestToDnsMsg(req)
	if err == nil || err.Error() != "dns: overflow unpacking uint16" {
		t.Errorf("RequestToDnsMsg error should be \"dns: overflow unpacking uint16\", not %v", err)
	}
}

func TestRequestToDnsMsgGET(t *testing.T) {
	req := buildDohRequest("GET", t)
	_, err := RequestToDnsMsg(req)
	if err != nil {
		t.Errorf("RequestToDnsMsg error: %v", err)
	}
}

func buildDnsMsg() *dns.Msg {
	msg := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:               uint16(26448),
			Response:         true,
			RecursionDesired: true,
		},
		Question: []dns.Question{
			{
				Name:   "example.org.",
				Qtype:  dns.TypeA,
				Qclass: dns.ClassINET,
			},
		},
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:     "example.org.",
					Rrtype:   dns.TypeA,
					Class:    dns.ClassINET,
					Ttl:      600,
					Rdlength: 4,
				},
				A: net.ParseIP("127.0.0.1"),
			},
		},
	}
	return msg
}

func TestDnsMsgToResponse(t *testing.T) {
	msg := buildDnsMsg()
	req := new(bfe_basic.Request)
	_, err := DnsMsgToResponse(req, msg)
	if err != nil {
		t.Errorf("DnsMsgToResponse error: %v", err)
	}
}

func TestGetTTL(t *testing.T) {
	msg := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Ttl: 600,
				},
			},
			&dns.AAAA{
				Hdr: dns.RR_Header{
					Ttl: 7200,
				},
			},
			&dns.A{
				Hdr: dns.RR_Header{
					Ttl: 300,
				},
			},
		},
	}

	ttl := getTTL(msg)
	if ttl != 300 {
		t.Errorf("ttl should be 300, not %d", ttl)
	}
}

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
	"io"
	"io/ioutil"
	"strconv"
)

import (
	"github.com/miekg/dns"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

const DnsMessage = "application/dns-message"

var maxPostMsgLength int64 = 8192

func unpackMsg(buf []byte) (*dns.Msg, error) {
	m := new(dns.Msg)
	err := m.Unpack(buf)
	return m, err
}

func requestToMsgPost(req *bfe_http.Request) (*dns.Msg, error) {
	bodyReader := io.LimitedReader{R: req.Body, N: maxPostMsgLength}
	buf, err := ioutil.ReadAll(&bodyReader)
	if err != nil {
		return nil, err
	}

	return unpackMsg(buf)
}

func requestToMsgGet(req *bfe_http.Request) (*dns.Msg, error) {
	values := req.URL.Query()
	dnsQuery, ok := values["dns"]
	if !ok {
		return nil, fmt.Errorf("\"dns\" query not found")
	}
	if len(dnsQuery) != 1 {
		return nil, fmt.Errorf("multiple \"dns\" query values found")
	}

	buf, err := base64.RawURLEncoding.DecodeString(dnsQuery[0])
	if err != nil {
		return nil, err
	}
	return unpackMsg(buf)
}

func setClientSubnet(req *bfe_basic.Request, dnsMsg *dns.Msg) {
	if req.RemoteAddr == nil {
		return
	}

	cip := req.RemoteAddr.IP
	if req.ClientAddr != nil {
		cip = req.ClientAddr.IP
	}

	var family uint16 = 1
	var sourceNetmask uint8 = 32
	if cip.To16() != nil {
		family = 2
		sourceNetmask = 128
	}

	subnet := &dns.EDNS0_SUBNET{
		Code:          dns.EDNS0SUBNET,
		Family:        family,
		SourceNetmask: sourceNetmask,
		SourceScope:   0,
		Address:       cip,
	}

	opt := new(dns.OPT)
	opt.Hdr.Name = "."
	opt.Hdr.Rrtype = dns.TypeOPT
	opt.SetUDPSize(dns.DefaultMsgSize)
	opt.Option = append(opt.Option, subnet)
	dnsMsg.Extra = append(dnsMsg.Extra, opt)
}

func RequestToDnsMsg(req *bfe_basic.Request) (*dns.Msg, error) {
	var dnsMsg *dns.Msg
	var err error

	httpRequest := req.HttpRequest
	switch httpRequest.Method {
	case "GET":
		dnsMsg, err = requestToMsgGet(httpRequest)
	case "POST":
		dnsMsg, err = requestToMsgPost(httpRequest)
	default:
		err = fmt.Errorf("unsupported method: %s", httpRequest.Method)
	}
	if err != nil {
		return nil, err
	}

	setClientSubnet(req, dnsMsg)
	return dnsMsg, nil
}

func getTTL(msg *dns.Msg) uint32 {
	if len(msg.Answer) < 1 {
		return 0
	}

	// Use the smallest TTL in the Answer section.
	// See section 5.1 of RFC 8484.
	ttl := msg.Answer[0].Header().Ttl
	for i := 1; i < len(msg.Answer); i++ {
		if ttl > msg.Answer[i].Header().Ttl {
			ttl = msg.Answer[i].Header().Ttl
		}
	}

	return ttl
}

func DnsMsgToResponse(req *bfe_basic.Request, msg *dns.Msg) (*bfe_http.Response, error) {
	data, err := msg.Pack()
	if err != nil {
		return nil, err
	}

	resp := bfe_basic.CreateInternalResp(req, bfe_http.StatusOK)
	resp.Header.Set("Content-Type", DnsMessage)
	resp.Header.Set("Cache-Control", fmt.Sprintf("max-age=%d", getTTL(msg)))
	resp.Header.Set("Content-Length", strconv.Itoa(len(data)))
	resp.Body = ioutil.NopCloser(bytes.NewReader(data))
	return resp, nil
}

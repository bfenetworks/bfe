// Copyright (c) 2019 Baidu, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     bfe_http://www.apache.org/licenses/LICENSE-2.0
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
	"github.com/baidu/bfe/bfe_basic"
	"github.com/baidu/bfe/bfe_http"
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

func RequestToDnsMsg(req *bfe_http.Request) (*dns.Msg, error) {
	switch req.Method {
	case "GET":
		return requestToMsgGet(req)
	case "POST":
		return requestToMsgPost(req)
	default:
		return nil, fmt.Errorf("unsupported method: %s", req.Method)
	}
}

func DnsMsgToResponse(req *bfe_basic.Request, msg *dns.Msg) (*bfe_http.Response, error) {
	data, err := msg.Pack()
	if err != nil {
		return nil, err
	}

	ttl := 0
	for _, record := range msg.Answer {
		if t, ok := record.(*dns.A); ok {
			ttl = int(t.Hdr.Ttl)
		}
	}

	resp := bfe_basic.CreateInternalResp(req, bfe_http.StatusOK)
	resp.Header.Set("Content-Type", DnsMessage)
	resp.Header.Set("Cache-Control", fmt.Sprintf("max-age=%d", ttl))
	resp.Header.Set("Content-Length", strconv.Itoa(len(data)))
	resp.Body = ioutil.NopCloser(bytes.NewReader(data))
	return resp, nil
}

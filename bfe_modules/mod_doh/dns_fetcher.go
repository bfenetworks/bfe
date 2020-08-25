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
	"time"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/miekg/dns"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

type DnsFetcher interface {
	Fetch(req *bfe_basic.Request) (*bfe_http.Response, error)
}

type DnsClient struct {
	address  string
	retryMax int
	client   dns.Client
}

func NewDnsClient(dnsConf *DnsConf) *DnsClient {
	dnsClient := new(DnsClient)
	dnsClient.address = dnsConf.Address
	dnsClient.retryMax = dnsConf.RetryMax
	dnsClient.client = dns.Client{
		Net:     "udp",
		Timeout: time.Duration(dnsConf.Timeout) * time.Millisecond,
		UDPSize: dns.MaxMsgSize,
	}

	return dnsClient
}

func (c *DnsClient) exchangeWithRetry(msg *dns.Msg) (*dns.Msg, error) {
	var reply *dns.Msg
	var err error

	for retry := 0; retry < c.retryMax+1; retry++ {
		reply, _, err = c.client.Exchange(msg, c.address)
		if err == nil {
			return reply, nil
		}

		if openDebug {
			log.Logger.Debug("dns client: Exchange error: %v, retry: %d", err, retry)
		}
	}

	return nil, err
}

func (c *DnsClient) Fetch(req *bfe_basic.Request) (*bfe_http.Response, error) {
	msg, err := RequestToDnsMsg(req)
	if err != nil {
		if openDebug {
			log.Logger.Debug("dns client: RequestToDnsMsg error: %v", err)
		}

		return nil, err
	}

	reply, err := c.exchangeWithRetry(msg)
	if err != nil {
		return nil, err
	}

	resp, err := DnsMsgToResponse(req, reply)
	if err != nil {
		if openDebug {
			log.Logger.Debug("dns client: DnsMsgToResponse error: %v", err)
		}

		return nil, err
	}

	return resp, nil
}

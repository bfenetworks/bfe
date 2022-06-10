// Copyright (c) 2022 The BFE Authors.
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

package bfeservertest

import (
	"testing"

	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/bfenetworks/bfe/bfe_server"
)

func TestClientConnServedInc(t *testing.T) {
	cases := []struct {
		name   string
		proto  string
		value  uint
		expect int64
		getFn  func(*bfe_server.ProxyState) int64
		preFn  func(*bfe_server.ProxyState)
	}{
		{
			"http served increase ok",
			"http",
			10,
			10,
			func(s *bfe_server.ProxyState) int64 {
				return s.HttpClientConnServed.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Counter
				var c1 metrics.Counter
				s.HttpClientConnServed = &c0
				s.ClientConnServed = &c1
			},
		},
		{
			"https served increase ok",
			"https",
			20,
			20,
			func(s *bfe_server.ProxyState) int64 {
				return s.HttpsClientConnServed.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Counter
				var c1 metrics.Counter
				s.HttpsClientConnServed = &c0
				s.ClientConnServed = &c1
			},
		},
		{
			"h2 served increase ok",
			"h2",
			10,
			10,
			func(s *bfe_server.ProxyState) int64 {
				return s.Http2ClientConnServed.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Counter
				var c1 metrics.Counter
				s.Http2ClientConnServed = &c0
				s.ClientConnServed = &c1
			},
		},
		{
			"spdy/3.1 served increase ok",
			"spdy/3.1",
			20,
			20,
			func(s *bfe_server.ProxyState) int64 {
				return s.SpdyClientConnServed.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Counter
				var c1 metrics.Counter
				s.SpdyClientConnServed = &c0
				s.ClientConnServed = &c1
			},
		},
		{
			"ws served increase ok",
			"ws",
			10,
			10,
			func(s *bfe_server.ProxyState) int64 {
				return s.WsClientConnServed.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Counter
				var c1 metrics.Counter
				s.WsClientConnServed = &c0
				s.ClientConnServed = &c1
			},
		},
		{
			"wss served increase ok",
			"wss",
			20,
			20,
			func(s *bfe_server.ProxyState) int64 {
				return s.WssClientConnServed.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Counter
				var c1 metrics.Counter
				s.WssClientConnServed = &c0
				s.ClientConnServed = &c1
			},
		},
		{
			"stream served increase ok",
			"stream",
			10,
			10,
			func(s *bfe_server.ProxyState) int64 {
				return s.StreamClientConnServed.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Counter
				var c1 metrics.Counter
				s.StreamClientConnServed = &c0
				s.ClientConnServed = &c1
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := &bfe_server.ProxyState{}
			c.preFn(s)
			preValue := c.getFn(s)
			preServed := s.ClientConnServed.Get()
			s.ClientConnServedInc(c.proto, c.value)
			gotValue := c.getFn(s)
			if gotValue != preValue+c.expect {
				t.Errorf("protocal %v, counter increase failed, got %d; want %d", c.proto, gotValue, preValue+c.expect)
			}
			afterServed := s.ClientConnServed.Get()
			if afterServed != preServed+c.expect {
				t.Errorf("protocal %v, whole counter increase failed, got %d, want %d", c.proto, afterServed, preServed+c.expect)
			}
		})
	}

}

func TestClientConnActiveInc(t *testing.T) {
	cases := []struct {
		name   string
		proto  string
		value  uint
		expect int64
		getFn  func(*bfe_server.ProxyState) int64
		preFn  func(*bfe_server.ProxyState)
	}{
		{
			"http active increase ok",
			"http",
			10,
			10,
			func(s *bfe_server.ProxyState) int64 {
				return s.HttpClientConnActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.HttpClientConnActive = &c0
				s.ClientConnActive = &c1
			},
		},
		{
			"https active increase ok",
			"https",
			20,
			20,
			func(s *bfe_server.ProxyState) int64 {
				return s.HttpsClientConnActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.HttpsClientConnActive = &c0
				s.ClientConnActive = &c1
			},
		},
		{
			"h2 active increase ok",
			"h2",
			10,
			10,
			func(s *bfe_server.ProxyState) int64 {
				return s.Http2ClientConnActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.Http2ClientConnActive = &c0
				s.ClientConnActive = &c1
			},
		},
		{
			"spdy/3.1 active increase ok",
			"spdy/3.1",
			20,
			20,
			func(s *bfe_server.ProxyState) int64 {
				return s.SpdyClientConnActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.SpdyClientConnActive = &c0
				s.ClientConnActive = &c1
			},
		},
		{
			"ws active increase ok",
			"ws",
			10,
			10,
			func(s *bfe_server.ProxyState) int64 {
				return s.WsClientConnActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.WsClientConnActive = &c0
				s.ClientConnActive = &c1
			},
		},
		{
			"wss active increase ok",
			"wss",
			20,
			20,
			func(s *bfe_server.ProxyState) int64 {
				return s.WssClientConnActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.WssClientConnActive = &c0
				s.ClientConnActive = &c1
			},
		},
		{
			"stream active increase ok",
			"stream",
			10,
			10,
			func(s *bfe_server.ProxyState) int64 {
				return s.StreamClientConnActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.StreamClientConnActive = &c0
				s.ClientConnActive = &c1
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := &bfe_server.ProxyState{}
			c.preFn(s)
			preValue := c.getFn(s)
			preWholeActive := s.ClientConnActive.Get()
			s.ClientConnActiveInc(c.proto, c.value)
			gotValue := c.getFn(s)
			if gotValue != preValue+c.expect {
				t.Errorf("protocal %v, counter increase failed, got %d; want %d", c.proto, gotValue, preValue+c.expect)
			}
			afterWholeActive := s.ClientConnActive.Get()
			if afterWholeActive != preWholeActive+c.expect {
				t.Errorf("protocal %v, whole counter increase failed, got %d, want %d", c.proto, afterWholeActive, preWholeActive+c.expect)
			}
		})
	}
}

func TestClientConnActiveDec(t *testing.T) {
	cases := []struct {
		name   string
		proto  string
		value  uint
		expect int64
		getFn  func(*bfe_server.ProxyState) int64
		preFn  func(*bfe_server.ProxyState)
	}{
		{
			"http active decrease ok",
			"http",
			10,
			-10,
			func(s *bfe_server.ProxyState) int64 {
				return s.HttpClientConnActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.HttpClientConnActive = &c0
				s.ClientConnActive = &c1
			},
		},
		{
			"https active decrease ok",
			"https",
			20,
			-20,
			func(s *bfe_server.ProxyState) int64 {
				return s.HttpsClientConnActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.HttpsClientConnActive = &c0
				s.ClientConnActive = &c1
			},
		},
		{
			"h2 active decrease ok",
			"h2",
			10,
			-10,
			func(s *bfe_server.ProxyState) int64 {
				return s.Http2ClientConnActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.Http2ClientConnActive = &c0
				s.ClientConnActive = &c1
			},
		},
		{
			"spdy/3.1 active decrease ok",
			"spdy/3.1",
			20,
			-20,
			func(s *bfe_server.ProxyState) int64 {
				return s.SpdyClientConnActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.SpdyClientConnActive = &c0
				s.ClientConnActive = &c1
			},
		},
		{
			"ws active decrease ok",
			"ws",
			10,
			-10,
			func(s *bfe_server.ProxyState) int64 {
				return s.WsClientConnActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.WsClientConnActive = &c0
				s.ClientConnActive = &c1
			},
		},
		{
			"wss active decrease ok",
			"wss",
			20,
			-20,
			func(s *bfe_server.ProxyState) int64 {
				return s.WssClientConnActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.WssClientConnActive = &c0
				s.ClientConnActive = &c1
			},
		},
		{
			"stream active decrease ok",
			"stream",
			10,
			-10,
			func(s *bfe_server.ProxyState) int64 {
				return s.StreamClientConnActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.StreamClientConnActive = &c0
				s.ClientConnActive = &c1
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := &bfe_server.ProxyState{}
			c.preFn(s)
			preValue := c.getFn(s)
			preWholeActive := s.ClientConnActive.Get()
			s.ClientConnActiveDec(c.proto, c.value)
			gotValue := c.getFn(s)
			if gotValue != preValue+c.expect {
				t.Errorf("protocal %v, counter decrease failed, got %d; want %d", c.proto, gotValue, preValue+c.expect)
			}
			afterWholeActive := s.ClientConnActive.Get()
			if afterWholeActive != preWholeActive+c.expect {
				t.Errorf("protocal %v, whole counter decrease failed, got %d, want %d", c.proto, afterWholeActive, preWholeActive+c.expect)
			}
		})
	}
}

func TestClientReqServedInc(t *testing.T) {
	cases := []struct {
		name   string
		proto  string
		value  uint
		expect int64
		getFn  func(*bfe_server.ProxyState) int64
		preFn  func(*bfe_server.ProxyState)
	}{
		{
			"http req served increase ok",
			"http",
			10,
			10,
			func(s *bfe_server.ProxyState) int64 {
				return s.HttpClientReqServed.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Counter
				var c1 metrics.Counter
				s.HttpClientReqServed = &c0
				s.ClientReqServed = &c1
			},
		},
		{
			"https req served increase ok",
			"https",
			20,
			20,
			func(s *bfe_server.ProxyState) int64 {
				return s.HttpsClientReqServed.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Counter
				var c1 metrics.Counter
				s.HttpsClientReqServed = &c0
				s.ClientReqServed = &c1
			},
		},
		{
			"h2 req served increase ok",
			"h2",
			10,
			10,
			func(s *bfe_server.ProxyState) int64 {
				return s.Http2ClientReqServed.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Counter
				var c1 metrics.Counter
				s.Http2ClientReqServed = &c0
				s.ClientReqServed = &c1
			},
		},
		{
			"spdy/3.1 req served increase ok",
			"spdy/3.1",
			20,
			20,
			func(s *bfe_server.ProxyState) int64 {
				return s.SpdyClientReqServed.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Counter
				var c1 metrics.Counter
				s.SpdyClientReqServed = &c0
				s.ClientReqServed = &c1
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := &bfe_server.ProxyState{}
			c.preFn(s)
			preValue := c.getFn(s)
			preWholeReqServed := s.ClientReqServed.Get()
			s.ClientReqServedInc(c.proto, c.value)
			gotValue := c.getFn(s)
			if gotValue != preValue+c.expect {
				t.Errorf("protocal %v, counter increase failed, got %d; want %d", c.proto, gotValue, preValue+c.expect)
			}
			afterWholeReqServed := s.ClientReqServed.Get()
			if afterWholeReqServed != preWholeReqServed+c.expect {
				t.Errorf("protocal %v, whole counter increase failed, got %d, want %d", c.proto, afterWholeReqServed, preWholeReqServed+c.expect)
			}
		})
	}
}

func TestClientReqActiveInc(t *testing.T) {
	cases := []struct {
		name   string
		proto  string
		value  uint
		expect int64
		getFn  func(*bfe_server.ProxyState) int64
		preFn  func(*bfe_server.ProxyState)
	}{
		{
			"http req active increase ok",
			"http",
			10,
			10,
			func(s *bfe_server.ProxyState) int64 {
				return s.HttpClientReqActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.HttpClientReqActive = &c0
				s.ClientReqActive = &c1
			},
		},
		{
			"https req active increase ok",
			"https",
			20,
			20,
			func(s *bfe_server.ProxyState) int64 {
				return s.HttpsClientReqActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.HttpsClientReqActive = &c0
				s.ClientReqActive = &c1
			},
		},
		{
			"h2 req active increase ok",
			"h2",
			10,
			10,
			func(s *bfe_server.ProxyState) int64 {
				return s.Http2ClientReqActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.Http2ClientReqActive = &c0
				s.ClientReqActive = &c1
			},
		},
		{
			"spdy/3.1 req active increase ok",
			"spdy/3.1",
			20,
			20,
			func(s *bfe_server.ProxyState) int64 {
				return s.SpdyClientReqActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.SpdyClientReqActive = &c0
				s.ClientReqActive = &c1
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := &bfe_server.ProxyState{}
			c.preFn(s)
			preWholeActiveReq := s.ClientReqActive.Get()
			preValue := c.getFn(s)
			s.ClientReqActiveInc(c.proto, c.value)
			gotValue := c.getFn(s)
			if gotValue != preValue+c.expect {
				t.Errorf("protocal %v, counter increase failed, got %d; want %d", c.proto, gotValue, preValue+c.expect)
			}
			afterWholeActiveReq := s.ClientReqActive.Get()
			if afterWholeActiveReq != preWholeActiveReq+c.expect {
				t.Errorf("protocal %v, whole counter increase failed, got %d, want %d", c.proto, afterWholeActiveReq, preWholeActiveReq+c.expect)
			}
		})
	}
}

func TestClientReqActiveDec(t *testing.T) {
	cases := []struct {
		name   string
		proto  string
		value  uint
		expect int64
		getFn  func(*bfe_server.ProxyState) int64
		preFn  func(*bfe_server.ProxyState)
	}{
		{
			"http req active decrease ok",
			"http",
			10,
			-10,
			func(s *bfe_server.ProxyState) int64 {
				return s.HttpClientReqActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.HttpClientReqActive = &c0
				s.ClientReqActive = &c1
			},
		},
		{
			"https req active decrease ok",
			"https",
			20,
			-20,
			func(s *bfe_server.ProxyState) int64 {
				return s.HttpsClientReqActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.HttpsClientReqActive = &c0
				s.ClientReqActive = &c1
			},
		},
		{
			"h2 req active decrease ok",
			"h2",
			10,
			-10,
			func(s *bfe_server.ProxyState) int64 {
				return s.Http2ClientReqActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.Http2ClientReqActive = &c0
				s.ClientReqActive = &c1
			},
		},
		{
			"spdy/3.1 req active decrease ok",
			"spdy/3.1",
			20,
			-20,
			func(s *bfe_server.ProxyState) int64 {
				return s.SpdyClientReqActive.Get()
			},
			func(s *bfe_server.ProxyState) {
				var c0 metrics.Gauge
				var c1 metrics.Gauge
				s.SpdyClientReqActive = &c0
				s.ClientReqActive = &c1
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := &bfe_server.ProxyState{}
			c.preFn(s)
			preWholeActiveReq := s.ClientReqActive.Get()
			preValue := c.getFn(s)
			s.ClientReqActiveDec(c.proto, c.value)
			gotValue := c.getFn(s)
			if gotValue != preValue+c.expect {
				t.Errorf("protocal %v, counter decrease failed, got %d; want %d", c.proto, gotValue, preValue+c.expect)
			}
			afterWholeActiveReq := s.ClientReqActive.Get()
			if afterWholeActiveReq != preWholeActiveReq+c.expect {
				t.Errorf("protocal %v, whole counter decrease failed, got %d, want %d", c.proto, afterWholeActiveReq, preWholeActiveReq+c.expect)
			}
		})
	}
}

// Copyright (c) 2019 Baidu, Inc.
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

package condition

import (
	"net"
	"testing"
)

import (
	"github.com/baidu/bfe/bfe_basic"
	"github.com/baidu/bfe/bfe_http"
)

func TestIn(t *testing.T) {
	if !in("xxx", []string{"xxx", "yyy"}) {
		t.Errorf("xxx should in pattern")
	}

	if in("xxx1", []string{"xxx", "yyy"}) {
		t.Errorf("xxx1 should not in pattern")
	}
}

func TestPrimitive_String(t *testing.T) {
	c, _ := Build("req_path_in(\"/ABC\",false)")

	p, ok := c.(*PrimitiveCond)
	if !ok {
		t.Fatalf("should be primitive")
	}

	if p.String() != "req_path_in(\"/ABC\",false)" {
		t.Fatalf("cond string got [%s], expect [%s]", p.String(), "req_path_in(\"/ABC\",false)")
	}
}

func TestIpInMatcher(t *testing.T) {
	matcher, err := NewIpInMatcher("1.1.1.1|2001:DB8:2de::e13")
	if err != nil {
		t.Errorf("NewIpInMatcher() error: %v", err)
		return
	}

	if !matcher.Match(net.ParseIP("1.1.1.1")) || !matcher.Match(net.ParseIP("2001:DB8:2de::e13")) {
		t.Errorf("should match 1.1.1.1 2001:DB8:2de::e13")
		return
	}
}

func TestIpMatcher(t *testing.T) {
	matcher, err := NewIPMatcher("2001:DB8:2de::e13", "2004:DB8:2de::e13")
	if err != nil {
		t.Errorf("NewIPMatcher() error: %v", err)
		return
	}

	if !matcher.Match(net.ParseIP("2002:DB8:2de::e13")) || !matcher.Match(net.ParseIP("2001:DB8:2de:333::111")) {
		t.Errorf("should match 2002:DB8:2de::e13 or 2001:DB8:2de:333::111")
		return
	}

	if matcher.Match(net.ParseIP("2004:DB8:2de:33f::113")) || matcher.Match("2001::e13") {
		t.Errorf("should not match 2004:DB8:2de:33f::113 or 2001::e13")
		return
	}

	matcher, err = NewIPMatcher("1.1.1.1", "2.2.2.2")
	if err != nil {
		t.Errorf("NewIPMatcher() error: %v", err)
		return
	}

	if !matcher.Match(net.ParseIP("1.1.1.32")) || matcher.Match(net.ParseIP("3.3.3.3")) {
		t.Errorf("should match 1.1.1.32, should not match 3.3.3.3")
		return
	}
}

// test HostFetcher, header host without port case
func TestHostFetcher_1(t *testing.T) {
	// prepare input data
	host := "www.baidu.com"
	hf := HostFetcher{}
	req := bfe_basic.Request{
		HttpRequest: &bfe_http.Request{
			Host: host,
		},
	}

	// Fetch
	hostIF, err := hf.Fetch(&req)
	if err != nil {
		t.Errorf("Fetch(): %v", err)
	}

	// check
	if hostIF.(string) != host {
		t.Errorf("Fetch host error, not %s", hostIF.(string))
	}
}

// test HostFetcher, header host with port case
func TestHostFetcher_2(t *testing.T) {
	// prepare input data
	host := "www.baidu.com"
	port := ":80"
	hf := HostFetcher{}
	req := bfe_basic.Request{
		HttpRequest: &bfe_http.Request{
			Host: host + port,
		},
	}

	// Fetch
	hostIF, err := hf.Fetch(&req)
	if err != nil {
		t.Errorf("Fetch(): %v", err)
	}

	// check
	if hostIF.(string) != host {
		t.Errorf("Fetch host error, not %s", hostIF.(string))
	}
}

// test HostFetcher, HttpRequest no set
func TestHostFetcher_3(t *testing.T) {
	// prepare input data
	hf := HostFetcher{}
	req := bfe_basic.Request{}

	// Fetch
	_, err := hf.Fetch(&req)
	if err == nil || err.Error() != "fetcher: nil pointer" {
		t.Errorf("Fetch(): wrong err: %v", err)
	}
}

// test HostMatcher, correct case
func TestHostMatcher_1(t *testing.T) {
	matcher, err := NewHostMatcher("www.baidu.com|map.baidu.com")
	if err != nil {
		t.Errorf("NewHostMatcher() error: %v", err)
		return
	}

	if !matcher.Match("www.baidu.com") || !matcher.Match("map.baidu.com") {
		t.Errorf("should match www.baidu.com and map.baidu.com")
		return
	}

	if !matcher.Match("www.BAIDU.com") || !matcher.Match("MAP.baidu.com") {
		t.Errorf("should match www.BAIDU.com and MAP.baidu.com")
		return
	}

	if matcher.Match("abc.BAIDU.com") || matcher.Match("1.baidu.com") {
		t.Errorf("should not match abc.BAIDU.com or 1.baidu.com")
		return
	}

	if matcher.Match(1) {
		t.Errorf("should not match 1")
		return
	}
}

// test HostMatcher, error case, host include port
func TestHostMatcher_2(t *testing.T) {
	_, err := NewHostMatcher("www.baidu.com:80|map.baidu.com")
	if err == nil || err.Error() != "port shoud not be included in host(www.baidu.com:80)" {
		t.Errorf("NewHostMatcher() return wrong error: %v", err)
	}
}

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

package condition

import (
	"net"
	"testing"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
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
	if err == nil || err.Error() != "port should not be included in host(www.baidu.com:80)" {
		t.Errorf("NewHostMatcher() return wrong error: %v", err)
	}
}

// test ContainMatcher, case-sensitive
func TestContainMatcher_1(t *testing.T) {
	matcher := NewContainMatcher("yingwen|中文|%e4%b8%ad%e6%96%87|YINGWEN", false)
	if !matcher.Match("yingwen") {
		t.Fatalf("should match yingwen")
	}

	if matcher.Match("Yingwen") {
		t.Fatalf("should not match Yingwen")
	}

	if !matcher.Match("hi，中文") {
		t.Fatalf("should match hi，中文")
	}

	if matcher.Match("文") {
		t.Fatalf("should not match 文")
	}

	if !matcher.Match("%e4%b8%ad%e6%96%87") {
		t.Fatalf("should match %%e4%%b8%%ad%%e6%%96%%87")
	}

	if !matcher.Match("YINGWEN") {
		t.Fatalf("should match YINGWEN")
	}
}

// test for ContainMatcher, ignore case
func TestContainMatcher_2(t *testing.T) {
	matcher := NewContainMatcher("yingwen", true)
	if !matcher.Match("Yingwen") {
		t.Fatalf("should match Yingwen")
	}
}

// test ContextFetcher
func TestContextValueFetcher(t *testing.T) {
	// prepare input data
	hf := ContextValueFetcher{"hello"}
	req := bfe_basic.NewRequest(nil, nil, nil, nil, nil)
	req.HttpRequest = &bfe_http.Request{}
	req.SetContext("hello", "world")
	// Fetch
	contextVal, err := hf.Fetch(req)
	if err != nil {
		t.Fatalf("Fetch(): %v", err)
	}

	// check
	if contextVal.(string) != "world" {
		t.Errorf("Fetch contextVal error, want=%v, got=%v", "world", contextVal)
	}
}

func TestPathElementPrefixMatcher(t *testing.T) {
	matcher := NewPathElementPrefixMatcher("/path|/path/ab", true)
	if !matcher.Match("/path/a/c") {
		t.Fatalf("should match /path/a/c")
	}
	if !matcher.Match("/path/ab") {
		t.Fatalf("should match /path/ab")
	}
	if matcher.Match("/pathabc") {
		t.Fatalf("should not match /pathabc")
	}
}

func TestTimeMatcher(t *testing.T) {
	matcher, err := NewTimeMatcher("20190204200000H", "20190205010000H")
	if err != nil {
		t.Fatalf("NewTimeMatcher() error: %v", err)
	}
	tm := time.Date(2019, 2, 4, 19, 59, 59, 0, time.FixedZone("CST", 8*60*60))
	if matcher.Match(tm) {
		t.Fatalf("should not match 2019-02-04 19:59:59 H")
	}
	tm = time.Date(2019, 2, 4, 11, 59, 59, 0, time.UTC)
	if matcher.Match(tm) {
		t.Fatalf("should not match 2019-02-04 11:59:59 Z")
	}
	tm = time.Date(2019, 2, 4, 20, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	if !matcher.Match(tm) {
		t.Fatalf("should match 2019-02-04 20:00:00 H")
	}
	tm = time.Date(2019, 2, 4, 12, 0, 0, 0, time.UTC)
	if !matcher.Match(tm) {
		t.Fatalf("should match 2019-02-04 12:00:00 Z")
	}
	tm = time.Date(2019, 2, 4, 20, 0, 1, 0, time.FixedZone("CST", 8*60*60))
	if !matcher.Match(tm) {
		t.Fatalf("should match 2019-02-04 20:00:01 H")
	}
	tm = time.Date(2019, 2, 4, 12, 0, 1, 0, time.UTC)
	if !matcher.Match(tm) {
		t.Fatalf("should match 2019-02-04 12:00:01 Z")
	}
	tm = time.Date(2019, 2, 5, 1, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	if !matcher.Match(tm) {
		t.Fatalf("should match 2019-02-05 01:00:00 H")
	}
	tm = time.Date(2019, 2, 4, 17, 0, 0, 0, time.UTC)
	if !matcher.Match(tm) {
		t.Fatalf("should match 2019-02-04 17:00:00 Z")
	}
	tm = time.Date(2019, 2, 5, 1, 0, 1, 0, time.FixedZone("CST", 8*60*60))
	if matcher.Match(tm) {
		t.Fatalf("should not match 2019-02-05 01:00:01 H")
	}
	tm = time.Date(2019, 2, 4, 17, 0, 1, 0, time.UTC)
	if matcher.Match(tm) {
		t.Fatalf("should not match 2019-02-04 17:00:01 Z")
	}
}

func TestPeriodicTimeMatcher(t *testing.T) {
	matcher, err := NewPeriodicTimeMatcher("200000H", "213000H", "")
	if err != nil {
		t.Fatalf("NewPeriodicTimeMatcher() error: %v", err)
	}
	_, err = NewPeriodicTimeMatcher("200000R", "213000H", "")
	if err == nil {
		t.Fatalf("NewPeriodicTimeMatcher() should failed")
	}
	_, err = NewPeriodicTimeMatcher("220000H", "213000H", "")
	if err == nil {
		t.Fatalf("NewPeriodicTimeMatcher() should failed")
	}
	_, err = NewPeriodicTimeMatcher("200000H", "213000H", "Monday")
	if err == nil {
		t.Fatalf("NewPeriodicTimeMatcher() should failed")
	}
	tm := time.Date(2019, 2, 4, 19, 59, 59, 0, time.FixedZone("CST", 8*60*60))
	if matcher.Match(tm) {
		t.Fatalf("should not match %v", tm)
	}
	tm = time.Date(2019, 2, 4, 11, 59, 59, 0, time.UTC)
	if matcher.Match(tm) {
		t.Fatalf("should not match %v", tm)
	}
	tm = time.Date(2019, 2, 4, 20, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	if !matcher.Match(tm) {
		t.Fatalf("should match %v", tm)
	}
	tm = time.Date(2019, 2, 4, 13, 30, 0, 0, time.UTC)
	if !matcher.Match(tm) {
		t.Fatalf("should match %v", tm)
	}
	tm = time.Date(2019, 2, 4, 21, 30, 1, 0, time.FixedZone("CST", 8*60*60))
	if matcher.Match(tm) {
		t.Fatalf("should not match %v", tm)
	}
	tm = time.Date(2019, 2, 4, 12, 30, 0, 0, time.UTC)
	if !matcher.Match(tm) {
		t.Fatalf("should match %v", tm)
	}
	tm = time.Date(2019, 2, 4, 13, 30, 1, 0, time.UTC)
	if matcher.Match(tm) {
		t.Fatalf("should not match %v", tm)
	}
}

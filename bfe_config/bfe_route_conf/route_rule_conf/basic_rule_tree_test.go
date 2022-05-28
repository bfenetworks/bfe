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

package route_rule_conf

import (
	"testing"
)

func TestHostMatchInBasicRuleTree(t *testing.T) {

	cluster := "c1"
	rule := &BasicRouteRuleFile{
		Hostname:    nil,
		Path:        nil,
		ClusterName: &cluster,
	}

	rule.Hostname = append(rule.Hostname, "")
	rt := NewBasicRouteRuleTree()
	if ret := rt.Insert(rule); ret == nil {
		t.Errorf("insert empty host is not allowed")
	}

	rule.Hostname = nil
	rule.Hostname = append(rule.Hostname, "*")
	rt.Insert(rule)

	c, ok := rt.Get("*", "/")
	if !ok || c != cluster {
		t.Errorf("should match")
	}

	c, ok = rt.Get("any", "/foo")
	if !ok || c != cluster {
		t.Errorf("should match")
	}

	c, ok = rt.Get("bar.foo.com", "/foo")
	if !ok || c != cluster {
		t.Errorf("should match")
	}

	rule.Hostname = nil
	rule.Hostname = append(rule.Hostname, "*.com")
	rt = NewBasicRouteRuleTree()
	rt.Insert(rule)

	c, ok = rt.Get("foo.com", "/foo")
	if !ok || c != cluster {
		t.Errorf("should match")
	}
	_, ok = rt.Get("bar.foo.com", "/foo")
	if ok {
		t.Errorf("should not match")
	}

	rule.Hostname = nil
	rule.Hostname = append(rule.Hostname, "*.foo.com")
	rt = NewBasicRouteRuleTree()
	rt.Insert(rule)

	c, ok = rt.Get("bar.foo.com", "/")
	if !ok || c != cluster {
		t.Errorf("should match")
	}
	c, ok = rt.Get("foo.com", "/")
	if ok {
		t.Errorf("should not match *")
	}

	rule.Hostname = nil
	rule.Hostname = append(rule.Hostname, "foo.com")
	rule.Hostname = append(rule.Hostname, "bar.com")

	rt = NewBasicRouteRuleTree()
	rt.Insert(rule)
	c, ok = rt.Get("foo.com", "/")
	if !ok || c != cluster {
		t.Errorf("should match")
	}
	rt.Insert(rule)
	c, ok = rt.Get("bar.com", "/")
	if !ok || c != cluster {
		t.Errorf("should match")
	}

	rt = NewBasicRouteRuleTree()
	rule.Hostname = nil
	rule.Hostname = append(rule.Hostname, "foo.com")
	cluster1 := "c1"
	rule.ClusterName = &cluster1
	rt.Insert(rule)

	rule.Hostname = nil
	rule.Hostname = append(rule.Hostname, "bar.com")
	cluster2 := "c2"
	rule.ClusterName = &cluster2
	rt.Insert(rule)

	c, ok = rt.Get("foo.com", "/")
	if !ok || c != cluster1 {
		t.Errorf("should match")
	}
	c, ok = rt.Get("bar.com", "/")
	if !ok || c != cluster2 {
		t.Errorf("should match")
	}

	rule.Hostname = nil
	rule.Hostname = append(rule.Hostname, "*.bar.com")
	cluster3 := "c3"
	rule.ClusterName = &cluster3
	rt.Insert(rule)

	c, ok = rt.Get("baz.bar.com", "/")
	if !ok || c != cluster3 {
		t.Errorf("should match")
	}

}

func TestPathMatchInBasicRuleTree(t *testing.T) {
	cluster := "c1"
	rule := &BasicRouteRuleFile{
		Hostname:    nil,
		Path:        nil,
		ClusterName: &cluster,
	}

	rule.Path = append(rule.Path, "")
	rt := NewBasicRouteRuleTree()
	if ret := rt.Insert(rule); ret == nil {
		t.Errorf("empty path is not allowed")
	}

	rule.Path = nil
	rule.Path = append(rule.Path, "*")
	rt.Insert(rule)

	c, ok := rt.Get("any", "/foo")
	if !ok || c != cluster {
		t.Errorf("should match")
	}

	c, ok = rt.Get("foo.com", "/foo")
	if !ok || c != cluster {
		t.Errorf("should match")
	}

	c, ok = rt.Get("foo.com", "foo")
	if !ok || c != cluster {
		t.Errorf("should match")
	}

	c, ok = rt.Get("*", "/foo")
	if !ok || c != cluster {
		t.Errorf("should match")
	}

	rule.Path = nil
	rule.Path = append(rule.Path, "/")
	rt = NewBasicRouteRuleTree()
	rt.Insert(rule)

	c, ok = rt.Get("foo.com", "/")
	if !ok || c != cluster {
		t.Errorf("should match")
	}

	c, ok = rt.Get("foo.com", "/foo")
	if ok {
		t.Errorf("should not match")
	}

	rule.Path = nil
	rule.Path = append(rule.Path, "/*")
	rt = NewBasicRouteRuleTree()
	rt.Insert(rule)

	c, ok = rt.Get("foo.com", "/")
	if !ok || c != cluster {
		t.Errorf("should match")
	}
	c, ok = rt.Get("foo.com", "/foo")
	if !ok || c != cluster {
		t.Errorf("should match")
	}
	c, ok = rt.Get("foo.com", "/foo/bar")
	if !ok || c != cluster {
		t.Errorf("should match")
	}
	c, ok = rt.Get("foo.com", "foo")
	if ok {
		t.Errorf("should not match")
	}

	rule.Path = nil
	rule.Path = append(rule.Path, "/foo")
	rt = NewBasicRouteRuleTree()
	rt.Insert(rule)

	c, ok = rt.Get("foo.com", "/foo")
	if !ok || c != cluster {
		t.Errorf("should match")
	}
	c, ok = rt.Get("foo.com", "/bar")
	if ok {
		t.Errorf("should not match")
	}
	c, ok = rt.Get("foo.com", "/foobar")
	if ok {
		t.Errorf("should not match")
	}
	c, ok = rt.Get("foo.com", "/foo/")
	if ok {
		t.Errorf("should not match")
	}

	rule.Path = nil
	rule.Path = append(rule.Path, "/foo/")
	rt = NewBasicRouteRuleTree()
	rt.Insert(rule)
	c, ok = rt.Get("foo.com", "/foo/")
	if !ok || c != cluster {
		t.Errorf("should match")
	}
	c, ok = rt.Get("foo.com", "/foo")
	if ok {
		t.Errorf("should not match")
	}

	rule.Path = nil
	rule.Path = append(rule.Path, "/foo*")
	rt = NewBasicRouteRuleTree()
	rt.Insert(rule)
	c, ok = rt.Get("foo.com", "/foo/")
	if !ok || c != cluster {
		t.Errorf("should match")
	}
	c, ok = rt.Get("foo.com", "/foo")
	if !ok || c != cluster {
		t.Errorf("should match")
	}
	c, ok = rt.Get("foo.com", "/foobar")
	if ok {
		t.Errorf("should not match")
	}
	c, ok = rt.Get("foo.com", "/bar")
	if ok {
		t.Errorf("should not match")
	}

	rule.Path = nil
	rule.Path = append(rule.Path, "/foo/*")
	rt = NewBasicRouteRuleTree()
	rt.Insert(rule)
	c, ok = rt.Get("foo.com", "/foo/")
	if !ok || c != cluster {
		t.Errorf("should match")
	}
	c, ok = rt.Get("foo.com", "/foo/bar")
	if !ok || c != cluster {
		t.Errorf("should match")
	}
	c, ok = rt.Get("foo.com", "/foo")
	if !ok || c != cluster {
		t.Errorf("should match")
	}

	rule.Path = nil
	rule.Path = append(rule.Path, "/aaa/bb*")
	rt = NewBasicRouteRuleTree()
	rt.Insert(rule)
	c, ok = rt.Get("foo.com", "/aaa/bbb")
	if ok {
		t.Errorf("should not match")
	}

	rule.Path = nil
	rule.Path = append(rule.Path, "/aaa/bbb*")
	rt = NewBasicRouteRuleTree()
	rt.Insert(rule)
	c, ok = rt.Get("foo.com", "/aaa/bbb")
	if !ok || c != cluster {
		t.Errorf("should match")
	}
	c, ok = rt.Get("foo.com", "/aaa/bbb/")
	if !ok || c != cluster {
		t.Errorf("should match")
	}
	c, ok = rt.Get("foo.com", "/aaa/bbbxyz")
	if ok {
		t.Errorf("should not match")
	}
	c, ok = rt.Get("foo.com", "/aaa/bbb/xyz")
	if !ok || c != cluster {
		t.Errorf("should match")
	}

	rule.Path = nil
	rule.Path = append(rule.Path, "/aaa/bbb/*")
	rt = NewBasicRouteRuleTree()
	rt.Insert(rule)
	c, ok = rt.Get("foo.com", "/aaa/bbb")
	if !ok || c != cluster {
		t.Errorf("should match")
	}

	rt = NewBasicRouteRuleTree()
	rule.Path = nil
	rule.Path = append(rule.Path, "/*")
	cluster1 := "c1"
	rule.ClusterName = &cluster1
	rt.Insert(rule)

	rule.Path = nil
	rule.Path = append(rule.Path, "/aaa*")
	cluster2 := "c2"
	rule.ClusterName = &cluster2
	rt.Insert(rule)

	c, ok = rt.Get("foo.com", "/aaa/ccc")
	if !ok || c != cluster2 {
		t.Errorf("should match")
	}

	rule.Path = nil
	rule.Path = append(rule.Path, "/aaa/bbb*")
	cluster3 := "c3"
	rule.ClusterName = &cluster3
	rt.Insert(rule)

	c, ok = rt.Get("foo.com", "/aaa/bbb")
	if !ok || c != cluster3 {
		t.Errorf("should match")
	}
	c, ok = rt.Get("foo.com", "/ccc")
	if !ok || c != cluster1 {
		t.Errorf("should match")
	}

	rt = NewBasicRouteRuleTree()
	rule.Path = nil
	rule.Path = append(rule.Path, "/foo")
	cluster1 = "c1"
	rule.ClusterName = &cluster1
	rt.Insert(rule)

	rule.Path = nil
	rule.Path = append(rule.Path, "/foo*")
	cluster2 = "c2"
	rule.ClusterName = &cluster2
	rt.Insert(rule)

	c, ok = rt.Get("foo.com", "/foo")
	if !ok || c != cluster1 {
		t.Errorf("should match")
	}

}

func TestHostAndPathMatchInBasicRuleTree(t *testing.T) {
	cluster := "c1"
	rule := &BasicRouteRuleFile{
		Hostname:    nil,
		Path:        nil,
		ClusterName: &cluster,
	}

	rule.Hostname = append(rule.Hostname, "aaa.foo.com")
	rule.Hostname = append(rule.Hostname, "bbb.foo.com")
	rule.Hostname = append(rule.Hostname, "ccc.foo.com")
	rule.Path = append(rule.Path, "/aaa/bbb")
	rule.Path = append(rule.Path, "/ccc/ddd")
	rule.Path = append(rule.Path, "/eee/fff*")
	cluster1 := "c1"
	rule.ClusterName = &cluster1
	rt := NewBasicRouteRuleTree()
	rt.Insert(rule)

	c, ok := rt.Get("aaa.foo.com", "/aaa/bbb")
	if !ok || c != cluster1 {
		t.Errorf("should match")
	}
	c, ok = rt.Get("ccc.foo.com", "/eee/fff/ggg")
	if !ok || c != cluster1 {
		t.Errorf("should match")
	}

}

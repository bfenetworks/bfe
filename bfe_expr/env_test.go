// Copyright (c) 2025 The BFE Authors.
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

/* env_test.go - tests for bfe_expr env functions */

package bfe_expr

import (
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_tls"
	"github.com/bfenetworks/bfe/bfe_util"
)

func newTestRequest() *bfe_basic.Request {
	header := make(bfe_http.Header)
	header.Set("X-Uid", "12345")
	header.Set("User-Agent", "TestAgent/1.0")

	return &bfe_basic.Request{
		HttpRequest: &bfe_http.Request{
			Method:     "GET",
			Host:       "www.example.com:8080",
			URL:        &url.URL{Path: "/api/user", RawQuery: "uid=100&tag=a&tag=b"},
			Header:     header,
			RequestURI: "/api/user?uid=100&tag=a&tag=b",
		},
		Session: &bfe_basic.Session{
			IsSecure: false,
			Proto:    "http",
			Vip:      net.ParseIP("10.1.1.1"),
			Vport:    443,
		},
		ClientAddr: &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 5678},
		Context:    make(map[interface{}]interface{}),
		LogId:      "test-logid",
		Route:      bfe_basic.RequestRoute{Product: "example", HostTag: "tag1"},
		Backend: bfe_basic.BackendInfo{
			ClusterName:    "clusterA",
			SubclusterName: "subA",
			BackendAddr:    "10.9.9.9",
			BackendPort:    8080,
		},
	}
}

func TestBaseEnvRequestAccess(t *testing.T) {
	env := NewEnv(newTestRequest())

	if env.ReqMethod() != "GET" {
		t.Errorf("ReqMethod() = %q", env.ReqMethod())
	}
	if env.ReqHost() != "www.example.com" {
		t.Errorf("ReqHost() = %q", env.ReqHost())
	}
	if env.ReqPort() != "8080" {
		t.Errorf("ReqPort() = %q", env.ReqPort())
	}
	if env.ReqVport() != "443" {
		t.Errorf("ReqVport() = %q", env.ReqVport())
	}
	if env.ReqPath() != "/api/user" {
		t.Errorf("ReqPath() = %q", env.ReqPath())
	}
	if env.ReqUri() != "/api/user?uid=100&tag=a&tag=b" {
		t.Errorf("ReqUri() = %q", env.ReqUri())
	}
	if env.ReqQuery("uid") != "100" {
		t.Errorf("ReqQuery(uid) = %q", env.ReqQuery("uid"))
	}
	if env.ReqHeader("X-Uid") != "12345" {
		t.Errorf("ReqHeader(X-Uid) = %q", env.ReqHeader("X-Uid"))
	}
	if env.ReqCip() != "1.2.3.4" {
		t.Errorf("ReqCip() = %q", env.ReqCip())
	}
	if env.ReqVip() != "10.1.1.1" {
		t.Errorf("ReqVip() = %q", env.ReqVip())
	}
	if env.RouteCluster() != "clusterA" || env.RouteSubCluster() != "subA" {
		t.Errorf("RouteCluster/RouteSubCluster = %q/%q", env.RouteCluster(), env.RouteSubCluster())
	}
	if env.BackendPort() != 8080 {
		t.Errorf("BackendPort() = %d", env.BackendPort())
	}

	// 缺失数据返回空值
	if env.ReqHeader("X-Not-Exist") != "" {
		t.Errorf("ReqHeader(not exist) should be empty")
	}
	if env.ResCode() != 0 || env.ResHeader("X") != "" {
		t.Errorf("response not ready should return empty value")
	}
}

func TestBaseEnvRequestQueryAllAndKeys(t *testing.T) {
	env := NewEnv(newTestRequest())

	tags := env.ReqQueryAll("tag")
	if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("ReqQueryAll(tag) = %v", tags)
	}

	keys := env.ReqQueryKeys()
	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}
	if !keySet["uid"] || !keySet["tag"] {
		t.Errorf("ReqQueryKeys() = %v", keys)
	}
}

func TestBaseEnvNilSafety(t *testing.T) {
	env := NewEnv(&bfe_basic.Request{})

	if env.ReqMethod() != "" || env.ReqHost() != "" || env.ReqPath() != "" ||
		env.ReqCip() != "" || env.ReqVip() != "" ||
		env.ReqQuery("x") != "" || env.ReqHeader("x") != "" || env.ReqCookie("x") != "" {
		t.Errorf("nil-safe accessors should return empty value")
	}
	if env.SesSecure() || env.SesTrusted() {
		t.Errorf("nil-safe bool accessors should return false")
	}
	if env.ReqQueryKeys() != nil || env.ReqTags("t") != nil {
		t.Errorf("nil-safe list accessors should return nil")
	}
}

func TestBaseEnvSessionTls(t *testing.T) {
	req := newTestRequest()
	req.Session.IsSecure = true
	req.Session.TlsState = &bfe_tls.ConnectionState{
		ServerName:   "example.com",
		ClientAuth:   true,
		ClientCAName: "ca1",
	}
	env := NewEnv(req)

	if env.SesSni() != "example.com" {
		t.Errorf("SesSni() = %q", env.SesSni())
	}
	if !env.SesSecure() {
		t.Errorf("SesSecure() should be true")
	}
	if env.SesTlsClientCa() != "ca1" {
		t.Errorf("SesTlsClientCa() = %q", env.SesTlsClientCa())
	}

	// 非 TLS 会话 SNI 为空
	req.Session.IsSecure = false
	if env.SesSni() != "" {
		t.Errorf("SesSni() on non-tls should be empty")
	}
}

func TestGetEnvSharedPerRequest(t *testing.T) {
	req := newTestRequest()

	e1 := GetEnv(req)
	e2 := GetEnv(req)
	if e1 != e2 {
		t.Errorf("GetEnv should return the same env instance within one request")
	}

	other := GetEnv(newTestRequest())
	if other == e1 {
		t.Errorf("GetEnv should create new env for different request")
	}
}

func TestHashDeterministic(t *testing.T) {
	env := NewEnv(newTestRequest())

	b1 := env.Hash("1.2.3.4")
	b2 := env.Hash("1.2.3.4")
	if b1 != b2 {
		t.Errorf("Hash should be deterministic, got %d and %d", b1, b2)
	}
	if b1 < 0 || b1 >= HashBucketSize {
		t.Errorf("Hash bucket %d out of range", b1)
	}

	// 大小写归一：大小写不同哈希相同
	if env.Hash("AbC") != env.Hash("abc") {
		t.Errorf("Hash should lowercase-normalize input")
	}
}

func TestHashEmptyInputStablePerRequest(t *testing.T) {
	env := NewEnv(newTestRequest())

	b1 := env.Hash("")
	b2 := env.Hash("")
	if b1 != b2 {
		t.Errorf("Hash(\"\") should be stable within one request, got %d and %d", b1, b2)
	}
	if b1 < 0 || b1 >= HashBucketSize {
		t.Errorf("Hash bucket %d out of range", b1)
	}
}

func TestIpInAndIpRange(t *testing.T) {
	env := NewEnv(newTestRequest())

	if !env.IpIn("10.1.2.3", []string{"10.0.0.0/8", "1.1.1.1"}) {
		t.Errorf("IpIn should hit CIDR")
	}
	if !env.IpIn("1.1.1.1", []string{"10.0.0.0/8", "1.1.1.1"}) {
		t.Errorf("IpIn should hit single IP")
	}
	if env.IpIn("2.2.2.2", []string{"10.0.0.0/8"}) {
		t.Errorf("IpIn should not hit")
	}
	if env.IpIn("bad-ip", []string{"10.0.0.0/8"}) {
		t.Errorf("IpIn with invalid ip should be false")
	}

	if !env.IpRange("10.1.1.5", "10.1.1.1", "10.1.1.9") {
		t.Errorf("IpRange should be true")
	}
	if env.IpRange("10.1.1.15", "10.1.1.1", "10.1.1.9") {
		t.Errorf("IpRange should be false")
	}
	if env.IpRange("10.1.1.5", "1.1.1.1", "::9") {
		t.Errorf("IpRange with v4/v6 mixed should be false")
	}
}

func TestRandAndSrand(t *testing.T) {
	req := newTestRequest()
	env := GetEnv(req)

	// RandInt 范围
	for i := 0; i < 100; i++ {
		if v := env.RandInt(10); v < 0 || v >= 10 {
			t.Fatalf("RandInt(10) = %d out of range", v)
		}
	}

	// 同一 seed 重播种后序列可复现（调试能力）
	r1 := env.Rand()
	env.Srand(42)
	a1, a2 := env.Rand(), env.Rand()
	env.Srand(42)
	b1, b2 := env.Rand(), env.Rand()
	if a1 != b1 || a2 != b2 {
		t.Errorf("Srand should make rand sequence reproducible")
	}
	_ = r1
}

func TestUsrVarTyped(t *testing.T) {
	req := newTestRequest()
	env := GetEnv(req)

	// typed 写入/读取
	if env.UsrVarSetInt("p", 42) != 42 {
		t.Errorf("UsrVarSetInt should return written value")
	}
	if env.UsrVarInt("p") != 42 {
		t.Errorf("UsrVarInt = %d", env.UsrVarInt("p"))
	}
	env.UsrVarSetFloat("f", 1.5)
	if env.UsrVarFloat("f") != 1.5 {
		t.Errorf("UsrVarFloat = %v", env.UsrVarFloat("f"))
	}
	env.UsrVarSetBool("b", true)
	if !env.UsrVarBool("b") {
		t.Errorf("UsrVarBool should be true")
	}

	// string 简写
	env.UsrVarSetStr("s", "hello")
	if env.UsrVar("s") != "hello" || env.UsrVarStr("s") != "hello" {
		t.Errorf("UsrVar/UsrVarStr should read string var")
	}

	// legacy USR_VAR_SET 动作写入的 string，typed getter 可解析
	req.SetUsrVariable("legacy-int", "123")
	if env.UsrVarInt("legacy-int") != 123 {
		t.Errorf("UsrVarInt should parse legacy string var, got %d", env.UsrVarInt("legacy-int"))
	}

	// typed 存储对 legacy GetUsrVariable 不可见（返回 ""）
	if req.GetUsrVariable("p") != "" {
		t.Errorf("legacy GetUsrVariable should not see typed var")
	}

	// 缺失/解析失败返回零值
	if env.UsrVarInt("not-exist") != 0 || env.UsrVarBool("not-exist") ||
		env.UsrVarStr("not-exist") != "" {
		t.Errorf("missing var should return zero value")
	}
	req.SetUsrVariable("bad-int", "abc")
	if env.UsrVarInt("bad-int") != 0 {
		t.Errorf("unparsable string should return 0")
	}
}

func TestInTimeRange(t *testing.T) {
	env := NewEnv(newTestRequest())

	// bfe_util.ParseTime 格式：YYYYMMDDHHMMSS + 时区字母（Z=UTC）
	past := time.Now().Add(-time.Hour).UTC().Format("20060102150405") + "Z"
	future := time.Now().Add(time.Hour).UTC().Format("20060102150405") + "Z"
	if !env.InTimeRange(past, future) {
		t.Errorf("InTimeRange should be true for covering range")
	}
	if env.InTimeRange(future, past) {
		t.Errorf("InTimeRange should be false for inverted range")
	}
	if env.InTimeRange("bad-format", future) {
		t.Errorf("InTimeRange with invalid format should be false")
	}
}

func TestLogFuncs(t *testing.T) {
	req := newTestRequest()
	env := NewEnv(req)

	// 返回 true，便于嵌入条件表达式
	if !env.LogInfo("hello") || !env.LogWarn("warn msg") {
		t.Errorf("LogInfo/LogWarn should return true")
	}

	// nil 请求也安全
	nilEnv := NewEnv(&bfe_basic.Request{})
	if !nilEnv.LogInfo("nil req") {
		t.Errorf("LogInfo should be nil-safe")
	}

	// 编译 + 求值：cond && LogInfo(...) 利用短路，仅条件命中时输出日志
	cond := MustCompile(`ReqPath() == "/api/user" && LogInfo("hit /api/user, uid=" + ReqQuery("uid"))`)
	if !cond.Match(req) {
		t.Errorf("cond with LogInfo should eval to true")
	}
	cond2 := MustCompile(`ReqPath() == "/other" && LogInfo("not printed")`)
	if cond2.Match(req) {
		t.Errorf("short-circuited LogInfo should eval to false")
	}
}

func TestNowDebugTimeHeader(t *testing.T) {
	req := newTestRequest()
	debugTime := "20240601120000H" // 2024-06-01 12:00:00 UTC+8
	req.HttpRequest.Header.Set("X-Bfe-Debug-Time", debugTime)
	env := NewEnv(req)

	now := env.Now()
	expected, err := bfe_util.ParseTime(debugTime)
	if err != nil {
		t.Fatalf("parse debug time: %v", err)
	}
	if !now.Equal(expected) {
		t.Errorf("Now() = %v, expect debug time %v", now, expected)
	}
}

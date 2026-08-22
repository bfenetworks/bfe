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

package mod_access_pb3

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"

	bfe_access_pb3 "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
)

func makeRequestLogTest(t *testing.T) (*ModuleAccessPb3, *bfe_basic.Request, *bfe_http.Response) {
	m := NewModuleAccessPb3()

	conn := newTestConn()
	session := bfe_basic.NewSession(conn)
	session.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}
	session.Vip = net.ParseIP("10.0.0.1")
	session.StartTime = time.Now().Add(-time.Second)
	session.SetTrustSource(true)
	session.SessionId = "100"
	session.Proto = "http"

	req, err := bfe_http.NewRequest("GET", "http://www.example.org/path?key=value", nil)
	if err != nil {
		t.Fatalf("NewRequest() error: %v", err)
	}
	req.RequestURI = "/path?key=value"
	req.Header.Set("User-Agent", "test-agent")
	req.Header.Set("Referer", "http://referer.example.org")
	req.Header.Set("X-Forwarded-For", "1.1.1.1,2.2.2.2")
	req.Header.Set("Accept-Language", "zh-CN")
	req.Header.Set("Authorization", "Bearer token123")
	req.Header.Set("Transfer-Encoding", "chunked")
	req.Header.Set("X-Bfe-Proto", "https")
	req.State = &bfe_http.RequestState{SerialNumber: 2}

	outReq, _ := bfe_http.NewRequest("GET", "http://backend/path?key=value", nil)
	outReq.State = &bfe_http.RequestState{
		ConnectBackendStart: time.Now().Add(-300 * time.Millisecond),
		ConnectBackendEnd:   time.Now().Add(-200 * time.Millisecond),
	}

	stat := bfe_basic.NewRequestStat(time.Now().Add(-500 * time.Millisecond))
	stat.ReadReqEnd = time.Now().Add(-400 * time.Millisecond)
	stat.ClusterStart = time.Now().Add(-300 * time.Millisecond)
	stat.ClusterEnd = time.Now().Add(-200 * time.Millisecond)
	stat.BackendStart = time.Now().Add(-250 * time.Millisecond)
	stat.BackendEnd = time.Now().Add(-150 * time.Millisecond)
	stat.ResponseStart = time.Now().Add(-100 * time.Millisecond)
	stat.ResponseEnd = time.Now().Add(-50 * time.Millisecond)
	stat.BackendFirst = time.Now().Add(-390 * time.Millisecond)
	stat.HeaderLenIn = 100
	stat.BodyLenIn = 200
	stat.HeaderLenOut = 80
	stat.BodyLenOut = 160

	bfeReq := bfe_basic.NewRequest(req, conn, stat, session, nil)
	bfeReq.OutRequest = outReq
	bfeReq.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 54321}
	bfeReq.ClientAddr = &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 33333}
	bfeReq.LogId = "12345"
	bfeReq.Route.Product = "unit-test"
	bfeReq.ErrCode = errors.New("err001")
	bfeReq.ErrMsg = "timeout"
	bfeReq.Backend = bfe_basic.BackendInfo{
		ClusterName:    "cluster1",
		SubclusterName: "subcluster1",
		BackendAddr:    "10.0.0.3",
		BackendPort:    8080,
		BackendName:    "backend1",
	}
	bfeReq.RetryTime = 1
	bfeReq.IsSse = true

	res := &bfe_http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Proto:      "HTTP/1.1",
		Header:     make(bfe_http.Header),
	}
	res.Header.Set("Content-Type", "text/html")
	res.Header.Set("Location", "http://redirect.example.org")
	res.Header.Set("Transfer-Encoding", "chunked")

	return m, bfeReq, res
}

func TestRequestLogGen(t *testing.T) {
	m, req, res := makeRequestLogTest(t)
	bfeLog := m.requestLogGen(req, res)

	if bfeLog == nil {
		t.Fatal("requestLogGen() returns nil")
	}
	if bfeLog.Product == nil || *bfeLog.Product != bfe_access_pb3.ProductID_BFE {
		t.Error("Product error")
	}
	if bfeLog.LogType == nil || *bfeLog.LogType != bfe_access_pb3.BfeLogType_Request {
		t.Error("LogType error")
	}
	if bfeLog.RequestLog == nil {
		t.Fatal("RequestLog is nil")
	}
}

func TestReqLogTagGen(t *testing.T) {
	tests := []struct {
		product string
		errCode error
		want    string
	}{
		{"unittest", nil, "req_unittest"},
		{"", nil, "req_bfe"},
		{"unit-test", nil, "req_unit_test"},
		{"unittest", errors.New("err"), "req_err_unittest"},
		{"", errors.New("err"), "req_err_bfe"},
	}

	for _, tt := range tests {
		req := &bfe_basic.Request{
			Route:   bfe_basic.RequestRoute{Product: tt.product},
			ErrCode: tt.errCode,
		}
		got := reqLogTagGen(req)
		if got != tt.want {
			t.Errorf("reqLogTagGen(%s, %v) got: %s, want: %s", tt.product, tt.errCode, got, tt.want)
		}
	}
}

func TestReqBasicInfoGen(t *testing.T) {
	_, req, _ := makeRequestLogTest(t)
	reqLog := &bfe_access_pb3.RequestLog{}
	reqBasicInfoGen(reqLog, req)

	if reqLog.ErrCode == nil || *reqLog.ErrCode != "err001" {
		t.Error("ErrCode error")
	}
	if reqLog.ErrMsg == nil || *reqLog.ErrMsg != "timeout" {
		t.Error("ErrMsg error")
	}
	if reqLog.ReqHeaderLen == nil || *reqLog.ReqHeaderLen != 100 {
		t.Error("ReqHeaderLen error")
	}
	if reqLog.ReqBodyLen == nil || *reqLog.ReqBodyLen != 200 {
		t.Error("ReqBodyLen error")
	}
	if reqLog.SessionId == nil || *reqLog.SessionId != 100 {
		t.Error("SessionId error")
	}
	if reqLog.AddrInfo == nil {
		t.Error("AddrInfo is nil")
	}
	if reqLog.ClientIp == nil || *reqLog.ClientIp == 0 {
		t.Error("ClientIp error")
	}
	if reqLog.ReqNum == nil || *reqLog.ReqNum != 2 {
		t.Error("ReqNum error")
	}
}

func TestReqClientNetworkGen(t *testing.T) {
	_, req, _ := makeRequestLogTest(t)
	reqLog := &bfe_access_pb3.RequestLog{}
	reqClientNetworkGen(reqLog, req)
	if reqLog.ClientNetwork == nil || *reqLog.ClientNetwork != bfe_access_pb3.NetType_Ipv4 {
		t.Errorf("ClientNetwork error, got: %v", reqLog.ClientNetwork)
	}
}

func TestReqClientNetworkGenIPv6(t *testing.T) {
	conn := newTestConn()
	session := bfe_basic.NewSession(conn)
	session.SetTrustSource(false)
	req, _ := bfe_http.NewRequest("GET", "http://www.example.org/", nil)
	bfeReq := bfe_basic.NewRequest(req, nil, nil, session, nil)
	bfeReq.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("::1"), Port: 12345}

	reqLog := &bfe_access_pb3.RequestLog{}
	reqClientNetworkGen(reqLog, bfeReq)
	if reqLog.ClientNetwork == nil || *reqLog.ClientNetwork != bfe_access_pb3.NetType_Ipv6 {
		t.Errorf("ClientNetwork error, got: %v", reqLog.ClientNetwork)
	}
}

func TestIsV6Addr(t *testing.T) {
	if !isV6Addr(net.ParseIP("::1")) {
		t.Error("::1 should be IPv6")
	}
	if isV6Addr(net.ParseIP("127.0.0.1")) {
		t.Error("127.0.0.1 should not be IPv6")
	}
}

func TestReqReqHeaderInfoGen(t *testing.T) {
	_, req, _ := makeRequestLogTest(t)
	reqLog := &bfe_access_pb3.RequestLog{}
	reqReqHeaderInfoGen(reqLog, req)

	if reqLog.Proto == nil || *reqLog.Proto != "https" {
		t.Errorf("Proto error, got: %s", *reqLog.Proto)
	}
	if reqLog.HeaderHost == nil || *reqLog.HeaderHost != "www.example.org" {
		t.Error("HeaderHost error")
	}
	if reqLog.OriginUri == nil || *reqLog.OriginUri != "/path?key=value" {
		t.Error("OriginUri error")
	}
	if reqLog.Referrer == nil || *reqLog.Referrer != "http://referer.example.org" {
		t.Error("Referrer error")
	}
	if reqLog.XForwardFor == nil || *reqLog.XForwardFor != "1.1.1.1,2.2.2.2" {
		t.Error("XForwardFor error")
	}
	if reqLog.AcceptLanguage == nil || *reqLog.AcceptLanguage != "zh-CN" {
		t.Error("AcceptLanguage error")
	}
	if reqLog.Authorization == nil || *reqLog.Authorization != "Bearer token123" {
		t.Error("Authorization error")
	}
	if reqLog.UserAgent == nil || *reqLog.UserAgent != "test-agent" {
		t.Error("UserAgent error")
	}
	if reqLog.TransferEncoding == nil || *reqLog.TransferEncoding != "chunked" {
		t.Error("TransferEncoding error")
	}
	if reqLog.Method == nil || *reqLog.Method != "GET" {
		t.Error("Method error")
	}
}

func TestReqFindLocationInfoGen(t *testing.T) {
	_, req, _ := makeRequestLogTest(t)
	reqLog := &bfe_access_pb3.RequestLog{}
	reqFindLocationInfoGen(reqLog, req)

	if reqLog.Product == nil || *reqLog.Product != "unit-test" {
		t.Error("Product error")
	}
	if reqLog.Cluster == nil || *reqLog.Cluster != "cluster1" {
		t.Error("Cluster error")
	}
	if reqLog.SubCluster == nil || *reqLog.SubCluster != "subcluster1" {
		t.Error("SubCluster error")
	}
	if reqLog.BackendInfo == nil {
		t.Error("BackendInfo is nil")
	}
	if reqLog.BackendRetry == nil || *reqLog.BackendRetry != 1 {
		t.Error("BackendRetry error")
	}
}

func TestReqResponseInfoGen(t *testing.T) {
	_, req, res := makeRequestLogTest(t)
	reqLog := &bfe_access_pb3.RequestLog{}
	reqResponseInfoGen(reqLog, req, res)

	if reqLog.ResStatusCode == nil || *reqLog.ResStatusCode != 200 {
		t.Error("ResStatusCode error")
	}
	if reqLog.ResHeaderLen == nil || *reqLog.ResHeaderLen != 80 {
		t.Error("ResHeaderLen error")
	}
	if reqLog.ResBodyLen == nil || *reqLog.ResBodyLen != 160 {
		t.Error("ResBodyLen error")
	}
	if reqLog.ResContentType == nil || *reqLog.ResContentType != "text/html" {
		t.Error("ResContentType error")
	}
	if reqLog.ResLocation == nil || *reqLog.ResLocation != "http://redirect.example.org" {
		t.Error("ResLocation error")
	}
	if reqLog.ResTransferEncoding == nil || *reqLog.ResTransferEncoding != "chunked" {
		t.Error("ResTransferEncoding error")
	}
}

func TestReqResponseInfoGenNilRes(t *testing.T) {
	_, req, _ := makeRequestLogTest(t)
	reqLog := &bfe_access_pb3.RequestLog{}
	reqResponseInfoGen(reqLog, req, nil)

	if reqLog.ResStatusCode != nil {
		t.Error("ResStatusCode should be nil when res is nil")
	}
}

func TestReqTimeInfoGen(t *testing.T) {
	_, req, _ := makeRequestLogTest(t)
	reqLog := &bfe_access_pb3.RequestLog{}
	reqTimeInfoGen(reqLog, req)

	if reqLog.AllTime == nil || *reqLog.AllTime == 0 {
		t.Error("AllTime error")
	}
	if reqLog.ReadClientTime == nil || *reqLog.ReadClientTime == 0 {
		t.Error("ReadClientTime error")
	}
	if reqLog.ClusterServeTime == nil || *reqLog.ClusterServeTime == 0 {
		t.Error("ClusterServeTime error")
	}
	if reqLog.BackendServeTime == nil || *reqLog.BackendServeTime == 0 {
		t.Error("BackendServeTime error")
	}
	if reqLog.WriteClientTime == nil || *reqLog.WriteClientTime == 0 {
		t.Error("WriteClientTime error")
	}
	if reqLog.SessionOffsetTime == nil || *reqLog.SessionOffsetTime == 0 {
		t.Error("SessionOffsetTime error")
	}
	if reqLog.ConnectBackendTime == nil {
		t.Error("ConnectBackendTime is nil")
	}
	if reqLog.ProxyDelayTime == nil || *reqLog.ProxyDelayTime == 0 {
		t.Error("ProxyDelayTime error")
	}
}

func TestIsStreamResponse(t *testing.T) {
	req := &bfe_basic.Request{IsSse: true}
	if !isStreamResponse(req, nil) {
		t.Error("isStreamResponse should return true for SSE request")
	}

	req.IsSse = false
	if isStreamResponse(req, nil) {
		t.Error("isStreamResponse should return false for non-SSE request")
	}
}

func TestReqAiInfoGen(t *testing.T) {
	_, req, res := makeRequestLogTest(t)

	aiInfo := &bfe_basic.AiBasicInfo{
		ClientKeyId:  "key-id-123",
		ClientModel:  "model-a",
		TargetModel:  "model-b",
		Provider:     "deepseek",
		RetryCount:   1,
		CostCurrency: "RMB",
		ClusterKeyNames: []bfe_basic.ClusterKeyName{
			{ClusterName: "cluster-a", KeyName: "key-001"},
		},
		TokenTimeInfo: bfe_basic.TokenTimeInfo{
			TTFT: 1000,
			TPOT: 2000,
		},
		AiAuthInfo: bfe_basic.AiAuthInfo{
			RejectReason:     "quota exhausted",
			RejectQuotaPlans: []string{"plan1", "plan2"},
			HitQuotaPlans:    []string{"plan3", "plan4"},
		},
	}
	usage := aiInfo.GetTokenUsage()
	usage.PromptTokens = 10
	usage.CompletionTokens = 20
	usage.UsedQuota = 30
	usage.CacheReadTokens = 5
	usage.CacheWriteTokens = 2
	usage.AudioInputTokens = 3
	usage.AudioOutputTokens = 4
	usage.UsedCost = 5000
	req.SetContext(bfe_basic.REQ_AI_BASIC_CONTEXT, aiInfo)

	req.SetAiRouteResult(&bfe_basic.AiRouteResult{
		RouteType: "apikey",
		Owner:     "ak_user_a",
		RuleName:  "user_a-rule1",
	})

	hitInfo := &bfe_basic.AiRateLimitHitInfo{
		HitPolicyDict: map[string]*bfe_basic.HitPolicyInfo{
			"policy1": {
				TpmRules:      []string{"tpm1"},
				RpmRules:      []string{"rpm1"},
				IsConcurrency: true,
				IsRedisError:  "redis_err",
			},
		},
	}
	req.SetContext(bfe_basic.REQ_AI_RATE_LIMIT_HIT, hitInfo)

	reqLog := &bfe_access_pb3.RequestLog{}
	reqAiInfoGen(reqLog, req, res)

	if reqLog.AiApikeyId == nil || *reqLog.AiApikeyId != "key-id-123" {
		t.Error("AiApikeyId error")
	}
	if reqLog.AiRequestedModel == nil || *reqLog.AiRequestedModel != "model-a" {
		t.Error("AiRequestedModel error")
	}
	if reqLog.AiTargetModel == nil || *reqLog.AiTargetModel != "model-b" {
		t.Error("AiTargetModel error")
	}
	if reqLog.AiProvider == nil || *reqLog.AiProvider != "deepseek" {
		t.Error("AiProvider error")
	}
	if reqLog.AiStream == nil || !*reqLog.AiStream {
		t.Error("AiStream error")
	}
	if reqLog.AiInputTokens == nil || *reqLog.AiInputTokens != 10 {
		t.Error("AiInputTokens error")
	}
	if reqLog.AiCacheReadTokens == nil || *reqLog.AiCacheReadTokens != 5 {
		t.Errorf("AiCacheReadTokens error, got: %v", reqLog.AiCacheReadTokens)
	}
	if reqLog.AiCacheWriteTokens == nil || *reqLog.AiCacheWriteTokens != 2 {
		t.Errorf("AiCacheWriteTokens error, got: %v", reqLog.AiCacheWriteTokens)
	}
	if reqLog.AiAudioInputTokens == nil || *reqLog.AiAudioInputTokens != 3 {
		t.Errorf("AiAudioInputTokens error, got: %v", reqLog.AiAudioInputTokens)
	}
	if reqLog.AiAudioOutputTokens == nil || *reqLog.AiAudioOutputTokens != 4 {
		t.Errorf("AiAudioOutputTokens error, got: %v", reqLog.AiAudioOutputTokens)
	}
	if reqLog.AiCostValue == nil || *reqLog.AiCostValue != 5000 {
		t.Error("AiCostValue error")
	}
	if reqLog.AiCostCurrency == nil || *reqLog.AiCostCurrency != "RMB" {
		t.Error("AiCostCurrency error")
	}
	if reqLog.AiRetryCount == nil || *reqLog.AiRetryCount != 1 {
		t.Error("AiRetryCount error")
	}
	if reqLog.AiTtftUs == nil || *reqLog.AiTtftUs != 1000 {
		t.Error("AiTtftUs error")
	}
	if reqLog.AiTpotUs == nil || *reqLog.AiTpotUs != 2000 {
		t.Error("AiTpotUs error")
	}
	if reqLog.AiAuthRejectReason == nil {
		t.Error("AiAuthRejectReason is nil")
	}
	if len(reqLog.AiAuthRejectQuotaPlans) != 2 {
		t.Error("AiAuthRejectQuotaPlans length error")
	}
	if len(reqLog.AiAuthHitQuotaPlans) != 2 {
		t.Error("AiAuthHitQuotaPlans length error")
	}
	if len(reqLog.AiRouteRuleHits) != 1 {
		t.Error("AiRouteRuleHits length error")
	}
	if len(reqLog.AiClusterKeyNames) != 1 {
		t.Error("AiClusterKeyNames length error")
	}
	if len(reqLog.AiRateLimitHits) != 4 {
		t.Errorf("AiRateLimitHits length error, got: %d", len(reqLog.AiRateLimitHits))
	}
}

func TestReqAiInfoGenNil(t *testing.T) {
	_, req, res := makeRequestLogTest(t)
	reqLog := &bfe_access_pb3.RequestLog{}
	reqAiInfoGen(reqLog, req, res)

	if reqLog.AiApikeyId != nil {
		t.Error("AiApikeyId should be nil when no ai info")
	}
}

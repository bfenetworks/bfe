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

package mod_ai_token_auth

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

const testConfRoot = "testdata/mod_ai_token_auth"

func newTestRequest(apiKey, product string) *bfe_basic.Request {
	httpReq, _ := bfe_http.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions", nil)
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req := bfe_basic.NewRequest(httpReq, nil, nil, nil, nil)
	req.Route = bfe_basic.RequestRoute{Product: product}
	return req
}

func prepareTestModule(t *testing.T) *ModuleAITokenAuth {
	m := NewModuleAITokenAuth()
	m.conf = &ConfModAITokenAuth{}
	m.conf.Basic.ProductRulePath = "testdata/mod_ai_token_auth/token_rule.data"
	m.conf.Redis.Bns = "localhost"
	m.conf.Redis.ConnectTimeout = 10
	m.conf.Redis.ReadTimeout = 10
	m.conf.Redis.WriteTimeout = 10
	if err := m.loadProductRuleConf(nil); err != nil {
		t.Fatalf("loadProductRuleConf failed: %s", err)
	}
	return m
}

func TestNewModuleAITokenAuthAndName(t *testing.T) {
	m := NewModuleAITokenAuth()
	if m == nil {
		t.Fatal("NewModuleAITokenAuth should not return nil")
	}
	if m.Name() != ModAITokenAuth {
		t.Errorf("expected name %s, got %s", ModAITokenAuth, m.Name())
	}
	if m.ruleTable == nil {
		t.Error("ruleTable should be initialized")
	}
}

func TestConfLoadSuccess(t *testing.T) {
	cfg, err := ConfLoad("testdata/mod_ai_token_auth/mod_ai_token_auth.conf", testConfRoot)
	if err != nil {
		t.Fatalf("ConfLoad failed: %s", err)
	}
	if cfg == nil {
		t.Fatal("ConfLoad should return non-nil config")
	}
	if !strings.Contains(cfg.Basic.ProductRulePath, "token_rule.data") {
		t.Errorf("unexpected ProductRulePath: %s", cfg.Basic.ProductRulePath)
	}
	if cfg.Redis.Bns != "localhost" {
		t.Errorf("unexpected Redis.Bns: %s", cfg.Redis.Bns)
	}
}

func TestConfLoadFileNotFound(t *testing.T) {
	_, err := ConfLoad("testdata/mod_ai_token_auth/non_existent.conf", testConfRoot)
	if err == nil {
		t.Error("expected error for missing conf file")
	}
}

func TestConfCheckValidationFailures(t *testing.T) {
	base := func() *ConfModAITokenAuth {
		cfg := &ConfModAITokenAuth{}
		cfg.Basic.ProductRulePath = "token_rule.data"
		cfg.Redis.Bns = "localhost"
		cfg.Redis.ConnectTimeout = 10
		cfg.Redis.ReadTimeout = 10
		cfg.Redis.WriteTimeout = 10
		return cfg
	}

	cases := []struct {
		name    string
		mutate  func(*ConfModAITokenAuth)
		wantErr string
	}{
		{
			name: "empty redis bns",
			mutate: func(cfg *ConfModAITokenAuth) {
				cfg.Redis.Bns = ""
			},
			wantErr: "Redis.Bns check err",
		},
		{
			name: "invalid redis bns",
			mutate: func(cfg *ConfModAITokenAuth) {
				cfg.Redis.Bns = "a,b"
			},
			wantErr: "Redis.Bns check err",
		},
		{
			name: "non-positive connect timeout",
			mutate: func(cfg *ConfModAITokenAuth) {
				cfg.Redis.ConnectTimeout = 0
			},
			wantErr: "Redis.ConnectTimeout must > 0",
		},
		{
			name: "non-positive read timeout",
			mutate: func(cfg *ConfModAITokenAuth) {
				cfg.Redis.ReadTimeout = -1
			},
			wantErr: "Redis.ReadTimeout/WriteTimeout must > 0",
		},
		{
			name: "non-positive write timeout",
			mutate: func(cfg *ConfModAITokenAuth) {
				cfg.Redis.WriteTimeout = -1
			},
			wantErr: "Redis.ReadTimeout/WriteTimeout must > 0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			tc.mutate(cfg)
			err := cfg.Check(testConfRoot)
			if err == nil {
				t.Fatalf("expected error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %s", tc.wantErr, err)
			}
		})
	}
}

func TestConfCheckDefaultProductRulePath(t *testing.T) {
	cfg := &ConfModAITokenAuth{}
	cfg.Redis.Bns = "localhost"
	cfg.Redis.ConnectTimeout = 10
	cfg.Redis.ReadTimeout = 10
	cfg.Redis.WriteTimeout = 10

	if err := cfg.Check(testConfRoot); err != nil {
		t.Fatalf("Check failed: %s", err)
	}
	if !strings.Contains(cfg.Basic.ProductRulePath, "mod_ai_token_auth/token_rule.data") {
		t.Errorf("unexpected default ProductRulePath: %s", cfg.Basic.ProductRulePath)
	}
}

func TestProductRuleConfLoadSuccess(t *testing.T) {
	conf, err := ProductRuleConfLoad("testdata/mod_ai_token_auth/token_rule.data")
	if err != nil {
		t.Fatalf("ProductRuleConfLoad failed: %s", err)
	}
	if conf.Version != "1.0" {
		t.Errorf("expected version 1.0, got %s", conf.Version)
	}
	if _, ok := conf.Config["AI_product"]; !ok {
		t.Error("expected AI_product rules")
	}
	if _, ok := conf.Tokens["AI_product"]; !ok {
		t.Error("expected AI_product tokens")
	}
}

func TestProductRuleConfLoadInvalid(t *testing.T) {
	dir := t.TempDir()
	filename := path.Join(dir, "invalid.data")
	content := `{"Version": "1.0", "Tokens": {}, "QuotaPlans": {}}`
	if err := ioutil.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatalf("write file failed: %s", err)
	}

	if _, err := ProductRuleConfLoad(filename); err == nil {
		t.Error("expected error for invalid rule conf")
	}
}

func TestModuleLoadProductRuleConf(t *testing.T) {
	m := prepareTestModule(t)

	rules, ok := m.ruleTable.Search("AI_product")
	if !ok {
		t.Fatal("expected AI_product rules in table")
	}
	if rules == nil || len(*rules) != 1 {
		t.Fatalf("expected one rule, got %v", rules)
	}

	tok, ok := m.ruleTable.GetToken("AI_product", "ak-123")
	if !ok {
		t.Fatal("expected token ak-123")
	}
	if tok.Key != "ak-123" {
		t.Errorf("unexpected token key %s", tok.Key)
	}
}

func TestMatchTokenRule(t *testing.T) {
	m := prepareTestModule(t)

	req := newTestRequest("", "AI_product")
	if !m.matchTokenRule(req) {
		t.Error("expected rule to match")
	}
	if v := m.state.ReqTotal.Get(); v != 1 {
		t.Errorf("expected ReqTotal 1, got %d", v)
	}

	req2 := newTestRequest("", "unknown_product")
	if m.matchTokenRule(req2) {
		t.Error("expected no rule for unknown product")
	}
}

func TestGetApiKey(t *testing.T) {
	cases := []struct {
		header   string
		expected string
	}{
		{"", ""},
		{"Bearer sk-abc123", "abc123"},
		{"sk-xyz789", "xyz789"},
		{"Bearer plainkey", "plainkey"},
	}

	for _, tc := range cases {
		req := newTestRequest("", "AI_product")
		if tc.header != "" {
			req.HttpRequest.Header.Set("Authorization", tc.header)
		}
		if got := GetApiKey(req); got != tc.expected {
			t.Errorf("GetApiKey(%q) = %q, want %q", tc.header, got, tc.expected)
		}
	}
}

func TestSetApiKey(t *testing.T) {
	req, _ := bfe_http.NewRequest(http.MethodGet, "http://example.com/", nil)
	SetApiKey(req, "")
	if req.Header.Get("Authorization") != "" {
		t.Error("empty api key should not set header")
	}

	SetApiKey(req, "mykey")
	if got := req.Header.Get("Authorization"); got != "Bearer mykey" {
		t.Errorf("unexpected Authorization header: %s", got)
	}
}

func TestCalcReqUsedQuota(t *testing.T) {
	if got := CalcReqUsedQuota(nil, -1, 10); got != 0 {
		t.Errorf("negative prompt should return 0, got %d", got)
	}
	if got := CalcReqUsedQuota(nil, 10, -1); got != 0 {
		t.Errorf("negative completion should return 0, got %d", got)
	}
	if got := CalcReqUsedQuota(nil, 3, 7); got != 10 {
		t.Errorf("expected 10, got %d", got)
	}
}

func TestGetPromptToken(t *testing.T) {
	body := strings.Repeat("a", 40)
	httpReq, _ := bfe_http.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions", strings.NewReader(body))
	req := bfe_basic.NewRequest(httpReq, nil, nil, nil, nil)

	if got := GetPromptToken(req); got != int64(len(body))/4 {
		t.Errorf("expected %d, got %d", int64(len(body))/4, got)
	}
}

func TestUpdateCtxByUsage(t *testing.T) {
	req := newTestRequest("", "AI_product")
	ai := req.InitAiBasicInfo()
	ctx := &TokenAuthContext{aiBasicInfo: ai}

	UpdateCtxByUsage(ctx, []byte(`{"usage":{"total_tokens":10,"prompt_tokens":3,"completion_tokens":7}}`))
	usage := ai.GetTokenUsage()
	if usage.UsedQuota != 10 || usage.PromptTokens != 3 || usage.CompletionTokens != 7 {
		t.Errorf("unexpected usage: %+v", usage)
	}

	ctx2 := &TokenAuthContext{aiBasicInfo: ai}
	UpdateCtxByUsage(ctx2, []byte(`{"usage":{"prompt_tokens":2,"completion_tokens":4}}`))
	usage = ai.GetTokenUsage()
	if usage.UsedQuota != 6 || usage.PromptTokens != 2 || usage.CompletionTokens != 4 {
		t.Errorf("unexpected usage: %+v", usage)
	}
}

func TestTokenAuthContext(t *testing.T) {
	req := newTestRequest("", "AI_product")
	ai := req.InitAiBasicInfo()
	tok := &Token{Key: "ak-123"}

	SetTokenAuthContext(req, tok, 5, []bfe_basic.ApikeyTag{{TagName: "t", TagValue: "v"}})
	ctx := GetTokenAuthContext(req)
	if ctx == nil {
		t.Fatal("expected token auth context")
	}
	if ctx.Token.Key != "ak-123" {
		t.Errorf("unexpected token in context: %s", ctx.Token.Key)
	}
	usage := ai.GetTokenUsage()
	if usage.PromptTokens != 5 || usage.CompletionTokens != bfe_basic.COMPLETION_TOKENS_UNKNOWN {
		t.Errorf("unexpected usage: %+v", usage)
	}
	if len(ai.ApikeyTags) != 1 || ai.ApikeyTags[0].TagValue != "v" {
		t.Errorf("unexpected tags: %+v", ai.ApikeyTags)
	}

	if GetTokenAuthContext(newTestRequest("", "")) != nil {
		t.Error("expected nil context for request without context")
	}
}

func TestTokenCheck(t *testing.T) {
	valid := func() *TokenFile {
		return &TokenFile{
			Key:            "ak-123",
			Status:         TokenStatusEnabled,
			ExpiredTime:    -1,
			UnlimitedQuota: true,
		}
	}

	if err := tokenCheck(valid()); err != nil {
		t.Errorf("valid token failed: %s", err)
	}

	cases := []struct {
		name   string
		mutate func(*TokenFile)
		errSub string
	}{
		{
			name: "missing key",
			mutate: func(tf *TokenFile) {
				tf.Key = ""
			},
			errSub: "no Key",
		},
		{
			name: "invalid status",
			mutate: func(tf *TokenFile) {
				tf.Status = 0
			},
			errSub: "invalid Status",
		},
		{
			name: "invalid expired time",
			mutate: func(tf *TokenFile) {
				tf.ExpiredTime = -2
			},
			errSub: "invalid ExpiredTime",
		},
		{
			name: "missing quota plans",
			mutate: func(tf *TokenFile) {
				tf.UnlimitedQuota = false
			},
			errSub: "QuotaPlans must be non-empty",
		},
		{
			name: "empty model",
			mutate: func(tf *TokenFile) {
				s := "gpt-4,"
				tf.Models = &s
			},
			errSub: "Models cannot contain empty strings",
		},
		{
			name: "empty block model",
			mutate: func(tf *TokenFile) {
				s := "gpt-4,"
				tf.BlockModels = &s
			},
			errSub: "BlockModels cannot contain empty strings",
		},
		{
			name: "invalid subnet",
			mutate: func(tf *TokenFile) {
				s := "invalid"
				tf.Subnet = &s
			},
			errSub: "invalid subnet",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tf := valid()
			tc.mutate(tf)
			err := tokenCheck(tf)
			if err == nil {
				t.Fatalf("expected error containing %q", tc.errSub)
			}
			if !strings.Contains(err.Error(), tc.errSub) {
				t.Errorf("expected error containing %q, got %s", tc.errSub, err)
			}
		})
	}
}

func TestTokenConvert(t *testing.T) {
	tf := TokenFile{
		Key:            "ak-123",
		Status:         TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: false,
		QuotaPlans:     []string{"plan1"},
	}

	planMap := QuotaPlanMap{"plan1": {Id: "plan1", Unlimited: true}}
	token, err := tokenConvert(tf, &planMap)
	if err != nil {
		t.Fatalf("tokenConvert failed: %s", err)
	}
	if token.Key != "ak-123" || len(token.QuotaPlans) != 1 {
		t.Errorf("unexpected token: %+v", token)
	}

	if _, err := tokenConvert(tf, nil); err == nil {
		t.Error("expected error when quotaPlansMap is nil")
	}

	missingMap := QuotaPlanMap{}
	if _, err := tokenConvert(tf, &missingMap); err == nil {
		t.Error("expected error when quota plan is missing")
	}
}

func TestActionFileCheck(t *testing.T) {
	if err := ActionFileCheck(&ActionFile{Cmd: ActionCheckToken}); err != nil {
		t.Errorf("valid action failed: %s", err)
	}
	if err := ActionFileCheck(&ActionFile{Cmd: "INVALID"}); err == nil {
		t.Error("expected error for invalid action")
	}
}

func TestGetUUID(t *testing.T) {
	u := GetUUID()
	if len(u) == 0 {
		t.Error("UUID should not be empty")
	}
	if strings.Contains(u, "-") {
		t.Error("UUID should not contain dashes")
	}
}

func TestTokenRuleTable(t *testing.T) {
	table := NewTokenRuleTable()
	if table == nil {
		t.Fatal("NewTokenRuleTable should not return nil")
	}

	conf, err := ProductRuleConfLoad("testdata/mod_ai_token_auth/token_rule.data")
	if err != nil {
		t.Fatalf("ProductRuleConfLoad failed: %s", err)
	}
	table.Update(conf)

	if rules, ok := table.Search("AI_product"); !ok || rules == nil || len(*rules) != 1 {
		t.Error("expected one rule for AI_product")
	}
	if _, ok := table.Search("unknown"); ok {
		t.Error("expected no rules for unknown product")
	}

	if tok, ok := table.GetToken("AI_product", "ak-123"); !ok || tok.Key != "ak-123" {
		t.Error("expected token ak-123")
	}
	if _, ok := table.GetToken("AI_product", "notexist"); ok {
		t.Error("expected token not found")
	}
}

func TestValidateUserToken(t *testing.T) {
	table := NewTokenRuleTable()
	conf, _ := ProductRuleConfLoad("testdata/mod_ai_token_auth/token_rule.data")
	table.Update(conf)

	if _, err := table.ValidateUserToken("AI_product", ""); err == nil {
		t.Error("expected error for empty key")
	}
	if _, err := table.ValidateUserToken("AI_product", "notexist"); err == nil {
		t.Error("expected error for missing token")
	}

	tok, err := table.ValidateUserToken("AI_product", "ak-123")
	if err != nil {
		t.Fatalf("valid token failed: %s", err)
	}
	if tok.Key != "ak-123" {
		t.Errorf("unexpected token: %s", tok.Key)
	}

	exhausted := &Token{Key: "ak-exhausted", Status: TokenStatusExhausted, Name: "ex", ExpiredTime: -1}
	disabled := &Token{Key: "ak-disabled", Status: TokenStatusDisabled, Name: "dis", ExpiredTime: -1}
	expired := &Token{Key: "ak-expired", Status: TokenStatusEnabled, Name: "exp", ExpiredTime: time.Now().Unix() - 10}
	table.lock.Lock()
	(*table.productTokens["AI_product"])["ak-exhausted"] = exhausted
	(*table.productTokens["AI_product"])["ak-disabled"] = disabled
	(*table.productTokens["AI_product"])["ak-expired"] = expired
	table.lock.Unlock()

	if _, err := table.ValidateUserToken("AI_product", "ak-exhausted"); err == nil {
		t.Error("expected error for exhausted token")
	}
	if _, err := table.ValidateUserToken("AI_product", "ak-disabled"); err == nil {
		t.Error("expected error for disabled token")
	}
	if _, err := table.ValidateUserToken("AI_product", "ak-expired"); err == nil {
		t.Error("expected error for expired token")
	}
}

func TestSetAiAuthInfo(t *testing.T) {
	req := newTestRequest("", "AI_product")
	ai := req.InitAiBasicInfo()
	SetAiAuthInfo(req, bfe_basic.CodeNoApiKey, []string{"plan1"})
	if ai.AiAuthInfo.RejectReason != bfe_basic.CodeNoApiKey {
		t.Errorf("unexpected reject reason: %s", ai.AiAuthInfo.RejectReason)
	}
	if len(ai.AiAuthInfo.RejectQuotaPlans) != 1 || ai.AiAuthInfo.RejectQuotaPlans[0] != "plan1" {
		t.Errorf("unexpected reject quota plans: %+v", ai.AiAuthInfo.RejectQuotaPlans)
	}
}

func TestTokenFoundProductHandlerNoAiBasicInfo(t *testing.T) {
	m := prepareTestModule(t)
	req := newTestRequest("", "AI_product")

	ret, resp := m.tokenFoundProductHandler(req)
	if ret != bfe_module.BfeHandlerGoOn || resp != nil {
		t.Errorf("expected goon with nil response, got %d, %v", ret, resp)
	}
}

func TestTokenFoundProductHandlerNoApiKey(t *testing.T) {
	m := prepareTestModule(t)
	req := newTestRequest("", "AI_product")
	req.InitAiBasicInfo()

	ret, resp := m.tokenFoundProductHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Errorf("expected response action, got %d", ret)
	}
	if resp == nil || resp.StatusCode != 401 {
		t.Errorf("expected 401 response, got %v", resp)
	}
}

func TestTokenFoundProductHandlerSuccess(t *testing.T) {
	m := prepareTestModule(t)
	req := newTestRequest("ak-123", "AI_product")
	req.InitAiBasicInfo()

	ret, resp := m.tokenFoundProductHandler(req)
	if ret != bfe_module.BfeHandlerGoOn || resp != nil {
		t.Errorf("expected goon with nil response, got %d, %v", ret, resp)
	}
	if GetTokenAuthContext(req) == nil {
		t.Error("expected token auth context set")
	}
}

func TestTokenReadResponseHandler(t *testing.T) {
	m := NewModuleAITokenAuth()
	req := newTestRequest("ak-123", "AI_product")
	ai := req.InitAiBasicInfo()
	SetTokenAuthContext(req, &Token{Key: "ak-123"}, 2, nil)

	body := `{"usage":{"total_tokens":20,"prompt_tokens":5,"completion_tokens":15}}`
	res := &bfe_http.Response{
		StatusCode:    200,
		ContentLength: int64(len(body)),
		Body:          ioutil.NopCloser(strings.NewReader(body)),
	}

	ret := m.tokenReadResponseHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected goon, got %d", ret)
	}
	usage := ai.GetTokenUsage()
	if usage.UsedQuota != 20 {
		t.Errorf("expected UsedQuota 20, got %d", usage.UsedQuota)
	}
}

func TestTokenRequestFinishHandler(t *testing.T) {
	m := NewModuleAITokenAuth()
	req := newTestRequest("ak-123", "AI_product")
	req.InitAiBasicInfo()

	if ret := m.tokenRequestFinishHandler(req, nil); ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected goon for nil response, got %d", ret)
	}

	res := &bfe_http.Response{StatusCode: 500}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected goon for non-200 response, got %d", ret)
	}

	res = &bfe_http.Response{StatusCode: 200}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected goon, got %d", ret)
	}

	req2 := newTestRequest("ak-123", "AI_product")
	ai2 := req2.InitAiBasicInfo()
	ai2.SetAllowEstimateToken(true)
	SetTokenAuthContext(req2, &Token{Key: "ak-123", UnlimitedQuota: true}, 4, nil)
	if ret := m.tokenRequestFinishHandler(req2, res); ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected goon, got %d", ret)
	}
}

func TestMonitorAndReloadHandlers(t *testing.T) {
	m := NewModuleAITokenAuth()
	mon := m.monitorHandlers()
	if mon == nil {
		t.Fatal("monitorHandlers should not return nil")
	}
	if _, ok := mon[ModAITokenAuth]; !ok {
		t.Error("missing state handler")
	}
	if _, ok := mon[ModAITokenAuth+".diff"]; !ok {
		t.Error("missing diff handler")
	}

	reload := m.reloadHandlers()
	if reload == nil {
		t.Fatal("reloadHandlers should not return nil")
	}
	if _, ok := reload[ModAITokenAuth]; !ok {
		t.Error("missing reload handler")
	}
}

func TestTokenRuleCheck(t *testing.T) {
	cond := "default_t()"
	action := &ActionFile{Cmd: ActionCheckToken}
	valid := tokenRuleFile{Cond: &cond, Action: action}
	if err := tokenRuleCheck(valid); err != nil {
		t.Errorf("valid rule failed: %s", err)
	}

	invalidCond := tokenRuleFile{Cond: nil, Action: action}
	if err := tokenRuleCheck(invalidCond); err == nil {
		t.Error("expected error for missing cond")
	}

	invalidAction := tokenRuleFile{Cond: &cond, Action: nil}
	if err := tokenRuleCheck(invalidAction); err == nil {
		t.Error("expected error for missing action")
	}

	badAction := tokenRuleFile{Cond: &cond, Action: &ActionFile{Cmd: "BAD"}}
	if err := tokenRuleCheck(badAction); err == nil {
		t.Error("expected error for bad action cmd")
	}
}

func TestQuotaPlanCheck(t *testing.T) {
	valid := QuotaPlan{Id: "p1", Unlimited: true, ExpiredTime: -1, ResetMode: 0}
	if err := quotaPlanCheck(&valid); err != nil {
		t.Errorf("valid quota plan failed: %s", err)
	}

	cases := []struct {
		name string
		plan QuotaPlan
		sub  string
	}{
		{"missing id", QuotaPlan{Unlimited: true}, "no Id"},
		{"invalid expired time", QuotaPlan{Id: "p1", Unlimited: true, ExpiredTime: -2}, "invalid ExpiredTime"},
		{"invalid token quota", QuotaPlan{Id: "p1", Unlimited: false, Quota: 0, Unit: "total_token"}, "invalid Quota"},
		{"invalid rmb quota", QuotaPlan{Id: "p1", Unlimited: false, Quota: -1, Unit: "RMB"}, "invalid Quota for RMB"},
		{"invalid unit", QuotaPlan{Id: "p1", Unlimited: true, Unit: "invalid"}, "invalid Unit"},
		{"invalid reset mode", QuotaPlan{Id: "p1", Unlimited: true, ResetMode: 2}, "invalid ResetMode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := quotaPlanCheck(&tc.plan)
			if err == nil || !strings.Contains(err.Error(), tc.sub) {
				t.Errorf("expected error containing %q, got %v", tc.sub, err)
			}
		})
	}
}

func TestRuleConvert(t *testing.T) {
	cond := "default_t()"
	rule, err := ruleConvert(tokenRuleFile{Cond: &cond, Action: &ActionFile{Cmd: ActionCheckToken}})
	if err != nil {
		t.Fatalf("ruleConvert failed: %s", err)
	}
	if rule.Cond == nil {
		t.Error("expected condition")
	}

	badCond := "unsupported_function_xyz()"
	if _, err := ruleConvert(tokenRuleFile{Cond: &badCond, Action: &ActionFile{Cmd: ActionCheckToken}}); err == nil {
		t.Error("expected error for invalid condition")
	}
}

func TestSubnetValidation(t *testing.T) {
	s := "10.0.0.0/8, 192.168.1.0/24"
	tf := &TokenFile{
		Key:            "ak-subnet",
		Status:         TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
		Subnet:         &s,
	}
	if err := tokenCheck(tf); err != nil {
		t.Fatalf("valid subnet failed: %s", err)
	}
	if len(tf.subnet) != 2 {
		t.Errorf("expected 2 subnets, got %d", len(tf.subnet))
	}

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/8")
	if !tf.subnet[0].Contains(ipNet.IP) && !tf.subnet[1].Contains(ipNet.IP) {
		t.Error("expected subnet to contain 10.0.0.0")
	}
}


// mockRedisClient is a simple in-memory redis client for unit tests.
type mockRedisClient struct {
	data map[string]int64
}

func newMockRedisClient() *mockRedisClient {
	return &mockRedisClient{data: make(map[string]int64)}
}

func (m *mockRedisClient) Setex(key string, value []byte, expire int) error {
	return nil
}

func (m *mockRedisClient) Get(key string) (interface{}, error) {
	if v, ok := m.data[key]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("key not found")
}

func (m *mockRedisClient) Expire(key string, expire int) error {
	return nil
}

func (m *mockRedisClient) Incr(key string) (int64, error) {
	m.data[key]++
	return m.data[key], nil
}

func (m *mockRedisClient) IncrAndExpire(key string, expire int) (int64, error) {
	return m.Incr(key)
}

func (m *mockRedisClient) Decr(key string) (int64, error) {
	m.data[key]--
	return m.data[key], nil
}

func (m *mockRedisClient) PIncr(keys []string) ([]int64, error) {
	return nil, nil
}

func (m *mockRedisClient) GetInt64(key string) (int64, error) {
	if v, ok := m.data[key]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("key not found")
}

func (m *mockRedisClient) IncrBy(key string, delta int64) (int64, error) {
	m.data[key] += delta
	return m.data[key], nil
}

func (m *mockRedisClient) NewScript(src string) redis_client.RedisScript {
	return &mockRedisScript{client: m, src: src}
}

type mockRedisScript struct {
	client *mockRedisClient
	src    string
}

func (s *mockRedisScript) Run(key string, args ...interface{}) (interface{}, error) {
	isRMB := strings.Contains(s.src, "raw == false")
	current := s.client.data[key]
	amount, _ := args[0].(int64)
	if isRMB {
		if _, ok := s.client.data[key]; !ok {
			initial, _ := args[1].(int64)
			s.client.data[key] = initial
			current = initial
		}
	}
	deduct := current
	if amount < current {
		deduct = amount
	}
	if deduct > 0 {
		s.client.data[key] = current - deduct
	}
	remaining := s.client.data[key]
	if remaining < 0 {
		remaining = 0
		s.client.data[key] = 0
	}
	return remaining, nil
}

func TestQuotaPlanDeduct(t *testing.T) {
	t.Run("token deduct", func(t *testing.T) {
		client := newMockRedisClient()
		client.data["token-key"] = 100
		plan := &QuotaPlan{Id: "p1", RedisKey: "token-key", Unit: "total_token", Quota: 100}
		remaining, err := plan.Deduct(client, 30)
		if err != nil {
			t.Fatalf("deduct failed: %v", err)
		}
		if remaining != 70 {
			t.Errorf("remaining = %d, want 70", remaining)
		}
		if client.data["token-key"] != 70 {
			t.Errorf("stored value = %d, want 70", client.data["token-key"])
		}
	})

	t.Run("rmb deduct", func(t *testing.T) {
		client := newMockRedisClient()
		plan := &QuotaPlan{Id: "p1", RedisKey: "rmb-key", Unit: "RMB", Quota: 1000}
		remaining, err := plan.Deduct(client, 200)
		if err != nil {
			t.Fatalf("deduct failed: %v", err)
		}
		if remaining != 800 {
			t.Errorf("remaining = %d, want 800", remaining)
		}
		if client.data["rmb-key"] != 800 {
			t.Errorf("stored value = %d, want 800", client.data["rmb-key"])
		}
	})

	t.Run("rmb deduct insufficient", func(t *testing.T) {
		client := newMockRedisClient()
		client.data["rmb-key"] = 100
		plan := &QuotaPlan{Id: "p1", RedisKey: "rmb-key", Unit: "RMB", Quota: 100}
		remaining, err := plan.Deduct(client, 200)
		if err != nil {
			t.Fatalf("deduct failed: %v", err)
		}
		if remaining != 0 {
			t.Errorf("remaining = %d, want 0", remaining)
		}
		if client.data["rmb-key"] != 0 {
			t.Errorf("stored value = %d, want 0", client.data["rmb-key"])
		}
	})

	t.Run("unlimited plan", func(t *testing.T) {
		client := newMockRedisClient()
		plan := &QuotaPlan{Id: "p1", RedisKey: "key", Unlimited: true, Unit: "RMB", Quota: 10000}
		remaining, err := plan.Deduct(client, 200)
		if err != nil {
			t.Fatalf("deduct failed: %v", err)
		}
		if remaining != 10000 {
			t.Errorf("remaining = %d, want 10000", remaining)
		}
	})
}

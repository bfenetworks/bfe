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
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_route/bfe_cluster"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
	"github.com/bfenetworks/go-lib/quota"
	"github.com/gomodule/redigo/redis"
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
	SetApiKey(req, "", bfe_basic.AuthStyleOpenAI)
	if req.Header.Get("Authorization") != "" {
		t.Error("empty api key should not set header")
	}

	SetApiKey(req, "mykey", bfe_basic.AuthStyleOpenAI)
	if got := req.Header.Get("Authorization"); got != "Bearer mykey" {
		t.Errorf("unexpected Authorization header: %s", got)
	}

	req2, _ := bfe_http.NewRequest(http.MethodGet, "http://example.com/", nil)
	SetApiKey(req2, "mykey", bfe_basic.AuthStyleAnthropic)
	if got := req2.Header.Get("x-api-key"); got != "mykey" {
		t.Errorf("unexpected x-api-key header: %s", got)
	}
	if req2.Header.Get("Authorization") != "" {
		t.Error("anthropic style should not set Authorization header")
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

func TestUpdateCtxByUsage_Cache(t *testing.T) {
	req := newTestRequest("", "AI_product")
	ai := req.InitAiBasicInfo()
	ctx := &TokenAuthContext{aiBasicInfo: ai}

	UpdateCtxByUsage(ctx, []byte(`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4,"cache_read_tokens":5,"cache_write_tokens":2}}`))
	usage := ai.GetTokenUsage()
	if usage.UsedQuota != 12 || usage.PromptTokens != 8 || usage.CompletionTokens != 4 {
		t.Errorf("unexpected base usage: %+v", usage)
	}
	if usage.CacheReadTokens != 5 {
		t.Errorf("expected CacheReadTokens 5, got %d", usage.CacheReadTokens)
	}
	if usage.CacheWriteTokens != 2 {
		t.Errorf("expected CacheWriteTokens 2, got %d", usage.CacheWriteTokens)
	}

	ctx2 := &TokenAuthContext{aiBasicInfo: ai}
	UpdateCtxByUsage(ctx2, []byte(`{"usage":{"prompt_tokens":3,"completion_tokens":1,"cache_read_tokens":1,"cache_write_tokens":1}}`))
	usage = ai.GetTokenUsage()
	if usage.UsedQuota != 4 || usage.PromptTokens != 3 || usage.CompletionTokens != 1 {
		t.Errorf("unexpected usage without total_tokens: %+v", usage)
	}
	if usage.CacheReadTokens != 1 || usage.CacheWriteTokens != 1 {
		t.Errorf("unexpected cache usage: %+v", usage)
	}
}

func TestUpdateCtxByUsage_DeepSeekCache(t *testing.T) {
	req := newTestRequest("", "AI_product")
	ai := req.InitAiBasicInfo()
	ctx := &TokenAuthContext{aiBasicInfo: ai}

	// DeepSeek: prompt_cache_hit_tokens
	UpdateCtxByUsage(ctx, []byte(`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4,"prompt_cache_hit_tokens":5}}`))
	usage := ai.GetTokenUsage()
	if usage.CacheReadTokens != 5 {
		t.Errorf("expected CacheReadTokens 5 for prompt_cache_hit_tokens, got %d", usage.CacheReadTokens)
	}

	// DeepSeek: prompt_tokens_details.cached_tokens
	ctx2 := &TokenAuthContext{aiBasicInfo: ai}
	UpdateCtxByUsage(ctx2, []byte(`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":6}}}`))
	usage = ai.GetTokenUsage()
	if usage.CacheReadTokens != 6 {
		t.Errorf("expected CacheReadTokens 6 for prompt_tokens_details.cached_tokens, got %d", usage.CacheReadTokens)
	}

	// Existing cache_read_tokens takes precedence when non-zero
	ctx3 := &TokenAuthContext{aiBasicInfo: ai}
	UpdateCtxByUsage(ctx3, []byte(`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4,"cache_read_tokens":3,"prompt_cache_hit_tokens":5}}`))
	usage = ai.GetTokenUsage()
	if usage.CacheReadTokens != 3 {
		t.Errorf("expected CacheReadTokens 3 (existing field precedence), got %d", usage.CacheReadTokens)
	}
}

func TestUpdateCtxByUsage_AnthropicCache(t *testing.T) {
	req := newTestRequest("", "AI_product")
	ai := req.InitAiBasicInfo()
	// In production AuthStyle is identified (GetApiKey / DetectAuthStyle)
	// before the response body is parsed.
	ai.AuthStyle = bfe_basic.AuthStyleAnthropic
	ctx := &TokenAuthContext{aiBasicInfo: ai}

	// Anthropic: input_tokens only counts fresh (cache-missing) tokens.
	// PromptTokens must be normalized to the total input
	// (input_tokens + cache_read + cache_write) so that cost splitting works.
	UpdateCtxByUsage(ctx, []byte(`{"usage":{"input_tokens":320,"output_tokens":150,"cache_read_input_tokens":8000,"cache_creation_input_tokens":200}}`))
	usage := ai.GetTokenUsage()
	if usage.PromptTokens != 8520 {
		t.Errorf("expected PromptTokens 8520 (320+8000+200), got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 150 {
		t.Errorf("expected CompletionTokens 150, got %d", usage.CompletionTokens)
	}
	if usage.CacheReadTokens != 8000 {
		t.Errorf("expected CacheReadTokens 8000, got %d", usage.CacheReadTokens)
	}
	if usage.CacheWriteTokens != 200 {
		t.Errorf("expected CacheWriteTokens 200, got %d", usage.CacheWriteTokens)
	}
	if usage.UsedQuota != 8670 {
		t.Errorf("expected UsedQuota 8670 (8520+150), got %d", usage.UsedQuota)
	}

	// Full cache hit: input_tokens = 0. Usage must still be recognized (not guessed).
	ai2 := newTestRequest("", "AI_product").InitAiBasicInfo()
	ai2.AuthStyle = bfe_basic.AuthStyleAnthropic
	ctx2 := &TokenAuthContext{aiBasicInfo: ai2}
	UpdateCtxByUsage(ctx2, []byte(`{"usage":{"input_tokens":0,"output_tokens":42,"cache_read_input_tokens":5000}}`))
	usage2 := ai2.GetTokenUsage()
	if usage2.PromptTokens != 5000 {
		t.Errorf("expected PromptTokens 5000 (0+5000), got %d", usage2.PromptTokens)
	}
	if usage2.CacheReadTokens != 5000 {
		t.Errorf("expected CacheReadTokens 5000, got %d", usage2.CacheReadTokens)
	}
	if usage2.UsedQuota != 5042 {
		t.Errorf("expected UsedQuota 5042 (5000+42), got %d", usage2.UsedQuota)
	}
}

func TestTokenAuthContext(t *testing.T) {
	req := newTestRequest("", "AI_product")
	ai := req.InitAiBasicInfo()
	tok := &Token{Key: "ak-123", KeyId: "ak-123-id"}

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
			KeyId:          "ak-123-id",
			Enabled:        true,
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
			name: "missing key_id",
			mutate: func(tf *TokenFile) {
				tf.KeyId = ""
			},
			errSub: "no KeyId",
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
		KeyId:          "ak-123-id",
		Enabled:        true,
		ExpiredTime:    -1,
		UnlimitedQuota: false,
		QuotaPlans:     []string{"plan1"},
	}

	planMap := QuotaPlanMap{"plan1": {Id: "plan1", Unlimited: true}}
	token, err := tokenConvert(tf, &planMap)
	if err != nil {
		t.Fatalf("tokenConvert failed: %s", err)
	}
	if token.Key != "ak-123" || token.KeyId != "ak-123-id" || len(token.QuotaPlans) != 1 {
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

	disabled := &Token{Key: "ak-disabled", KeyId: "ak-disabled-id", Enabled: false, ExpiredTime: -1}
	expired := &Token{Key: "ak-expired", KeyId: "ak-expired-id", Enabled: true, ExpiredTime: time.Now().Unix() - 10}
	table.lock.Lock()
	(*table.productTokens["AI_product"])["ak-disabled"] = disabled
	(*table.productTokens["AI_product"])["ak-expired"] = expired
	table.lock.Unlock()

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
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id"}, 2, nil)

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
	SetTokenAuthContext(req2, &Token{Key: "ak-123", KeyId: "ak-123-id", UnlimitedQuota: true}, 4, nil)
	if ret := m.tokenRequestFinishHandler(req2, res); ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected goon, got %d", ret)
	}
}

func TestTokenRequestFinishHandler_SkipCountTokens(t *testing.T) {
	m := NewModuleAITokenAuth()
	client := newMockRedisClient()
	m.redisClient = client

	clusterName := "claude-backup"
	model := "claude-opus-4-6"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)
	req.HttpRequest.RequestURI = "/anthropic/v1/messages/count_tokens"

	cluster := buildTestClusterConf(model, 0.00000452, 0.00002262)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-CountTokens",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id", QuotaPlans: []*QuotaPlan{rmbPlan}}, 100, nil)

	res := &bfe_http.Response{StatusCode: 200}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	if _, ok := client.data[rmbPlan.RedisKey]; ok {
		t.Errorf("count_tokens should not trigger any deduction")
	}
}

func TestTokenRequestFinishHandler_NoDuplicateDeduction(t *testing.T) {
	m := NewModuleAITokenAuth()
	client := newMockRedisClient()
	m.redisClient = client

	clusterName := "deepseek-backup"
	model := "deepseek-v4-flash"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)

	cluster := buildTestClusterConf(model, 0.000003, 0.000009)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-NoDuplicate",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id", QuotaPlans: []*QuotaPlan{rmbPlan}}, 0, nil)

	ai := req.GetAiBasicInfo()
	usage := ai.GetTokenUsage()
	usage.PromptTokens = 100
	usage.CompletionTokens = 200
	usage.UsedQuota = 300
	ai.MarkFinalUsageSeen()

	res := &bfe_http.Response{StatusCode: 200, ContentLength: -1}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("first finish handler failed: %d", ret)
	}

	expectedCost := quota.RmbToFixedPoint(0.0021)
	if client.data[rmbPlan.RedisKey] != rmbPlan.Quota-expectedCost {
		t.Fatalf("expected remaining %d after first deduction, got %d",
			rmbPlan.Quota-expectedCost, client.data[rmbPlan.RedisKey])
	}

	// Simulate HandleRequestFinish being triggered a second time.
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("second finish handler failed: %d", ret)
	}

	if client.data[rmbPlan.RedisKey] != rmbPlan.Quota-expectedCost {
		t.Errorf("duplicate deduction detected: expected remaining %d, got %d",
			rmbPlan.Quota-expectedCost, client.data[rmbPlan.RedisKey])
	}
}

func TestTokenReadResponseHandlerDoesNotCalcCost(t *testing.T) {
	m := NewModuleAITokenAuth()
	req := newTestRequest("ak-123", "AI_product")
	ai := req.InitAiBasicInfo()
	ai.SetAllowEstimateToken(true)

	// Simulate a non-streaming response with usage body.
	body := `{"usage":{"total_tokens":20,"prompt_tokens":5,"completion_tokens":15}}`
	res := &bfe_http.Response{
		StatusCode:    200,
		ContentLength: int64(len(body)),
		Body:          ioutil.NopCloser(strings.NewReader(body)),
	}

	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id"}, 2, nil)

	ret := m.tokenReadResponseHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected goon, got %d", ret)
	}

	usage := ai.GetTokenUsage()
	if usage.UsedQuota != 20 {
		t.Errorf("expected UsedQuota 20, got %d", usage.UsedQuota)
	}
	// UsedCost should NOT be calculated at read-response stage anymore.
	if usage.UsedCost != 0 {
		t.Errorf("expected UsedCost 0 at read-response stage, got %d", usage.UsedCost)
	}
}

type mockServerDataConf struct {
	clusters map[string]*bfe_cluster.BfeCluster
}

func (m *mockServerDataConf) ClusterTableLookup(clusterName string) (*bfe_cluster.BfeCluster, error) {
	return m.clusters[clusterName], nil
}

func (m *mockServerDataConf) HostTableLookup(hostname string) (string, error) {
	return hostname, nil
}

func newTestRequestWithCluster(apiKey, product, clusterName, targetModel string) *bfe_basic.Request {
	req := newTestRequest(apiKey, product)
	req.Route.ClusterName = clusterName
	ai := req.InitAiBasicInfo()
	ai.ClientModel = targetModel
	ai.TargetModel = targetModel
	return req
}

func buildTestClusterConf(model string, inputCost, outputCost float64) *bfe_cluster.BfeCluster {
	return buildTestClusterConfWithCache(model, inputCost, outputCost, 0, 0)
}

func buildTestClusterConfWithCache(model string, inputCost, outputCost, cacheReadCost, cacheWriteCost float64) *bfe_cluster.BfeCluster {
	return buildTestClusterConfWithAudio(model, inputCost, outputCost, cacheReadCost, cacheWriteCost, 0, 0)
}

func buildTestClusterConfWithAudio(model string, inputCost, outputCost, cacheReadCost, cacheWriteCost, audioInputCost, audioOutputCost float64) *bfe_cluster.BfeCluster {
	modelTable := &cluster_conf.ModelTable{
		Currency: "RMB",
		Models: []cluster_conf.ModelPrice{
			{
				Model:     model,
				BaseModel: model,
				Mode:      "chat",
				Prices: cluster_conf.PriceMap{
					cluster_conf.PriceInputCostPerToken:           inputCost,
					cluster_conf.PriceOutputCostPerToken:          outputCost,
					cluster_conf.PriceCacheReadInputTokenCost:     cacheReadCost,
					cluster_conf.PriceCacheCreationInputTokenCost: cacheWriteCost,
					cluster_conf.PriceInputCostPerAudioToken:      audioInputCost,
					cluster_conf.PriceOutputCostPerAudioToken:     audioOutputCost,
				},
			},
		},
	}
	// ModelTableCheck builds price index and converts float prices to fixed-point integers.
	if err := cluster_conf.ModelTableCheck(modelTable); err != nil {
		panic(fmt.Sprintf("ModelTableCheck failed: %v", err))
	}

	c := bfe_cluster.NewBfeCluster("test-cluster")
	c.AIConf = &cluster_conf.AIConf{
		ModelTable: modelTable,
	}
	return c
}

func TestTokenRequestFinishHandler_RMB_Streaming(t *testing.T) {
	m := NewModuleAITokenAuth()
	client := newMockRedisClient()
	m.redisClient = client

	clusterName := "deepseek-backup"
	model := "deepseek-v4-flash"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)

	cluster := buildTestClusterConf(model, 0.000003, 0.000009)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-ZEAoKAKdGnPpck1uPoUsdNCb",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id", QuotaPlans: []*QuotaPlan{rmbPlan}}, 0, nil)

	// Simulate streaming: mod_body_process has filled token usage after context was set.
	ai := req.GetAiBasicInfo()
	usage := ai.GetTokenUsage()
	usage.PromptTokens = 100
	usage.CompletionTokens = 200
	usage.UsedQuota = 300
	ai.MarkFinalUsageSeen()

	// input_cost=0.000003 yuan/token, output_cost=0.000009 yuan/token
	// Expected cost = 100*0.000003 + 200*0.000009 = 0.0021 yuan
	// In fixed point: 0.0021 * 1e8 = 210000

	// Streaming response: ContentLength = -1.
	res := &bfe_http.Response{StatusCode: 200, ContentLength: -1}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	expectedCost := quota.RmbToFixedPoint(0.0021)
	if usage.UsedCost != expectedCost {
		t.Errorf("expected UsedCost %d, got %d", expectedCost, usage.UsedCost)
	}

	remaining := client.data[rmbPlan.RedisKey]
	if remaining != rmbPlan.Quota-expectedCost {
		t.Errorf("expected remaining %d, got %d", rmbPlan.Quota-expectedCost, remaining)
	}
}

func TestTokenRequestFinishHandler_RMB_NonStreaming(t *testing.T) {
	m := NewModuleAITokenAuth()
	client := newMockRedisClient()
	m.redisClient = client

	clusterName := "deepseek-backup"
	model := "deepseek-v4-flash"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)

	body := `{"usage":{"total_tokens":30,"prompt_tokens":10,"completion_tokens":20}}`
	res := &bfe_http.Response{
		StatusCode:    200,
		ContentLength: int64(len(body)),
		Body:          ioutil.NopCloser(strings.NewReader(body)),
	}

	cluster := buildTestClusterConf(model, 0.000003, 0.000009)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-NonStreaming",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id", QuotaPlans: []*QuotaPlan{rmbPlan}}, 0, nil)

	// First pass through read-response handler to parse usage.
	if ret := m.tokenReadResponseHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("read response handler failed: %d", ret)
	}

	// Then finish handler should calculate cost and deduct.
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	// Expected cost = 10*0.000003 + 20*0.000009 = 0.00021 yuan
	expectedCost := quota.RmbToFixedPoint(0.00021)
	usage := req.GetAiBasicInfo().GetTokenUsage()
	if usage.UsedCost != expectedCost {
		t.Errorf("expected UsedCost %d, got %d", expectedCost, usage.UsedCost)
	}

	remaining := client.data[rmbPlan.RedisKey]
	if remaining != rmbPlan.Quota-expectedCost {
		t.Errorf("expected remaining %d, got %d", rmbPlan.Quota-expectedCost, remaining)
	}
}

func TestTokenRequestFinishHandler_RMB_Cache_NonStreaming(t *testing.T) {
	m := NewModuleAITokenAuth()
	client := newMockRedisClient()
	m.redisClient = client

	clusterName := "claude-backup"
	model := "claude-opus-4-6"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)

	body := `{"usage":{"total_tokens":9500,"prompt_tokens":8000,"completion_tokens":1500,"cache_read_tokens":5000,"cache_write_tokens":1000}}`
	res := &bfe_http.Response{
		StatusCode:    200,
		ContentLength: int64(len(body)),
		Body:          ioutil.NopCloser(strings.NewReader(body)),
	}

	// Prices chosen so that RmbToFixedPoint yields exact integers:
	// input=452, output=2262, cache_read=45, cache_write=565.
	cluster := buildTestClusterConfWithCache(model, 0.00000452, 0.00002262, 0.00000045, 0.00000565)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-CacheNonStreaming",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id", QuotaPlans: []*QuotaPlan{rmbPlan}}, 0, nil)

	if ret := m.tokenReadResponseHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("read response handler failed: %d", ret)
	}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	// normal_input = 8000 - 5000 - 1000 = 2000
	// cost = 2000*452 + 5000*45 + 1000*565 + 1500*2262 = 5087000
	expectedCost := int64(5087000)
	usage := req.GetAiBasicInfo().GetTokenUsage()
	if usage.UsedCost != expectedCost {
		t.Errorf("expected UsedCost %d, got %d", expectedCost, usage.UsedCost)
	}

	remaining := client.data[rmbPlan.RedisKey]
	if remaining != rmbPlan.Quota-expectedCost {
		t.Errorf("expected remaining %d, got %d", rmbPlan.Quota-expectedCost, remaining)
	}
}

func TestTokenRequestFinishHandler_RMB_Cache_Streaming(t *testing.T) {
	m := NewModuleAITokenAuth()
	client := newMockRedisClient()
	m.redisClient = client

	clusterName := "claude-backup"
	model := "claude-opus-4-6"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)

	cluster := buildTestClusterConfWithCache(model, 0.00000452, 0.00002262, 0.00000045, 0.00000565)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-CacheStreaming",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id", QuotaPlans: []*QuotaPlan{rmbPlan}}, 0, nil)

	// Simulate streaming: mod_body_process has filled token usage (including cache fields).
	ai := req.GetAiBasicInfo()
	usage := ai.GetTokenUsage()
	usage.PromptTokens = 8000
	usage.CompletionTokens = 1500
	usage.CacheReadTokens = 5000
	usage.CacheWriteTokens = 1000
	usage.UsedQuota = 9500
	ai.MarkFinalUsageSeen()

	res := &bfe_http.Response{StatusCode: 200, ContentLength: -1}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	// normal_input = 8000 - 5000 - 1000 = 2000
	// cost = 2000*452 + 5000*45 + 1000*565 + 1500*2262 = 5087000
	expectedCost := int64(5087000)
	if usage.UsedCost != expectedCost {
		t.Errorf("expected UsedCost %d, got %d", expectedCost, usage.UsedCost)
	}

	remaining := client.data[rmbPlan.RedisKey]
	if remaining != rmbPlan.Quota-expectedCost {
		t.Errorf("expected remaining %d, got %d", rmbPlan.Quota-expectedCost, remaining)
	}
}

func TestCalcCostUnits_Cache(t *testing.T) {
	m := NewModuleAITokenAuth()
	clusterName := "claude-backup"
	model := "claude-opus-4-6"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)
	cluster := buildTestClusterConfWithCache(model, 0.00000452, 0.00002262, 0.00000045, 0.00000565)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	usage := &bfe_basic.TokenUsage{
		PromptTokens:     8000,
		CompletionTokens: 1500,
		CacheReadTokens:  5000,
		CacheWriteTokens: 1000,
	}

	// normal_input = 8000 - 5000 - 1000 = 2000
	// cost = 2000*452 + 5000*45 + 1000*565 + 1500*2262 = 5087000
	expectedCost := int64(5087000)
	got := m.calcCostUnits(req, req.SvrDataConf, usage)
	if got != expectedCost {
		t.Errorf("expected cost %d, got %d", expectedCost, got)
	}
}

func TestCalcCostUnits_CacheFallback(t *testing.T) {
	m := NewModuleAITokenAuth()
	clusterName := "deepseek-backup"
	model := "deepseek-v4-flash"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)
	cluster := buildTestClusterConf(model, 0.000003, 0.000009)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	usage := &bfe_basic.TokenUsage{
		PromptTokens:     100,
		CompletionTokens: 200,
		CacheReadTokens:  50,
		CacheWriteTokens: 20,
	}

	// No cache prices configured, should fallback to legacy formula.
	expectedCost := quota.RmbToFixedPoint(100*0.000003 + 200*0.000009)
	got := m.calcCostUnits(req, req.SvrDataConf, usage)
	if got != expectedCost {
		t.Errorf("expected fallback cost %d, got %d", expectedCost, got)
	}
}

func TestCalcCostUnits_CacheReadExceedsPrompt(t *testing.T) {
	m := NewModuleAITokenAuth()
	clusterName := "claude-backup"
	model := "claude-opus-4-6"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)
	cluster := buildTestClusterConfWithCache(model, 0.00000452, 0.00002262, 0.00000045, 0.00000565)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	usage := &bfe_basic.TokenUsage{
		PromptTokens:     8000,
		CompletionTokens: 1500,
		CacheReadTokens:  10000, // exceeds prompt, should NOT be truncated (Anthropic semantics)
		CacheWriteTokens: 1000,
	}

	// normal_input = 0, cache read is billed using the real cache read count
	// cost = 10000*45 + 1000*565 + 1500*2262 = 4408000
	expectedCost := int64(4408000)
	got := m.calcCostUnits(req, req.SvrDataConf, usage)
	if got != expectedCost {
		t.Errorf("expected cost %d, got %d", expectedCost, got)
	}
}

func TestCalcCostUnits_AnthropicHighCacheHit(t *testing.T) {
	m := NewModuleAITokenAuth()
	clusterName := "claude-backup"
	model := "claude-opus-4-6"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)
	cluster := buildTestClusterConfWithCache(model, 0.00000452, 0.00002262, 0.00000045, 0.00000565)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	// Anthropic raw usage: input_tokens only contains fresh (cache-missing)
	// tokens; cache_read_input_tokens / cache_creation_input_tokens are extra.
	// After parse-time normalization, PromptTokens = 320 + 8000 + 200 = 8520.
	usage := &bfe_basic.TokenUsage{
		PromptTokens:     8520, // total input: 320 fresh + 8000 cache read + 200 cache write
		CompletionTokens: 150,
		CacheReadTokens:  8000, // cache hit
		CacheWriteTokens: 200,
	}

	// normal_input = 8520 - 8000 - 200 = 320
	// cost = 320*452 + 8000*45 + 200*565 + 150*2262 = 956940
	expectedCost := int64(956940)
	got := m.calcCostUnits(req, req.SvrDataConf, usage)
	if got != expectedCost {
		t.Errorf("expected anthropic cache cost %d, got %d", expectedCost, got)
	}
}

func TestCalcCostUnits_Audio(t *testing.T) {
	m := NewModuleAITokenAuth()
	clusterName := "audio-backup"
	model := "gpt-audio-1.5"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)
	cluster := buildTestClusterConfWithAudio(model, 0.00000178, 0.00000715, 0, 0, 0.00002288, 0.00004576)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	usage := &bfe_basic.TokenUsage{
		PromptTokens:      4000,
		CompletionTokens:  500,
		AudioInputTokens:  1000,
		AudioOutputTokens: 200,
	}

	expectedCost := int64(3951700)
	got := m.calcCostUnits(req, req.SvrDataConf, usage)
	if got != expectedCost {
		t.Errorf("expected cost %d, got %d", expectedCost, got)
	}
}

func TestCalcCostUnits_AudioFallback(t *testing.T) {
	m := NewModuleAITokenAuth()
	clusterName := "deepseek-backup"
	model := "deepseek-v4-flash"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)
	cluster := buildTestClusterConf(model, 0.000003, 0.000009)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	usage := &bfe_basic.TokenUsage{
		PromptTokens:      100,
		CompletionTokens:  200,
		AudioInputTokens:  30,
		AudioOutputTokens: 20,
	}

	// No audio prices configured, should fallback to legacy formula.
	expectedCost := quota.RmbToFixedPoint(100*0.000003 + 200*0.000009)
	got := m.calcCostUnits(req, req.SvrDataConf, usage)
	if got != expectedCost {
		t.Errorf("expected fallback cost %d, got %d", expectedCost, got)
	}
}

func TestCalcCostUnits_AudioInputExceedsPrompt(t *testing.T) {
	m := NewModuleAITokenAuth()
	clusterName := "audio-backup"
	model := "gpt-audio-1.5"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)
	cluster := buildTestClusterConfWithAudio(model, 0.00000178, 0.00000715, 0, 0, 0.00002288, 0.00004576)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	usage := &bfe_basic.TokenUsage{
		PromptTokens:      4000,
		CompletionTokens:  500,
		AudioInputTokens:  5000, // exceeds prompt, should be truncated
		AudioOutputTokens: 200,
	}

	// normal_input = 0 after truncation, audio_input = 4000
	expectedCost := int64(4000*2288 + 300*715 + 200*4576)
	got := m.calcCostUnits(req, req.SvrDataConf, usage)
	if got != expectedCost {
		t.Errorf("expected truncated cost %d, got %d", expectedCost, got)
	}
}

func TestCalcCostUnits_AudioOutputExceedsCompletion(t *testing.T) {
	m := NewModuleAITokenAuth()
	clusterName := "audio-backup"
	model := "gpt-audio-1.5"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)
	cluster := buildTestClusterConfWithAudio(model, 0.00000178, 0.00000715, 0, 0, 0.00002288, 0.00004576)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	usage := &bfe_basic.TokenUsage{
		PromptTokens:      4000,
		CompletionTokens:  500,
		AudioInputTokens:  1000,
		AudioOutputTokens: 800, // exceeds completion, should be truncated
	}

	// normal_output = 0 after truncation, audio_output = 500
	expectedCost := int64(3000*178 + 1000*2288 + 500*4576)
	got := m.calcCostUnits(req, req.SvrDataConf, usage)
	if got != expectedCost {
		t.Errorf("expected truncated cost %d, got %d", expectedCost, got)
	}
}

func TestCalcCostUnits_CacheAndAudio(t *testing.T) {
	m := NewModuleAITokenAuth()
	clusterName := "multi-backup"
	model := "claude-audio-4-6"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)
	cluster := buildTestClusterConfWithAudio(model, 0.00000452, 0.00002262, 0.00000045, 0.00000565, 0.00002288, 0.00004576)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	usage := &bfe_basic.TokenUsage{
		PromptTokens:      8000,
		CompletionTokens:  500,
		CacheReadTokens:   3000,
		CacheWriteTokens:  1000,
		AudioInputTokens:  1000,
		AudioOutputTokens: 200,
	}

	// normal_input = 8000 - 3000 - 1000 (cache) - 1000 (audio) = 3000
	// normal_output = 500 - 200 = 300
	expectedCost := int64(3000*452 + 3000*45 + 1000*565 + 1000*2288 + 300*2262 + 200*4576)
	got := m.calcCostUnits(req, req.SvrDataConf, usage)
	if got != expectedCost {
		t.Errorf("expected cost %d, got %d", expectedCost, got)
	}
}

func TestUpdateCtxByUsage_Audio(t *testing.T) {
	req := newTestRequest("", "AI_product")
	ai := req.InitAiBasicInfo()
	ctx := &TokenAuthContext{aiBasicInfo: ai}

	data := []byte(`{"usage":{"total_tokens":4500,"prompt_tokens":4000,"completion_tokens":500,"audio_input_tokens":1000,"audio_output_tokens":200}}`)
	UpdateCtxByUsage(ctx, data)

	usage := ai.GetTokenUsage()
	if usage.UsedQuota != 4500 {
		t.Errorf("expected UsedQuota 4500, got %d", usage.UsedQuota)
	}
	if usage.PromptTokens != 4000 {
		t.Errorf("expected PromptTokens 4000, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 500 {
		t.Errorf("expected CompletionTokens 500, got %d", usage.CompletionTokens)
	}
	if usage.AudioInputTokens != 1000 {
		t.Errorf("expected AudioInputTokens 1000, got %d", usage.AudioInputTokens)
	}
	if usage.AudioOutputTokens != 200 {
		t.Errorf("expected AudioOutputTokens 200, got %d", usage.AudioOutputTokens)
	}
}

func TestTokenRequestFinishHandler_RMB_Audio_NonStreaming(t *testing.T) {
	m := NewModuleAITokenAuth()
	client := newMockRedisClient()
	m.redisClient = client

	clusterName := "audio-backup"
	model := "gpt-audio-1.5"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)

	cluster := buildTestClusterConfWithAudio(model, 0.00000178, 0.00000715, 0, 0, 0.00002288, 0.00004576)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-AudioNonStreaming",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id", QuotaPlans: []*QuotaPlan{rmbPlan}}, 0, nil)

	res := &bfe_http.Response{StatusCode: 200, ContentLength: int64(len(`{"usage":{"prompt_tokens":4000,"completion_tokens":500,"audio_input_tokens":1000,"audio_output_tokens":200}}`))}
	res.Body = ioutil.NopCloser(bytes.NewBufferString(`{"usage":{"prompt_tokens":4000,"completion_tokens":500,"audio_input_tokens":1000,"audio_output_tokens":200}}`))
	if ret := m.tokenReadResponseHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	usage := req.GetAiBasicInfo().GetTokenUsage()
	expectedCost := int64(3951700)
	if usage.UsedCost != expectedCost {
		t.Errorf("expected UsedCost %d, got %d", expectedCost, usage.UsedCost)
	}

	remaining := client.data[rmbPlan.RedisKey]
	if remaining != rmbPlan.Quota-expectedCost {
		t.Errorf("expected remaining %d, got %d", rmbPlan.Quota-expectedCost, remaining)
	}
}

func TestTokenRequestFinishHandler_RMB_Audio_Streaming(t *testing.T) {
	m := NewModuleAITokenAuth()
	client := newMockRedisClient()
	m.redisClient = client

	clusterName := "audio-backup"
	model := "gpt-audio-1.5"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)

	cluster := buildTestClusterConfWithAudio(model, 0.00000178, 0.00000715, 0, 0, 0.00002288, 0.00004576)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-AudioStreaming",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id", QuotaPlans: []*QuotaPlan{rmbPlan}}, 0, nil)

	// Simulate streaming: mod_body_process has filled token usage (including audio fields).
	ai := req.GetAiBasicInfo()
	usage := ai.GetTokenUsage()
	usage.PromptTokens = 4000
	usage.CompletionTokens = 500
	usage.AudioInputTokens = 1000
	usage.AudioOutputTokens = 200
	usage.UsedQuota = 4500
	ai.MarkFinalUsageSeen()

	res := &bfe_http.Response{StatusCode: 200, ContentLength: -1}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	expectedCost := int64(3951700)
	if usage.UsedCost != expectedCost {
		t.Errorf("expected UsedCost %d, got %d", expectedCost, usage.UsedCost)
	}

	remaining := client.data[rmbPlan.RedisKey]
	if remaining != rmbPlan.Quota-expectedCost {
		t.Errorf("expected remaining %d, got %d", rmbPlan.Quota-expectedCost, remaining)
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
	valid := QuotaPlan{Id: "p1", Unlimited: true, ExpiredTime: -1}
	if err := quotaPlanCheck(&valid); err != nil {
		t.Errorf("valid quota plan failed: %s", err)
	}

	zeroQuota := QuotaPlan{Id: "p1", Unlimited: false, Quota: 0, Unit: "total_token", ExpiredTime: -1}
	if err := quotaPlanCheck(&zeroQuota); err != nil {
		t.Errorf("zero quota plan should be allowed: %s", err)
	}

	cases := []struct {
		name string
		plan QuotaPlan
		sub  string
	}{
		{"missing id", QuotaPlan{Unlimited: true}, "no Id"},
		{"invalid expired time", QuotaPlan{Id: "p1", Unlimited: true, ExpiredTime: -2}, "invalid ExpiredTime"},
		{"invalid token quota", QuotaPlan{Id: "p1", Unlimited: false, Quota: -1, Unit: "total_token"}, "invalid Quota"},
		{"invalid rmb quota", QuotaPlan{Id: "p1", Unlimited: false, Quota: -1, Unit: "RMB"}, "invalid Quota for RMB"},
		{"invalid unit", QuotaPlan{Id: "p1", Unlimited: true, Unit: "invalid"}, "invalid Unit"},
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
		KeyId:          "ak-subnet-id",
		Enabled:        true,
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
	return 0, redis.ErrNil
}

func (m *mockRedisClient) GetInt64Batch(keys []string) ([]int64, error) {
	result := make([]int64, len(keys))
	for i, key := range keys {
		if v, ok := m.data[key]; ok {
			result[i] = v
		} else {
			result[i] = 0
		}
	}
	return result, nil
}

func (m *mockRedisClient) IncrBy(key string, delta int64) (int64, error) {
	m.data[key] += delta
	return m.data[key], nil
}

func (m *mockRedisClient) Delete(key string) error {
	delete(m.data, key)
	return nil
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

func TestQuotaPlanHasBalance(t *testing.T) {
	t.Run("missing key means no balance", func(t *testing.T) {
		client := newMockRedisClient()
		plan := &QuotaPlan{Id: "p1", RedisKey: "absent-key", Unit: "total_token", Quota: 0}
		has, remaining, err := plan.HasBalance(client)
		if err != nil {
			t.Fatalf("HasBalance failed: %v", err)
		}
		if has {
			t.Error("expected no balance for missing key")
		}
		if remaining != 0 {
			t.Errorf("remaining = %d, want 0", remaining)
		}
	})

	t.Run("zero balance", func(t *testing.T) {
		client := newMockRedisClient()
		client.data["zero-key"] = 0
		plan := &QuotaPlan{Id: "p1", RedisKey: "zero-key", Unit: "total_token", Quota: 0}
		has, _, err := plan.HasBalance(client)
		if err != nil {
			t.Fatalf("HasBalance failed: %v", err)
		}
		if has {
			t.Error("expected no balance for zero value")
		}
	})

	t.Run("positive balance", func(t *testing.T) {
		client := newMockRedisClient()
		client.data["pos-key"] = 50
		plan := &QuotaPlan{Id: "p1", RedisKey: "pos-key", Unit: "total_token", Quota: 100}
		has, remaining, err := plan.HasBalance(client)
		if err != nil {
			t.Fatalf("HasBalance failed: %v", err)
		}
		if !has {
			t.Error("expected balance for positive value")
		}
		if remaining != 50 {
			t.Errorf("remaining = %d, want 50", remaining)
		}
	})
}

func buildTestClusterConfWithTiers(model string, inputCost, outputCost, cacheReadCost float64,
	peakInputCost, peakOutputCost, peakCacheReadCost float64) *bfe_cluster.BfeCluster {
	modelTable := &cluster_conf.ModelTable{
		Currency: "RMB",
		TimeZone: "Asia/Shanghai",
		Tiers: []cluster_conf.PriceTier{
			{
				Name: "peak",
				TimeRanges: []cluster_conf.TimeRange{
					{Weekdays: []int{1, 2, 3, 4, 5}, Start: "09:00", End: "12:00"},
					{Weekdays: []int{1, 2, 3, 4, 5}, Start: "14:00", End: "18:00"},
				},
			},
		},
		Models: []cluster_conf.ModelPrice{
			{
				Model:     model,
				BaseModel: model,
				Mode:      "chat",
				Prices: cluster_conf.PriceMap{
					cluster_conf.PriceInputCostPerToken:       inputCost,
					cluster_conf.PriceOutputCostPerToken:      outputCost,
					cluster_conf.PriceCacheReadInputTokenCost: cacheReadCost,
				},
				TierPrices: cluster_conf.TierPriceMap{
					"peak": {
						cluster_conf.PriceInputCostPerToken:       peakInputCost,
						cluster_conf.PriceOutputCostPerToken:      peakOutputCost,
						cluster_conf.PriceCacheReadInputTokenCost: peakCacheReadCost,
					},
				},
			},
		},
	}
	if err := cluster_conf.ModelTableCheck(modelTable); err != nil {
		panic(fmt.Sprintf("ModelTableCheck failed: %v", err))
	}

	c := bfe_cluster.NewBfeCluster("test-cluster")
	c.AIConf = &cluster_conf.AIConf{
		ModelTable: modelTable,
	}
	return c
}

func TestCalcChatCost_Tier(t *testing.T) {
	cluster := buildTestClusterConfWithTiers("deepseek-v4-pro",
		0.0000045, 0.0000135, 0.00000015,
		0.000009, 0.000027, 0.0000003)
	entry := cluster.AIConf.ModelTable.Models[0]

	usage := &bfe_basic.TokenUsage{
		PromptTokens:     1000,
		CompletionTokens: 500,
	}

	// off-peak: 1000 * 4.5e-6 + 500 * 1.35e-5
	offPeakExpected := 1000*quota.RmbToFixedPoint(0.0000045) + 500*quota.RmbToFixedPoint(0.0000135)
	if got := calcChatCost(&entry, usage, ""); got != offPeakExpected {
		t.Errorf("off-peak cost = %d, want %d", got, offPeakExpected)
	}

	// peak: 1000 * 9e-6 + 500 * 2.7e-5
	peakExpected := 1000*quota.RmbToFixedPoint(0.000009) + 500*quota.RmbToFixedPoint(0.000027)
	if got := calcChatCost(&entry, usage, "peak"); got != peakExpected {
		t.Errorf("peak cost = %d, want %d", got, peakExpected)
	}
}

func TestCalcChatCost_TierWithCache(t *testing.T) {
	cluster := buildTestClusterConfWithTiers("deepseek-v4-pro",
		0.0000045, 0.0000135, 0.00000015,
		0.000009, 0.000027, 0.0000003)
	entry := cluster.AIConf.ModelTable.Models[0]

	usage := &bfe_basic.TokenUsage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		CacheReadTokens:  400,
	}

	// peak with cache: 600 * 9e-6 + 400 * 3e-7 + 500 * 2.7e-5
	peakExpected := 600*quota.RmbToFixedPoint(0.000009) + 400*quota.RmbToFixedPoint(0.0000003) + 500*quota.RmbToFixedPoint(0.000027)
	if got := calcChatCost(&entry, usage, "peak"); got != peakExpected {
		t.Errorf("peak+cache cost = %d, want %d", got, peakExpected)
	}

	// off-peak with cache: 600 * 4.5e-6 + 400 * 1.5e-7 + 500 * 1.35e-5
	offPeakExpected := 600*quota.RmbToFixedPoint(0.0000045) + 400*quota.RmbToFixedPoint(0.00000015) + 500*quota.RmbToFixedPoint(0.0000135)
	if got := calcChatCost(&entry, usage, ""); got != offPeakExpected {
		t.Errorf("off-peak+cache cost = %d, want %d", got, offPeakExpected)
	}
}

func TestCalcCostUnits_Tier(t *testing.T) {
	m := NewModuleAITokenAuth()
	clusterName := "deepseek-tiered"
	model := "deepseek-v4-pro"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)
	cluster := buildTestClusterConfWithTiers(model,
		0.0000045, 0.0000135, 0,
		0.000009, 0.000027, 0)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	usage := &bfe_basic.TokenUsage{
		PromptTokens:     1000,
		CompletionTokens: 500,
	}

	got := m.calcCostUnits(req, req.SvrDataConf, usage)
	// Result depends on whether current Asia/Shanghai time hits peak tier.
	// Just verify it is one of the two expected values.
	offPeakExpected := 1000*quota.RmbToFixedPoint(0.0000045) + 500*quota.RmbToFixedPoint(0.0000135)
	peakExpected := 1000*quota.RmbToFixedPoint(0.000009) + 500*quota.RmbToFixedPoint(0.000027)
	if got != offPeakExpected && got != peakExpected {
		t.Errorf("cost = %d, want either off-peak %d or peak %d", got, offPeakExpected, peakExpected)
	}
}

// Issue #1352: a client abort (RST/close/write-fail) before the final usage
// must not be billed by full-request estimation.
func TestTokenRequestFinishHandler_ClientAbortNoFinalUsage(t *testing.T) {
	m := NewModuleAITokenAuth()
	client := newMockRedisClient()
	m.redisClient = client

	clusterName := "deepseek-backup"
	model := "deepseek-v4-flash"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)

	cluster := buildTestClusterConf(model, 0.000003, 0.000009)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-ClientAbortNoFinalUsage",
		Unit:     "RMB",
		Quota:    100000000,
	}
	// Seed the prompt token estimate as done at auth time when EstimateToken=true.
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id", QuotaPlans: []*QuotaPlan{rmbPlan}}, 100, nil)
	ai := req.GetAiBasicInfo()
	ai.SetAllowEstimateToken(true)

	// Client aborts right after message_start: ErrClientWrite, no final usage.
	req.ErrCode = bfe_basic.ErrClientWrite

	res := &bfe_http.Response{StatusCode: 200, ContentLength: -1}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	if _, ok := client.data[rmbPlan.RedisKey]; ok {
		t.Errorf("client abort without final usage must not be deducted")
	}
	if !GetTokenAuthContext(req).deducted {
		t.Errorf("expected request context to be marked deducted")
	}

	// EstimateToken=false behaves the same.
	req2 := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)
	req2.SvrDataConf = req.SvrDataConf
	rmbPlan2 := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-ClientAbortNoFinalUsage2",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req2, &Token{Key: "ak-123", KeyId: "ak-123-id", QuotaPlans: []*QuotaPlan{rmbPlan2}}, 100, nil)
	req2.ErrCode = bfe_basic.ErrClientWrite
	if ret := m.tokenRequestFinishHandler(req2, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}
	if _, ok := client.data[rmbPlan2.RedisKey]; ok {
		t.Errorf("client abort without final usage must not be deducted (EstimateToken=false)")
	}
}

// Issue #1352: if the final usage was already seen, a later client abort is
// still billed by the actual usage.
func TestTokenRequestFinishHandler_ClientAbortWithFinalUsage(t *testing.T) {
	m := NewModuleAITokenAuth()
	client := newMockRedisClient()
	m.redisClient = client

	clusterName := "deepseek-backup"
	model := "deepseek-v4-flash"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)

	cluster := buildTestClusterConf(model, 0.000003, 0.000009)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-ClientAbortWithFinalUsage",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id", QuotaPlans: []*QuotaPlan{rmbPlan}}, 0, nil)

	ai := req.GetAiBasicInfo()
	usage := ai.GetTokenUsage()
	usage.PromptTokens = 100
	usage.CompletionTokens = 200
	usage.UsedQuota = 300
	ai.MarkFinalUsageSeen()

	req.ErrCode = bfe_basic.ErrClientWrite

	res := &bfe_http.Response{StatusCode: 200, ContentLength: -1}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	// Expected cost = 100*0.000003 + 200*0.000009 = 0.0021 yuan
	expectedCost := quota.RmbToFixedPoint(0.0021)
	if remaining := client.data[rmbPlan.RedisKey]; remaining != rmbPlan.Quota-expectedCost {
		t.Errorf("expected remaining %d, got %d", rmbPlan.Quota-expectedCost, remaining)
	}
}

// Issue #1352: EstimateToken estimation only applies to completed responses;
// a response cut before completion must not be estimated by full request size.
func TestTokenRequestFinishHandler_EstimateRequiresCompletedResponse(t *testing.T) {
	m := NewModuleAITokenAuth()
	client := newMockRedisClient()
	m.redisClient = client

	clusterName := "deepseek-backup"
	model := "deepseek-v4-flash"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)

	cluster := buildTestClusterConf(model, 0.000003, 0.000009)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-EstimateRequiresCompletion",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id", QuotaPlans: []*QuotaPlan{rmbPlan}}, 100, nil)
	ai := req.GetAiBasicInfo()
	ai.SetAllowEstimateToken(true)
	ai.GetTokenUsage().CompletionTokens = 50

	res := &bfe_http.Response{StatusCode: 200, ContentLength: -1}

	// Response not completed yet: estimation must not kick in.
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}
	if _, ok := client.data[rmbPlan.RedisKey]; ok {
		t.Errorf("incomplete response must not be estimated and deducted")
	}

	// Response completes (e.g. message_stop seen): estimation applies.
	// The first call reset the estimated values, so seed them again.
	ctx := GetTokenAuthContext(req)
	ctx.deducted = false
	ai.MarkResponseCompleted()
	ai.GetTokenUsage().PromptTokens = 100
	ai.GetTokenUsage().CompletionTokens = 50
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	// Expected cost = 100*0.000003 + 50*0.000009 = 0.00075 yuan
	expectedCost := quota.RmbToFixedPoint(0.00075)
	if remaining := client.data[rmbPlan.RedisKey]; remaining != rmbPlan.Quota-expectedCost {
		t.Errorf("expected remaining %d, got %d", rmbPlan.Quota-expectedCost, remaining)
	}
}

// Issue #1364: when the final usage was not confirmed, the request-finish
// guard must clear ALL billing fields. Previously only Prompt/Completion/
// UsedQuota were cleared and a surviving CacheReadTokens was billed alone
// (cache-read only), losing the fresh input and output charges.
func TestTokenRequestFinishHandler_GuardClearsSubTokenFields(t *testing.T) {
	m := NewModuleAITokenAuth()
	client := newMockRedisClient()
	m.redisClient = client

	clusterName := "deepseek-backup"
	model := "deepseek-v4-flash"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)

	cluster := buildTestClusterConfWithCache(model, 0.000003, 0.000009, 0.000001, 0.0000015)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-GuardClearsSubTokenFields",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id", QuotaPlans: []*QuotaPlan{rmbPlan}}, 0, nil)

	// Simulate the issue #1364 failure shape: usage fields populated (e.g.
	// by mod_body_process) but neither final usage nor completion marked.
	ai := req.GetAiBasicInfo()
	usage := ai.GetTokenUsage()
	usage.PromptTokens = 0
	usage.CompletionTokens = 0
	usage.CacheReadTokens = 7395200
	usage.UsedQuota = 0

	res := &bfe_http.Response{StatusCode: 200, ContentLength: -1}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	if _, ok := client.data[rmbPlan.RedisKey]; ok {
		t.Errorf("unconfirmed final usage must not be deducted from surviving sub-token fields")
	}
	if usage.CacheReadTokens != 0 || usage.CacheWriteTokens != 0 ||
		usage.PromptTokens != 0 || usage.CompletionTokens != 0 || usage.UsedQuota != 0 {
		t.Errorf("guard must clear all billing fields, got %+v", usage)
	}
}

// Issue #1364: a completed non-streaming Anthropic response is billed in
// full: fresh input at input price, cache read/write at their prices,
// output at output price.
func TestTokenRequestFinishHandler_RMB_NonStreamingAnthropic(t *testing.T) {
	m := NewModuleAITokenAuth()
	client := newMockRedisClient()
	m.redisClient = client

	clusterName := "deepseek-backup"
	model := "deepseek-v4-flash"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)

	cluster := buildTestClusterConfWithCache(model, 0.000003, 0.000009, 0.000001, 0.0000015)
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}

	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-NonStreamingAnthropic",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id", QuotaPlans: []*QuotaPlan{rmbPlan}}, 0, nil)

	// Parsed from an Anthropic non-stream body by the composed chain:
	// PromptTokens is normalized to fresh + cacheRead + cacheWrite.
	ai := req.GetAiBasicInfo()
	usage := ai.GetTokenUsage()
	usage.PromptTokens = 8520 // 320 fresh + 8000 cacheRead + 200 cacheWrite
	usage.CompletionTokens = 150
	usage.CacheReadTokens = 8000
	usage.CacheWriteTokens = 200
	usage.UsedQuota = 8670
	ai.MarkFinalUsageSeen()
	ai.MarkResponseCompleted()

	res := &bfe_http.Response{StatusCode: 200, ContentLength: -1}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	// Expected cost = 320*0.000003 + 8000*0.000001 + 200*0.0000015 + 150*0.000009
	//               = 0.01061 yuan
	expectedCost := quota.RmbToFixedPoint(0.01061)
	if remaining := client.data[rmbPlan.RedisKey]; remaining != rmbPlan.Quota-expectedCost {
		t.Errorf("expected remaining %d, got %d", rmbPlan.Quota-expectedCost, remaining)
	}
}

// Issue #1364: when the auth style detected from the request does not
// match the response body format (e.g. Bearer key detected as openai
// while the backend returns an Anthropic body), the single adapter parses
// nothing and the composed cross-protocol chain must recover the usage.
func TestUpdateCtxByUsage_CrossProtocolFallback(t *testing.T) {
	req := newTestRequest("", "AI_product")
	ai := req.InitAiBasicInfo()
	ai.AuthStyle = bfe_basic.AuthStyleOpenAI
	ctx := &TokenAuthContext{aiBasicInfo: ai}

	UpdateCtxByUsage(ctx, []byte(`{"id":"msg_01","type":"message","role":"assistant","usage":{"input_tokens":320,"output_tokens":150,"cache_read_input_tokens":8000,"cache_creation_input_tokens":200}}`))
	usage := ai.GetTokenUsage()
	if usage.PromptTokens != 8520 {
		t.Errorf("expected PromptTokens 8520 (320+8000+200), got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 150 {
		t.Errorf("expected CompletionTokens 150, got %d", usage.CompletionTokens)
	}
	if usage.CacheReadTokens != 8000 || usage.CacheWriteTokens != 200 {
		t.Errorf("unexpected cache tokens: %+v", usage)
	}
	if usage.UsedQuota != 8670 {
		t.Errorf("expected UsedQuota 8670 (8520+150), got %d", usage.UsedQuota)
	}

	// Full cache hit with input_tokens = 0 must still be recognized.
	ai2 := newTestRequest("", "AI_product").InitAiBasicInfo()
	ai2.AuthStyle = bfe_basic.AuthStyleOpenAI
	ctx2 := &TokenAuthContext{aiBasicInfo: ai2}
	UpdateCtxByUsage(ctx2, []byte(`{"type":"message","usage":{"input_tokens":0,"output_tokens":42,"cache_read_input_tokens":5000}}`))
	usage2 := ai2.GetTokenUsage()
	if usage2.PromptTokens != 5000 || usage2.CacheReadTokens != 5000 || usage2.UsedQuota != 5042 {
		t.Errorf("unexpected full-cache-hit usage: %+v", usage2)
	}
}

func TestCalcChatCost_HighPrecision(t *testing.T) {
	// Prices with more than 8 decimal places (from the generated model
	// catalog) must not be truncated at config load time. The old
	// fixed-point conversion mapped 7.6234102728e-08 to 7 (1e-8 yuan),
	// undercharging by ~8%; the float64 + per-item rounding path keeps
	// the full precision until the final rounding step.
	entry := &cluster_conf.ModelPrice{
		Model: "qwen2.5-omni-7b",
		Mode:  "chat",
		Prices: cluster_conf.PriceMap{
			cluster_conf.PriceInputCostPerToken:  6.0168984e-09,
			cluster_conf.PriceOutputCostPerToken: 7.6234102728e-08,
		},
	}

	usage := &bfe_basic.TokenUsage{
		PromptTokens:     1000000,
		CompletionTokens: 1000000,
	}

	// input:  round(1e6 * 6.0168984e-09 * 1e8) = round(601689.84)    = 601690
	// output: round(1e6 * 7.6234102728e-08 * 1e8) = round(7623410.2728) = 7623410
	expectedCost := int64(601690 + 7623410)
	if got := calcChatCost(entry, usage, ""); got != expectedCost {
		t.Errorf("high precision cost = %d, want %d", got, expectedCost)
	}
}

func TestCalcChatCost_HighPrecisionTier(t *testing.T) {
	entry := &cluster_conf.ModelPrice{
		Model: "glm-4.6",
		Mode:  "chat",
		Prices: cluster_conf.PriceMap{
			cluster_conf.PriceInputCostPerToken:  3.0084492e-06,
			cluster_conf.PriceOutputCostPerToken: 1.4049457764e-05,
		},
		TierPrices: cluster_conf.TierPriceMap{
			"peak": {
				cluster_conf.PriceOutputCostPerToken: 2.8098915528e-05,
			},
		},
	}

	usage := &bfe_basic.TokenUsage{
		PromptTokens:     10000,
		CompletionTokens: 5000,
	}

	// peak: input falls back to default (round(10000*3.0084492e-06*1e8)=3008449),
	// output uses tier price (round(5000*2.8098915528e-05*1e8)=14049458)
	expectedCost := int64(3008449 + 14049458)
	if got := calcChatCost(entry, usage, "peak"); got != expectedCost {
		t.Errorf("peak high precision cost = %d, want %d", got, expectedCost)
	}

	// off-peak: round(5000*1.4049457764e-05*1e8)=7024729
	expectedCost = int64(3008449 + 7024729)
	if got := calcChatCost(entry, usage, ""); got != expectedCost {
		t.Errorf("off-peak high precision cost = %d, want %d", got, expectedCost)
	}
}

func TestCalcImageGenerationCost_HighPrecision(t *testing.T) {
	entry := &cluster_conf.ModelPrice{
		Model: "doubao-seedream-5-0",
		Mode:  "image_generation",
		Prices: cluster_conf.PriceMap{
			cluster_conf.PriceOutputCostPerImage:     0.220619608,
			cluster_conf.PriceInputCostPerImageToken: 1.7816e-06,
		},
	}

	usage := &bfe_basic.TokenUsage{
		ImageCount:       3,
		ImageInputTokens: 2000,
	}

	// round(3*0.220619608*1e8) = 66185882
	// round(2000*1.7816e-06*1e8) = 356320
	expectedCost := int64(66185882 + 356320)
	if got := calcImageGenerationCost(entry, usage, ""); got != expectedCost {
		t.Errorf("image generation cost = %d, want %d", got, expectedCost)
	}
}

func TestCalcVideoGenerationCost_HighPrecision(t *testing.T) {
	entry := &cluster_conf.ModelPrice{
		Model: "kling-video-pro",
		Mode:  "video_generation",
		Prices: cluster_conf.PriceMap{
			cluster_conf.PriceOutputCostPerVideo: 0.20056328,
		},
	}

	usage := &bfe_basic.TokenUsage{VideoCount: 2}

	// round(2*0.20056328*1e8) = 40112656
	expectedCost := int64(40112656)
	if got := calcVideoGenerationCost(entry, usage, ""); got != expectedCost {
		t.Errorf("video generation cost = %d, want %d", got, expectedCost)
	}
}

func buildTestEntryWithPrices(model string, prices cluster_conf.PriceMap, tierPrices cluster_conf.TierPriceMap) *cluster_conf.ModelPrice {
	modelTable := &cluster_conf.ModelTable{
		Currency: "RMB",
		Models: []cluster_conf.ModelPrice{
			{
				Model:      model,
				BaseModel:  model,
				Mode:       "chat",
				Prices:     prices,
				TierPrices: tierPrices,
			},
		},
	}
	if err := cluster_conf.ModelTableCheck(modelTable); err != nil {
		panic(fmt.Sprintf("ModelTableCheck failed: %v", err))
	}
	return &modelTable.Models[0]
}

// gpt-5.5 style pricing: 272k length tier on both sides.
func buildTestEntryGpt55() *cluster_conf.ModelPrice {
	return buildTestEntryWithPrices("gpt-5.5", cluster_conf.PriceMap{
		cluster_conf.PriceInputCostPerToken:                 2.431e-05,
		cluster_conf.PriceOutputCostPerToken:                0.00014586,
		cluster_conf.PriceCacheReadInputTokenCost:           2.431e-06,
		cluster_conf.PriceInputCostPerTokenAbove272kTokens:  4.862e-05,
		cluster_conf.PriceOutputCostPerTokenAbove272kTokens: 0.00021879,
	}, nil)
}

func TestCalcChatCost_LengthTier272k(t *testing.T) {
	entry := buildTestEntryGpt55()

	// 300k input tokens exceeds the 272k tier: whole request billed at tier prices.
	usage := &bfe_basic.TokenUsage{
		PromptTokens:     300000,
		CompletionTokens: 50000,
	}
	expected := quota.CalcCostUnits(300000, 4.862e-05) + quota.CalcCostUnits(50000, 0.00021879)
	if got := calcChatCost(entry, usage, ""); got != expected {
		t.Errorf("272k tier cost = %d, want %d", got, expected)
	}

	// below (and exactly at) the threshold: base prices, as before.
	usageBelow := &bfe_basic.TokenUsage{
		PromptTokens:     272000,
		CompletionTokens: 50000,
	}
	expectedBelow := quota.CalcCostUnits(272000, 2.431e-05) + quota.CalcCostUnits(50000, 0.00014586)
	if got := calcChatCost(entry, usageBelow, ""); got != expectedBelow {
		t.Errorf("below-tier cost = %d, want %d", got, expectedBelow)
	}
}

func TestCalcChatCost_LengthTierInputOnly(t *testing.T) {
	// only the input tier key is configured: input billed at the tier price,
	// output keeps the base price.
	entry := buildTestEntryWithPrices("qwen3.6-flash", cluster_conf.PriceMap{
		cluster_conf.PriceInputCostPerToken:                1e-05,
		cluster_conf.PriceOutputCostPerToken:               2e-05,
		cluster_conf.PriceInputCostPerTokenAbove256kTokens: 3e-05,
	}, nil)

	usage := &bfe_basic.TokenUsage{
		PromptTokens:     300000,
		CompletionTokens: 1000,
	}
	expected := quota.CalcCostUnits(300000, 3e-05) + quota.CalcCostUnits(1000, 2e-05)
	if got := calcChatCost(entry, usage, ""); got != expected {
		t.Errorf("input-only tier cost = %d, want %d", got, expected)
	}
}

func TestCalcChatCost_LengthTierPeak(t *testing.T) {
	// peak tier overrides the 272k tier prices; the output side falls back
	// to the default tier key because peak does not configure it.
	entry := buildTestEntryWithPrices("gpt-5.5", cluster_conf.PriceMap{
		cluster_conf.PriceInputCostPerToken:                 2.431e-05,
		cluster_conf.PriceOutputCostPerToken:                0.00014586,
		cluster_conf.PriceInputCostPerTokenAbove272kTokens:  4.862e-05,
		cluster_conf.PriceOutputCostPerTokenAbove272kTokens: 0.00021879,
	}, cluster_conf.TierPriceMap{
		"peak": {
			cluster_conf.PriceInputCostPerTokenAbove272kTokens: 9.724e-05,
		},
	})

	usage := &bfe_basic.TokenUsage{
		PromptTokens:     300000,
		CompletionTokens: 1000,
	}
	expected := quota.CalcCostUnits(300000, 9.724e-05) + quota.CalcCostUnits(1000, 0.00021879)
	if got := calcChatCost(entry, usage, "peak"); got != expected {
		t.Errorf("peak tier cost = %d, want %d", got, expected)
	}
}

func TestCalcChatCost_CacheWrite1hSplit(t *testing.T) {
	// claude-opus-4-8 style pricing: base cache write price + 1h price.
	entry := buildTestEntryWithPrices("claude-opus-4-8", cluster_conf.PriceMap{
		cluster_conf.PriceInputCostPerToken:             3.077e-05,
		cluster_conf.PriceOutputCostPerToken:            0.00015385,
		cluster_conf.PriceCacheCreationInputTokenCost:   3.84625e-05,
		cluster_conf.PriceCacheCreationInputTokenCost1h: 6.154e-05,
	}, nil)

	// 1h cache write 10000, 5m cache write 5000 (total 15000).
	usage := &bfe_basic.TokenUsage{
		PromptTokens:       20000,
		CompletionTokens:   1000,
		CacheWriteTokens:   15000,
		CacheWriteTokens1h: 10000,
	}
	expected := quota.CalcCostUnits(5000, 3.077e-05) + // normal input
		quota.CalcCostUnits(5000, 3.84625e-05) + // 5m cache write
		quota.CalcCostUnits(10000, 6.154e-05) + // 1h cache write
		quota.CalcCostUnits(1000, 0.00015385) // output
	if got := calcChatCost(entry, usage, ""); got != expected {
		t.Errorf("1h split cost = %d, want %d", got, expected)
	}

	// 1h portion larger than the total cache write: clamped to the total;
	// the whole cache write is billed at the 1h price.
	usageClamp := &bfe_basic.TokenUsage{
		PromptTokens:       20000,
		CompletionTokens:   1000,
		CacheWriteTokens:   15000,
		CacheWriteTokens1h: 20000,
	}
	expectedClamp := quota.CalcCostUnits(5000, 3.077e-05) +
		quota.CalcCostUnits(15000, 6.154e-05) +
		quota.CalcCostUnits(1000, 0.00015385)
	if got := calcChatCost(entry, usageClamp, ""); got != expectedClamp {
		t.Errorf("clamped 1h split cost = %d, want %d", got, expectedClamp)
	}

	// negative 1h portion: sanitized to zero, all cache write billed at the
	// base 5m price.
	usageNeg := &bfe_basic.TokenUsage{
		PromptTokens:       20000,
		CompletionTokens:   1000,
		CacheWriteTokens:   15000,
		CacheWriteTokens1h: -5,
	}
	expectedNeg := quota.CalcCostUnits(5000, 3.077e-05) +
		quota.CalcCostUnits(15000, 3.84625e-05) +
		quota.CalcCostUnits(1000, 0.00015385)
	if got := calcChatCost(entry, usageNeg, ""); got != expectedNeg {
		t.Errorf("negative-1h sanitized cost = %d, want %d", got, expectedNeg)
	}
}

func TestCalcChatCost_CacheWrite1hNotConfigured(t *testing.T) {
	// no 1h price configured: all cache writes billed at the base price,
	// identical to the pre-change behavior.
	entry := buildTestEntryWithPrices("claude-opus-4-8", cluster_conf.PriceMap{
		cluster_conf.PriceInputCostPerToken:           3.077e-05,
		cluster_conf.PriceOutputCostPerToken:          0.00015385,
		cluster_conf.PriceCacheCreationInputTokenCost: 3.84625e-05,
	}, nil)

	usage := &bfe_basic.TokenUsage{
		PromptTokens:       20000,
		CompletionTokens:   1000,
		CacheWriteTokens:   15000,
		CacheWriteTokens1h: 10000,
	}
	expected := quota.CalcCostUnits(5000, 3.077e-05) +
		quota.CalcCostUnits(15000, 3.84625e-05) +
		quota.CalcCostUnits(1000, 0.00015385)
	if got := calcChatCost(entry, usage, ""); got != expected {
		t.Errorf("no-1h-price cost = %d, want %d", got, expected)
	}
}

func TestUpdateCtxByUsage_CacheWrite1h(t *testing.T) {
	// Anthropic extended-TTL: 1h cache write tokens are reported separately
	// under usage.cache_creation.ephemeral_1h_input_tokens.
	req := newTestRequest("", "AI_product")
	ai := req.InitAiBasicInfo()
	ai.AuthStyle = bfe_basic.AuthStyleAnthropic
	ctx := &TokenAuthContext{aiBasicInfo: ai}

	UpdateCtxByUsage(ctx, []byte(`{"usage":{"input_tokens":320,"output_tokens":150,"cache_creation_input_tokens":1200,"cache_creation":{"ephemeral_1h_input_tokens":1000}}}`))
	usage := ai.GetTokenUsage()
	if usage.CacheWriteTokens != 1200 {
		t.Errorf("expected CacheWriteTokens 1200, got %d", usage.CacheWriteTokens)
	}
	if usage.CacheWriteTokens1h != 1000 {
		t.Errorf("expected CacheWriteTokens1h 1000, got %d", usage.CacheWriteTokens1h)
	}

	// Relay fallback field: usage.cache_creation_input_tokens_1h.
	ai2 := newTestRequest("", "AI_product").InitAiBasicInfo()
	ai2.AuthStyle = bfe_basic.AuthStyleAnthropic
	ctx2 := &TokenAuthContext{aiBasicInfo: ai2}
	UpdateCtxByUsage(ctx2, []byte(`{"usage":{"input_tokens":320,"output_tokens":150,"cache_creation_input_tokens":1200,"cache_creation_input_tokens_1h":800}}`))
	usage2 := ai2.GetTokenUsage()
	if usage2.CacheWriteTokens1h != 800 {
		t.Errorf("expected CacheWriteTokens1h 800 (fallback field), got %d", usage2.CacheWriteTokens1h)
	}
}

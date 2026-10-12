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
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

const whitelistTestGeminiBody = `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`

func newWhitelistTestRequest(t *testing.T, path, body string) *bfe_basic.Request {
	t.Helper()
	httpReq, err := bfe_http.NewRequest(http.MethodPost, "http://example.com"+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}
	httpReq.Header.Set("x-goog-api-key", "key1")
	req := bfe_basic.NewRequest(httpReq, nil, nil, nil, nil)
	req.Route.Product = "p1"
	req.InitAiBasicInfo()
	return req
}

func newWhitelistTestModule(token *Token) *ModuleAITokenAuth {
	m := NewModuleAITokenAuth()
	tm := tokenMap{token.Key: token}
	m.ruleTable.productTokens = ProductTokens{"p1": &tm}
	return m
}

func newWhitelistTestToken(models, blockModels []string) *Token {
	return &Token{
		Key:            "key1",
		KeyId:          "key-1",
		Enabled:        true,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
		Models:         models,
		BlockModels:    blockModels,
	}
}

// issue #1387: the model allow/block check moved from the auth stage to the
// forward stage (ValidateTargetModel, on the resolved target model).
// ValidateUserTokenByReq must no longer reject by model lists, so that a
// request can pass authentication and reach model resolution (route target
// override, cluster prefix stripping, cluster model mapping) before the
// allow/block decision.
func TestValidateUserTokenByReqModelCheckRemoved(t *testing.T) {
	geminiPath := "/v1beta/models/gemini-2.5-flash:generateContent"

	// allow list does not contain the request model: passes at auth stage
	m := newWhitelistTestModule(newWhitelistTestToken([]string{"gpt-4"}, nil))
	tok, err := m.ValidateUserTokenByReq(newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"evil-model"}`))
	if err != nil {
		t.Fatalf("model not in allow list: got err %v, want pass at auth stage", err)
	}
	if tok == nil || tok.KeyId != "key-1" {
		t.Fatalf("model not in allow list: got token %v, want key-1", tok)
	}

	// block list contains the request model: still passes at auth stage
	m = newWhitelistTestModule(newWhitelistTestToken(nil, []string{"evil-model"}))
	_, err = m.ValidateUserTokenByReq(newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"evil-model"}`))
	if err != nil {
		t.Fatalf("model in block list: got err %v, want pass at auth stage", err)
	}

	// request without any model: passes at auth stage (the forward stage
	// turns this into the historical 400 when lists are configured)
	m = newWhitelistTestModule(newWhitelistTestToken([]string{"gpt-4"}, nil))
	_, err = m.ValidateUserTokenByReq(newWhitelistTestRequest(t, "/v1/chat/completions", `{"messages":[]}`))
	if err != nil {
		t.Fatalf("missing model: got err %v, want pass at auth stage", err)
	}

	// gemini path model with allow list: passes at auth stage
	m = newWhitelistTestModule(newWhitelistTestToken([]string{"gemini-2.5-flash"}, nil))
	_, err = m.ValidateUserTokenByReq(newWhitelistTestRequest(t, geminiPath, whitelistTestGeminiBody))
	if err != nil {
		t.Fatalf("gemini path model: got err %v, want pass at auth stage", err)
	}
}

// issue #1418: every rejection path after the token is found must still
// leave the identity/audit block (ClientKeyId + ApikeyTags) in AiBasicInfo,
// so that rejected requests remain attributable to their Entity in the
// access log.
func TestValidateUserTokenByReqRejectionKeepsIdentity(t *testing.T) {
	testTags := []bfe_basic.ApikeyTag{{TagName: "entity.type", TagValue: "entity.svc", TagLevel: 3}}

	assertIdentity := func(t *testing.T, req *bfe_basic.Request, wantReason string) {
		t.Helper()
		ai := req.GetAiBasicInfo()
		if ai == nil {
			t.Fatal("AiBasicInfo is nil")
		}
		if ai.ClientKeyId != "key-1" {
			t.Errorf("ClientKeyId = %q, want key-1", ai.ClientKeyId)
		}
		if len(ai.ApikeyTags) != 1 || ai.ApikeyTags[0] != testTags[0] {
			t.Errorf("ApikeyTags = %v, want %v", ai.ApikeyTags, testTags)
		}
		if ai.AiAuthInfo.RejectReason != wantReason {
			t.Errorf("RejectReason = %q, want %q", ai.AiAuthInfo.RejectReason, wantReason)
		}
	}

	t.Run("key disabled", func(t *testing.T) {
		tok := newWhitelistTestToken(nil, nil)
		tok.Enabled = false
		tok.Tags = testTags
		m := newWhitelistTestModule(tok)

		req := newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"gpt-4"}`)
		if _, err := m.ValidateUserTokenByReq(req); err == nil {
			t.Fatal("expected rejection for disabled key")
		}
		assertIdentity(t, req, bfe_basic.CodeKeyDisabled)
	})

	t.Run("key expired", func(t *testing.T) {
		tok := newWhitelistTestToken(nil, nil)
		tok.ExpiredTime = 1 // long past
		tok.Tags = testTags
		m := newWhitelistTestModule(tok)

		req := newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"gpt-4"}`)
		if _, err := m.ValidateUserTokenByReq(req); err == nil {
			t.Fatal("expected rejection for expired key")
		}
		assertIdentity(t, req, bfe_basic.CodeKeyExpired)
	})

	t.Run("subnet not allowed", func(t *testing.T) {
		_, cidr, err := net.ParseCIDR("10.0.0.0/8")
		if err != nil {
			t.Fatalf("ParseCIDR failed: %v", err)
		}
		tok := newWhitelistTestToken(nil, nil)
		tok.Subnet = []*net.IPNet{cidr}
		tok.Tags = testTags
		m := newWhitelistTestModule(tok)

		// the test request carries no client/remote address, so it can never
		// match the allowed subnet.
		req := newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"gpt-4"}`)
		if _, err := m.ValidateUserTokenByReq(req); err == nil {
			t.Fatal("expected rejection for out-of-subnet client")
		}
		assertIdentity(t, req, bfe_basic.CodeSubnetNotAllowed)
	})

	t.Run("quota exhausted", func(t *testing.T) {
		client := newMockRedisClient()
		client.data["quota-key"] = 0
		tok := newWhitelistTestToken(nil, nil)
		tok.UnlimitedQuota = false
		tok.Tags = testTags
		tok.QuotaPlans = []*QuotaPlan{{
			Id:          "qplan-1",
			RedisKey:    "quota-key",
			ExpiredTime: -1,
			Unit:        "total_token",
			Quota:       100,
		}}
		m := newWhitelistTestModule(tok)
		m.redisClient = client

		req := newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"gpt-4"}`)
		_, err := m.ValidateUserTokenByReq(req)
		if err == nil {
			t.Fatal("expected rejection for exhausted quota")
		}
		if err.Code != bfe_basic.CodeQuotaExhausted {
			t.Fatalf("err.Code = %q, want %q", err.Code, bfe_basic.CodeQuotaExhausted)
		}
		assertIdentity(t, req, bfe_basic.CodeQuotaExhausted)
		if got := req.GetAiBasicInfo().AiAuthInfo.RejectQuotaPlans; len(got) != 1 || got[0] != "qplan-1" {
			t.Errorf("RejectQuotaPlans = %v, want [qplan-1]", got)
		}
	})

	t.Run("invalid api key writes no identity", func(t *testing.T) {
		m := newWhitelistTestModule(newWhitelistTestToken(nil, nil))

		req := newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"gpt-4"}`)
		req.HttpRequest.Header.Set("x-goog-api-key", "unknown-key")
		if _, err := m.ValidateUserTokenByReq(req); err == nil {
			t.Fatal("expected rejection for unknown key")
		}
		ai := req.GetAiBasicInfo()
		if ai.ClientKeyId != "" {
			t.Errorf("ClientKeyId = %q, want empty for unknown key", ai.ClientKeyId)
		}
		if len(ai.ApikeyTags) != 0 {
			t.Errorf("ApikeyTags = %v, want empty for unknown key", ai.ApikeyTags)
		}
	})
}

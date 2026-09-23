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

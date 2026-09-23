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
	tm := tokenMap{token.Key: token}
	m := &ModuleAITokenAuth{ruleTable: NewTokenRuleTable()}
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

// issue #1384: the Gemini native protocol carries the model in the request
// path, so the allow/block model check must extract it from the path when the
// body has no "model" field.
func TestValidateUserTokenByReqModelWhitelistGeminiPath(t *testing.T) {
	geminiPath := "/v1beta/models/gemini-2.5-flash:generateContent"

	// allow list contains the path model: request passes
	m := newWhitelistTestModule(newWhitelistTestToken([]string{"gemini-2.5-flash"}, nil))
	tok, err := m.ValidateUserTokenByReq(newWhitelistTestRequest(t, geminiPath, whitelistTestGeminiBody))
	if err != nil {
		t.Fatalf("gemini path model in allow list: got err %v, want pass", err)
	}
	if tok == nil || tok.KeyId != "key-1" {
		t.Fatalf("gemini path model in allow list: got token %v, want key-1", tok)
	}

	// allow list does not contain the path model: MODEL_NOT_ALLOWED
	m = newWhitelistTestModule(newWhitelistTestToken([]string{"gpt-4"}, nil))
	_, err = m.ValidateUserTokenByReq(newWhitelistTestRequest(t, geminiPath, whitelistTestGeminiBody))
	if err == nil || err.Code != bfe_basic.CodeModelNotAllowed {
		t.Fatalf("gemini path model not in allow list: got %v, want code %s", err, bfe_basic.CodeModelNotAllowed)
	}
	if err.Details == nil || err.Details.Model != "gemini-2.5-flash" {
		t.Fatalf("gemini path model not in allow list: details.Model = %v, want gemini-2.5-flash", err.Details)
	}

	// block list contains the path model: MODEL_NOT_ALLOWED
	m = newWhitelistTestModule(newWhitelistTestToken(nil, []string{"gemini-2.5-flash"}))
	_, err = m.ValidateUserTokenByReq(newWhitelistTestRequest(t, geminiPath, whitelistTestGeminiBody))
	if err == nil || err.Code != bfe_basic.CodeModelNotAllowed {
		t.Fatalf("gemini path model in block list: got %v, want code %s", err, bfe_basic.CodeModelNotAllowed)
	}

	// streamGenerateContent action also extracts the path model
	m = newWhitelistTestModule(newWhitelistTestToken([]string{"gemini-2.5-flash"}, nil))
	_, err = m.ValidateUserTokenByReq(newWhitelistTestRequest(t,
		"/v1beta/models/gemini-2.5-flash:streamGenerateContent", whitelistTestGeminiBody))
	if err != nil {
		t.Fatalf("gemini streamGenerateContent path: got err %v, want pass", err)
	}

	// body model takes precedence over the path model
	m = newWhitelistTestModule(newWhitelistTestToken([]string{"gpt-4"}, nil))
	_, err = m.ValidateUserTokenByReq(newWhitelistTestRequest(t, geminiPath, `{"model":"gpt-4"}`))
	if err != nil {
		t.Fatalf("body model precedence: got err %v, want pass", err)
	}
}

// non-Gemini requests keep the existing body-only behavior, including the
// error code and message of the missing-model 400.
func TestValidateUserTokenByReqModelWhitelistBodyUnchanged(t *testing.T) {
	// openai body model in allow list: passes as before
	m := newWhitelistTestModule(newWhitelistTestToken([]string{"gpt-4"}, nil))
	_, err := m.ValidateUserTokenByReq(newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"gpt-4"}`))
	if err != nil {
		t.Fatalf("openai body model in allow list: got err %v, want pass", err)
	}

	// non-gemini path without body model: still 400 INVALID_REQUEST
	m = newWhitelistTestModule(newWhitelistTestToken([]string{"gpt-4"}, nil))
	_, err = m.ValidateUserTokenByReq(newWhitelistTestRequest(t, "/v1/chat/completions", `{"messages":[]}`))
	if err == nil || err.Code != bfe_basic.CodeInvalidRequest {
		t.Fatalf("missing model on non-gemini path: got %v, want code %s", err, bfe_basic.CodeInvalidRequest)
	}
	if !strings.Contains(err.Message, "Model not found in request body") {
		t.Fatalf("missing model on non-gemini path: message = %q, want unchanged body error", err.Message)
	}

	// gemini path shape but empty model segment: still 400
	m = newWhitelistTestModule(newWhitelistTestToken([]string{"gpt-4"}, nil))
	_, err = m.ValidateUserTokenByReq(newWhitelistTestRequest(t, "/v1beta/models/", whitelistTestGeminiBody))
	if err == nil || err.Code != bfe_basic.CodeInvalidRequest {
		t.Fatalf("empty gemini model segment: got %v, want code %s", err, bfe_basic.CodeInvalidRequest)
	}
}

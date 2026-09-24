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
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_module"
)

// issue #1387: the key allow/block model lists are validated against the
// resolved target model (route override + strip prefix + model mapping
// applied), not the raw client model. ValidateTargetModel is invoked at the
// forward stage with the final target model.
func TestValidateTargetModel(t *testing.T) {
	// no TokenAuthContext (no token rule matched): pass, no counter
	m := NewModuleAITokenAuth()
	req := newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"glm-5.2-abc"}`)
	if aiErr := m.ValidateTargetModel(req, "glm-5.2-abc"); aiErr != nil {
		t.Fatalf("no token context: got %v, want pass", aiErr)
	}
	if v := m.state.ReqAuthFail.Get(); v != 0 {
		t.Fatalf("no token context: ReqAuthFail = %d, want 0", v)
	}

	// token without allow/block lists: pass
	m = NewModuleAITokenAuth()
	req = newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"anything"}`)
	SetTokenAuthContext(req, newWhitelistTestToken(nil, nil), 0, nil)
	if aiErr := m.ValidateTargetModel(req, "anything"); aiErr != nil {
		t.Fatalf("no lists configured: got %v, want pass", aiErr)
	}

	// allow list contains the target model (redirected name): pass
	m = NewModuleAITokenAuth()
	req = newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"glm-5.2-abc"}`)
	SetTokenAuthContext(req, newWhitelistTestToken([]string{"glm-5.2"}, nil), 0, nil)
	if aiErr := m.ValidateTargetModel(req, "glm-5.2"); aiErr != nil {
		t.Fatalf("target model in allow list: got %v, want pass", aiErr)
	}
	if v := m.state.ReqAuthFail.Get(); v != 0 {
		t.Fatalf("target model in allow list: ReqAuthFail = %d, want 0", v)
	}

	// allow list does not contain the target model: MODEL_NOT_ALLOWED
	m = NewModuleAITokenAuth()
	req = newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"glm-5.2-abc"}`)
	SetTokenAuthContext(req, newWhitelistTestToken([]string{"glm-4"}, nil), 0, nil)
	aiErr := m.ValidateTargetModel(req, "glm-5.2")
	if aiErr == nil || aiErr.Code != bfe_basic.CodeModelNotAllowed {
		t.Fatalf("target model not in allow list: got %v, want code %s", aiErr, bfe_basic.CodeModelNotAllowed)
	}
	if aiErr.Details == nil || aiErr.Details.Model != "glm-5.2" {
		t.Fatalf("target model not in allow list: details.Model = %v, want glm-5.2 (target, not client model)", aiErr.Details)
	}
	if aiErr.Details.KeyId != "key-1" || aiErr.Details.ApiKey != "key1" {
		t.Fatalf("target model not in allow list: details = %+v, want key1/key-1", aiErr.Details)
	}
	if got := req.GetAiBasicInfo().AiAuthInfo.RejectReason; got != bfe_basic.CodeModelNotAllowed {
		t.Fatalf("reject reason = %q, want %s", got, bfe_basic.CodeModelNotAllowed)
	}
	if v := m.state.ReqAuthFail.Get(); v != 1 {
		t.Fatalf("target model not in allow list: ReqAuthFail = %d, want 1", v)
	}

	// block list contains the target model: MODEL_NOT_ALLOWED (block is
	// symmetric: it also validates against the target model)
	m = NewModuleAITokenAuth()
	req = newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"glm-5.2-abc"}`)
	SetTokenAuthContext(req, newWhitelistTestToken(nil, []string{"glm-5.2"}), 0, nil)
	aiErr = m.ValidateTargetModel(req, "glm-5.2")
	if aiErr == nil || aiErr.Code != bfe_basic.CodeModelNotAllowed {
		t.Fatalf("target model in block list: got %v, want code %s", aiErr, bfe_basic.CodeModelNotAllowed)
	}

	// block list contains only the client model, target is redirected:
	// pass (block matches the target model, not the raw client model)
	m = NewModuleAITokenAuth()
	req = newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"glm-5.2-abc"}`)
	SetTokenAuthContext(req, newWhitelistTestToken(nil, []string{"glm-5.2-abc"}), 0, nil)
	if aiErr := m.ValidateTargetModel(req, "glm-5.2"); aiErr != nil {
		t.Fatalf("client model in block list but target redirected: got %v, want pass", aiErr)
	}

	// block takes precedence over allow
	m = NewModuleAITokenAuth()
	req = newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"m"}`)
	SetTokenAuthContext(req, newWhitelistTestToken([]string{"m"}, []string{"m"}), 0, nil)
	aiErr = m.ValidateTargetModel(req, "m")
	if aiErr == nil || aiErr.Code != bfe_basic.CodeModelNotAllowed {
		t.Fatalf("block precedence: got %v, want code %s", aiErr, bfe_basic.CodeModelNotAllowed)
	}

	// empty target model with lists configured: 400 INVALID_REQUEST, the
	// historical missing-model semantics
	m = NewModuleAITokenAuth()
	req = newWhitelistTestRequest(t, "/v1/chat/completions", `{"messages":[]}`)
	SetTokenAuthContext(req, newWhitelistTestToken([]string{"gpt-4"}, nil), 0, nil)
	aiErr = m.ValidateTargetModel(req, "")
	if aiErr == nil || aiErr.Code != bfe_basic.CodeInvalidRequest {
		t.Fatalf("empty target model: got %v, want code %s", aiErr, bfe_basic.CodeInvalidRequest)
	}

	// surrounding whitespace on the target model is trimmed before matching
	m = NewModuleAITokenAuth()
	req = newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"glm-5.2"}`)
	SetTokenAuthContext(req, newWhitelistTestToken([]string{"glm-5.2"}, nil), 0, nil)
	if aiErr := m.ValidateTargetModel(req, " glm-5.2 "); aiErr != nil {
		t.Fatalf("whitespace-trimmed target model: got %v, want pass", aiErr)
	}
}

// TestTargetModelCheckFilter: the HandleAfterAITargetModel callback wrapper
// reads the resolved target model from aiMeta (set by doSingleAIForward) and
// converts a rejection into BfeHandlerFinish with the error response.
func TestTargetModelCheckFilter(t *testing.T) {
	// allow list contains the resolved target model: pass
	m := NewModuleAITokenAuth()
	req := newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"glm-5.2-abc"}`)
	req.GetAiBasicInfo().TargetModel = "glm-5.2"
	SetTokenAuthContext(req, newWhitelistTestToken([]string{"glm-5.2"}, nil), 0, nil)
	ret, resp := m.targetModelCheckFilter(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("allow hit: ret = %d, want BfeHandlerGoOn", ret)
	}
	if resp != nil {
		t.Fatalf("allow hit: resp = %v, want nil", resp)
	}

	// allow list misses the resolved target model: finish with 400 response
	m = NewModuleAITokenAuth()
	req = newWhitelistTestRequest(t, "/v1/chat/completions", `{"model":"glm-5.2-abc"}`)
	req.GetAiBasicInfo().TargetModel = "glm-5.2"
	SetTokenAuthContext(req, newWhitelistTestToken([]string{"glm-4"}, nil), 0, nil)
	ret, resp = m.targetModelCheckFilter(req)
	if ret != bfe_module.BfeHandlerFinish {
		t.Fatalf("allow miss: ret = %d, want BfeHandlerFinish", ret)
	}
	if resp == nil || resp.StatusCode != 400 {
		t.Fatalf("allow miss: resp = %v, want 400 response", resp)
	}
}

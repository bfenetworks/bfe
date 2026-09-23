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
	"strings"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

// ValidateTargetModel validates the resolved target model against the token's
// effective allow/block model lists (issue #1387). It must be called only
// after the target model has been fully resolved (route target override,
// cluster prefix stripping and cluster model mapping all applied), i.e. on
// the final model name forwarded to the backend, not the raw client model.
//
// Requests without a TokenAuthContext (no token rule matched, hence no
// authenticated key) and tokens without allow/block lists pass unchanged,
// preserving the behavior where the model check is tied to token
// authentication.
//
// On rejection it records the reject reason via SetAiAuthInfo, counts
// ReqAuthFail and returns the AiError; the caller converts it into a
// response before invoking the backend.
func (m *ModuleAITokenAuth) ValidateTargetModel(req *bfe_basic.Request, targetModel string) *bfe_basic.AiError {
	ctx := GetTokenAuthContext(req)
	if ctx == nil || ctx.Token == nil {
		return nil
	}
	tok := ctx.Token
	if len(tok.Models) == 0 && len(tok.BlockModels) == 0 {
		return nil
	}

	model := strings.TrimSpace(targetModel)
	if model == "" {
		// Same semantics as the pre-change auth-stage check: a configured
		// allow/block list requires a determinable model. The message keeps
		// the historical "Model not found in request body" wording.
		SetAiAuthInfo(req, bfe_basic.CodeInvalidRequest, nil)
		m.state.ReqAuthFail.Inc(1)
		return bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeInvalidRequest, bfe_basic.TypeInvalidRequestError,
			fmt.Sprintf("Model not found in request body: %v", nil),
			&bfe_basic.AiErrorDetail{
				ApiKey: tok.Key,
				KeyId:  tok.KeyId,
			})
	}

	for _, blockModel := range tok.BlockModels {
		if blockModel == model {
			SetAiAuthInfo(req, bfe_basic.CodeModelNotAllowed, nil)
			m.state.ReqAuthFail.Inc(1)
			return bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeModelNotAllowed, bfe_basic.TypeInvalidRequestError,
				fmt.Sprintf("Model %s blocked by key %s", model, tok.Key),
				&bfe_basic.AiErrorDetail{
					ApiKey: tok.Key,
					KeyId:  tok.KeyId,
					Model:  model,
				})
		}
	}

	if len(tok.Models) > 0 {
		inModels := false
		for _, allowed := range tok.Models {
			if allowed == model {
				inModels = true
				break
			}
		}
		if !inModels {
			SetAiAuthInfo(req, bfe_basic.CodeModelNotAllowed, nil)
			m.state.ReqAuthFail.Inc(1)
			return bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeModelNotAllowed, bfe_basic.TypeInvalidRequestError,
				fmt.Sprintf("Model %s not allowed by key %s", model, tok.Key),
				&bfe_basic.AiErrorDetail{
					ApiKey: tok.Key,
					KeyId:  tok.KeyId,
					Model:  model,
				})
		}
	}

	return nil
}

// targetModelCheckFilter adapts ValidateTargetModel to the
// HandleAfterAITargetModel callback point. The callback fires per cluster
// attempt (key rotation and fallback recompute their own target model), so
// the allow/block check keeps validating every attempt's resolved target
// model, exactly as the original doSingleAIForward injection did.
func (m *ModuleAITokenAuth) targetModelCheckFilter(req *bfe_basic.Request) (int, *bfe_http.Response) {
	meta := req.GetAiBasicInfo()
	if meta == nil {
		return bfe_module.BfeHandlerGoOn, nil
	}
	if aiErr := m.ValidateTargetModel(req, meta.TargetModel); aiErr != nil {
		return bfe_module.BfeHandlerFinish, aiErr.CreateErrorResponse(req)
	}
	return bfe_module.BfeHandlerGoOn, nil
}

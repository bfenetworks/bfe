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

package mod_ai_rate_limit

import (
	"fmt"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/action"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_modules/mod_ai_batch"
)

// isBatchOpRequest reports whether the request is a classified files/batches
// operation (mod_ai_batch endpoint family).
func isBatchOpRequest(req *bfe_basic.Request, meta *bfe_basic.AiBasicInfo) bool {
	if meta == nil || (meta.Mode != bfe_basic.ModeFile && meta.Mode != bfe_basic.ModeBatch) {
		return false
	}
	op := bfe_basic.ClassifyBatchOp(req.HttpRequest.Method, req.HttpRequest.URL.Path)
	return op != ""
}

// checkBatchLimits evaluates the batch section of one policy against the
// current operation. Returns "" when allowed, or the hit limit type
// ("batch_create_rpm" / "batch_active" / "batch_file").
func (m *ModuleAiRateLimit) checkBatchLimits(req *bfe_basic.Request, meta *bfe_basic.AiBasicInfo,
	policyId string, policy *PolicyConf) string {
	b := policy.Rules.Batch
	op := bfe_basic.ClassifyBatchOp(req.HttpRequest.Method, req.HttpRequest.URL.Path)

	switch op {
	case bfe_basic.BatchOpCreate:
		ls := m.limiterManager.getLimiterPolicySet(policyId)
		if ls != nil && b.MaxCreateRPM > 0 && !ls.checkBatchCreate(req, m.redisAgent, m.isRejectOnRedisError) {
			return "batch_create_rpm"
		}
		if b.MaxActiveBatches > 0 {
			if int64(mod_ai_batch.BatchActiveCount(meta.ClientKeyId)) >= b.MaxActiveBatches {
				return "batch_active"
			}
		}
	case bfe_basic.BatchOpUpload:
		// known Content-Length only; chunked streams are enforced by
		// mod_ai_batch's counting reader via the effective limits
		if b.MaxFileBytes > 0 && req.HttpRequest.ContentLength > b.MaxFileBytes {
			return "batch_file"
		}
	}
	return ""
}

// executeBatchPolicyAction rejects like executePolicyAction but with the
// batch limit error semantics (batch_file / batch_rate limit types).
func (m *ModuleAiRateLimit) executeBatchPolicyAction(req *bfe_basic.Request, meta *bfe_basic.AiBasicInfo,
	policyId string, policy *PolicyConf, rule *productRule, hitLimit string) (int, *bfe_http.Response) {
	if rule.hitAction.Cmd == action.ActionClose {
		return bfe_module.BfeHandlerClose, nil
	}

	errorCode := bfe_basic.CodeRpmLimitExceeded
	limitType := bfe_basic.LimitTypeRpm
	if hitLimit == "batch_file" {
		errorCode = bfe_basic.CodeBatchFileTooLarge
		limitType = bfe_basic.LimitTypeBatchFile
	}

	aiErr := bfe_basic.NewAiErrorWithDetails(
		errorCode,
		bfe_basic.TypeRateLimitError,
		fmt.Sprintf("Batch limit exceeded for policy %s (%s)", policy.Name, hitLimit),
		&bfe_basic.AiErrorDetail{
			ApiKey:    meta.ClientApiKey,
			LimitType: limitType,
		},
	)
	return bfe_module.BfeHandlerFinish, aiErr.CreateErrorResponse(req)
}

// resolveBatchFileLimits computes the effective per-request file limits as
// min(bound policies' batch_limits, mod_ai_batch.data global hard ceilings)
// and publishes them into AiBasicInfo for mod_ai_batch (single computation
// point: its pre-forward checks and its counting reader read the same
// values). 0 means unlimited.
func resolveBatchFileLimits(meta *bfe_basic.AiBasicInfo, policyIds []string,
	getPolicy func(string) *PolicyConf) {
	if meta == nil || (meta.Mode != bfe_basic.ModeFile && meta.Mode != bfe_basic.ModeBatch) {
		return
	}

	var effBytes, effLines int64
	for _, policyId := range policyIds {
		policy := getPolicy(policyId)
		if policy == nil || !policy.Enabled || policy.Rules == nil || policy.Rules.Batch == nil {
			continue
		}
		b := policy.Rules.Batch
		if b.MaxFileBytes > 0 && (effBytes == 0 || b.MaxFileBytes < effBytes) {
			effBytes = b.MaxFileBytes
		}
		if b.MaxFileLines > 0 && (effLines == 0 || b.MaxFileLines < effLines) {
			effLines = b.MaxFileLines
		}
	}

	gBytes, gLines := mod_ai_batch.BatchGlobalFileCeilings()
	if gBytes > 0 && (effBytes == 0 || gBytes < effBytes) {
		effBytes = gBytes
	}
	if gLines > 0 && (effLines == 0 || gLines < effLines) {
		effLines = gLines
	}

	meta.BatchEffMaxFileBytes = effBytes
	meta.BatchEffMaxFileLines = effLines
}

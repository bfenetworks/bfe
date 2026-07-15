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

package mod_body_process

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

// calc quota in response stage
type QuotaUsageProcessor struct {
	aiBasicInfo *bfe_basic.AiBasicInfo
}

func NewQuotaUsageProcessor(req *bfe_basic.Request, res *bfe_http.Response) *QuotaUsageProcessor {
	if res.StatusCode != bfe_http.StatusOK {
		// only count used quota for successful requests
		return nil
	}

	aiBasicInfo := req.GetAiBasicInfo()

	return &QuotaUsageProcessor{aiBasicInfo: aiBasicInfo}
}

func (caf *QuotaUsageProcessor) Process(events []Event) ([]Event, error) {
	tctx := caf.aiBasicInfo.GetTokenUsage()
	for _, ev := range events {
		// data, err := GetAuditData(ev)
		// if err != nil {
		// 	log.Logger.Error("failed to get audit data: %v", err)
		// 	continue // 如果获取数据失败，跳过当前事件
		// }
		curCompletionToken := int64(0)
		if tctx.UsedQuota <= 0 {
			rquota := ev.GetQuotaUsage()
			curCompletionToken = rquota.CurrentTokens
			if !rquota.IsGuess {
				// not got usage yet, try to get from event data
				//mod_ai_token_auth.UpdateCtxByUsage(tctx, data)
				if rquota.UsedQuota > 0 {
					tctx.CompletionTokens = rquota.CompletionTokens
					tctx.PromptTokens = rquota.PromptTokens
					tctx.UsedQuota = rquota.UsedQuota
				} else if rquota.PromptTokens > 0 || rquota.CompletionTokens > 0 {
					tctx.UsedQuota = rquota.PromptTokens + rquota.CompletionTokens
					tctx.PromptTokens = rquota.PromptTokens
					tctx.CompletionTokens = rquota.CompletionTokens
				}
			}
		}

		if tctx.UsedQuota <= 0 {
			// still not got usage, estimate from content length
			if tctx.CompletionTokens == -1 {
				tctx.CompletionTokens = 0 // 初始化为0
			}
			// 累加事件的token数
			tctx.CompletionTokens += curCompletionToken
		}
	}
	return events, nil
}

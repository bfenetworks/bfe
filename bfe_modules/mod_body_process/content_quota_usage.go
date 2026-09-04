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
		rquota := ev.GetQuotaUsage()
		// Track response completion for billing decisions (issue #1352),
		// regardless of whether usage was already collected.
		if rquota.IsFinalUsage || (!rquota.IsGuess && (rquota.ImageCount > 0 || rquota.VideoCount > 0)) {
			caf.aiBasicInfo.MarkFinalUsageSeen()
		}
		if rquota.IsTermination {
			caf.aiBasicInfo.MarkResponseCompleted()
		}

		curCompletionToken := int64(0)
		// The final usage event (e.g. Anthropic message_delta) must always be
		// processed, even if an initial usage (message_start) was already
		// collected; otherwise the completion tokens would be lost.
		if tctx.UsedQuota <= 0 || rquota.IsFinalUsage {
			curCompletionToken = rquota.CurrentTokens
			if !rquota.IsGuess {
				// not got usage yet, try to get from event data
				//mod_ai_token_auth.UpdateCtxByUsage(tctx, data)
				if rquota.ImageCount > 0 {
					tctx.ImageCount = rquota.ImageCount
					tctx.UsedQuota = rquota.ImageCount
				} else if rquota.VideoCount > 0 {
					tctx.VideoCount = rquota.VideoCount
					tctx.UsedQuota = rquota.VideoCount
				} else if rquota.UsedQuota > 0 {
					if rquota.IsFinalUsage && rquota.PromptTokens == 0 && tctx.PromptTokens > 0 {
						// Anthropic message_delta carries only output tokens;
						// keep the prompt (and sub-token) fields parsed earlier
						// from message_start.
						rquota.PromptTokens = tctx.PromptTokens
						rquota.CacheReadTokens = tctx.CacheReadTokens
						rquota.CacheWriteTokens = tctx.CacheWriteTokens
						rquota.AudioInputTokens = tctx.AudioInputTokens
						rquota.AudioOutputTokens = tctx.AudioOutputTokens
						rquota.ImageInputTokens = tctx.ImageInputTokens
						rquota.VideoCount = tctx.VideoCount
						rquota.UsedQuota = rquota.PromptTokens + rquota.CompletionTokens
					}
					tctx.CompletionTokens = rquota.CompletionTokens
					tctx.PromptTokens = rquota.PromptTokens
					tctx.CacheReadTokens = rquota.CacheReadTokens
					tctx.CacheWriteTokens = rquota.CacheWriteTokens
					tctx.AudioInputTokens = rquota.AudioInputTokens
					tctx.AudioOutputTokens = rquota.AudioOutputTokens
					tctx.ImageInputTokens = rquota.ImageInputTokens
					tctx.VideoCount = rquota.VideoCount
					tctx.UsedQuota = rquota.UsedQuota
				} else if rquota.PromptTokens > 0 || rquota.CompletionTokens > 0 {
					tctx.UsedQuota = rquota.PromptTokens + rquota.CompletionTokens
					tctx.PromptTokens = rquota.PromptTokens
					tctx.CompletionTokens = rquota.CompletionTokens
					tctx.CacheReadTokens = rquota.CacheReadTokens
					tctx.CacheWriteTokens = rquota.CacheWriteTokens
					tctx.AudioInputTokens = rquota.AudioInputTokens
					tctx.AudioOutputTokens = rquota.AudioOutputTokens
					tctx.ImageInputTokens = rquota.ImageInputTokens
					tctx.VideoCount = rquota.VideoCount
				}
			}
		}

		if tctx.UsedQuota <= 0 && caf.aiBasicInfo.IsAllowEstimateToken() {
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

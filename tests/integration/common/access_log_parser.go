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

package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/bfenetworks/bfe-access-pb/b2log"
	bfe_access_pb "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
)

// ParseAccessLog reads the b2log file written by mod_access_pb3 and returns
// all decoded RequestLog records. It polls briefly if the file is not yet
// populated.
func ParseAccessLog(logDir string, timeout time.Duration) ([]*bfe_access_pb.RequestLog, error) {
	logPath := filepath.Join(logDir, "pb_access3.log")
	deadline := time.Now().Add(timeout)

	var data []byte
	var err error
	for time.Now().Before(deadline) {
		data, err = os.ReadFile(logPath)
		if err == nil && len(data) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return nil, fmt.Errorf("read access log %s failed: %w", logPath, err)
	}

	records, _ := b2log.BuffParse(data)
	var reqLogs []*bfe_access_pb.RequestLog
	for _, rec := range records {
		bfeLog := new(bfe_access_pb.BfeLog)
		if err := proto.Unmarshal(rec, bfeLog); err != nil {
			return nil, fmt.Errorf("unmarshal BfeLog failed: %w", err)
		}
		if bfeLog.RequestLog != nil {
			reqLogs = append(reqLogs, bfeLog.RequestLog)
		}
	}
	return reqLogs, nil
}

// ParseAccessLogAfterStop reads the b2log file after BFE has been stopped,
// ensuring all buffered logs are flushed.
func ParseAccessLogAfterStop(logDir string) ([]*bfe_access_pb.RequestLog, error) {
	return ParseAccessLog(logDir, 2*time.Second)
}

// FormatAccessLogError returns a string with all request log fields for
// debugging test failures.
func FormatAccessLogError(reqLog *bfe_access_pb.RequestLog) string {
	var b strings.Builder
	b.WriteString("RequestLog{ ")
	if reqLog.AiApikeyId != nil {
		b.WriteString(fmt.Sprintf("ai_apikey_id=%s ", *reqLog.AiApikeyId))
	}
	if len(reqLog.AiApikeytags) > 0 {
		b.WriteString(fmt.Sprintf("ai_apikeytags=%v ", reqLog.AiApikeytags))
	}
	if reqLog.AiRequestedModel != nil {
		b.WriteString(fmt.Sprintf("ai_requested_model=%s ", *reqLog.AiRequestedModel))
	}
	if reqLog.AiTargetModel != nil {
		b.WriteString(fmt.Sprintf("ai_target_model=%s ", *reqLog.AiTargetModel))
	}
	if reqLog.AiStream != nil {
		b.WriteString(fmt.Sprintf("ai_stream=%v ", *reqLog.AiStream))
	}
	if reqLog.AiInputTokens != nil {
		b.WriteString(fmt.Sprintf("ai_input_tokens=%d ", *reqLog.AiInputTokens))
	}
	if reqLog.AiOutputTokens != nil {
		b.WriteString(fmt.Sprintf("ai_output_tokens=%d ", *reqLog.AiOutputTokens))
	}
	if reqLog.AiTotalTokens != nil {
		b.WriteString(fmt.Sprintf("ai_total_tokens=%d ", *reqLog.AiTotalTokens))
	}
	if reqLog.AiTtftUs != nil {
		b.WriteString(fmt.Sprintf("ai_ttft_us=%d ", *reqLog.AiTtftUs))
	}
	if reqLog.AiTpotUs != nil {
		b.WriteString(fmt.Sprintf("ai_tpot_us=%d ", *reqLog.AiTpotUs))
	}
	if len(reqLog.AiRateLimitHits) > 0 {
		b.WriteString(fmt.Sprintf("ai_rate_limit_hits=%v ", reqLog.AiRateLimitHits))
	}
	if reqLog.AiAuthRejectReason != nil {
		b.WriteString(fmt.Sprintf("ai_auth_reject_reason=%s ", *reqLog.AiAuthRejectReason))
	}
	if len(reqLog.AiAuthRejectQuotaPlans) > 0 {
		b.WriteString(fmt.Sprintf("ai_auth_reject_quota_plans=%v ", reqLog.AiAuthRejectQuotaPlans))
	}
	if reqLog.AiProvider != nil {
		b.WriteString(fmt.Sprintf("ai_provider=%s ", *reqLog.AiProvider))
	}
	if reqLog.AiRetryCount != nil {
		b.WriteString(fmt.Sprintf("ai_retry_count=%d ", *reqLog.AiRetryCount))
	}
	if reqLog.AiCostValue != nil {
		b.WriteString(fmt.Sprintf("ai_cost_value=%d ", *reqLog.AiCostValue))
	}
	if reqLog.AiCostCurrency != nil {
		b.WriteString(fmt.Sprintf("ai_cost_currency=%s ", *reqLog.AiCostCurrency))
	}
	if len(reqLog.AiRouteRuleHits) > 0 {
		b.WriteString(fmt.Sprintf("ai_route_rule_hits=%v ", reqLog.AiRouteRuleHits))
	}
	if len(reqLog.AiClusterKeyNames) > 0 {
		b.WriteString(fmt.Sprintf("ai_cluster_key_names=%v ", reqLog.AiClusterKeyNames))
	}
	if len(reqLog.AiAuthHitQuotaPlans) > 0 {
		b.WriteString(fmt.Sprintf("ai_auth_hit_quota_plans=%v ", reqLog.AiAuthHitQuotaPlans))
	}
	b.WriteString("}")
	return b.String()
}

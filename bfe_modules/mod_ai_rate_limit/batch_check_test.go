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
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
)

func TestBatchLimitsConvert(t *testing.T) {
	file := &LimitRulesConfFile{
		Batch: &BatchLimitsConfFile{
			MaxCreateRPM:     10,
			MaxActiveBatches: 5,
			MaxFileBytes:     104857600,
			MaxFileLines:     50000,
			RedisKey:         "RL_BATCH_rlp-0001_rpm",
		},
	}
	conf := file.Convert()
	if conf.Batch == nil {
		t.Fatalf("batch section lost in convert")
	}
	if conf.Batch.MaxCreateRPM != 10 || conf.Batch.MaxActiveBatches != 5 ||
		conf.Batch.MaxFileBytes != 104857600 || conf.Batch.MaxFileLines != 50000 ||
		conf.Batch.RedisKey != "RL_BATCH_rlp-0001_rpm" {
		t.Fatalf("batch section convert wrong: %+v", conf.Batch)
	}

	// nil batch section stays nil (policy does not restrict batch traffic)
	empty := (&LimitRulesConfFile{}).Convert()
	if empty.Batch != nil {
		t.Fatalf("nil batch section should stay nil")
	}
}

func TestResolveBatchFileLimits(t *testing.T) {
	policies := map[string]*PolicyConf{
		"rlp-a": {
			Enabled: true,
			Rules: &LimitRulesConf{Batch: &BatchLimitsConf{
				MaxFileBytes: 100, MaxFileLines: 1000,
			}},
		},
		"rlp-b": {
			Enabled: true,
			Rules: &LimitRulesConf{Batch: &BatchLimitsConf{
				MaxFileBytes: 50, // stricter: wins the min
			}},
		},
		"rlp-c": {
			Enabled: false, // disabled policies do not participate
			Rules:   &LimitRulesConf{Batch: &BatchLimitsConf{MaxFileBytes: 1}},
		},
		"rlp-d": {
			Enabled: true, // no batch section
		},
	}
	getPolicy := func(id string) *PolicyConf { return policies[id] }

	aiMeta := &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeFile}
	resolveBatchFileLimits(aiMeta, []string{"rlp-a", "rlp-b", "rlp-c", "rlp-d"}, getPolicy)
	// rlp-b wins bytes (50); only rlp-a configures lines (1000)
	if aiMeta.BatchEffMaxFileBytes != 50 {
		t.Fatalf("eff bytes = %d, want 50", aiMeta.BatchEffMaxFileBytes)
	}
	if aiMeta.BatchEffMaxFileLines != 1000 {
		t.Fatalf("eff lines = %d, want 1000", aiMeta.BatchEffMaxFileLines)
	}

	// non-batch mode: untouched
	chat := &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeChat}
	resolveBatchFileLimits(chat, []string{"rlp-a"}, getPolicy)
	if chat.BatchEffMaxFileBytes != 0 || chat.BatchEffMaxFileLines != 0 {
		t.Fatalf("chat mode must not get batch limits")
	}
}

func TestIsBatchOpRequest(t *testing.T) {
	// minimal request stubs are not needed: mode gate decides first
	meta := &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeChat}
	if isBatchOpRequest(nil, meta) {
		t.Fatalf("chat mode is not a batch op")
	}
}

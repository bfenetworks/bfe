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
)

// clearDefaultModule removes the package-global module so the batch quota
// primitives behave like a deployment without Redis, and restores it after the
// test.
func clearDefaultModule(t *testing.T) {
	t.Helper()
	prev := defaultTokenAuthModule
	defaultTokenAuthModule = nil
	t.Cleanup(func() { defaultTokenAuthModule = prev })
}

// A release contract that found nothing to release (already released by
// another path, or no Redis) must not be reported as applied: mod_ai_batch
// would otherwise re-mark the batch as released from a stale outcome.
func TestBatchContractHandlerReleaseNotApplied(t *testing.T) {
	clearDefaultModule(t)

	aiMeta := &bfe_basic.AiBasicInfo{
		BatchSettle:   bfe_basic.BatchSettleRelease,
		BatchSettleId: "batch-1",
	}
	ctx := &TokenAuthContext{aiBasicInfo: aiMeta}
	m := &ModuleAITokenAuth{}

	if !m.batchContractHandler(&bfe_basic.Request{}, ctx) {
		t.Fatalf("the release contract must be claimed by the batch path")
	}
	if !ctx.deducted {
		t.Fatalf("claiming the contract must mark the request as deducted")
	}
	if aiMeta.BatchSettleApplied {
		t.Fatalf("an ineffective release must not be marked applied")
	}
}

// No contract (or BatchSettleNone) leaves the standard pricing path untouched.
func TestBatchContractHandlerNoContract(t *testing.T) {
	clearDefaultModule(t)

	for _, settle := range []string{"", bfe_basic.BatchSettleNone} {
		ctx := &TokenAuthContext{aiBasicInfo: &bfe_basic.AiBasicInfo{BatchSettle: settle}}
		m := &ModuleAITokenAuth{}
		if m.batchContractHandler(&bfe_basic.Request{}, ctx) {
			t.Fatalf("settle=%q must not be handled by the batch path", settle)
		}
		if ctx.deducted || ctx.aiBasicInfo.BatchSettleApplied {
			t.Fatalf("settle=%q must not touch the settlement flags", settle)
		}
	}
}

// The exported primitives are fail-open: without a module/Redis they report
// "nothing happened" instead of panicking.
func TestBatchQuotaPrimitivesWithoutRedis(t *testing.T) {
	clearDefaultModule(t)

	req := &bfe_basic.Request{}
	if !BatchPreCheck(req, 100, 100) {
		t.Fatalf("pre-check must fail open without redis")
	}
	if BatchReserve(req, "batch-1", 100, 100) {
		t.Fatalf("reserve without redis must report false")
	}
	if BatchRelease("batch-1") {
		t.Fatalf("release without redis must report false")
	}
}

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

package bfe_server

import (
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_http"
)

func TestSelectTargetSingle(t *testing.T) {
	targets := []bfe_basic.AiRouteTarget{
		{ClusterName: "cluster_a", Model: "model_a", Weight: 100},
	}
	selected := SelectTarget(targets)
	if selected.ClusterName != "cluster_a" {
		t.Errorf("expected cluster_a, got %s", selected.ClusterName)
	}
}

func TestSelectTargetDistribution(t *testing.T) {
	targets := []bfe_basic.AiRouteTarget{
		{ClusterName: "cluster_a", Model: "", Weight: 70},
		{ClusterName: "cluster_b", Model: "", Weight: 30},
	}

	counts := make(map[string]int)
	for i := 0; i < 1000; i++ {
		selected := SelectTarget(targets)
		counts[selected.ClusterName]++
	}

	if counts["cluster_a"] == 0 || counts["cluster_b"] == 0 {
		t.Errorf("expected both targets to be selected, got %v", counts)
	}

	if counts["cluster_a"] < counts["cluster_b"] {
		t.Errorf("expected cluster_a selected more often than cluster_b, got %v", counts)
	}
}

func TestSelectTargetZeroWeightNotSelected(t *testing.T) {
	targets := []bfe_basic.AiRouteTarget{
		{ClusterName: "cluster_a", Model: "", Weight: 100},
		{ClusterName: "cluster_b", Model: "", Weight: 0},
	}

	for i := 0; i < 100; i++ {
		selected := SelectTarget(targets)
		if selected.ClusterName == "cluster_b" {
			t.Error("cluster_b has zero weight and should not be selected")
		}
	}
}

func TestShouldTriggerFallback(t *testing.T) {
	if !shouldTriggerFallback(nil, bfe_http.ConnectError{}) {
		t.Error("expected fallback on connect error")
	}

	res := &bfe_http.Response{StatusCode: 500}
	if !shouldTriggerFallback(res, nil) {
		t.Error("expected fallback on 5xx")
	}

	res = &bfe_http.Response{StatusCode: 404}
	if shouldTriggerFallback(res, nil) {
		t.Error("expected no fallback on 4xx")
	}

	res = &bfe_http.Response{StatusCode: 200}
	if shouldTriggerFallback(res, nil) {
		t.Error("expected no fallback on 2xx")
	}
}

func TestGetResponseStatus(t *testing.T) {
	if getResponseStatus(nil) != 0 {
		t.Error("expected 0 for nil response")
	}

	res := &bfe_http.Response{StatusCode: 200}
	if getResponseStatus(res) != 200 {
		t.Errorf("expected 200, got %d", getResponseStatus(res))
	}
}

func TestSelectAIKeyDistribution(t *testing.T) {
	keys := []cluster_conf.AIKey{
		{Name: "key-a", Key: "ak-a", Weight: 70},
		{Name: "key-b", Key: "ak-b", Weight: 30},
	}

	counts := make(map[string]int)
	for i := 0; i < 1000; i++ {
		key, _ := selectAIKey(keys)
		counts[key.Name]++
	}

	if counts["key-a"] == 0 || counts["key-b"] == 0 {
		t.Errorf("expected both keys to be selected, got %v", counts)
	}

	if counts["key-a"] < counts["key-b"] {
		t.Errorf("expected key-a selected more often than key-b, got %v", counts)
	}
}

func TestSelectAIKeyZeroWeightNotSelected(t *testing.T) {
	keys := []cluster_conf.AIKey{
		{Name: "key-a", Key: "ak-a", Weight: 100},
		{Name: "key-b", Key: "ak-b", Weight: 0},
	}

	for i := 0; i < 100; i++ {
		key, _ := selectAIKey(keys)
		if key.Name == "key-b" {
			t.Error("key-b has zero weight and should not be selected")
		}
	}
}

func TestChooseNextAIKeyRotateOn429(t *testing.T) {
	keys := []cluster_conf.AIKey{
		{Name: "key-a", Key: "ak-a", Weight: 50},
		{Name: "key-b", Key: "ak-b", Weight: 50},
	}

	state := newAIKeyAttemptState()

	// first selection
	idx1, key1, ok := chooseNextAIKey(keys, state)
	if !ok {
		t.Fatal("expected to select a key")
	}

	// mark first key as 429 used
	state.usedSet[idx1] = struct{}{}

	// second selection should choose the other key
	idx2, key2, ok := chooseNextAIKey(keys, state)
	if !ok {
		t.Fatal("expected to select a key")
	}
	if idx2 == idx1 {
		t.Errorf("expected different key after 429, got same index %d", idx2)
	}
	if key2.Name == key1.Name {
		t.Errorf("expected different key name after 429, got %s", key2.Name)
	}

	// mark second key as 429 used, no eligible keys left
	state.usedSet[idx2] = struct{}{}
	_, _, ok = chooseNextAIKey(keys, state)
	if !ok {
		t.Error("expected reset used_set and reselect when all alive keys used")
	}
}

func TestChooseNextAIKeyDeadOn403(t *testing.T) {
	keys := []cluster_conf.AIKey{
		{Name: "key-a", Key: "ak-a", Weight: 100},
	}

	state := newAIKeyAttemptState()
	state.deadSet[0] = struct{}{}

	_, _, ok := chooseNextAIKey(keys, state)
	if ok {
		t.Error("expected no key available when the only key is dead")
	}
}

func TestCalcBackoff(t *testing.T) {
	// attempt 1: initial value (with ±20% jitter)
	b1 := calcBackoff(100, 1000, 1)
	if b1 < time.Duration(90)*time.Millisecond || b1 > time.Duration(110)*time.Millisecond {
		t.Errorf("expected backoff around 100ms, got %v", b1)
	}

	// attempt 2: doubled (with ±20% jitter)
	b2 := calcBackoff(100, 1000, 2)
	if b2 < time.Duration(180)*time.Millisecond || b2 > time.Duration(220)*time.Millisecond {
		t.Errorf("expected backoff around 200ms, got %v", b2)
	}

	// attempt 5: capped at max (with ±20% jitter)
	b5 := calcBackoff(100, 500, 5)
	if b5 < time.Duration(450)*time.Millisecond || b5 > time.Duration(550)*time.Millisecond {
		t.Errorf("expected backoff capped around 500ms, got %v", b5)
	}
}

func TestDefaultAIKeyPolicy(t *testing.T) {
	policy := defaultAIKeyPolicy()
	if policy.Strategy != "weighted_random" {
		t.Errorf("expected strategy weighted_random, got %s", policy.Strategy)
	}
	if policy.MaxRetries != 0 {
		t.Errorf("expected max_retries 0, got %d", policy.MaxRetries)
	}
	if policy.RetryBackoffInitial != 500 {
		t.Errorf("expected retry_backoff_initial 500, got %d", policy.RetryBackoffInitial)
	}
	if policy.RetryBackoffMax != 5000 {
		t.Errorf("expected retry_backoff_max 5000, got %d", policy.RetryBackoffMax)
	}
}

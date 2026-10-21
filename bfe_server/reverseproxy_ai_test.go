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
	"encoding/json"
	"errors"
	"io/ioutil"
	"net"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/go-lib/web-monitor/metrics"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_route"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
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
	if !shouldTriggerFallback(nil, bfe_http.ConnectError{}, "") {
		t.Error("expected fallback on connect error")
	}

	// Issue #1440 (step 3): (nil, nil) is not a decidable result and must
	// be treated as fallback-eligible (hard invariant).
	if !shouldTriggerFallback(nil, nil, "") {
		t.Error("expected fallback on (nil, nil)")
	}

	// 5xx always triggers fallback
	res := &bfe_http.Response{StatusCode: 500}
	if !shouldTriggerFallback(res, nil, "") {
		t.Error("expected fallback on 5xx")
	}

	res = &bfe_http.Response{StatusCode: 503}
	if !shouldTriggerFallback(res, nil, "") {
		t.Error("expected fallback on 503")
	}

	// 2xx/3xx do not trigger fallback
	res = &bfe_http.Response{StatusCode: 200}
	if shouldTriggerFallback(res, nil, "") {
		t.Error("expected no fallback on 2xx")
	}

	res = &bfe_http.Response{StatusCode: 302}
	if shouldTriggerFallback(res, nil, "") {
		t.Error("expected no fallback on 3xx")
	}

	// Specific 4xx (aligned with issue #1317 and Bifrost classification)
	fallback4xx := []int{400, 401, 402, 403, 422, 429}
	for _, code := range fallback4xx {
		res = &bfe_http.Response{StatusCode: code}
		if !shouldTriggerFallback(res, nil, "") {
			t.Errorf("expected fallback on %d", code)
		}
	}

	// Other 4xx should not trigger fallback
	nonFallback4xx := []int{404, 405, 406, 408, 409, 410, 413}
	for _, code := range nonFallback4xx {
		res = &bfe_http.Response{StatusCode: code}
		if shouldTriggerFallback(res, nil, "") {
			t.Errorf("expected no fallback on %d", code)
		}
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

func TestStripProviderPrefix(t *testing.T) {
	model := "openrouter/anthropic/claude-sonnet-4.6"
	stripped, ok := stripProviderPrefix(model, "openrouter/")
	if !ok {
		t.Error("expected stripping to succeed")
	}
	if stripped != "anthropic/claude-sonnet-4.6" {
		t.Errorf("expected stripped model anthropic/claude-sonnet-4.6, got %s", stripped)
	}
}

func TestStripProviderPrefixNoMatch(t *testing.T) {
	model := "anthropic/claude-sonnet-4.6"
	stripped, ok := stripProviderPrefix(model, "openrouter/")
	if ok {
		t.Error("expected stripping to be skipped when prefix does not match")
	}
	if stripped != model {
		t.Errorf("expected model unchanged, got %s", stripped)
	}
}

func TestStripProviderPrefixEmptyResult(t *testing.T) {
	model := "openrouter/"
	stripped, ok := stripProviderPrefix(model, "openrouter/")
	if ok {
		t.Error("expected stripping to be skipped when result is empty")
	}
	if stripped != model {
		t.Errorf("expected model unchanged, got %s", stripped)
	}
}

func TestComputeTargetModelNoOverride(t *testing.T) {
	model := computeTargetModel("gpt-4", "", nil)
	if model != "gpt-4" {
		t.Errorf("expected gpt-4, got %s", model)
	}
}

func TestComputeTargetModelAttemptOverride(t *testing.T) {
	model := computeTargetModel("gpt-4", "gpt-3.5", nil)
	if model != "gpt-3.5" {
		t.Errorf("expected gpt-3.5, got %s", model)
	}
}

func TestComputeTargetModelStripPrefix(t *testing.T) {
	aiConf := &cluster_conf.AIConf{
		MatchPrefix: "openrouter/",
		StripPrefix: true,
	}
	model := computeTargetModel("openrouter/modelA", "", aiConf)
	if model != "modelA" {
		t.Errorf("expected modelA, got %s", model)
	}
}

func TestComputeTargetModelStripPrefixAfterOverride(t *testing.T) {
	aiConf := &cluster_conf.AIConf{
		MatchPrefix: "openrouter/",
		StripPrefix: true,
	}
	model := computeTargetModel("clientprefix/mymodel", "openrouter/modelA", aiConf)
	if model != "modelA" {
		t.Errorf("expected modelA, got %s", model)
	}
}

func TestComputeTargetModelModelMapping(t *testing.T) {
	mappings := map[string]string{"modelA": "mapped-model"}
	aiConf := &cluster_conf.AIConf{
		ModelMapping: &mappings,
	}
	model := computeTargetModel("modelA", "", aiConf)
	if model != "mapped-model" {
		t.Errorf("expected mapped-model, got %s", model)
	}
}

func TestComputeTargetModelFullChain(t *testing.T) {
	mappings := map[string]string{"modelA": "mapped-model"}
	aiConf := &cluster_conf.AIConf{
		MatchPrefix:  "openrouter/",
		StripPrefix:  true,
		ModelMapping: &mappings,
	}
	model := computeTargetModel("clientprefix/original", "openrouter/modelA", aiConf)
	if model != "mapped-model" {
		t.Errorf("expected mapped-model, got %s", model)
	}
}

func TestComputeTargetModelFallbackRecompute(t *testing.T) {
	// Simulates fallback scenario: primary cluster mapped clientprefix/mymodel
	// to mapped-primary-model; fallback cluster should recompute from ClientModel.
	primaryMappings := map[string]string{"modelA": "mapped-primary-model"}
	primaryConf := &cluster_conf.AIConf{
		MatchPrefix:  "openrouter/",
		StripPrefix:  true,
		ModelMapping: &primaryMappings,
	}
	primaryModel := computeTargetModel("clientprefix/mymodel", "openrouter/modelA", primaryConf)
	if primaryModel != "mapped-primary-model" {
		t.Errorf("expected mapped-primary-model, got %s", primaryModel)
	}

	fallbackModel := computeTargetModel("clientprefix/mymodel", "", nil)
	if fallbackModel != "clientprefix/mymodel" {
		t.Errorf("expected fallback to recompute from ClientModel, got %s", fallbackModel)
	}
}

func TestComputeTargetModelStripPrefixNoMatch(t *testing.T) {
	aiConf := &cluster_conf.AIConf{
		MatchPrefix: "openrouter/",
		StripPrefix: true,
	}
	model := computeTargetModel("anthropic/claude", "", aiConf)
	if model != "anthropic/claude" {
		t.Errorf("expected anthropic/claude unchanged, got %s", model)
	}
}

func TestComputeTargetModelMappingNoMatch(t *testing.T) {
	mappings := map[string]string{"modelA": "mapped-model"}
	aiConf := &cluster_conf.AIConf{
		ModelMapping: &mappings,
	}
	model := computeTargetModel("modelB", "", aiConf)
	if model != "modelB" {
		t.Errorf("expected modelB unchanged, got %s", model)
	}
}

// mockRedisClient is a simple in-memory redis_client.Client for unit tests.
type mockRedisClient struct {
	data map[string]struct {
		value  []byte
		expire int
	}
	failGet bool
	failSet bool
}

func newMockRedisClient() *mockRedisClient {
	return &mockRedisClient{
		data: make(map[string]struct {
			value  []byte
			expire int
		}),
	}
}

func (m *mockRedisClient) Setex(key string, value []byte, expire int) error {
	if m.failSet {
		return errors.New("redis setex failed")
	}
	m.data[key] = struct {
		value  []byte
		expire int
	}{value: value, expire: expire}
	return nil
}

func (m *mockRedisClient) Get(key string) (interface{}, error) {
	if m.failGet {
		return nil, errors.New("redis get failed")
	}
	v, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	return v.value, nil
}

func (m *mockRedisClient) Expire(key string, expire int) error {
	if v, ok := m.data[key]; ok {
		v.expire = expire
		m.data[key] = v
	}
	return nil
}

func (m *mockRedisClient) Incr(key string) (int64, error) {
	panic("not implemented")
}

func (m *mockRedisClient) IncrAndExpire(key string, expire int) (int64, error) {
	panic("not implemented")
}

func (m *mockRedisClient) Decr(key string) (int64, error) {
	panic("not implemented")
}

func (m *mockRedisClient) PIncr(keys []string) ([]int64, error) {
	panic("not implemented")
}

func (m *mockRedisClient) GetInt64(key string) (int64, error) {
	panic("not implemented")
}

func (m *mockRedisClient) GetInt64Batch(keys []string) ([]int64, error) {
	panic("not implemented")
}

func (m *mockRedisClient) IncrBy(key string, delta int64) (int64, error) {
	panic("not implemented")
}

func (m *mockRedisClient) Delete(key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockRedisClient) HGetAll(key string) (map[string]string, error) {
	return nil, nil
}

func (m *mockRedisClient) NewScript(src string) redis_client.RedisScript {
	panic("not implemented")
}

func TestDefaultAIKeyPolicyIncludesAffinityDefaults(t *testing.T) {
	policy := defaultAIKeyPolicy()
	if policy.SessionAffinity != false {
		t.Errorf("expected SessionAffinity false, got %v", policy.SessionAffinity)
	}
	if policy.SessionAffinityTTL != 600 {
		t.Errorf("expected SessionAffinityTTL 600, got %d", policy.SessionAffinityTTL)
	}
	if policy.SessionAffinityRedisPrefix != "bfe:ai:key_affinity" {
		t.Errorf("expected SessionAffinityRedisPrefix bfe:ai:key_affinity, got %s", policy.SessionAffinityRedisPrefix)
	}
	if policy.SessionAffinityPenaltyEnable != true {
		t.Errorf("expected SessionAffinityPenaltyEnable true, got %v", policy.SessionAffinityPenaltyEnable)
	}
}

func TestChooseAIKeyWithAffinity_SingleKey(t *testing.T) {
	keys := []cluster_conf.AIKey{
		{Name: "key-a", Key: "ak-a", Weight: 100},
	}
	policy := cluster_conf.AIKeyPolicy{
		Strategy:                     "weighted_random",
		SessionAffinity:              true,
		SessionAffinityTTL:           300,
		SessionAffinityRedisPrefix:   "bfe:ai:key_affinity",
		SessionAffinityPenaltyEnable: true,
	}
	state := newAIKeyAttemptState()
	client := newMockRedisClient()

	idx, key, boundName, ok := chooseAIKeyWithAffinity("cluster", keys, policy, state, client, "session-1", nil, nil)
	if !ok {
		t.Fatal("expected to select key")
	}
	if idx != 0 || key.Name != "key-a" {
		t.Errorf("expected key-a, got %s", key.Name)
	}
	if boundName != "key-a" {
		t.Errorf("expected bound name key-a for single key, got %s", boundName)
	}
	// single key should not touch redis
	if len(client.data) != 0 {
		t.Errorf("expected no redis access for single key, got %d entries", len(client.data))
	}
}

func TestChooseAIKeyWithAffinity_Hit(t *testing.T) {
	keys := []cluster_conf.AIKey{
		{Name: "key-a", Key: "ak-a", Weight: 50},
		{Name: "key-b", Key: "ak-b", Weight: 50},
	}
	policy := cluster_conf.AIKeyPolicy{
		Strategy:                     "weighted_random",
		SessionAffinity:              true,
		SessionAffinityTTL:           300,
		SessionAffinityRedisPrefix:   "bfe:ai:key_affinity",
		SessionAffinityPenaltyEnable: true,
	}
	state := newAIKeyAttemptState()
	client := newMockRedisClient()
	client.Setex("bfe:ai:key_affinity:cluster:session-1", []byte("key-b"), 300)

	proxyState := &ProxyState{
		ReqAiKeyAffinityHit: new(metrics.Counter),
	}

	_, key, boundName, ok := chooseAIKeyWithAffinity("cluster", keys, policy, state, client, "session-1", proxyState, nil)
	if !ok {
		t.Fatal("expected to select key")
	}
	if key.Name != "key-b" {
		t.Errorf("expected key-b from binding, got %s", key.Name)
	}
	if boundName != "key-b" {
		t.Errorf("expected boundName key-b, got %s", boundName)
	}
	if proxyState.ReqAiKeyAffinityHit.Get() != 1 {
		t.Errorf("expected affinity hit counter 1, got %d", proxyState.ReqAiKeyAffinityHit.Get())
	}
}

func TestChooseAIKeyWithAffinity_Miss(t *testing.T) {
	keys := []cluster_conf.AIKey{
		{Name: "key-a", Key: "ak-a", Weight: 50},
		{Name: "key-b", Key: "ak-b", Weight: 50},
	}
	policy := cluster_conf.AIKeyPolicy{
		Strategy:                     "weighted_random",
		SessionAffinity:              true,
		SessionAffinityTTL:           300,
		SessionAffinityRedisPrefix:   "bfe:ai:key_affinity",
		SessionAffinityPenaltyEnable: true,
	}
	state := newAIKeyAttemptState()
	client := newMockRedisClient()

	proxyState := &ProxyState{
		ReqAiKeyAffinityMiss: new(metrics.Counter),
	}

	idx, _, boundName, ok := chooseAIKeyWithAffinity("cluster", keys, policy, state, client, "session-1", proxyState, nil)
	if !ok {
		t.Fatal("expected to select key")
	}
	if idx < 0 || idx >= len(keys) {
		t.Errorf("expected valid index, got %d", idx)
	}
	if boundName != "" {
		t.Errorf("expected no previous binding, got %s", boundName)
	}
	if proxyState.ReqAiKeyAffinityMiss.Get() != 1 {
		t.Errorf("expected affinity miss counter 1, got %d", proxyState.ReqAiKeyAffinityMiss.Get())
	}
	// binding should be written
	if _, exists := client.data["bfe:ai:key_affinity:cluster:session-1"]; !exists {
		t.Error("expected binding to be written to redis")
	}
}

func TestChooseAIKeyWithAffinity_RedisErrFallback(t *testing.T) {
	keys := []cluster_conf.AIKey{
		{Name: "key-a", Key: "ak-a", Weight: 50},
		{Name: "key-b", Key: "ak-b", Weight: 50},
	}
	policy := cluster_conf.AIKeyPolicy{
		Strategy:                     "weighted_random",
		SessionAffinity:              true,
		SessionAffinityTTL:           300,
		SessionAffinityRedisPrefix:   "bfe:ai:key_affinity",
		SessionAffinityPenaltyEnable: true,
	}
	state := newAIKeyAttemptState()
	client := newMockRedisClient()
	client.failGet = true

	proxyState := &ProxyState{
		ReqAiKeyAffinityRedisErr: new(metrics.Counter),
	}

	idx, _, _, ok := chooseAIKeyWithAffinity("cluster", keys, policy, state, client, "session-1", proxyState, nil)
	if !ok {
		t.Fatal("expected fallback to random")
	}
	if idx < 0 || idx >= len(keys) {
		t.Errorf("expected valid index, got %d", idx)
	}
	if proxyState.ReqAiKeyAffinityRedisErr.Get() != 1 {
		t.Errorf("expected redis err counter 1, got %d", proxyState.ReqAiKeyAffinityRedisErr.Get())
	}
}

func TestChooseAIKeyWithAffinity_PenaltySkip(t *testing.T) {
	keys := []cluster_conf.AIKey{
		{Name: "key-a", Key: "ak-a", Weight: 50},
		{Name: "key-b", Key: "ak-b", Weight: 50},
	}
	policy := cluster_conf.AIKeyPolicy{
		Strategy:                     "weighted_random",
		SessionAffinity:              true,
		SessionAffinityTTL:           300,
		SessionAffinityRedisPrefix:   "bfe:ai:key_affinity",
		SessionAffinityPenaltyEnable: true,
	}
	state := newAIKeyAttemptState()
	client := newMockRedisClient()
	client.Setex("bfe:ai:key_affinity:penalty:cluster:key-a", []byte("429"), 60)

	proxyState := &ProxyState{
		ReqAiKeyAffinityPenaltySkip: new(metrics.Counter),
	}

	// force deterministic selection: only key-b is eligible
	_, key, _, ok := chooseAIKeyWithAffinity("cluster", keys, policy, state, client, "session-1", proxyState, nil)
	if !ok {
		t.Fatal("expected to select key")
	}
	if key.Name != "key-b" {
		t.Errorf("expected key-b after penalty skip, got %s", key.Name)
	}
	if proxyState.ReqAiKeyAffinityPenaltySkip.Get() != 1 {
		t.Errorf("expected penalty skip counter 1, got %d", proxyState.ReqAiKeyAffinityPenaltySkip.Get())
	}
}

func TestChooseAIKeyWithAffinity_Disabled(t *testing.T) {
	keys := []cluster_conf.AIKey{
		{Name: "key-a", Key: "ak-a", Weight: 50},
		{Name: "key-b", Key: "ak-b", Weight: 50},
	}
	policy := cluster_conf.AIKeyPolicy{
		Strategy:                     "weighted_random",
		SessionAffinity:              false,
		SessionAffinityTTL:           300,
		SessionAffinityRedisPrefix:   "bfe:ai:key_affinity",
		SessionAffinityPenaltyEnable: true,
	}
	state := newAIKeyAttemptState()
	client := newMockRedisClient()

	idx, _, boundName, ok := chooseAIKeyWithAffinity("cluster", keys, policy, state, client, "session-1", nil, nil)
	if !ok {
		t.Fatal("expected to select key")
	}
	if idx < 0 || idx >= len(keys) {
		t.Errorf("expected valid index, got %d", idx)
	}
	if boundName != "" {
		t.Errorf("expected no binding when disabled, got %s", boundName)
	}
	if len(client.data) != 0 {
		t.Errorf("expected no redis access when disabled, got %d entries", len(client.data))
	}
}

// isBatchPassthroughMode + shouldRewriteBodyModel guard the request-body
// model rewrite for provider-native passthrough traffic (issue #1426):
// /v1/files multipart uploads and /v1/batches create JSON must reach the
// backend byte-for-byte even when the AI route carries a non-empty target
// model.
func TestShouldRewriteBodyModel(t *testing.T) {
	jsonHeader := func(ct string) bfe_http.Header {
		h := make(bfe_http.Header)
		if ct != "" {
			h.Set("Content-Type", ct)
		}
		return h
	}
	chatMeta := &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeChat}
	fileMeta := &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeFile}
	batchMeta := &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeBatch}

	cases := []struct {
		name        string
		meta        *bfe_basic.AiBasicInfo
		contentType string
		target      string
		body        string
		want        bool
	}{
		{"chat json body rewritten", chatMeta, "application/json", "m-a", "m-b", true},
		{"chat json with charset rewritten", chatMeta, "application/json; charset=utf-8", "m-a", "m-b", true},
		{"chat +json suffix rewritten", chatMeta, "application/problem+json", "m-a", "m-b", true},
		// clients that send a JSON body without Content-Type keep working
		{"chat missing content-type rewritten", chatMeta, "", "m-a", "m-b", true},
		{"chat malformed content-type rewritten", chatMeta, "not a media type;==", "m-a", "m-b", true},
		{"equal model not rewritten", chatMeta, "application/json", "m-a", "m-a", false},
		{"empty target not rewritten", chatMeta, "application/json", "", "m-b", false},
		// issue #1426: passthrough modes never rewrite, whatever the Content-Type
		{"file multipart not rewritten", fileMeta, "multipart/form-data; boundary=x", "m-a", "", false},
		{"batch json not rewritten", batchMeta, "application/json", "m-a", "", false},
		// defense in depth: explicit non-JSON Content-Type wins over the mode
		{"chat multipart not rewritten", chatMeta, "multipart/form-data; boundary=x", "m-a", "", false},
		{"chat text not rewritten", chatMeta, "text/plain", "m-a", "", false},
		{"chat octet-stream not rewritten", chatMeta, "application/octet-stream", "m-a", "", false},
	}
	for _, c := range cases {
		if got := shouldRewriteBodyModel(c.meta, jsonHeader(c.contentType), c.target, c.body); got != c.want {
			t.Errorf("%s: shouldRewriteBodyModel = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestAiClusterInvokeExhaustionReturnsSynthetic429 verifies issue #1440:
// when no AI key can be selected (here: the only configured key has
// Weight=0), aiClusterInvoke must return a decidable synthetic 429 response
// instead of (nil, nil), so the outer ServeHTTPForAI fallback decision can
// trigger cluster fallback or hand the client a retryable 429.
func TestAiClusterInvokeExhaustionReturnsSynthetic429(t *testing.T) {
	intPtr := func(v int) *int { return &v }
	boolPtr := func(v bool) *bool { return &v }
	strPtr := func(v string) *string { return &v }

	clusterName := "exhausted-cluster"
	clusterConfs := cluster_conf.BfeClusterConf{
		Version: strPtr("test"),
		Config: &cluster_conf.ClusterToConf{
			clusterName: {
				ClusterBasic: &cluster_conf.ClusterBasicConf{
					TimeoutReadClient:      intPtr(1000),
					TimeoutWriteClient:     intPtr(1000),
					TimeoutReadClientAgain: intPtr(1000),
					ReqWriteBufferSize:     intPtr(4096),
					ReqFlushInterval:       intPtr(0),
					ResFlushInterval:       intPtr(0),
					CancelOnClientClose:    boolPtr(false),
					DisableHostHeader:      boolPtr(false),
					DisableHealthCheck:     boolPtr(true),
				},
				AIConf: &cluster_conf.AIConf{
					Keys: []cluster_conf.AIKey{
						{Name: "key-zero", Key: "ak-zero", Weight: 0},
					},
					// KeyPolicy nil -> defaultAIKeyPolicy(): MaxRetries=0,
					// SessionAffinity=false, no redis needed.
				},
			},
		},
	}
	clusterTable := new(bfe_route.ClusterTable)
	clusterTable.BasicInit(clusterConfs)
	serverConf := &bfe_route.ServerDataConf{ClusterTable: clusterTable}

	conn, _ := net.Pipe()
	defer conn.Close()
	httpReq := &bfe_http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/v1/chat/completions"},
		Header: bfe_http.Header{},
		Body:   ioutil.NopCloser(strings.NewReader(`{"model":"m"}`)),
	}
	basicReq := &bfe_basic.Request{HttpRequest: httpReq, Connection: conn}

	rp := NewReverseProxy(nil, new(ProxyState))
	// dev branch dereferences srv (AIKeyAffinityRedis) unconditionally
	srv := &BfeServer{}
	attempt := aiForwardAttempt{ClusterName: clusterName, Model: "m"}

	res, _, cluster, err, _ := rp.aiClusterInvoke(srv, serverConf, basicReq, nil, attempt, nil, "m")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if cluster == nil || cluster.Name != clusterName {
		t.Fatalf("expected cluster %s, got %v", clusterName, cluster)
	}
	if res == nil {
		t.Fatal("expected synthetic 429 response, got nil (issue #1440)")
	}
	if res.StatusCode != 429 {
		t.Fatalf("expected status 429, got %d", res.StatusCode)
	}
	// issue #1441: the ErrCode sentinel carries the error code for access-log
	// attribution; it must NOT be ErrAiRateLimit (that would suppress
	// cluster fallback and stop key rotation).
	if basicReq.ErrCode != bfe_basic.ErrAiNoAvailableKey {
		t.Errorf("expected ErrCode %v, got %v", bfe_basic.ErrAiNoAvailableKey, basicReq.ErrCode)
	}
	if basicReq.ErrCode == bfe_basic.ErrAiRateLimit {
		t.Error("ErrCode must not be ErrAiRateLimit (would suppress fallback)")
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON content type, got %q", ct)
	}
	// The unified AiError form marks the response as gateway-generated so
	// normalizeUpstreamError (AIConf.NormalizeUpstreamError) leaves it alone.
	if res.Header.Get(bfe_basic.HeaderBfeGwError) != "1" {
		t.Errorf("expected %s header on gateway-generated synthetic 429", bfe_basic.HeaderBfeGwError)
	}
	// issue #1441: unified AiError body with error.code (§3.2 必填),
	// retry_after_seconds detail and a Retry-After header.
	if res.Header.Get("Retry-After") != strconv.Itoa(aiKeyPenaltyTTL429) {
		t.Errorf("expected Retry-After %d, got %q", aiKeyPenaltyTTL429, res.Header.Get("Retry-After"))
	}
	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		t.Fatalf("read body failed: %v", readErr)
	}
	var aiErrBody bfe_basic.AiErrorBody
	if jsonErr := json.Unmarshal(body, &aiErrBody); jsonErr != nil {
		t.Fatalf("body is not a unified AiError JSON: %v, body: %s", jsonErr, body)
	}
	if aiErrBody.Error.Code != bfe_basic.CodeNoAvailableKey {
		t.Errorf("expected error.code %q, got %q", bfe_basic.CodeNoAvailableKey, aiErrBody.Error.Code)
	}
	if aiErrBody.Error.Type != bfe_basic.TypeRateLimitError {
		t.Errorf("expected error.type %q, got %q", bfe_basic.TypeRateLimitError, aiErrBody.Error.Type)
	}
	if aiErrBody.Error.Details == nil || aiErrBody.Error.Details.RetryAfterSeconds != aiKeyPenaltyTTL429 {
		t.Errorf("expected error.details.retry_after_seconds=%d, got %+v", aiKeyPenaltyTTL429, aiErrBody.Error.Details)
	}
}

// TestFilterPenaltyKeys_ExcludesZeroWeight verifies issue #1439: a Weight=0
// key never counts as a penalty-filter candidate, so a penalized usable key
// with only Weight=0 companions yields no candidates (circuit break).
func TestFilterPenaltyKeys_ExcludesZeroWeight(t *testing.T) {
	policy := cluster_conf.AIKeyPolicy{SessionAffinityRedisPrefix: "bfe:ai:key_affinity"}
	client := newMockRedisClient()
	client.Setex("bfe:ai:key_affinity:penalty:cluster:key-a", []byte("429"), 60)

	keys := []cluster_conf.AIKey{
		{Name: "key-a", Key: "ak-a", Weight: 100},
		{Name: "key-z", Key: "ak-z", Weight: 0},
	}
	filtered, skipped := filterPenaltyKeys("cluster", keys, policy, client)
	if len(filtered) != 0 {
		t.Fatalf("expected no candidates (circuit break), got %v", filtered)
	}
	if skipped != 1 {
		t.Errorf("expected skipped=1 (only the penalized usable key), got %d", skipped)
	}
}

// TestFilterPenaltyKeys_CircuitBreakWhenAllPenalized verifies the issue
// #1439 ruling (方案乙): when every usable key is penalized the filter
// returns no candidates, instead of falling back to the penalized list.
func TestFilterPenaltyKeys_CircuitBreakWhenAllPenalized(t *testing.T) {
	policy := cluster_conf.AIKeyPolicy{SessionAffinityRedisPrefix: "bfe:ai:key_affinity"}
	client := newMockRedisClient()
	client.Setex("bfe:ai:key_affinity:penalty:cluster:key-a", []byte("429"), 60)
	client.Setex("bfe:ai:key_affinity:penalty:cluster:key-b", []byte("429"), 60)

	keys := []cluster_conf.AIKey{
		{Name: "key-a", Key: "ak-a", Weight: 50},
		{Name: "key-b", Key: "ak-b", Weight: 50},
	}
	filtered, skipped := filterPenaltyKeys("cluster", keys, policy, client)
	if len(filtered) != 0 {
		t.Fatalf("expected no candidates when all usable keys penalized, got %v", filtered)
	}
	if skipped != 2 {
		t.Errorf("expected skipped=2, got %d", skipped)
	}
}

// TestFilterPenaltyKeys_ZeroWeightNotCandidate verifies a Weight=0 key alone
// cannot keep the filter non-empty (it is excluded before penalty checks).
func TestFilterPenaltyKeys_ZeroWeightNotCandidate(t *testing.T) {
	policy := cluster_conf.AIKeyPolicy{SessionAffinityRedisPrefix: "bfe:ai:key_affinity"}
	client := newMockRedisClient()

	keys := []cluster_conf.AIKey{
		{Name: "key-z", Key: "ak-z", Weight: 0},
		{Name: "key-a", Key: "ak-a", Weight: 100},
	}
	filtered, skipped := filterPenaltyKeys("cluster", keys, policy, client)
	if len(filtered) != 1 || filtered[0].Name != "key-a" {
		t.Fatalf("expected [key-a], got %v", filtered)
	}
	if skipped != 0 {
		t.Errorf("expected skipped=0 (Weight=0 exclusion is not a penalty skip), got %d", skipped)
	}
}

// TestChooseAIKeyWithAffinity_CircuitBreak verifies the end-to-end selection
// path: the only usable key penalized (with only Weight=0 companions) means
// no key can be selected, and the caller (aiClusterInvoke, issue #1440)
// turns ok=false into a synthetic 429 for cluster fallback.
func TestChooseAIKeyWithAffinity_CircuitBreak(t *testing.T) {
	keys := []cluster_conf.AIKey{
		{Name: "key-a", Key: "ak-a", Weight: 100},
		{Name: "key-z", Key: "ak-z", Weight: 0},
	}
	policy := cluster_conf.AIKeyPolicy{
		Strategy:                     "weighted_random",
		SessionAffinity:              true,
		SessionAffinityTTL:           300,
		SessionAffinityRedisPrefix:   "bfe:ai:key_affinity",
		SessionAffinityPenaltyEnable: true,
	}
	state := newAIKeyAttemptState()
	client := newMockRedisClient()
	client.Setex("bfe:ai:key_affinity:penalty:cluster:key-a", []byte("429"), 60)

	proxyState := &ProxyState{
		ReqAiKeyAffinityPenaltySkip: new(metrics.Counter),
	}

	_, _, _, ok := chooseAIKeyWithAffinity("cluster", keys, policy, state, client, "session-1", proxyState, nil)
	if ok {
		t.Fatal("expected circuit break (no key selected), but a key was selected")
	}
	if proxyState.ReqAiKeyAffinityPenaltySkip.Get() != 1 {
		t.Errorf("expected penalty skip counter 1, got %d", proxyState.ReqAiKeyAffinityPenaltySkip.Get())
	}
}

// TestBackendUnavailableErrorResponseContract verifies issue #1441 step 3:
// the gateway-side synthesized failures (no cluster, backend
// connect/transport error, ...) now return the unified AiError body
// (BACKEND_UNAVAILABLE / 502 / application/json) instead of a bare empty-body
// 500.
func TestBackendUnavailableErrorResponseContract(t *testing.T) {
	basicReq := &bfe_basic.Request{HttpRequest: &bfe_http.Request{Header: bfe_http.Header{}}}
	res := bfe_basic.NewAiError(bfe_basic.CodeBackendUnavailable, bfe_basic.TypeInternalError,
		"upstream backend unavailable for this request").CreateErrorResponse(basicReq)

	if res.StatusCode != 502 {
		t.Fatalf("expected status 502, got %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	// issue #1441 regression guard: other error codes must NOT get a
	// Retry-After header (only the key-exhaustion call site sets it).
	if res.Header.Get("Retry-After") != "" {
		t.Errorf("BACKEND_UNAVAILABLE must not carry Retry-After, got %q", res.Header.Get("Retry-After"))
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	var aiErrBody bfe_basic.AiErrorBody
	if jsonErr := json.Unmarshal(body, &aiErrBody); jsonErr != nil {
		t.Fatalf("body is not a unified AiError JSON: %v, body: %s", jsonErr, body)
	}
	if aiErrBody.Error.Code != bfe_basic.CodeBackendUnavailable {
		t.Errorf("expected error.code %q, got %q", bfe_basic.CodeBackendUnavailable, aiErrBody.Error.Code)
	}
	if aiErrBody.Error.Type != bfe_basic.TypeInternalError {
		t.Errorf("expected error.type %q, got %q", bfe_basic.TypeInternalError, aiErrBody.Error.Type)
	}
	if aiErrBody.Error.Message == "" {
		t.Error("expected non-empty error.message")
	}
}

// TestIsAIClientAbort verifies the issue #1441 boundary predicate: only
// client-write/close/reset sentinels keep the legacy bare-500 record; other
// error codes (backend failures, exhaustion) get the unified AiError body.
func TestIsAIClientAbort(t *testing.T) {
	for _, code := range []error{bfe_basic.ErrClientWrite, bfe_basic.ErrClientClose, bfe_basic.ErrClientReset} {
		if !isAIClientAbort(code) {
			t.Errorf("expected %v to be treated as client abort", code)
		}
	}
	for _, code := range []error{nil, bfe_basic.ErrBkNoCluster, bfe_basic.ErrBkConnectBackend, bfe_basic.ErrAiRateLimit, bfe_basic.ErrAiNoAvailableKey} {
		if isAIClientAbort(code) {
			t.Errorf("expected %v NOT to be treated as client abort", code)
		}
	}
}

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

	"github.com/bfenetworks/bfe/bfe_basic"
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

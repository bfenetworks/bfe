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
	"net/url"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_http"
)

func TestRewriteUpstreamPath(t *testing.T) {
	cases := []struct {
		name      string
		reqPath   string
		authStyle string
		aiConf    *cluster_conf.AIConf
		want      string
	}{
		// scenario table: all surveyed provider shapes
		{"nil aiConf passthrough", "/v1/messages", bfe_basic.AuthStyleAnthropic, nil, "/v1/messages"},
		{"nil protocol paths passthrough", "/v1/messages", bfe_basic.AuthStyleAnthropic,
			&cluster_conf.AIConf{}, "/v1/messages"},
		{"bailian anthropic", "/v1/messages", bfe_basic.AuthStyleAnthropic,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"anthropic": "/apps/anthropic"}},
			"/apps/anthropic/v1/messages"},
		{"bailian openai", "/v1/chat/completions", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/compatible-mode/v1"}},
			"/compatible-mode/v1/chat/completions"},
		{"deepseek anthropic", "/v1/messages", bfe_basic.AuthStyleAnthropic,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"anthropic": "/anthropic"}},
			"/anthropic/v1/messages"},
		{"kimi code anthropic", "/v1/messages", bfe_basic.AuthStyleAnthropic,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"anthropic": "/coding"}},
			"/coding/v1/messages"},
		{"kimi code openai", "/v1/chat/completions", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/coding/v1"}},
			"/coding/v1/chat/completions"},
		{"volcengine openai", "/v1/chat/completions", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}},
			"/api/v3/chat/completions"},
		{"identity openai base", "/v1/chat/completions", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/v1"}},
			"/v1/chat/completions"},
		{"volcengine anthropic compatible", "/v1/messages", bfe_basic.AuthStyleAnthropic,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"anthropic": "/api/compatible"}},
			"/api/compatible/v1/messages"},
		{"openai responses endpoint", "/v1/responses", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}},
			"/api/v3/responses"},
		{"anthropic model list", "/v1/models", bfe_basic.AuthStyleAnthropic,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"anthropic": "/apps/anthropic"}},
			"/apps/anthropic/v1/models"},
		// boundaries
		{"unconfigured protocol passthrough", "/v1/messages", bfe_basic.AuthStyleAnthropic,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}}, "/v1/messages"},
		{"unknown auth style passthrough", "/v1/messages", bfe_basic.AuthStyleUnknown,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"anthropic": "/apps/anthropic"}}, "/v1/messages"},
		{"exact /v1 anthropic", "/v1", bfe_basic.AuthStyleAnthropic,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"anthropic": "/apps/anthropic"}},
			"/apps/anthropic/v1"},
		{"exact /v1 openai", "/v1", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}}, "/api/v3"},
		{"/v10 not standard prefix", "/v10/xxx", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}}, "/v10/xxx"},
		{"v1beta path not standard prefix", "/v1beta/models/gemini:generateContent", bfe_basic.AuthStyleGemini,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"gemini": "/gemini"}},
			"/v1beta/models/gemini:generateContent"},
		{"non-standard entry passthrough", "/messages", bfe_basic.AuthStyleAnthropic,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"anthropic": "/apps/anthropic"}}, "/messages"},
		{"empty path passthrough", "", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}}, ""},
		// issue #1379: openai base path applies with or without the /v1 client prefix
		{"issue1379 no-v1 chat completions", "/chat/completions", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/compatible-mode/v1"}},
			"/compatible-mode/v1/chat/completions"},
		{"issue1379 no-v1 entry passthrough without config", "/chat/completions", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{}, "/chat/completions"},
		{"issue1379 v1 entry passthrough without config", "/v1/chat/completions", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{}, "/v1/chat/completions"},
		// openai endpoints without the /v1 prefix
		{"no-v1 completions", "/completions", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}}, "/api/v3/completions"},
		{"no-v1 embeddings", "/embeddings", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}}, "/api/v3/embeddings"},
		{"no-v1 responses", "/responses", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}}, "/api/v3/responses"},
		{"no-v1 model list", "/models", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}}, "/api/v3/models"},
		{"no-v1 model item", "/models/gpt-4", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}}, "/api/v3/models/gpt-4"},
		// passthrough protection: a configured base must not rewrite non-endpoint paths
		{"provider-native full path passthrough", "/compatible-mode/v1/chat/completions", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/compatible-mode/v1"}},
			"/compatible-mode/v1/chat/completions"},
		{"custom path passthrough", "/custom/path", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}}, "/custom/path"},
		{"messages path not openai endpoint", "/messages", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}}, "/messages"},
		{"exact /v1 slash openai", "/v1/", bfe_basic.AuthStyleOpenAI,
			&cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}}, "/api/v3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rewriteUpstreamPath(c.reqPath, c.authStyle, c.aiConf); got != c.want {
				t.Errorf("rewriteUpstreamPath(%q, %q) = %q, want %q",
					c.reqPath, c.authStyle, got, c.want)
			}
		})
	}
}

func TestIsStandardV1Prefix(t *testing.T) {
	cases := map[string]bool{
		"/v1":          true,
		"/v1/":         true,
		"/v1/messages": true,
		"":             false,
		"/":            false,
		"/v10":         false,
		"/v10/xxx":     false,
		"/v1beta/x":    false,
		"/api/v1/x":    false,
	}
	for path, want := range cases {
		if got := isStandardV1Prefix(path); got != want {
			t.Errorf("isStandardV1Prefix(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestApplyAIProtocolPathRewrite(t *testing.T) {
	newReq := func(path string) *bfe_http.Request {
		return &bfe_http.Request{URL: &url.URL{Path: path}}
	}

	t.Run("rewrite applies to private URL copy", func(t *testing.T) {
		outreq := newReq("/v1/messages")
		inboundURL := outreq.URL
		aiConf := &cluster_conf.AIConf{ProtocolPaths: map[string]string{"anthropic": "/apps/anthropic"}}

		applyAIProtocolPathRewrite(outreq, bfe_basic.AuthStyleAnthropic, aiConf)

		if outreq.URL.Path != "/apps/anthropic/v1/messages" {
			t.Errorf("outreq.URL.Path = %q, want /apps/anthropic/v1/messages", outreq.URL.Path)
		}
		if outreq.URL == inboundURL {
			t.Error("expected outreq to hold a private URL copy after rewrite")
		}
	})

	t.Run("no rewrite keeps shared URL pointer", func(t *testing.T) {
		outreq := newReq("/v1/messages")
		inboundURL := outreq.URL

		applyAIProtocolPathRewrite(outreq, bfe_basic.AuthStyleAnthropic, &cluster_conf.AIConf{})
		applyAIProtocolPathRewrite(outreq, bfe_basic.AuthStyleAnthropic, nil)

		if outreq.URL != inboundURL {
			t.Error("expected shared URL pointer preserved when no rewrite applies")
		}
		if outreq.URL.Path != "/v1/messages" {
			t.Errorf("outreq.URL.Path = %q, want /v1/messages", outreq.URL.Path)
		}
	})

	t.Run("rewrite clears stale RawPath", func(t *testing.T) {
		outreq := &bfe_http.Request{URL: &url.URL{Path: "/v1/messages", RawPath: "/v1/messages"}}
		aiConf := &cluster_conf.AIConf{ProtocolPaths: map[string]string{"anthropic": "/apps/anthropic"}}

		applyAIProtocolPathRewrite(outreq, bfe_basic.AuthStyleAnthropic, aiConf)

		if outreq.URL.RawPath != "" {
			t.Errorf("RawPath = %q, want empty after rewrite", outreq.URL.RawPath)
		}
	})

	t.Run("nil request and nil URL are tolerated", func(t *testing.T) {
		aiConf := &cluster_conf.AIConf{ProtocolPaths: map[string]string{"anthropic": "/apps/anthropic"}}
		applyAIProtocolPathRewrite(nil, bfe_basic.AuthStyleAnthropic, aiConf)
		applyAIProtocolPathRewrite(&bfe_http.Request{}, bfe_basic.AuthStyleAnthropic, aiConf)
	})
}

// TestApplyAIProtocolPathRewriteFallbackRecompute mimics doSingleAIForward:
// every cluster attempt shallow-copies the inbound request into a fresh
// outreq, and the rewrite never touches the inbound request. A route-level
// fallback to a cluster with a different ProtocolPaths must therefore
// recompute the upstream path from the original client path.
func TestApplyAIProtocolPathRewriteFallbackRecompute(t *testing.T) {
	inbound := &bfe_http.Request{URL: &url.URL{Path: "/v1/messages"}}
	clusterA := &cluster_conf.AIConf{ProtocolPaths: map[string]string{"anthropic": "/apps/anthropic"}}
	clusterB := &cluster_conf.AIConf{ProtocolPaths: map[string]string{"anthropic": "/coding"}}

	outreqA := new(bfe_http.Request)
	*outreqA = *inbound
	applyAIProtocolPathRewrite(outreqA, bfe_basic.AuthStyleAnthropic, clusterA)

	outreqB := new(bfe_http.Request)
	*outreqB = *inbound
	applyAIProtocolPathRewrite(outreqB, bfe_basic.AuthStyleAnthropic, clusterB)

	if outreqA.URL.Path != "/apps/anthropic/v1/messages" {
		t.Errorf("attempt A path = %q, want /apps/anthropic/v1/messages", outreqA.URL.Path)
	}
	if outreqB.URL.Path != "/coding/v1/messages" {
		t.Errorf("attempt B (fallback) path = %q, want /coding/v1/messages", outreqB.URL.Path)
	}
	if inbound.URL.Path != "/v1/messages" {
		t.Errorf("inbound path mutated to %q, want /v1/messages", inbound.URL.Path)
	}
}

// TestApplyAIProtocolPathRewriteMixedFallback covers a fallback between a
// cluster with rewrite and one without: the second attempt must see the
// original client path, not the first attempt's rewritten one.
func TestApplyAIProtocolPathRewriteMixedFallback(t *testing.T) {
	inbound := &bfe_http.Request{URL: &url.URL{Path: "/v1/chat/completions"}}
	clusterA := &cluster_conf.AIConf{ProtocolPaths: map[string]string{"openai": "/api/v3"}}
	clusterB := &cluster_conf.AIConf{}

	outreqA := new(bfe_http.Request)
	*outreqA = *inbound
	applyAIProtocolPathRewrite(outreqA, bfe_basic.AuthStyleOpenAI, clusterA)

	outreqB := new(bfe_http.Request)
	*outreqB = *inbound
	applyAIProtocolPathRewrite(outreqB, bfe_basic.AuthStyleOpenAI, clusterB)

	if outreqA.URL.Path != "/api/v3/chat/completions" {
		t.Errorf("attempt A path = %q, want /api/v3/chat/completions", outreqA.URL.Path)
	}
	if outreqB.URL.Path != "/v1/chat/completions" {
		t.Errorf("attempt B (fallback, no rewrite) path = %q, want original /v1/chat/completions",
			outreqB.URL.Path)
	}
}

func TestIsStandardV1PrefixLongPath(t *testing.T) {
	if !isStandardV1Prefix("/v1/" + strings.Repeat("a", 1000)) {
		t.Error("expected long /v1/ prefixed path to match")
	}
}

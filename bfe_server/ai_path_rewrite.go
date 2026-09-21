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
	"strings"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_http"
)

// rewriteUpstreamPath computes the upstream request path from the client
// request path, the request protocol (AuthStyle) and the cluster AIConf.
//
// ProtocolPaths semantics: the configured base path is the path part of the
// protocol's official SDK base_url:
//   - openai: base ends with /v1 (e.g. /compatible-mode/v1, /api/v3,
//     /coding/v1); the OpenAI SDK appends /chat/completions to base_url, so
//     the client entry may or may not carry the /v1 version prefix. The
//     rewrite strips an optional leading /v1 and applies the base path to
//     recognized OpenAI API endpoints only (see openAIEndpoints); any other
//     path is forwarded unchanged (transparent passthrough).
//   - anthropic: base has no /v1 (e.g. /apps/anthropic, /coding); the
//     Anthropic SDK appends /v1/messages to base_url. Only standard entry
//     paths ("/v1" or "/v1/...") are rewritten.
func rewriteUpstreamPath(reqPath string, authStyle string, aiConf *cluster_conf.AIConf) string {
	if aiConf == nil {
		return reqPath
	}
	base, ok := aiConf.ProtocolPaths[authStyle]
	if !ok {
		return reqPath
	}
	if authStyle == bfe_basic.AuthStyleAnthropic {
		if !isStandardV1Prefix(reqPath) {
			return reqPath
		}
		return base + reqPath
	}
	// openai: base is the upstream API base path. Whether the client entry
	// carries the /v1 version prefix must not change the final upstream path:
	//   /v1/chat/completions -> base + /chat/completions
	//   /chat/completions    -> base + /chat/completions
	if reqPath == "/v1" || reqPath == "/v1/" {
		return base
	}
	rest := stripV1Prefix(reqPath)
	if !isOpenAIEndpoint(rest) {
		return reqPath
	}
	return base + rest
}

// stripV1Prefix strips a leading "/v1" version prefix: "/v1/chat/completions"
// -> "/chat/completions"; "/chat/completions" is returned unchanged;
// "/v10/xxx" does not match (no "/v1/" prefix) and is returned unchanged.
func stripV1Prefix(reqPath string) string {
	if strings.HasPrefix(reqPath, "/v1/") {
		return reqPath[len("/v1"):]
	}
	return reqPath
}

// openAIEndpoints lists the OpenAI API endpoints recognized for the base-path
// rewrite, aligned with the mode endpoints of DetectModeFromPath plus the
// read-only endpoints it does not cover. The rewrite applies to these
// endpoints only, so provider-native full paths (e.g. a client already
// calling /compatible-mode/v1/chat/completions) and custom passthrough paths
// are never double-prefixed.
var openAIEndpoints = []string{
	"/audio/speech",
	"/audio/transcriptions",
	"/audio/translations",
	"/chat/completions",
	"/completions",
	"/embeddings",
	"/images/edits",
	"/images/generations",
	"/models",
	"/moderations",
	"/responses",
	"/rerank",
	"/video/generations",
}

// isOpenAIEndpoint reports whether path (already stripped of an optional
// /v1 prefix) is a recognized OpenAI API endpoint: either exactly an endpoint
// or an endpoint followed by a subpath (e.g. /models/{model}).
func isOpenAIEndpoint(path string) bool {
	for _, ep := range openAIEndpoints {
		if path == ep || strings.HasPrefix(path, ep+"/") {
			return true
		}
	}
	return false
}

// isStandardV1Prefix reports whether reqPath is exactly "/v1" or starts
// with "/v1/". Paths like "/v10/xxx" do not match.
func isStandardV1Prefix(reqPath string) bool {
	return reqPath == "/v1" || strings.HasPrefix(reqPath, "/v1/")
}

// applyAIProtocolPathRewrite rewrites outreq.URL.Path according to the
// per-protocol upstream base paths configured on the cluster (see
// rewriteUpstreamPath). The rewrite result is written into a private copy of
// the URL: the inbound request URL is never modified, so every cluster
// attempt (including route-level fallback) recomputes the upstream path from
// the original client path. Requests without a configured rewrite keep the
// original (shared) URL pointer and behave exactly as before.
func applyAIProtocolPathRewrite(outreq *bfe_http.Request, authStyle string, aiConf *cluster_conf.AIConf) {
	if outreq == nil || outreq.URL == nil {
		return
	}
	newPath := rewriteUpstreamPath(outreq.URL.Path, authStyle, aiConf)
	if newPath == outreq.URL.Path {
		return
	}
	u := *outreq.URL
	u.Path = newPath
	u.RawPath = ""
	outreq.URL = &u
}

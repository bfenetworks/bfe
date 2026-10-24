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

package mod_traffic_mirror

import (
	"net/http"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
)

// mirrorTask is a fully self-contained snapshot of one mirror request. It
// never references *bfe_basic.Request: everything the async goroutine needs
// (headers, body, target identification) is copied here at submit time,
// because SvrDataConf is reset to nil after clusterInvoke and holding the
// whole request alive would pin its memory for the mirror duration.
type mirrorTask struct {
	Method  string      // original request method
	URI     string      // request URI (path + query) after path rewrite
	Host    string      // original Host header value
	Header  http.Header // header snapshot for the mirror request
	Body    []byte      // request body (already rewritten if configured)
	Cluster string      // mirror target cluster name
	Product string      // product of the original request
	Model   string      // requested model, for metrics labeling
	StartAt time.Time   // task creation time
}

// buildMirrorTask snapshots everything needed to send one mirror request.
// All fallible steps return nil task plus the skip reason; the caller only
// counts and goes on, never blocking or failing the main request. The
// rewriteFailed flag reports that a body rewrite was requested but fell back
// to the original body.
func buildMirrorTask(req *bfe_basic.Request, rule *MirrorRuleConf, maxBodyBytes int64) (task *mirrorTask, rewriteFailed bool, skipReason string) {
	accessor, err := req.HttpRequest.GetBodyAccessor()
	if err != nil || accessor == nil {
		return nil, false, "body_limit"
	}
	body, all := accessor.GetBytes()
	if !all || int64(len(body)) > maxBodyBytes {
		return nil, false, "body_limit"
	}

	// body rewrite (FR-6, phase 1: model field). A rewrite failure falls
	// back to mirroring the original body and is counted by the caller.
	mBody := body
	if len(rule.BodyRewrites) > 0 {
		if rewritten, err := rewriteMirrorBody(body, rule.BodyRewrites); err != nil {
			rewriteFailed = true
			mBody = body
		} else {
			mBody = rewritten
		}
	}

	uri := req.HttpRequest.URL.RequestURI()
	if rule.PathRewrite != "" {
		// full path replacement, query string preserved
		uri = rule.PathRewrite
		if qs := req.HttpRequest.URL.RawQuery; qs != "" {
			uri += "?" + qs
		}
	}

	task = &mirrorTask{
		Method:  req.HttpRequest.Method,
		URI:     uri,
		Host:    req.HttpRequest.Host,
		Header:  buildMirrorHeader(req, rule),
		Body:    mBody,
		Cluster: rule.MirrorCluster,
		Product: req.Route.Product,
		StartAt: time.Now(),
	}
	if aiInfo := req.GetAiBasicInfo(); aiInfo != nil {
		task.Model = aiInfo.ClientModel
	}

	return task, rewriteFailed, ""
}

// buildMirrorHeader clones the request headers and applies:
//  1. strip hop-by-hop headers (bfe_basic.HopHeaders)
//  2. strip rule-configured sensitive headers (FR-5)
//  3. inject rule-configured headers plus X-Bfe-Mirror and X-Bfe-Logid
func buildMirrorHeader(req *bfe_basic.Request, rule *MirrorRuleConf) http.Header {
	src := req.HttpRequest.Header
	dst := make(http.Header, len(src))

	for k, vv := range src {
		skip := false
		for _, h := range bfe_basic.HopHeaders {
			if k == h {
				skip = true
				break
			}
		}
		if !skip {
			cp := make([]string, len(vv))
			copy(cp, vv)
			dst[k] = cp
		}
	}

	// strip sensitive headers configured by the rule
	for _, h := range rule.RemoveHeaders {
		dst.Del(h)
	}

	// Content-Length is recomputed by the http client from Body
	dst.Del("Content-Length")

	// inject identification headers
	for k, v := range rule.SetHeaders {
		dst.Set(k, v)
	}
	if dst.Get(DefaultMirrorHeader) == "" {
		dst.Set(DefaultMirrorHeader, MirrorHeaderValue)
	}
	if req.LogId != "" {
		dst.Set("X-Bfe-Logid", req.LogId)
	}

	return dst
}

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

package mod_ai_cache

import (
	"io"

	"github.com/tidwall/gjson"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func (m *ModuleAiCache) cacheResponseHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	// only requests that went upstream with a cache context are candidates
	ctx := getAiCacheContext(req)
	if ctx == nil {
		return bfe_module.BfeHandlerGoOn
	}

	if res == nil || res.StatusCode != bfe_http.StatusOK || res.Body == nil {
		return bfe_module.BfeHandlerGoOn
	}

	// the client requested no cache
	if m.isSkip(req) {
		return bfe_module.BfeHandlerGoOn
	}

	// wrap the body: pass everything through to the client while
	// accumulating the answer for the cache write-back.
	res.Body = newCacheCaptureBody(res.Body, ctx.Rule.MaxValueBytes,
		func(body []byte, complete bool) {
			m.writeBack(req, ctx, body, complete)
		})

	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleAiCache) isSkip(req *bfe_basic.Request) bool {
	aiInfo := req.GetAiBasicInfo()
	return aiInfo != nil && aiInfo.AiCacheStatus == CacheStatusSkip
}

// writeBack extracts the answer from the fully read response and stores it
// in redis. It runs at response close; any failure is logged and swallowed
// (fail-open).
func (m *ModuleAiCache) writeBack(req *bfe_basic.Request, ctx *aiCacheContext, body []byte, complete bool) {
	// only cache responses that completed normally: a client abort or a
	// truncated upstream response must not poison the cache
	if !complete {
		if openDebug {
			log.Logger.Debug("%s: response incomplete, skip cache write-back, key[%s]", m.name, ctx.Key)
		}
		return
	}

	value := extractAnswer(body, ctx)
	if value == "" {
		if openDebug {
			log.Logger.Debug("%s: empty answer, skip cache write-back, key[%s]", m.name, ctx.Key)
		}
		return
	}

	if int64(len(value)) > ctx.Rule.MaxValueBytes {
		m.ruleTable.incValueTooLarge()
		m.state.Inc("VALUE_TOO_LARGE", 1)
		if openDebug {
			log.Logger.Debug("%s: answer too large, skip cache write-back, key[%s]", m.name, ctx.Key)
		}
		return
	}

	m.redisCache.Setex(ctx.Key, []byte(value), ctx.Rule.CacheTTL)
}

// extractAnswer pulls the answer content out of a complete response body,
// stream (SSE) or non-stream (JSON).
func extractAnswer(body []byte, ctx *aiCacheContext) string {
	if ctx.Stream {
		return extractStreamAnswer(body, ctx.Rule.CacheStreamFrom)
	}

	if !gjson.ValidBytes(body) {
		return ""
	}
	return gjson.GetBytes(body, ctx.Rule.CacheValueFrom).String()
}

// cacheCaptureBody wraps an upstream response body: reads pass through
// unchanged while content is accumulated up to limit. onClose is invoked
// once when the body is closed with the accumulated bytes and a flag
// telling whether the stream was read to completion (io.EOF).
type cacheCaptureBody struct {
	src      io.ReadCloser
	buf      []byte
	limit    int64
	onClose  func(body []byte, complete bool)
	done     bool
	complete bool
}

func newCacheCaptureBody(src io.ReadCloser, limit int64, onClose func([]byte, bool)) *cacheCaptureBody {
	return &cacheCaptureBody{
		src:     src,
		limit:   limit,
		onClose: onClose,
	}
}

func (b *cacheCaptureBody) Read(p []byte) (int, error) {
	n, err := b.src.Read(p)
	if n > 0 && int64(len(b.buf)) < b.limit {
		remain := int(b.limit) - len(b.buf)
		if remain > n {
			remain = n
		}
		b.buf = append(b.buf, p[:remain]...)
	}
	if err == io.EOF {
		b.complete = true
	}
	if err != nil && err != io.EOF {
		// read error (e.g. client abort surfaces here in some paths):
		// the accumulated content is not complete
		b.complete = false
	}
	return n, err
}

func (b *cacheCaptureBody) Close() error {
	if !b.done {
		b.done = true
		if b.onClose != nil {
			b.onClose(b.buf, b.complete)
		}
	}
	return b.src.Close()
}

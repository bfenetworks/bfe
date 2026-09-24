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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	// header to skip cache for a single request
	HeaderSkipAiCache = "x-bfe-skip-ai-cache"
	SkipCacheOn       = "on"

	ContentTypeJson = "application/json"
)

// aiCacheContext carries the state needed by the response phase to write
// the answer back to the cache.
type aiCacheContext struct {
	Key    string           // cache key
	Stream bool             // whether the client requested a streaming answer
	Rule   *ProductRuleConf // matched cache rule
}

func (m *ModuleAiCache) cacheRequestHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	aiInfo := req.GetAiBasicInfo()
	if aiInfo == nil {
		return bfe_module.BfeHandlerGoOn, nil
	}

	// 1. match cache rule: locate the rule list by product, then match the
	// first rule whose condition holds. No product / no rule means the
	// request passes without caching (consistent with other AI modules).
	rules, ok := m.ruleTable.Search(req.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn, nil
	}

	rule := m.ruleTable.Match(req, rules)
	if rule == nil {
		return bfe_module.BfeHandlerGoOn, nil
	}
	m.ruleTable.incReqTotal()
	m.state.Inc("REQ_TOTAL", 1)

	// 2. skip-cache header: neither read nor write for this request
	if strings.EqualFold(req.HttpRequest.Header.Get(HeaderSkipAiCache), SkipCacheOn) {
		m.ruleTable.incCacheSkip()
		m.state.Inc("CACHE_SKIP", 1)
		m.setCacheStatus(req, CacheStatusSkip)
		return bfe_module.BfeHandlerGoOn, nil
	}

	// 3. only application/json requests are cacheable
	contentType := req.HttpRequest.Header.Get("Content-Type")
	if !strings.Contains(contentType, ContentTypeJson) {
		return bfe_module.BfeHandlerGoOn, nil
	}

	// 4. read request body (bounded by maxBodyBytes)
	body, ok := m.readRequestBody(req, rule.MaxBodyBytes)
	if !ok {
		return bfe_module.BfeHandlerGoOn, nil
	}

	// 5. build cache key from the question extracted per cacheKeyStrategy
	key, stream, err := m.buildCacheKey(req, body, rule)
	if err != nil || key == "" {
		if openDebug {
			log.Logger.Debug("%s: build cache key failed or empty, err[%v]", m.name, err)
		}
		return bfe_module.BfeHandlerGoOn, nil
	}
	setCacheKey(req, key)

	// 6. exact lookup in redis
	if value, hit := m.redisCache.Get(key); hit {
		m.ruleTable.incCacheHit()
		m.state.Inc("CACHE_HIT", 1)
		m.setCacheStatus(req, CacheStatusHit)
		if openDebug {
			log.Logger.Debug("%s: cache hit, key[%s]", m.name, key)
		}
		return bfe_module.BfeHandlerFinish, m.buildHitResponse(req, rule, value, stream)
	}

	// 7. miss: save the context for the response phase and continue upstream
	m.ruleTable.incCacheMiss()
	m.state.Inc("CACHE_MISS", 1)
	m.setCacheStatus(req, CacheStatusMiss)
	setAiCacheContext(req, &aiCacheContext{Key: key, Stream: stream, Rule: rule})

	return bfe_module.BfeHandlerGoOn, nil
}

// readRequestBody returns the buffered request body. It returns ok=false
// (request passes without caching) when the body is not fully buffered or
// exceeds the configured limit.
func (m *ModuleAiCache) readRequestBody(req *bfe_basic.Request, maxBodyBytes int64) ([]byte, bool) {
	httpReq := req.HttpRequest
	if httpReq == nil {
		return nil, false
	}

	if httpReq.ContentLength > maxBodyBytes {
		if openDebug {
			log.Logger.Debug("%s: request body too large, contentLength[%d] > maxBodyBytes[%d]",
				m.name, httpReq.ContentLength, maxBodyBytes)
		}
		return nil, false
	}

	bodyAccessor, err := httpReq.GetBodyAccessor()
	if err != nil || bodyAccessor == nil {
		return nil, false
	}

	body, all := bodyAccessor.GetBytes()
	if !all {
		if openDebug {
			log.Logger.Debug("%s: request body not fully buffered, skip cache", m.name)
		}
		return nil, false
	}

	if int64(len(body)) > maxBodyBytes {
		return nil, false
	}

	return body, true
}

// buildCacheKey extracts the user question(s) from the request body and
// hashes them into a cache key. The key is prefixed with the tenant id
// (ClientKeyId) so that different tenants can never read each other's cache.
func (m *ModuleAiCache) buildCacheKey(req *bfe_basic.Request, body []byte, rule *ProductRuleConf) (string, bool, error) {
	if !gjson.ValidBytes(body) {
		return "", false, fmt.Errorf("request body is not valid json")
	}

	question := ""
	switch rule.CacheKeyStrategy {
	case CacheKeyStrategyLastQuestion:
		path := rule.CacheKeyFrom
		if path == "" {
			path = DefaultLastQuestionPath
		}
		question = gjson.GetBytes(body, path).String()
	case CacheKeyStrategyAllQuestions:
		question = joinUserQuestions(body, rule.CacheKeyFrom)
	default:
		return "", false, fmt.Errorf("unknown cacheKeyStrategy[%s]", rule.CacheKeyStrategy)
	}

	if question == "" {
		return "", false, nil
	}

	stream := gjson.GetBytes(body, "stream").Bool()

	sum := sha256.Sum256([]byte(question))
	key := fmt.Sprintf("%s:%s:%s", m.cacheKeyPrefix, tenantOf(req), hex.EncodeToString(sum[:16]))

	return key, stream, nil
}

// tenantOf returns the tenant identifier used to isolate cache keys. It
// falls back to "unknown" when the request was not authenticated, which
// simply gives those requests their own key space.
func tenantOf(req *bfe_basic.Request) string {
	aiInfo := req.GetAiBasicInfo()
	if aiInfo == nil || aiInfo.ClientKeyId == "" {
		return "unknown"
	}
	return aiInfo.ClientKeyId
}

// joinUserQuestions concatenates the content of all role=user messages as
// the cache key material for the allQuestions strategy.
func joinUserQuestions(body []byte, keyFrom string) string {
	var sb strings.Builder
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return ""
	}

	for _, msg := range messages.Array() {
		if msg.Get("role").String() != "user" {
			continue
		}
		content := msg.Get("content").String()
		if keyFrom != "" {
			content = msg.Get(keyFrom).String()
		}
		if content == "" {
			continue
		}
		sb.WriteString(content)
		sb.WriteString("\n")
	}

	return sb.String()
}

// buildHitResponse constructs the response served from cache. For streaming
// requests a complete SSE sequence is returned; the cached content is JSON
// escaped and substituted for the %s placeholder of the template.
func (m *ModuleAiCache) buildHitResponse(req *bfe_basic.Request, rule *ProductRuleConf, value string, stream bool) *bfe_http.Response {
	res := new(bfe_http.Response)
	res.StatusCode = bfe_http.StatusOK
	res.Header = make(bfe_http.Header)
	res.Header.Set("Server", "bfe")
	res.Header.Set("X-Bfe-Ai-Cache", CacheStatusHit)

	var body string
	if stream {
		res.Header.Set("Content-Type", "text/event-stream")
		body = strings.Replace(rule.StreamResponseTemplate, "%s", jsonQuote(value), 1)
	} else {
		res.Header.Set("Content-Type", ContentTypeJson)
		body = strings.Replace(rule.ResponseTemplate, "%s", jsonQuote(value), 1)
	}

	res.Body = ioutil.NopCloser(strings.NewReader(body))
	res.ContentLength = int64(len(body))

	req.HttpResponse = res
	return res
}

// jsonQuote returns s escaped as a JSON string content (without the
// surrounding quotes), suitable for substituting the %s placeholder inside
// the JSON string of a response template.
func jsonQuote(s string) string {
	quoted := strconv.Quote(s)
	return quoted[1 : len(quoted)-1]
}

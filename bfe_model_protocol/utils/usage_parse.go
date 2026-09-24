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

package utils

import (
	"github.com/tidwall/gjson"
)

// UsageFields is the protocol-neutral result of usage extraction. The two
// callers of the extraction chains (mod_body_process.QuotaUsage and
// bfe_basic.TokenUsage) each map these fields onto their own structures, so
// this package stays free of dependencies on bfe_basic / bfe_modules.
type UsageFields struct {
	UsedQuota          int64
	PromptTokens       int64
	CompletionTokens   int64
	CacheReadTokens    int64
	CacheWriteTokens   int64
	CacheWriteTokens1h int64 // 1h-TTL cache write tokens, already included in CacheWriteTokens
	AudioInputTokens   int64
	AudioOutputTokens  int64
	ImageInputTokens   int64
	ImageCount         int64
	VideoCount         int64
}

// parseCacheWriteTokens1h extracts the 1h-TTL cache write token count.
// Anthropic-family upstreams report it separately as
// usage.cache_creation.ephemeral_1h_input_tokens; some relays flatten it to
// usage.cache_creation_input_tokens_1h. It is already included in the total
// cache write tokens. prefix is "usage" for a top-level usage object or
// "message.usage" for the streaming message_start nested shape.
func parseCacheWriteTokens1h(data []byte, prefix string) int64 {
	v := gjson.GetBytes(data, prefix+".cache_creation.ephemeral_1h_input_tokens").Int()
	if v == 0 {
		v = gjson.GetBytes(data, prefix+".cache_creation_input_tokens_1h").Int()
	}
	return v
}

// parseOpenAIUsageFieldsWithPrefix extracts the OpenAI main-chain fields
// rooted at the given prefix ("usage" for chat completions bodies,
// "response.usage" for the Responses API). leafIn/leafOut name the
// prompt/completion token fields: "prompt_tokens"/"completion_tokens" for
// chat completions, "input_tokens"/"output_tokens" for the Responses API.
func parseOpenAIUsageFieldsWithPrefix(data []byte, prefix, leafIn, leafOut string) UsageFields {
	var fields UsageFields

	fields.UsedQuota = gjson.GetBytes(data, prefix+".total_tokens").Int()
	fields.PromptTokens = gjson.GetBytes(data, prefix+"."+leafIn).Int()
	fields.CompletionTokens = gjson.GetBytes(data, prefix+"."+leafOut).Int()
	fields.CacheReadTokens = gjson.GetBytes(data, prefix+".cache_read_tokens").Int()
	fields.CacheWriteTokens = gjson.GetBytes(data, prefix+".cache_write_tokens").Int()
	fields.CacheWriteTokens1h = parseCacheWriteTokens1h(data, prefix)
	fields.AudioInputTokens = gjson.GetBytes(data, prefix+".audio_input_tokens").Int()
	fields.AudioOutputTokens = gjson.GetBytes(data, prefix+".audio_output_tokens").Int()
	fields.ImageInputTokens = gjson.GetBytes(data, prefix+".input_tokens_details.image_tokens").Int()
	if fields.ImageInputTokens == 0 {
		fields.ImageInputTokens = gjson.GetBytes(data, prefix+".input_token_details.image_tokens").Int()
	}
	if fields.ImageInputTokens == 0 {
		fields.ImageInputTokens = gjson.GetBytes(data, prefix+".image_input_tokens").Int()
	}
	fields.ImageCount = gjson.GetBytes(data, prefix+".image_count").Int()
	if fields.ImageCount == 0 {
		fields.ImageCount = gjson.GetBytes(data, "data.#").Int()
	}
	fields.VideoCount = gjson.GetBytes(data, prefix+".video_count").Int()
	if fields.VideoCount == 0 {
		fields.VideoCount = gjson.GetBytes(data, "data.#").Int()
	}

	// DeepSeek fallback: prompt_cache_hit_tokens / prompt_tokens_details.cached_tokens
	if fields.CacheReadTokens == 0 {
		fields.CacheReadTokens = gjson.GetBytes(data, prefix+".prompt_cache_hit_tokens").Int()
	}
	if fields.CacheReadTokens == 0 {
		fields.CacheReadTokens = gjson.GetBytes(data, prefix+".prompt_tokens_details.cached_tokens").Int()
	}

	// Responses API cache-read: input_tokens_details.cached_tokens (some
	// relays flatten it as input_token_details.cached_tokens).
	if fields.CacheReadTokens == 0 {
		fields.CacheReadTokens = gjson.GetBytes(data, prefix+".input_tokens_details.cached_tokens").Int()
	}
	if fields.CacheReadTokens == 0 {
		fields.CacheReadTokens = gjson.GetBytes(data, prefix+".input_token_details.cached_tokens").Int()
	}

	return fields
}

// ParseOpenAIUsageFields extracts usage fields from OpenAI-family response
// bodies: the OpenAI chat main chain plus the DeepSeek and Responses API
// cache-read fallbacks, and the Responses API input/output_tokens chains
// (issue #1381). The Claude (Anthropic) fallback chain is NOT part of this
// extraction; it lives in ParseAnthropicUsageFields.
func ParseOpenAIUsageFields(data []byte) UsageFields {
	fields := parseOpenAIUsageFieldsWithPrefix(data, "usage", "prompt_tokens", "completion_tokens")

	// Responses API (issue #1381): the streaming response.completed event
	// nests the final usage under "response.usage"; the non-streaming
	// create-response object keeps usage at the top level but names the
	// fields input/output_tokens (gated on the Responses-API-specific
	// input_token_details so Anthropic bodies still fall through to
	// ParseAnthropicUsageFields, which owns their cache normalization).
	if fields.PromptTokens == 0 && fields.CompletionTokens == 0 {
		var resp UsageFields
		switch {
		case gjson.GetBytes(data, "response.usage").Exists():
			resp = parseOpenAIUsageFieldsWithPrefix(data, "response.usage", "input_tokens", "output_tokens")
		case gjson.GetBytes(data, "usage.input_tokens_details").Exists() ||
			gjson.GetBytes(data, "usage.input_token_details").Exists():
			resp = parseOpenAIUsageFieldsWithPrefix(data, "usage", "input_tokens", "output_tokens")
		}
		if resp.PromptTokens != 0 || resp.CompletionTokens != 0 {
			// OpenAI subset semantics (issue #1389): input_tokens already
			// includes cached_tokens (total_tokens = input_tokens +
			// output_tokens), so PromptTokens needs no normalization,
			// exactly like the Chat Completions main chain. Only the
			// Anthropic chain normalizes, its input_tokens excluding cache.
			fields = resp
		}
	}

	return fields
}

// ParseUsageFieldsCrossProtocol extracts usage fields by composing the
// protocol chains: the OpenAI-family chain first, then the Anthropic
// (Claude) chain when no OpenAI-style prompt/completion tokens are
// present, then the Gemini chain when still none are present. This mirrors
// the legacy all-chain extraction field-for-field and is
// response-format-agnostic: it recovers the full usage even when the auth
// style detected from the request does not match the format of the
// response body (issue #1364, e.g. a Bearer key detected as openai while
// the backend returns an Anthropic body). The Gemini fields are
// camelCase and never conflict with the OpenAI snake_case fields, but the
// chain is kept as an explicit third stage to preserve the cross-protocol
// mismatch fallback.
func ParseUsageFieldsCrossProtocol(data []byte) UsageFields {
	fields := ParseOpenAIUsageFields(data)
	if fields.PromptTokens == 0 && fields.CompletionTokens == 0 {
		claude := ParseAnthropicUsageFields(data)
		fields.PromptTokens = claude.PromptTokens
		fields.CompletionTokens = claude.CompletionTokens
		if fields.CacheReadTokens == 0 {
			fields.CacheReadTokens = claude.CacheReadTokens
		}
		if fields.CacheWriteTokens == 0 {
			fields.CacheWriteTokens = claude.CacheWriteTokens
		}
		if fields.CacheWriteTokens1h == 0 {
			fields.CacheWriteTokens1h = claude.CacheWriteTokens1h
		}
		if fields.UsedQuota == 0 {
			fields.UsedQuota = claude.UsedQuota
		}
	}
	if fields.PromptTokens == 0 && fields.CompletionTokens == 0 {
		gemini := ParseGeminiUsageFields(data)
		fields.PromptTokens = gemini.PromptTokens
		fields.CompletionTokens = gemini.CompletionTokens
		if fields.CacheReadTokens == 0 {
			fields.CacheReadTokens = gemini.CacheReadTokens
		}
		if fields.UsedQuota == 0 {
			fields.UsedQuota = gemini.UsedQuota
		}
	}
	return fields
}

// ParseAnthropicUsageFields extracts usage fields from Anthropic (Claude)
// response bodies: input_tokens / output_tokens /
// cache_read_input_tokens / cache_creation_input_tokens.
//
// Anthropic input_tokens only counts fresh (cache-missing) tokens and
// excludes cache read/write tokens. Normalize PromptTokens to the total
// input token count so downstream cost splitting (prompt - cacheRead -
// cacheWrite) works the same as the OpenAI/DeepSeek semantics.
//
// In streaming responses the initial usage is nested under message.usage
// (message_start); the final usage arrives in the top-level usage of the
// message_delta event. Both shapes are accepted.
func ParseAnthropicUsageFields(data []byte) UsageFields {
	var fields UsageFields

	prompt := gjson.GetBytes(data, "usage.input_tokens")
	completion := gjson.GetBytes(data, "usage.output_tokens")
	if !prompt.Exists() && !completion.Exists() {
		// streaming message_start nests usage under message.usage
		prompt = gjson.GetBytes(data, "message.usage.input_tokens")
		completion = gjson.GetBytes(data, "message.usage.output_tokens")
	}
	fields.PromptTokens = prompt.Int()
	fields.CompletionTokens = completion.Int()
	fields.CacheReadTokens = gjson.GetBytes(data, "usage.cache_read_input_tokens").Int()
	if fields.CacheReadTokens == 0 {
		fields.CacheReadTokens = gjson.GetBytes(data, "message.usage.cache_read_input_tokens").Int()
	}
	fields.CacheWriteTokens = gjson.GetBytes(data, "usage.cache_creation_input_tokens").Int()
	if fields.CacheWriteTokens == 0 {
		fields.CacheWriteTokens = gjson.GetBytes(data, "message.usage.cache_creation_input_tokens").Int()
	}
	fields.CacheWriteTokens1h = parseCacheWriteTokens1h(data, "usage")
	if fields.CacheWriteTokens1h == 0 {
		fields.CacheWriteTokens1h = parseCacheWriteTokens1h(data, "message.usage")
	}
	fields.PromptTokens += fields.CacheReadTokens + fields.CacheWriteTokens
	fields.UsedQuota = gjson.GetBytes(data, "usage.total_tokens").Int()
	if fields.UsedQuota == 0 {
		fields.UsedQuota = gjson.GetBytes(data, "message.usage.total_tokens").Int()
	}
	if fields.UsedQuota == 0 {
		fields.UsedQuota = fields.PromptTokens + fields.CompletionTokens
	}

	return fields
}

// ParseGeminiUsageFields extracts usage fields from Gemini response
// bodies. The usage block is the top-level "usageMetadata" object with
// camelCase fields:
//
//	promptTokenCount          -> PromptTokens (already includes the cached content tokens)
//	candidatesTokenCount      -> CompletionTokens
//	cachedContentTokenCount   -> CacheReadTokens (a subset of PromptTokens)
//	totalTokenCount           -> UsedQuota
//
// When totalTokenCount is absent, UsedQuota falls back to
// promptTokenCount + candidatesTokenCount. In streaming responses every
// chunk carries the accumulated usageMetadata, so callers must consume
// the last usage-bearing chunk (see the gemini adapter's
// IsFinalUsageEvent).
func ParseGeminiUsageFields(data []byte) UsageFields {
	var fields UsageFields

	fields.PromptTokens = gjson.GetBytes(data, "usageMetadata.promptTokenCount").Int()
	fields.CompletionTokens = gjson.GetBytes(data, "usageMetadata.candidatesTokenCount").Int()
	fields.CacheReadTokens = gjson.GetBytes(data, "usageMetadata.cachedContentTokenCount").Int()
	fields.UsedQuota = gjson.GetBytes(data, "usageMetadata.totalTokenCount").Int()
	if fields.UsedQuota == 0 {
		fields.UsedQuota = fields.PromptTokens + fields.CompletionTokens
	}

	return fields
}

// EstimateContentToken estimates the token count of a response body chunk.
// It assumes roughly 4 bytes per token. Moved verbatim from
// bfe_modules/mod_body_process/llm_util.go.
func EstimateContentToken(val string) int64 {
	return int64(len(val)) / 4
}

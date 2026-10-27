<!--
This changelog should always be read on `master` branch. Its contents on other branches
does not necessarily reflect the changes.
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Add `Server.AIUpstreamAcceptEncoding` (`auto`/`identity`/`off`, default `auto`): how `Accept-Encoding` is rewritten on an AI upstream request, so content negotiation stays inside the coding set the gateway can decode (`gzip`, `deflate`). `auto` keeps only the decodable codings of the client's list (q values preserved, an empty intersection is left untouched, `identity;q=0` is honoured), `identity` strips compression for diagnostics, `off` keeps the previous behaviour. The rewrite only touches the outbound header: the inbound request, the access log, `mod_ai_cache` and `mod_compress` still see the client value (issue #1468)
- Add `bfe_util.NewContentDecoder` / `CanDecodeContentEncoding` / `SupportedContentEncodings`, a streaming `Content-Encoding` decoder covering `gzip`/`x-gzip` and `deflate` (zlib header sniffed, raw DEFLATE accepted). `DecodeContentEncoding` now delegates to it, so the non-streaming parse-only copy also understands `deflate` (issue #1468)
- Add the `ResStreamCompressed` / `ResStreamDecoded` / `ResStreamDecodeSkipped` / `ResStreamRawOnly` `mod_body_process` counters: how many responses carried a `Content-Encoding`, how many got the decompress-for-parsing tap, how many could not be decoded and how many degraded mid-stream. The last two mean "this request is billed by estimation", i.e. the only signal that an upstream ignored the narrowed `Accept-Encoding` (issue #1468)

### Changed
- An AI upstream request whose client `Accept-Encoding` contains codings the gateway cannot decode no longer receives them (`Server.AIUpstreamAcceptEncoding = auto`, the default): the upstream is offered `gzip`/`deflate` only, so a client that asked for `zstd` may now receive `gzip` instead (still inside its own `Accept-Encoding`, <1% size difference). Set `Server.AIUpstreamAcceptEncoding = off` to keep the previous negotiation (issue #1468)

### Fixed
- Parse the usage of a compressed streaming (SSE) response instead of estimating it: `mod_body_process` bypassed any `Content-Encoding` body to a byte-wise pass-through, so `cache_read_tokens` stayed 0, `prompt_tokens` fell back to the request-body estimate and `completion_tokens` to the compressed-bytes estimate, and the whole prompt was billed at the input price — measured on the field at a 96% cache-hit rate that overcharges by roughly 46x and writes `ai_usage_source=estimated`. Compressed SSE now goes through `CompressedStreamDecoder`, which tees the source into a decompressor used only for parsing while the original compressed bytes are forwarded verbatim together with the `Content-Encoding` header; a decompression failure or an unsupported coding degrades to a byte-exact pass-through plus the estimate, visibly (issue #1468)
- Report a skipped `stream_options.include_usage` injection when the request body exceeds `Server.AccessibleBodySize`: the injection used to fail silently, which raised the probability of an upstream returning no usage at all (issue #1468)

## [v1.8.9] - 2026-10-24

### Added
- Add `ai_zero_cost_reason` access-log marker (requires `bfe-access-pb >= v0.3.16`): why a 2xx request holding an RMB quota plan was charged 0, together with the `BillingZeroCost` and `AbortBillingSkipped` module counters (issue #1414). Requests that never became a model call (batch/files management endpoints) and AI cache hits are excluded from `BillingZeroCost`, since their zero cost is by design rather than an anomaly
- Support condition primitive `time_weekday_in(weekday_list, zone)`: judge whether the weekday of the current time is in the given list (`0`=Sunday, `1`=Monday, …, `6`=Saturday, the same convention as the `weekdays` field of the cluster pricing tiers). `zone` is a single military time zone letter (see `bfe_util.TimeZoneMap`, e.g. `H` for UTC+8) and defaults to `Z` (UTC) when omitted — the weekday depends on the time zone, so deployments in UTC+8 should pass `"H"`. `weekday_list` is a `|`-separated enumeration (no range notation), validated at config load time; the current time is read from the system clock, or from the `X-Bfe-Debug-Time` header when present
- Add `mod_ai_batch`, an OpenAI Batch API (`/v1/files`, `/v1/batches`) passthrough module: dedicated file/batch request modes so batch traffic stops falling into the default chat billing mode, operation classification, upload line/byte counting with large-file streaming (no retry buffering), batch-level key affinity bindings (SETNX claim-first, 404 delete plus penalty failover, create chains bound via `input_file_id` parsed from the buffered body before key selection), post-paid quota (reserve at create with balance pre-check, settle per-model usage at batch prices on result-file download, release on cancel/terminal status, with idempotent `BATCH_RESERVE` / `BATCH_SETTLED` Redis markers), batch rate-limit dimensions (`batch_limits`: create RPM, active batches, file bytes/lines; batch-only policies are now valid) and access-log batch fields (bfe-access-pb >= v0.3.13, fields 901-907) with ownership checks on file download (404 / `allow_log` policy). Requires the new `mod_ai_batch` module and its `mod_ai_batch.data` config
- Add `mod_ai_cache`, an exact-match Redis AI cache: rule matching at `HandleFoundProduct`, before route rule evaluation, so a cache hit short-circuits routing; skip header `x-bfe-skip-ai-cache`, question extraction (last/all questions), tenant-isolated keys, template-based hit responses (JSON and SSE with `[DONE]`), response capture with a complete flag (aborted responses are never cached), size limits and fail-open Redis access; cache hits skip RMB deduction and log `ai_cache_status` / `ai_cache_key` (bfe-access-pb >= v0.3.7, fields 789-790)
- Extend `mod_ai_cache` with a semantic cache phase: OpenAI-compatible embeddings client plus Chroma vector lookup (cosine space, tenant- and TTL-enforced queries), hybrid exact-first / semantic-fallback with dual write-back, per-rule `enableSemanticCache` (hot-reloadable, backward compatible with phase-1 rule files); a provider init failure degrades semantic caching globally with a WARN and never fails startup. Adds the `hit_semantic` status, `ai_cache_semantic` / `ai_cache_similarity` log fields (bfe-access-pb >= v0.3.10, fields 791-792) and `SEMANTIC_HIT` / `EMBEDDING_ERR` / `VECTOR_ERR` / `SEMANTIC_SKIPPED` counters; compatible with the Chroma 0.6.x UUID-only collections API
- Add `mod_ai_intent`, a semantic-routing intent module: lazy intent resolve triggered by condition evaluation (zero overhead for rules without intent conditions), System One decision-service client (Laya/Kev/Jev interchangeable via config) with timeout and circuit breaker, hot-reloadable `intent_questions.data` (question set with choice/score and per-question/global `MinConfidence`), explicit `X-AI-Intent` header precedence, in-process LRU+TTL cache, and the new `req_ai_intent_in(String, String [, Float])` condition primitive; the consumed intent is logged in `ai_intent_*` access-log fields (bfe-access-pb >= v0.3.9, fields 803-809) from resolved-intent context only, never triggering a resolve. An empty question set disables intent classification
- Add `mod_ai_context`, an AI context compression/pruning module (phase 1, OpenAI chat format, fail-open): a lossless trim layer (tool-result truncation, latest-N inline images, thinking-block removal) plus a rule-based rewrite layer with tombstone preservation and a fidelity gate, registered at `HandleAfterAITargetModel` so the token budget is computed against the resolved target model and cache hits never pay for compression; adds access-log fields 793-796 (bfe-access-pb >= v0.3.11)
- Add `mod_traffic_mirror`, a traffic mirroring module for AI gateway shadow traffic: matched AI requests (method/path/headers/body, with model rewrite, path rewrite and header sanitize rules) are snapshotted at `HandleForward` and mirrored asynchronously to a shadow cluster via the new side-effect-free `BalanceGslb.PickBackend()`; the async side drains SSE responses to `[DONE]` while parsing usage/error/finish_reason and reports via a module-private Prometheus registry — the main request is never blocked, retried or double-mirrored on AI fallback (once-marker). Adds `mirror_hit` / `mirror_cluster` access-log fields (bfe-access-pb >= v0.3.8, fields 842-843); mirrored traffic is billing-isolated
- Add upstream error normalization behind `AIConf.NormalizeUpstreamError` (`Enabled` / `StreamEnabled` / `UnrecognizedAction` / `MaxBodyBytes` / `RedactSecrets`): per-protocol `ErrorParser` / `StreamErrorParser` adapters (openai/anthropic/gemini) map upstream errors into a unified `UPSTREAM_*` catalog; the interception point normalizes upstream failures after the fallback loop, with an SSE error-event filter, truncation detection and credential redaction covering raw/base64/URL/JSON-escaped forms; gateway-generated responses are skipped via the `X-Bfe-Gw-Error` marker. Adds `ai_upstream_status` / `ai_upstream_err_code` / `ai_err_normalized` / `ai_err_normalize_miss` / `ai_stream_error_rewritten` / `ai_stream_truncated` access-log fields (bfe-access-pb >= v0.3.12, fields 810-815); see `ai_error_codes.md` §2.5
- Add field-level `enc$v1$` encryption at rest for distributed config files: sensitive fields in `token_rule.data` (Tokens outer keys) and `cluster_conf.data` (`AIConf.Keys[].Key`) may carry `enc$v1$` AES-256-GCM ciphertext produced by the control plane at export, decrypted in memory at load time via a keyring configured in the new `bfe.conf` `[Security]` section (multiple keyIDs coexist, fail-closed parsing, error messages never carry ciphertext); hot reload re-reads the keyring on every data-file load, an unreadable or wrong keyring fails the reload and keeps the old config (fail-fast on first startup); plaintext without the marker passes through, keeping canary/rollback compatible
- Make the "usage unrecognized and not estimatable" billing bypass observable (issue #1410): the request-finish guard keeps its yardstick behavior (billing fields zeroed, no RMB deduction) but now increments the `UsageUnrecognized` module counter (exported via monitorHandlers, alertable) and logs a warn with cluster/key_id/model and the `response_completed`/`allow_estimate` sub-criteria. The access log gains `ai_usage_source` (bfe-access-pb >= v0.3.15, field 764) distinguishing `parsed` (final usage from the upstream response) / `estimated` (approved EstimateToken fallback) / `unrecognized` (billing skipped), orthogonal to the non-2xx suppression, so billing bypasses become directly queryable instead of requiring out-of-band reconciliation

### Changed
- `X-Bfe-Log-Id` and the access-log `logid` are now a decimal string denoting a value in `[1, 2^63-1]` instead of a 32-char hex string, so the id converts losslessly into the `uint64` `logid` field of the access log; the 63-bit bound keeps it inside the signed `BIGINT` of the report databases. Any consumer that parses or joins on the 32-hex form (`X-Bfe-Log-Id`, the text access log `log_id`, `%bfe_log_id`, WAF/mirroring logs) must be updated (bfenetworks/bfe#1417)
- Sessions/requests whose log id cannot be converted (mod_logid not loaded, malformed value, or a legacy 32-char hex header during a rolling upgrade) now carry a locally regenerated non-zero id and are counted by the new `LogidRegenerated` / `SessionIdRegenerated` module counters, instead of silently writing 0 (bfenetworks/bfe#1417)
- AI key session affinity now uses a server-owned Redis connection configured via the new `[AIKeyAffinity]` section in `bfe.conf`, instead of implicitly borrowing the `mod_ai_rate_limit` module's Redis. **Config migration required**: deployments that enabled affinity (`AIConf.KeyPolicy.SessionAffinity=true`) must add the `[AIKeyAffinity]` section (with `Disabled=false` and the Redis `ServiceConf`) before upgrading; otherwise affinity silently turns off after the upgrade (fail-open, no error, bindings lost)
- `Server.EstimateToken` now applies only when the upstream returned **no usage at all**, decided by the presence of the usage object: a response whose usage object reports all zeros is billed at 0 instead of being estimated, and is reported as `ai_usage_source=parsed`. Only that edge case changes amount (downward); requests whose response carries no usage keep the previous estimate behaviour (issue #1414)
- `ai_cost_value` is now written for every observed 2xx row, 0 included, so "charged nothing" is distinguishable from "field absent" (non-2xx / never billed). Reconciliation that tests `ai_cost_value IS NULL` for "no cost" should switch to `ai_cost_value = 0` / `IS NULL` (issue #1414)
- `mod_ai_token_auth.UsageUnrecognized` no longer counts client aborts — they are counted by the new `AbortBillingSkipped` counter, so the two causes (upstream/parsing problem vs client-side disconnect) stay separable; existing thresholds on `UsageUnrecognized` need to be realigned. `ai_usage_source` gained the fourth value `aborted` (no new protobuf field: the value is added to the existing `ai_usage_source`) (issue #1414)

### Fixed
- Populate the access-log `logid` with the request's real log id: `strconv.ParseUint(req.LogId, 10, 64)` parsed the hex log id as decimal and discarded the error, so `logid` was written as 0 for every request. Together with the report table's second-granularity dedup key and its full-row `ON DUPLICATE KEY UPDATE`, one API key and model firing n concurrent requests inside one second kept only 1 row — the measured loss was exactly (n-1)/n, 13.5% of a production hour. The session id column (`session_id`, and the session log's `logid`) had the same defect (bfenetworks/bfe#1417)
- Make usage recognition presence-based instead of value-based: an upstream that explicitly reports all-zero usage is no longer misclassified as "usage unrecognized" nor over-billed by `EstimateToken` (issue #1414)
- Report every zero-cost 2xx request that holds an RMB plan through the new `BillingZeroCost` counter and `ai_zero_cost_reason` marker, covering unrecognized usage, client aborts, batch releases, price-table gaps and pricing configuration loss at deduction time (issue #1414)
- Count failed quota deductions (`DeductFailed`) instead of leaving them as a log line only (issue #1414)
- Fix the token-triple consistency check (`AiUsageInconsistent`) returning early on `ai_total_tokens == 0`, which disabled it for exactly the `input > 0 while total == 0` rows it exists to catch (issue #1414)
- Batch settlement now marks price misses (`ai_price_miss`), warns when the model table is missing and records a release without billing instead of staying silent (issue #1414)
- Measure the batch single-file limits (`batch_limits.max_file_bytes` / `max_file_lines`) on the multipart `file` part content instead of the request envelope: a 1-line/203-byte upload was rejected as `BATCH_FILE_TOO_LARGE` because the envelope (boundaries, part headers, `purpose` field) read as 11 lines / 509 bytes, and even a pre-accepted upload was aborted mid-stream by the same envelope count. The per-policy `Content-Length` pre-check now applies to raw uploads only, a pre-checked multipart upload no longer carries the streaming gate, and the gate parses the multipart envelope incrementally so only the `file` part is measured (also making chunked upload bindings exact instead of approximate) (bfenetworks/bfe#1434)
- Fix zeroed token usage in access logs and billing for requests without a final usage event (issue #1398): the request-finish reset now applies to a billing copy only, so the shared `TokenUsage` (read by mod_access_pb3) keeps observed/estimated values instead of silently logging `total_tokens=0`; final usage events with zero output tokens (input-only / pure cache-hit responses) are recognized instead of being dropped into the reset/estimate path; `Server.InjectStreamUsage` (default on) injects `stream_options.include_usage=true` into OpenAI streaming chat-completion requests when the client did not set it, so the upstream returns the real final usage chunk
- Parse the final usage embedded in the streaming finish chunk's choice (`choices[N].usage`, cache field `cached_tokens`) used by kimi-for-coding and other OpenAI-compatible upstreams that ignore the injected `stream_options.include_usage` (issue #1401): the real usage is no longer demoted to a guess, so `EstimateToken=true` no longer logs/bills a forged ContentLength/4 estimate and `EstimateToken=false` no longer logs `(0, -1, 0)` and bills 0; the negative `CompletionTokens` sentinel (auth-time "unknown" marker) is clamped to 0 at the access log boundary. `EstimateToken` and `InjectStreamUsage` now default to true as program defaults (`ConfigBasic.SetDefaultConf`), so the fixes no longer silently disappear when a binary is upgraded without the new conf keys
- Fix token usage collection for non-streaming gzip responses (`Content-Encoding: gzip` with `Transfer-Encoding: chunked`) (issue #1406): the compressed body is now decompressed on a parse-only copy (forwarding bytes untouched), the whole-body parse gate accepts chunked non-SSE JSON bodies, and the content-type decoder tolerates media type parameters (e.g. `application/json; charset=utf-8`), so `ai_output_tokens` / `ai_total_tokens` / `ai_cache_read_tokens` and RMB billing reflect the real usage instead of 0
- Fall back to the model's chat-mode price when the `(model, mode)` price entry is missing but a chat entry exists (product decision: an unpriced mode bills the same as chat), and mark such requests with the `ai_price_miss` access-log field (bfe-access-pb >= v0.3.14, field 763) counted by `PriceFallbackUsed`, so under-priced rows stay visible in reconciliation (issue #1408)
- Fix token-priced image_generation/video_generation rules billing 0 (issue #1420): the billing formula was picked by request mode alone, so an entry whose prices only carry token unit keys was billed with the per-image/per-video formula and skipped (costUnits > 0 gate). The billing method is now entry-driven — `calcImageGenerationCost` / `calcVideoGenerationCost` fall back to the token formula when the matched entry lacks the per-unit price key; discrimination is key presence, not value, so a mixed entry bills per unit and an explicit `output_cost_per_image = 0` stays per-image billing at zero cost
- Drop the EstimateToken residue (auth-time prompt seeding / response-byte completion guessing) from the shared `TokenUsage` and suppress the token fields in the access log for non-2xx responses (new usage disposition, `UsageDispositionNone`): a failed request no longer logs request-body/4 as `ai_input_tokens` while `ai_total_tokens` stays 0 (issue #1409)
- Bill streaming (SSE) responses that end at a clean HTTP EOF without the protocol termination event (`[DONE]` / `message_stop`) and without any usage, instead of silently deducting 0 (issue #1415): the response is marked completed once the body has been fully delivered (200 SSE, successful copy) — a chunked response only EOFs at its terminating chunk, so client aborts and mid-stream upstream drops still surface as copy errors and never satisfy the billing gate
- Keep the client connection reusable after internally constructed AI responses (issue #1416): the no-route 404 (`AI route not found`), the `findProduct` 500, the protocol-mismatch rejection, the `HandleAfterAITargetModel` rejection and the no-cluster 500 no longer set the server-level `closeAfterReply` — BFE was closing the connection without advertising `Connection: close`, so the next request on a pooled keep-alive connection failed with EOF; module-contract finish/close paths and the protocol-level close shapes (HTTP/1.0, `Connection: close`, undrainable bodies) still close with advertisement
- Return a decidable synthetic 429 when no AI key can be selected for the target cluster (all keys penalized / `Weight=0` / dead) instead of `(res=nil, err=nil)` (issue #1440): `aiClusterInvoke` now surfaces the result so the fallback decision can act on it — a configured fallback cluster is actually tried, and without one the client gets a retryable unified-error 429 instead of an opaque empty-body 500; `shouldTriggerFallback` treats `(nil, nil)` as fallback-eligible as a hard invariant
- Exclude `Weight=0` keys from the session-affinity penalty filter and replace the "fallback to the original list" branch with a circuit breaker (issue #1439): when every usable key is penalized the filter returns no candidates — key selection fails, the synthetic 429 triggers cluster fallback, and the cluster makes zero upstream interaction until the penalty TTL expires (new 429s extend the window). Previously one upstream 429 against the only usable key was amplified into a 60s cluster-wide outage by a `Weight=0` sibling key
- Emit the unified error contract on gateway-synthesized AI failures (issue #1441): the key-exhaustion 429 now carries `error.code = NO_AVAILABLE_KEY` (429, `rate_limit_error`), `error.details.retry_after_seconds` (equal to the 429 penalty TTL, single-sourced) and a standard `Retry-After` header, and sets `err_code = NO_AVAILABLE_KEY` for access-log attribution without suppressing cluster fallback; gateway-side failures in `ServeHTTPForAI` (findProduct failure, no cluster, backend connect/transport errors) return `BACKEND_UNAVAILABLE` (502) with the unified body instead of a bare empty-body 500, and client-abort paths keep the legacy empty-body record. Gateway-generated error responses now also carry request correlation identifiers: `X-Bfe-Log-Id` is always written (fallback chain request logid → request header → generated, with the latter two written back so the access-log logid stays same-source) and a client-supplied `X-Request-ID` is echoed back, so clients can correlate an error response with the access log and the key selection/penalty trail
- Make the outbound credential headers mutually exclusive, so a client-supplied credential can no longer override the gateway-injected upstream key (issue #1447): once BFE takes ownership of the upstream credential, the outbound request keeps exactly the one credential header the request's `model_protocol` adapter injects and drops the others (`Authorization` / `x-api-key` / `x-goog-api-key`, the sanitize set derived from the adapter registry via the new `ProtocolAdapter.CredentialHeader()`); the deletion runs on a cloned header map so the inbound client credential stays available to authentication, billing, access-log masking and the key-affinity session id. Previously `InjectAuth` only overwrote its own header, so a client sending both `Authorization` and `x-api-key` left its gateway-level `x-api-key` on the outbound request — kimi authenticated with the client token, answered 401, and that 401 was charged to the selected key: two requests marked both kimi keys dead for 3600s and every user was silently degraded to the fallback cluster while the client still saw 200. Clusters without configured keys (BYOK passthrough) are unaffected
- Release old backends when a subcluster disappears from `cluster_table` on reload (issue #1436): a K8s instance pool scaled to 0 exports an empty cluster entry, and the reload previously skipped the missing subcluster and kept the stale backends, so requests got HTTP 200 against released instances instead of the required 500 `BK_NO_BACKEND`; the empty pool is now loaded with an empty backend list (releasing old instances), and a subcluster with no `weight>0` backends no longer fails the whole table load (warning + accept, the empty pool is covered by alerting)
- Keep `ai_apikeytags` in the access log for rejected requests (issue #1418): quota pre-check rejections (429 `QUOTA_EXHAUSTED`) and the other post-lookup rejection paths (`KEY_DISABLED` / `KEY_EXPIRED` / `QUOTA_EXPIRED` / `INTERNAL_QUOTA_ERROR` / `SUBNET_NOT_ALLOWED`) returned before `SetTokenAuthContext`, the only writer of `AiBasicInfo.ApikeyTags`, so the rejected log carried the key id but missed the tags and lost entity attribution; `ApikeyTags` is now written together with `ClientKeyId` right after the token is found
- Rebuild the EPP runtime on hot reload when non-scheduling parameters change (issue #218): `EPPTLS`, `EPPTimeout.Connect`, `EPPCheck.Disabled` and the EPP address list now form a runtime signature — a changed signature rebuilds the runtime (reusing the graceful retire path and inheriting active/health state when the address table is unchanged), an unchanged one updates in place without resetting the circuit-breaker sliding window; the `EPPTLS` default flips from `{Insecure: true}` (TLS with verification skipped) to `{Plaintext: true}` (plaintext dial), while explicit `{"Insecure": true}` / `{"CAFile": ...}` still use TLS
- Fix batch settlement and terminal-state persistence for completed passthrough batches (rainway-ai-gateway/ai-gateway-api#254): batch result rows nest `model`/`usage` under `response.body` (OpenAI contract), which the usage scanner now parses, so downloading the output file actually settles per-model usage at batch prices (`BATCH_SETTLED` idempotent claim, and `SETTLE_USAGE_PARSE_ERR` is now counted instead of silent); the finish handlers persist the terminal facts the control plane needs (`reserved_units`, `created_at`, `endpoint`, `request_counts`, `settle_units`, `settle_status`), and the recorded output-file size describes the file rather than the HTTP envelope
- Skip the request-body model rewrite for batch/file passthrough traffic (bfenetworks/bfe#1426): with a non-empty AI route target model, the rewrite replaced the whole multipart body of `POST /v1/files` with `{"model":...}` (providers failed multipart parsing with `400 multipart: NextPart: EOF`, breaking the batch upload/create chain) and injected a model field into `POST /v1/batches`; the rewrite now skips `ModeFile`/`ModeBatch` traffic and any request whose `Content-Type` explicitly says non-JSON (missing/malformed headers and `*/*+json` still allow it)
- Apply the global `batch_limits` file byte/line hard caps to API keys bound to no rate-limit policy: the limit resolution previously ran only on the policy-bound path, so an unbound key bypassed the global hard top (a 1.5MB upload passed under a 1MB cap); the unbound branch now resolves the global hard top only, matching the "policy without batch section ⇒ global hard cap only" design

## [v1.8.8] - 2026-09-24

### Added
- Add per-protocol upstream path rewrite via `AIConf.ProtocolPaths` ([Pull #1373](https://github.com/bfenetworks/bfe/pull/1373))
- Add plaintext dial option for the EPP client and formalize the `EPPTLS` default ([Pull #1376](https://github.com/bfenetworks/bfe/pull/1376))

### Changed
- Improve WRR/WLC load balancing policies ([Pull #1375](https://github.com/bfenetworks/bfe/pull/1375))

### Fixed
- Fix `proxy_delay_time` uint32 wraparound in access logs for requests that never reached a backend (401/404/redirect/close), which corrupted downstream MySQL batch inserts ([Pull #1390](https://github.com/bfenetworks/bfe/pull/1390))
- Fix duplicate billing of Responses API `cached_tokens` by restoring subset semantics (`input_tokens` already includes cached tokens) ([Pull #1388](https://github.com/bfenetworks/bfe/pull/1388))
- Validate API-Key allowed/blocked models against the post-routing target model, so model redirection composes with model whitelists ([Pull #1385](https://github.com/bfenetworks/bfe/pull/1385))
- Match AI rate limit policies by post-routing target model via the new `HandleAfterAITargetModel` callback point ([Pull #1385](https://github.com/bfenetworks/bfe/pull/1385))
- Fix token model whitelist check for native Gemini path models (no more false 400 rejects) ([Pull #1383](https://github.com/bfenetworks/bfe/pull/1383))
- Fix OpenAI provider path rewrite for client entry paths without the `/v1` prefix ([Pull #1380](https://github.com/bfenetworks/bfe/pull/1380))
- Fix AI request mode detection for client entry paths without the `/v1` prefix, with the endpoint table shared in `bfe_basic` ([Pull #1380](https://github.com/bfenetworks/bfe/pull/1380))
- Fix provider-native `/compatible-mode/v1/responses` misdetected as Chat mode, causing price miss ([Pull #1380](https://github.com/bfenetworks/bfe/pull/1380))
- Fix unrecognized Responses API `response.completed` events that left Codex streaming requests unbilled ([Pull #1380](https://github.com/bfenetworks/bfe/pull/1380))
- Recognize Anthropic `message_delta` as final usage in the openai adapter, fixing lost usage on cross-protocol streams ([Pull #1371](https://github.com/bfenetworks/bfe/pull/1371))


## [v1.8.7] - 2026-09-13

### Added
- Support Gemini protocol with adapter-based usage extraction and a three-stage cross-protocol usage fallback chain ([Pull #1366](https://github.com/bfenetworks/bfe/pull/1366))
- Support responses/video_generation modes, image input token and output_cost_per_video billing ([Pull #1346](https://github.com/bfenetworks/bfe/pull/1346))
- Support length-tier pricing (`input/output_cost_per_token_above_{200k,256k,272k,512k}_tokens`) and 1h-TTL cache write cost ([Pull #1365](https://github.com/bfenetworks/bfe/pull/1365))
- Add `ai_cache_write_1h_tokens`, `ai_image_input_tokens` and `ai_video_count` access log fields ([Pull #1365](https://github.com/bfenetworks/bfe/pull/1365), [Pull #1346](https://github.com/bfenetworks/bfe/pull/1346))
- Production-harden BFE/EPP ext-proc integration: inference-pool metadata, primary/backup EPP addresses with gRPC health check, failover hysteresis and circuit breaker, TLS/timeouts config, `/monitor/epp_metrics` observability ([Pull #1358](https://github.com/bfenetworks/bfe/pull/1358))

### Changed
- Converge AI protocol handling (auth injection, usage extraction, error normalization) into the `bfe_model_protocol` adapter layer ([Pull #1350](https://github.com/bfenetworks/bfe/pull/1350))
- Keep model prices as float64 and convert per billing item to preserve 10-12 decimal price precision ([Pull #1360](https://github.com/bfenetworks/bfe/pull/1360))
- Allow zero total_token quota plans and treat a missing Redis balance as exhausted ([Pull #1353](https://github.com/bfenetworks/bfe/pull/1353))
- Add tzdata package to Dockerfile ([Pull #1347](https://github.com/bfenetworks/bfe/pull/1347))

### Fixed
- Fix fresh input token billing for Anthropic high cache hit ([Pull #1350](https://github.com/bfenetworks/bfe/pull/1350))
- Fix three billing issues: cacheRead truncation, skip /count_tokens endpoint, duplicate deduction guard ([Pull #1346](https://github.com/bfenetworks/bfe/pull/1346))
- Skip full-request estimation billing on client abort ([Pull #1351](https://github.com/bfenetworks/bfe/pull/1351))
- Bill complete usage for cross-protocol non-streaming responses ([Pull #1362](https://github.com/bfenetworks/bfe/pull/1362))
- Mask raw API Key in access log (authorization header, ai_route_rule_hits rule owner, ai_auth_hit_quota_plans) ([Pull #1356](https://github.com/bfenetworks/bfe/pull/1356))
- Relocate client CA/CRL base dirs on tls_conf reload with path ([Pull #1355](https://github.com/bfenetworks/bfe/pull/1355))
- Attach HttpResponse for internally-constructed responses to fix panic when mod_header rewrites them ([Pull #1358](https://github.com/bfenetworks/bfe/pull/1358))
- Send :path/:method pseudo-headers in EPP ext_proc request so chat-completions payloads parse correctly ([Pull #1365](https://github.com/bfenetworks/bfe/pull/1365))
- Make joinPath separator-aware on Windows ([Pull #1360](https://github.com/bfenetworks/bfe/pull/1360))


## [v1.8.6] - 2026-08-29

### Added
- Support Anthropic Claude protocol forwarding with model protocol matching ([Pull #1331](https://github.com/bfenetworks/bfe/pull/1331))
- Support RMB tiered pricing with peak/off-peak tiers ([Pull #1335](https://github.com/bfenetworks/bfe/pull/1335))
- Support cache token billing for AI requests ([Pull #1330](https://github.com/bfenetworks/bfe/pull/1330))
- Support audio token RMB billing ([Pull #1331](https://github.com/bfenetworks/bfe/pull/1331))
- Support image generation per-image billing ([Pull #1331](https://github.com/bfenetworks/bfe/pull/1331))
- Recognize DeepSeek cache fields in usage parsing ([Pull #1337](https://github.com/bfenetworks/bfe/pull/1337))
- Add session-level API-Key affinity within the same cluster ([Pull #1337](https://github.com/bfenetworks/bfe/pull/1337))
- Add `req_body_larger_than` and `req_body_less_than` condition primitives ([Pull #1336](https://github.com/bfenetworks/bfe/pull/1336))
- Add `GetInt64Batch` to redis_client for batch quota remaining reads ([Pull #1337](https://github.com/bfenetworks/bfe/pull/1337))
- Use control-plane generated redis_key for AI rate limit counters ([Pull #1331](https://github.com/bfenetworks/bfe/pull/1331))
- Support full Redis key cleanup exported by rate-limit export ([Pull #1333](https://github.com/bfenetworks/bfe/pull/1333))
- Add BFE AI gateway error codes documentation ([Pull #1339](https://github.com/bfenetworks/bfe/pull/1339))

### Changed
- Change session-level API-Key affinity TTL to idle timeout with default 600s ([Pull #1337](https://github.com/bfenetworks/bfe/pull/1337))
- Simplify token_rule.data fields and add TagLevel in mod_ai_token_auth ([Pull #1329](https://github.com/bfenetworks/bfe/pull/1329))

### Fixed
- Fix model modification when retry between clusters in ServeHTTPForAI ([Pull #1334](https://github.com/bfenetworks/bfe/pull/1334))
- Fix PriceMap/TierPriceMap JSON serialization to preserve 8-decimal precision ([Pull #1338](https://github.com/bfenetworks/bfe/pull/1338))


## [v1.8.5] - 2026-08-21

### Added
- Add multi-API-Key support for AI backend clusters ([Pull #1314](https://github.com/bfenetworks/bfe/pull/1314))
- Add RMB quota support in mod_ai_token_auth ([Pull #1314](https://github.com/bfenetworks/bfe/pull/1314))
- Add key_id field for API-Key token in access logs ([Pull #1318](https://github.com/bfenetworks/bfe/pull/1318))
- Add provider/model prefix routing support ([Pull #1315](https://github.com/bfenetworks/bfe/pull/1315))
- Add req_body_json_prefix_in condition primitive ([Pull #1314](https://github.com/bfenetworks/bfe/pull/1314))
- Adapt bfe-access-pb v0.2.0 AI observability fields in access logs ([Pull #1321](https://github.com/bfenetworks/bfe/pull/1321))
- Support body rewind when fallback ([Pull #1313](https://github.com/bfenetworks/bfe/pull/1313))
- Add AccessibleBodySize config in bfe.conf ([Pull #1313](https://github.com/bfenetworks/bfe/pull/1313))
- Monitor and limit TotalBodyBufferSize ([Pull #1313](https://github.com/bfenetworks/bfe/pull/1313))
- Add release target to Makefile

### Changed
- Trigger cluster-level fallback on 4xx status codes ([Pull #1318](https://github.com/bfenetworks/bfe/pull/1318))
- Merge multiple AI model body rewrites into one ([Pull #1321](https://github.com/bfenetworks/bfe/pull/1321))

### Fixed
- Fix ai token auth default rule path ([Pull #1251](https://github.com/bfenetworks/bfe/pull/1251))
- Fix RMB cost calculation at request finish for streaming responses ([Pull #1318](https://github.com/bfenetworks/bfe/pull/1318))
- Fix log messages ([Pull #1313](https://github.com/bfenetworks/bfe/pull/1313))


## [v1.8.4] - 2026-08-05

### Added
- Add mod_ai_route module for AI gateway routing
- Add EstimateToken switch ([Pull #1285](https://github.com/bfenetworks/bfe/pull/1285))

### Fixed
- Fix token calculation
- Fix allow_models check in mod_ai_token_auth
- Fix redisdata unmarshal bug in mod_session_sticky
- Fix "unsafe legacy renegotiation disabled" error when connecting from OpenSSL 3.4+ ([Pull #1299](https://github.com/bfenetworks/bfe/pull/1299))


## [v1.8.3] - 2026-07-10

### Added
- add EnableAiGateway flag  ([Pull #1248](https://github.com/bfenetworks/bfe/pull/1255))
- add mod_ai_rate_limit     ([Pull #1248](https://github.com/bfenetworks/bfe/pull/1255))
- update mod_ai_token_auth  ([Pull #1248](https://github.com/bfenetworks/bfe/pull/1256))
- add mod_access_pb         ([Pull #1248](https://github.com/bfenetworks/bfe/pull/1257))
- add mod_session_sticky    ([Pull #1248](https://github.com/bfenetworks/bfe/pull/1262))


## [v1.8.2] - 2026-05-08

### Added

- support Epp balance  ([Pull #1248](https://github.com/bfenetworks/bfe/pull/1248))
- improve sse support  ([Pull #1247](https://github.com/bfenetworks/bfe/pull/1247))


## [v1.8.1] - 2026-02-12

### Added

- External Web Server Support: Enabled the functionality to call an external web server, enhancing integration capabilities with other services.
- Comprehensive Kubernetes Demo: Introduced a complete Kubernetes demo covering data plane, control plane, and service discovery. This includes updated kustomize manifests for easier deployment and customization.
- Build Process Improvements: Enhanced build processes by adding Makefile targets (make docker, make docker-push) to facilitate building Docker images and pushing them to registries, streamlining CI/CD pipeline operations.

### Fixed

- Fix AI Functionality Bugs: Addressed several bugs related to AI functionalities, ensuring more reliable and accurate processing within AI-driven features.


## [v1.8.0] - 2025-09-03

### Added

- Support third-party WAF integration  ([Pull #1222](https://github.com/bfenetworks/bfe/pull/1222))
- Support basic functions of an AI gateway ([Pull #1223](https://github.com/bfenetworks/bfe/pull/1223))
- Support forwarding requests to backend RS over HTTPS


## [v1.7.0] - 2025-01-19

### Added

- Support building docker images for multiple architectures
- Support WASM to allow dynamic plugin

### Fixed

- Upgrade versions of some third-paty modules for security reasons
- Fix format issue in bfe_http/request_test.go
- Temporarily remove some test cases using legacy test data
- mod_tcp_keepalive: fix broken unit tests
- http2: close connections when receiving too many headers
- Fix readloop goroutine leak

### Removed

- The previous experimental dynamic plugin is deprecated

## [v1.6.0] - 2022-10-21

### Added

- Allow user to disable monitor port ([Issue #936](https://github.com/bfenetworks/bfe/issues/936))
- Support HTTP2 fingerprint ([Issue #1071](https://github.com/bfenetworks/bfe/issues/1071))
- Documents optimization

### Changed

- Optimize idle connection handling  ([Pull #1044](https://github.com/bfenetworks/bfe/pull/1044))
- Performance optimize in smooth least connection balancing algorithm([Pull #1062](https://github.com/bfenetworks/bfe/pull/1062))
- Miscellaneous golang dependency updates
- Miscellaneous improvements in makefile and other CI tools

### Fixed

- mod_trust_clientip: fix incorrect private IP address range ([Issue #856](https://github.com/bfenetworks/bfe/issues/856))
- arm build error in golang 1.18

## [v1.6.0] - 2022-10-21

### Added

- Allow user to disable monitor port ([Issue #936](https://github.com/bfenetworks/bfe/issues/936))
- Support HTTP2 fingerprint ([Issue #1071](https://github.com/bfenetworks/bfe/issues/1071))
- Documents optimization

### Changed

- Optimize idle connection handling  ([Pull #1044](https://github.com/bfenetworks/bfe/pull/1044))
- Performance optimize in smooth least connection balancing algorithm([Pull #1062](https://github.com/bfenetworks/bfe/pull/1062))
- Miscellaneous golang dependency updates
- Miscellaneous improvements in makefile and other CI tools

### Fixed

- mod_trust_clientip: fix incorrect private IP address range ([Issue #856](https://github.com/bfenetworks/bfe/issues/856))
- arm build error in golang 1.18


## [v1.5.0] - 2022-01-11

### Added

- Add mod_tcp_keepalive to support custom TCP KeepAlive configuration
- Documents optimization

### Changed

- Change JSON parser: use json-iterator to replace encoding/json
- Optimize error handling in io.Write

### Fixed

- Fix issue in go.mod which may cause build error (Issue #933)

### Security

- Fix http security issue (Issue #940)
- Fix http security issue (Issue #941)

## [v1.4.0] - 2021-12-10

### Added

- Documents optimization

### Changed

- Upgrade golang version from 1.13 to 1.17
- mod_markdown: upgrade bluemonday version to 1.0.16
- Optimize mutex
- Improve Makefile and pre-commit

## [v1.3.0] - 2021-09-16

### Added

- Support basic route rules that are similar to k8s ingress rules
- Documents optimization

### Changed

- Ignore GREASE values for JA3 fingerprint

## [v1.2.0] - 2021-06-21

### Added

- Set GOMAXPROCS according to real CPU quota if running on container
- Support condition primitive bfe_time_range/bfe_periodic_time_range
- Support URI hash strategy
- mod_block: support global request rules and ALLOW action
- mod_header: support variables about client certificate
- mod_access: support log to single file and stdout

### Changed

- Change primitive params of req_context_value_in

## [v1.1.0] - 2021-04-08

### Added

- Support JA3 fingerprint for SSL/TLS client
- Support Slow‑Start to allow a backend instance gradually recover its weight
- Add maxConnPerHost to limit the number of connections to a backend
- mod_header: add header renaming actions
- Merge some updates from golang/net/textproto
- Merge some updates from golang/net/http
- Merge some updates from golang/net/http2
- Documents optimization

### Changed

- Change outlierDetectionLevel to OutlierDetectionHttpCode

### Fixed

- Fix panic when write internal response timeout
- Fix unit test in bfe_spdy/frame_test.go under go 1.16

### Security

- Fix config loading for multi-value option

## [v1.0.0] - 2021-01-15

### Added

- Add condition primitive: req_path_contain/req_path_element_prefix_in/req_context_value_in
- Add outlier detection options
- Add mod_waf with rule to detect exploitation of "Shellshock" GNU Bash RCE vulnerability.

### Fixed

- Fix build issue under go1.15 environment
- Fix processing X-Forwarded-For header value
- Fix write timeout of internal response generated by bfe

## [v0.12.0] - 2020-09-03

### Added

- Support gRPC over HTTP/2
- Support communication with FastCGI server
- Reduce objects number by release some objects that are not used as soon as possible
- Replace ffjson with json-iterator
- net/textproto: turn an ancient DoS BUG annotation into a comment
- Optimize processing of new connections under PROXY mode
- Support integration testing in CI

## [v0.11.0] - 2020-07-24

### Added

- Add mod_cors to enable cross-origin resource sharing
- Add mod_secure_link to check authenticity and limit lifetime of links
- Support PROXY protocol for TCP connections to backend
- Support checking revocation status of the client certificate
- mod_auth_request: send request with X-Forwarded-Method/X-Forwarded-Uri headers
- mod_userid: support global rules
- mod_key_log: support conditional logging
- Add arm64 support for goreleaser

### Changed

- Create listeners in the final initialization step
- Change package name to 'github.com/bfenetworks/bfe'

### Removed

- Drops words like blacklist from programming to make vocabulary more inclusive
- Remove legacy signature of response

### Fixed

- Fix getTransport() and modify transports map with write lock

### Security

- Fix textproto: not normalize headers with spaces before the colon (CVE-2019-16276)

## [v0.10.0] - 2020-05-25

### Added

- mod_auth_request: authorize clients based on thirdparty authorization service
- mod_trace: support tracing based on Elastic APM
- mod_compress: support brotli algorithm
- mod_rewrite: add HostSuffixReplace action
- Add condition primitive: req_host_tag_in/req_host_suffix_in
- Add static content of website based on mkdocs
- Documents optimization

### Changed

- Refactor and simplify mod_auth_jwt

### Removed

- Remove legacy type of Layer4LoadBalancer

## [v0.9.0] - 2020-04-16

### Added

- Support loading dynamic modules that may be written and complied by thirdparty vendors
- Add mod_auth_jwt for JWT authentication
- mod_trace: support distributed tracing based on jaeger
- mod_doh: support timeout for DNS requests
- mod_doh: support edns-client-subnet (ECS)
- mod_doh: optimize max-age for DoH response
- Optimize textproto reader cache
- Documents optimization

### Changed

- Change base directory of ServerCertFile/ServerKeyFile to ConfRoot

### Fixed

- Fix failed unit tests about parsing request line under go1.14 environment

## [v0.8.0] - 2020-03-27

### Added

- Add mod_trace for distributed tracing based on zipkin
- Add mod_doh for processing DNS over HTTPS(DoH) requests
- Support TLS protocol with SM related ciphersuites
- mod_header: add cookie related actions
- Documents optimization

### Changed

- Disable sanitization of cookie value

## [v0.7.0] - 2020-02-26

### Added

- mod_access support NCSA Common Log Format(CLF) and W3C Extended Log Format(ELF)
- mod_static suuport HTTP HEAD method
- Add mod_userid for client identification
- Add mod_tag for tagging and tracking groups of requests
- http cookie: support Expires attribute in GMT format
- http cookie: support SameSite attribute
- Add static check in Makefile
- Compiling on Windows OS is supported
- Documents optimization

### Fixed

- Fix a bug that causes invalid ips are parsed and treated as domain names

## [v0.6.0] - 2020-01-21

### Added

- Add mod_prison to limit the amount of requests a user can make in a given period of time.
- Add condition primitive: ses_tls_sni_in/ses_tls_client_auth/ses_tls_client_ca_in
- Add tls mutual authentication
- mod_header support client cert related variables
- mod_header support geo related variables
- mod_static support customized mime rules
- mod_static allow sending precompressed files instead of regular files
- Expose information about module handlers in web monitor
- Optimize number of accept goroutines
- Optimize lock of bfe_balance.BalTable
- Optimize io.Copy while forwarding responses
- Compiling on Mac OS is supported
- Documents optimization

### Changed

- Change default Layer4LoadBalancer to NONE
- Upgrade from go1.12 to go1.13

## [v0.5.0] - 2019-12-12

### Added

- Add mod_geo to determine user geolocation by MaxMind database
- Add mod_auth_basic to restrict access from unknown users
- Add mod_compress to compress response by customize rules
- Add mod_errors to replace unexpected resposne
- mod_static detect content type based on file extension or content
- Expose pprof items in web monitor
- Add default resFlushInterval for SSE(Server-Sent Events) response
- Support X-Forwarded-Host Header
- Support docker image in Makefile
- Documents optimization

### Changed

- Change default monitor port to 8421
- Rename HANDLE_READ_BACKEND to HANDLE_READ_RESPONSE

### Removed

- Remove some legacy fields from mod_access

## [v0.4.0] - 2019-11-19

### Added

- Expose metrics in prometheus text-based format
- Add default value for core configs to reduce configuration cost
- Add mod_static to serve static content by customize rules
- Documents optimization

### Changed

- mod_logid: random logid in hex encoding format

### Removed

- Remove signal SIGSTKFLT

## [v0.3.0] - 2019-10-17

### Added

- Add condition primitive: req_cip_hash_in/req_header_value_hash_in/req_cookie_value_hash_in/req_query_value_hash_in
- Add mod_header variable: bfe_log_id
- Add mod_http_code to maintain basic status about response forwarded

### Security

- Fix http2: limit number of control frames in server send queue (CVE-2019-9512, CVE-2019-9514)

## [v0.2.0] - 2019-09-26

### Added

- Add proxy protocol to be compatible with F5 BigIP/Citrix ADC etc
- Add mod_access to write request/session log in customized format
- Add mod_key_log to wirte tls key log so that external programs(eg. wireshark) can decrypt TLS connections for trouble shooting
- Add security grade 'A+' in tls
- Add condition primitive: req_query_value_contain/req_header_value_contain/req_cookie_value_contain
- Documents optimization

### Changed

- reverseproxy: flush response header immediately if flushInterval<0

## [v0.1.0] - 2019-08-01

### Added

- Multiple protocols supported, including HTTP, HTTPS, SPDY, HTTP2, WebSocket, TLS, etc
- Content based routing, support user-defined routing rule in advanced domain-specific language
- Support multiple load balancing policies
- Flexible plugin framework to extend functionality. Based on the framework, developer can add new features rapidly
- Detailed built-in metrics available for service status monitor

[v1.8.9]: https://github.com/bfenetworks/bfe/compare/v1.8.8...v1.8.9
[v1.8.8]: https://github.com/bfenetworks/bfe/compare/v1.8.7...v1.8.8
[v1.8.7]: https://github.com/bfenetworks/bfe/compare/v1.8.6...v1.8.7
[v1.8.6]: https://github.com/bfenetworks/bfe/compare/v1.8.5...v1.8.6
[v1.8.5]: https://github.com/bfenetworks/bfe/compare/v1.8.4...v1.8.5
[v1.8.4]: https://github.com/bfenetworks/bfe/compare/v1.8.3...v1.8.4
[v1.6.0]: https://github.com/bfenetworks/bfe/compare/v1.5.0...v1.6.0
[v1.5.0]: https://github.com/bfenetworks/bfe/compare/v1.4.0...v1.5.0
[v1.4.0]: https://github.com/bfenetworks/bfe/compare/v1.3.0...v1.4.0
[v1.3.0]: https://github.com/bfenetworks/bfe/compare/v1.2.0...v1.3.0
[v1.2.0]: https://github.com/bfenetworks/bfe/compare/v1.1.0...v1.2.0
[v1.1.0]: https://github.com/bfenetworks/bfe/compare/v1.0.0...v1.1.0
[v1.0.0]: https://github.com/bfenetworks/bfe/compare/v0.12.0...v1.0.0
[v0.12.0]: https://github.com/bfenetworks/bfe/compare/v0.11.0...v0.12.0
[v0.11.0]: https://github.com/bfenetworks/bfe/compare/v0.10.0...v0.11.0
[v0.10.0]: https://github.com/bfenetworks/bfe/compare/v0.9.0...v0.10.0
[v0.9.0]: https://github.com/bfenetworks/bfe/compare/v0.8.0...v0.9.0
[v0.8.0]: https://github.com/bfenetworks/bfe/compare/v0.7.0...v0.8.0
[v0.7.0]: https://github.com/bfenetworks/bfe/compare/v0.6.0...v0.7.0
[v0.6.0]: https://github.com/bfenetworks/bfe/compare/v0.5.0...v0.6.0
[v0.5.0]: https://github.com/bfenetworks/bfe/compare/v0.4.0...v0.5.0
[v0.4.0]: https://github.com/bfenetworks/bfe/compare/v0.3.0...v0.4.0
[v0.3.0]: https://github.com/bfenetworks/bfe/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/bfenetworks/bfe/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/bfenetworks/bfe/releases/tag/v0.1.0

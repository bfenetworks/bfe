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

package mod_ai_context

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
)

// rewriteMarker is appended to the content of every message processed by P2;
// a later pass (retry/reconstructed request) skips marked messages to avoid
// double compression (design 7.3).
const rewriteMarker = "[COMPRESSED:rewrite]"

// redundancy rewrite rule set: ordered (old, new) pairs. "lite" covers
// politeness fillers and redundant connectors in English and Chinese; "full"
// adds a few heavier paraphrase rules. Deliberately small (design: 若干条即
// 可，不必穷举); rules only apply to unprotected text.
type rewriteRule struct {
	old string
	new string
}

var rewriteLiteRules = []rewriteRule{
	// English politeness / filler
	{"Please note that ", ""},
	{"It is important to note that ", ""},
	{"It's important to note that ", ""},
	{"As an AI language model, ", ""},
	{"It should be noted that ", ""},
	{"I hope this helps.", ""},
	{"Hope this helps.", ""},
	{"Let me know if you need more details.", ""},
	{"In conclusion, ", ""},
	// Chinese politeness / filler
	{"请注意，", ""},
	{"需要注意的是，", ""},
	{"作为AI语言模型，", ""},
	{"希望对你有所帮助。", ""},
	{"希望对您有所帮助。", ""},
	{"综上所述，", ""},
	{"总而言之，", ""},
	{"我想说的是，", ""},
}

var rewriteFullExtraRules = []rewriteRule{
	{"In order to ", "To "},
	{"Due to the fact that ", "Because "},
	{"due to the fact that ", "because "},
	{"At this point in time", "Now"},
	{"a large number of ", "many "},
	{"is able to ", "can "},
	{"Is able to ", "Can "},
	{"不可否认的是，", ""},
	{"与此同时，", "同时，"},
	{"在这种情况下，", "此时，"},
	{"进行一个", "进行"},
	{"如果能够的话", "如果可能"},
}

var (
	multiSpace     = regexp.MustCompile(`[ \t]{2,}`)
	multiBlankLine = regexp.MustCompile(`\n{3,}`)
)

// protected span patterns, applied in order. Code spans go first because
// they may contain URLs, paths, versions and numbers that must stay intact.
var (
	codeFenceRe  = regexp.MustCompile("(?s)```.*?```")
	inlineCodeRe = regexp.MustCompile("`[^`\n]+`")
	urlRe        = regexp.MustCompile(`https?://[^\s"'，。）\]}>]+`)
	pathRe       = regexp.MustCompile(`(?:[A-Za-z]:[\\/]|/)[\w\-./\\]*[\w\-./]`)
	versionRe    = regexp.MustCompile(`\bv\d+\.\d+(?:\.\d+)?\b`)
	jsonKeyRe    = regexp.MustCompile(`"[^"\n]{1,64}"\s*:`)
	numberRe     = regexp.MustCompile(`\b\d+(?:\.\d+)?\b`)
)

// tombstone sentinels use private-use code points around a random hex id, so
// no rewrite rule can accidentally match (or split) a sentinel.
const (
	sentinelPrefix = "CTX"
	sentinelSuffix = ""
)

// protectedSpan is one protected region of the original text.
type protectedSpan struct {
	sentinel string
	original string
	tokens   int64 // heuristic token weight of the original span
}

// outcome of the P2 rewrite pass over the whole conversation.
type rewriteOutcome struct {
	changed        bool
	survivedTokens int64
	totalTokens    int64
}

// rewriteConversation is P2: tombstone-protect structured fragments, apply
// the redundancy rewrite rules to the unprotected text, then restore the
// tombstones. The last user message and system messages are never touched.
// Survival statistics of the protected spans are collected for the fidelity
// gate.
func rewriteConversation(msgs []Message, strength string, protectedUserIdx int,
	est TokenEstimator, p EstimateParams) rewriteOutcome {
	outcome := rewriteOutcome{}
	for i := range msgs {
		if i == protectedUserIdx || msgs[i].Role == "system" {
			continue
		}
		changed := rewriteMessage(&msgs[i], strength, est, p, &outcome)
		outcome.changed = outcome.changed || changed
	}
	return outcome
}

// rewriteMessage rewrites a single message in place; messages already
// carrying the rewrite marker are skipped.
func rewriteMessage(m *Message, strength string, est TokenEstimator, p EstimateParams,
	outcome *rewriteOutcome) bool {
	if strings.Contains(messageText(m), rewriteMarker) {
		return false
	}
	changed := false

	if s, ok := m.contentString(); ok && strings.TrimSpace(s) != "" {
		if rewritten, surv, total := rewriteText(s, strength, est, p); rewritten != s {
			m.setContentString(rewritten + "\n\n" + rewriteMarker)
			outcome.survivedTokens += surv
			outcome.totalTokens += total
			changed = true
		}
	}

	parts, ok := m.contentParts()
	if !ok {
		return changed
	}
	partsChanged := false
	var survived, total int64
	for j := range parts {
		if parts[j].Type != "text" || strings.TrimSpace(parts[j].Text) == "" {
			continue
		}
		rewritten, surv, tot := rewriteText(parts[j].Text, strength, est, p)
		if rewritten != parts[j].Text {
			parts[j].Text = rewritten
			survived += surv
			total += tot
			partsChanged = true
		}
	}
	if partsChanged {
		parts = append(parts, ContentPart{Type: "text", Text: rewriteMarker})
		m.setContentParts(parts)
		outcome.survivedTokens += survived
		outcome.totalTokens += total
		changed = true
	}
	return changed
}

// rewriteText protects, rewrites and restores one text fragment. It returns
// the restored text and the token-weighted survival statistics of the
// protected spans (used by the fidelity gate).
func rewriteText(s string, strength string, est TokenEstimator, p EstimateParams) (string, int64, int64) {
	masked, spans := protectText(s, est, p)
	out := applyRewriteRules(masked, strength)
	restored, survived := restoreText(out, spans)
	return restored, survived, totalSpanTokens(spans)
}

// protectText replaces protected fragments with random sentinels and returns
// the masked text together with the span table for restoration.
func protectText(s string, est TokenEstimator, p EstimateParams) (string, []protectedSpan) {
	spans := []protectedSpan{}
	mask := func(re *regexp.Regexp, in string) string {
		return re.ReplaceAllStringFunc(in, func(match string) string {
			sentinel := fmt.Sprintf("%s%x%s", sentinelPrefix, rand.Uint64(), sentinelSuffix)
			spans = append(spans, protectedSpan{
				sentinel: sentinel,
				original: match,
				tokens:   est.EstimateText(match, p),
			})
			return sentinel
		})
	}
	masked := mask(codeFenceRe, s)
	masked = mask(inlineCodeRe, masked)
	masked = mask(urlRe, masked)
	masked = mask(pathRe, masked)
	masked = mask(versionRe, masked)
	masked = mask(jsonKeyRe, masked)
	masked = mask(numberRe, masked)
	return masked, spans
}

// restoreText puts the protected originals back. A span that cannot be found
// (sentinel damaged by the rewrite) counts as not survived.
func restoreText(s string, spans []protectedSpan) (string, int64) {
	var survived int64
	for _, sp := range spans {
		if idx := strings.Index(s, sp.sentinel); idx >= 0 {
			s = s[:idx] + sp.original + s[idx+len(sp.sentinel):]
			survived += sp.tokens
		}
	}
	return s, survived
}

func totalSpanTokens(spans []protectedSpan) int64 {
	var total int64
	for _, sp := range spans {
		total += sp.tokens
	}
	return total
}

// applyRewriteRules applies the redundancy rule set to the masked text.
func applyRewriteRules(s string, strength string) string {
	rules := rewriteLiteRules
	if strength == RewriteStrengthFull {
		rules = append(append([]rewriteRule{}, rewriteLiteRules...), rewriteFullExtraRules...)
	}
	for _, r := range rules {
		if r.old == "" {
			continue
		}
		s = strings.ReplaceAll(s, r.old, r.new)
	}
	// collapse repeated whitespace left behind by the removals
	s = multiSpace.ReplaceAllString(s, " ")
	s = multiBlankLine.ReplaceAllString(s, "\n\n")
	return s
}

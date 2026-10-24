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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextRuleConfLoad(t *testing.T) {
	conf, err := ContextRuleConfLoad("testdata/mod_ai_context/context_rule.data")
	require.NoError(t, err)

	require.NotNil(t, conf.Version)
	assert.Equal(t, "1.0", *conf.Version)

	// Defaults block parsed
	require.NotNil(t, conf.Defaults)
	assert.Equal(t, 0.7, conf.Defaults.TriggerRatio)
	assert.Equal(t, 1, conf.Defaults.KeepLatestImages)
	assert.Equal(t, 10, conf.Defaults.ToolResultMaxChars)
	assert.Equal(t, ThinkingPolicyTrimAllButLast, conf.Defaults.ThinkingPolicy)
	assert.Equal(t, 4, conf.Defaults.CharsPerToken)
	assert.Equal(t, 100, conf.Defaults.ImageTokenEstimate)
	assert.Equal(t, RewriteStrengthLite, conf.Defaults.RewriteStrength)
	assert.InDelta(t, 0.95, conf.Defaults.ProtectedSurvivalRate, 1e-9)

	// forward compat: Defaults.summary (phase-2 block) and the rule-level
	// override field are ignored and counted, not rejected
	assert.Equal(t, 2, conf.UnknownFields)

	// product rules parsed; budget overrides applied
	require.NotNil(t, conf.Config)
	rules := (*conf.Config)["tiny_budget_product"]
	require.NotNil(t, rules)
	require.Len(t, *rules, 1)
	rule := (*rules)[0]
	assert.Equal(t, ModeBalanced, rule.Mode)
	assert.Equal(t, int64(1000), rule.MaxContextTokens)
	assert.Equal(t, int64(100), rule.ReserveTokens)
}

func TestContextRuleConfLoadMinimal(t *testing.T) {
	// no Defaults block at all: built-in defaults kick in
	path := writeTempRule(t, `{
		"Version": "1.0",
		"Config": {"p": [{"cond": "default_t()", "mode": "off"}]}
	}`)
	conf, err := ContextRuleConfLoad(path)
	require.NoError(t, err)

	assert.Equal(t, 0, conf.UnknownFields)
	assert.Equal(t, DefaultTriggerRatio, conf.Defaults.TriggerRatio)
	assert.Equal(t, DefaultKeepLatestImages, conf.Defaults.KeepLatestImages)
	assert.Equal(t, DefaultToolResultMaxChars, conf.Defaults.ToolResultMaxChars)
	assert.Equal(t, DefaultThinkingPolicy, conf.Defaults.ThinkingPolicy)
	assert.Equal(t, DefaultCharsPerToken, conf.Defaults.CharsPerToken)
	assert.Equal(t, DefaultImageTokenEstimate, conf.Defaults.ImageTokenEstimate)
	assert.Equal(t, DefaultRewriteStrength, conf.Defaults.RewriteStrength)
	assert.InDelta(t, DefaultProtectedSurvivalRate, conf.Defaults.ProtectedSurvivalRate, 1e-9)

	rule := (*(*conf.Config)["p"])[0]
	assert.Equal(t, int64(0), rule.MaxContextTokens)
	assert.Equal(t, int64(0), rule.ReserveTokens)
}

func TestContextRuleConfLoadUnknownFieldsCountedNotRejected(t *testing.T) {
	path := writeTempRule(t, `{
		"Version": "1.0",
		"GlobalSummary": {"x": 1},
		"Defaults": {"triggerRatio": 0.5, "summary": {"y": 2}, "rewrite": {"strength": "full", "extra": 3}},
		"Config": {"p": [{"cond": "default_t()", "mode": "balanced", "override": {"z": 4}, "maxContextTokens": 0}]}
	}`)
	conf, err := ContextRuleConfLoad(path)
	require.NoError(t, err)

	// GlobalSummary + Defaults.summary + rewrite.extra + rule.override
	assert.Equal(t, 4, conf.UnknownFields)
	// known fields still parsed
	assert.Equal(t, 0.5, conf.Defaults.TriggerRatio)
	assert.Equal(t, RewriteStrengthFull, conf.Defaults.RewriteStrength)
	assert.Equal(t, ModeBalanced, (*(*conf.Config)["p"])[0].Mode)
}

func TestContextRuleConfLoadRejects(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "missing mode",
			content: `{"Version":"1.0","Config":{"p":[{"cond":"default_t()"}]}}`,
		},
		{
			name:    "empty mode",
			content: `{"Version":"1.0","Config":{"p":[{"cond":"default_t()","mode":""}]}}`,
		},
		{
			name:    "invalid mode",
			content: `{"Version":"1.0","Config":{"p":[{"cond":"default_t()","mode":"medium"}]}}`,
		},
		{
			name:    "missing cond",
			content: `{"Version":"1.0","Config":{"p":[{"mode":"off"}]}}`,
		},
		{
			name:    "cond compile failure",
			content: `{"Version":"1.0","Config":{"p":[{"cond":"no_such_primitive()","mode":"off"}]}}`,
		},
		{
			name:    "negative maxContextTokens",
			content: `{"Version":"1.0","Config":{"p":[{"cond":"default_t()","mode":"off","maxContextTokens":-1}]}}`,
		},
		{
			name:    "negative reserveTokens",
			content: `{"Version":"1.0","Config":{"p":[{"cond":"default_t()","mode":"off","reserveTokens":-5}]}}`,
		},
		{
			name:    "duplicate cond in product",
			content: `{"Version":"1.0","Config":{"p":[{"cond":"default_t()","mode":"off"},{"cond":"default_t()","mode":"balanced"}]}}`,
		},
		{
			name:    "missing Version",
			content: `{"Config":{"p":[{"cond":"default_t()","mode":"off"}]}}`,
		},
		{
			name:    "missing Config",
			content: `{"Version":"1.0"}`,
		},
		{
			name:    "invalid triggerRatio",
			content: `{"Version":"1.0","Defaults":{"triggerRatio":1.5},"Config":{"p":[{"cond":"default_t()","mode":"off"}]}}`,
		},
		{
			name:    "invalid thinkingPolicy",
			content: `{"Version":"1.0","Defaults":{"thinkingPolicy":"drop"},"Config":{"p":[{"cond":"default_t()","mode":"off"}]}}`,
		},
		{
			name:    "invalid rewrite strength",
			content: `{"Version":"1.0","Defaults":{"rewrite":{"strength":"max"}},"Config":{"p":[{"cond":"default_t()","mode":"off"}]}}`,
		},
		{
			name:    "invalid protectedSurvivalRate",
			content: `{"Version":"1.0","Defaults":{"rewrite":{"protectedSurvivalRate":0}},"Config":{"p":[{"cond":"default_t()","mode":"off"}]}}`,
		},
		{
			name:    "invalid charsPerToken",
			content: `{"Version":"1.0","Defaults":{"charsPerToken":0},"Config":{"p":[{"cond":"default_t()","mode":"off"}]}}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeTempRule(t, c.content)
			_, err := ContextRuleConfLoad(path)
			assert.Error(t, err, "content: %s", c.content)
		})
	}
}

func TestContextRuleConfLoadValidModes(t *testing.T) {
	for _, mode := range []string{ModeOff, ModeConservative, ModeBalanced, ModeAggressive} {
		path := writeTempRule(t, `{"Version":"1.0","Config":{"p":[{"cond":"default_t()","mode":"`+mode+`"}]}}`)
		_, err := ContextRuleConfLoad(path)
		assert.NoError(t, err, "mode: %s", mode)
	}
}

func TestConfLoad(t *testing.T) {
	cfg, err := ConfLoad("testdata/mod_ai_context/mod_ai_context.conf", "./testdata")
	require.NoError(t, err)
	assert.Contains(t, cfg.Basic.ProductRulePath, "context_rule.data")
	assert.False(t, cfg.Log.OpenDebug)
}

func TestConfLoadMissingRulePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mod_ai_context.conf")
	require.NoError(t, os.WriteFile(path, []byte("[basic]\n"), 0644))
	_, err := ConfLoad(path, "./testdata")
	assert.Error(t, err)
}

// writeTempRule writes content to a temp rule file and returns its path.
func writeTempRule(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "context_rule.data")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

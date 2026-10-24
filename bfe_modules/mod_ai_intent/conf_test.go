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

package mod_ai_intent

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfFile(t *testing.T, content string) (string, string) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mod_ai_intent.conf")
	require.NoError(t, ioutil.WriteFile(path, []byte(content), 0644))
	return path, dir
}

func TestConfLoad(t *testing.T) {
	content := `
[basic]
DecisionServiceAddr = http://127.0.0.1:8000
QuestionsPath = mod_ai_intent/intent_questions.data
TimeoutMs = 200
MaxStateChars = 500
CacheSize = 100
CacheTTLSeconds = 60
ExplicitIntentHeader = X-Custom-Intent

[breaker]
FailureThreshold = 2
ProbeIntervalMs = 1000

[log]
OpenDebug = true
`
	path, dir := writeConfFile(t, content)
	cfg, err := ConfLoad(path, dir)
	require.NoError(t, err)

	assert.Equal(t, "http://127.0.0.1:8000", cfg.Basic.DecisionServiceAddr)
	// ConfPathProc joins with confRoot using path.Join (slash separators)
	assert.Equal(t, dir+"/mod_ai_intent/intent_questions.data", cfg.Basic.QuestionsPath)
	assert.Equal(t, 200, cfg.Basic.TimeoutMs)
	assert.Equal(t, 500, cfg.Basic.MaxStateChars)
	assert.Equal(t, 100, cfg.Basic.CacheSize)
	assert.Equal(t, 60, cfg.Basic.CacheTTLSeconds)
	assert.Equal(t, "X-Custom-Intent", cfg.Basic.ExplicitIntentHeader)
	assert.Equal(t, 2, cfg.Breaker.FailureThreshold)
	assert.Equal(t, 1000, cfg.Breaker.ProbeIntervalMs)
	assert.True(t, cfg.Log.OpenDebug)
}

func TestConfLoadDefaults(t *testing.T) {
	content := `
[basic]
DecisionServiceAddr = http://127.0.0.1:8000
QuestionsPath = intent_questions.data
`
	path, dir := writeConfFile(t, content)
	cfg, err := ConfLoad(path, dir)
	require.NoError(t, err)

	assert.Equal(t, DefaultTimeoutMs, cfg.Basic.TimeoutMs)
	assert.Equal(t, DefaultMaxStateChars, cfg.Basic.MaxStateChars)
	assert.Equal(t, DefaultCacheSize, cfg.Basic.CacheSize)
	assert.Equal(t, DefaultCacheTTLSeconds, cfg.Basic.CacheTTLSeconds)
	assert.Equal(t, DefaultExplicitIntentHeader, cfg.Basic.ExplicitIntentHeader)
	assert.Equal(t, DefaultFailureThreshold, cfg.Breaker.FailureThreshold)
	assert.Equal(t, DefaultProbeIntervalMs, cfg.Breaker.ProbeIntervalMs)
	assert.False(t, cfg.Log.OpenDebug)
}

func TestConfLoadErrors(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"missing addr", `
[basic]
QuestionsPath = intent_questions.data
`},
		{"missing questions path", `
[basic]
DecisionServiceAddr = http://127.0.0.1:8000
`},
		{"negative cache size", `
[basic]
DecisionServiceAddr = http://127.0.0.1:8000
QuestionsPath = intent_questions.data
CacheSize = -1
`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path, dir := writeConfFile(t, c.content)
			_, err := ConfLoad(path, dir)
			assert.Error(t, err)
		})
	}

	// unreadable file
	_, err := ConfLoad(filepath.Join(t.TempDir(), "not_exist.conf"), t.TempDir())
	assert.Error(t, err)
}

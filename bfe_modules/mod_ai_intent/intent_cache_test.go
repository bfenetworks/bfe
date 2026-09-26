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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/bfenetworks/bfe/bfe_basic"
)

func testIntent(v string) *bfe_basic.AiIntent {
	return &bfe_basic.AiIntent{
		QuestionsVersion: v,
		Answers:          map[string]*bfe_basic.IntentAnswer{},
		Resolved:         true,
	}
}

func TestIntentCacheBasic(t *testing.T) {
	c := NewIntentCache(10, time.Minute)
	assert.True(t, c.Enabled())
	assert.Equal(t, 0, c.Len())

	v := testIntent("v1")
	c.Set("k1", v)
	assert.Equal(t, 1, c.Len())
	assert.Same(t, v, c.Get("k1"))
	assert.Nil(t, c.Get("missing"))

	// overwrite refreshes value
	v2 := testIntent("v2")
	c.Set("k1", v2)
	assert.Equal(t, 1, c.Len())
	assert.Same(t, v2, c.Get("k1"))

	// nil value ignored
	c.Set("knil", nil)
	assert.Nil(t, c.Get("knil"))
}

func TestIntentCacheLRU(t *testing.T) {
	c := NewIntentCache(2, time.Minute)

	k1, k2, k3 := "k1", "k2", "k3"
	c.Set(k1, testIntent("v"))
	c.Set(k2, testIntent("v"))
	// refresh k1: k2 becomes the eviction candidate
	c.Get(k1)
	c.Set(k3, testIntent("v"))

	assert.NotNil(t, c.Get(k1))
	assert.Nil(t, c.Get(k2), "least recently used entry evicted")
	assert.NotNil(t, c.Get(k3))
	assert.Equal(t, 2, c.Len())
}

func TestIntentCacheTTL(t *testing.T) {
	c := NewIntentCache(10, 50*time.Millisecond)
	c.Set("k", testIntent("v"))
	assert.NotNil(t, c.Get("k"))

	time.Sleep(80 * time.Millisecond)
	assert.Nil(t, c.Get("k"), "expired entry must be dropped")
	assert.Equal(t, 0, c.Len())
}

func TestIntentCacheDisabled(t *testing.T) {
	c := NewIntentCache(0, time.Minute)
	assert.False(t, c.Enabled())
	c.Set("k", testIntent("v"))
	assert.Nil(t, c.Get("k"))
	assert.Equal(t, 0, c.Len())

	c = NewIntentCache(10, 0)
	assert.False(t, c.Enabled())
	c.Set("k", testIntent("v"))
	assert.Nil(t, c.Get("k"))
}

func TestCacheKey(t *testing.T) {
	k1 := cacheKey("apikey", "v1", "text")
	// same input -> same key
	assert.Equal(t, k1, cacheKey("apikey", "v1", "text"))
	// version is part of the key: questions change invalidates entries
	assert.NotEqual(t, k1, cacheKey("apikey", "v2", "text"))
	// apikey and text also differentiate
	assert.NotEqual(t, k1, cacheKey("other", "v1", "text"))
	assert.NotEqual(t, k1, cacheKey("apikey", "v1", "other"))
}

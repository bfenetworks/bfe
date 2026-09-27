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
	"container/list"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"sync"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
)

// cacheKey builds the process-wide cache key. The questions Version is part
// of the key, so any questions conf change (with a version bump) naturally
// invalidates previous entries.
func cacheKey(apiKey, questionsVersion, text string) string {
	h := sha1.New()
	io.WriteString(h, apiKey)
	io.WriteString(h, "\x00")
	io.WriteString(h, questionsVersion)
	io.WriteString(h, "\x00")
	io.WriteString(h, text)
	return hex.EncodeToString(h.Sum(nil))
}

type cacheItem struct {
	key      string
	value    *bfe_basic.AiIntent
	expireAt time.Time
	element  *list.Element
}

// IntentCache is a process-wide LRU cache with TTL for classification
// results. Self-implemented (map + list), no third-party dependency.
type IntentCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	ll       *list.List // front: newest, back: oldest
	items    map[string]*cacheItem
}

func NewIntentCache(capacity int, ttl time.Duration) *IntentCache {
	if capacity < 0 {
		capacity = 0
	}
	if ttl < 0 {
		ttl = 0
	}
	return &IntentCache{
		capacity: capacity,
		ttl:      ttl,
		ll:       list.New(),
		items:    make(map[string]*cacheItem),
	}
}

// Enabled reports whether the cache stores anything.
func (c *IntentCache) Enabled() bool {
	return c.capacity > 0 && c.ttl > 0
}

// Len returns the current number of live entries (expired ones excluded).
func (c *IntentCache) Len() int {
	if !c.Enabled() {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictExpired()
	return c.ll.Len()
}

func (c *IntentCache) Get(key string) *bfe_basic.AiIntent {
	if !c.Enabled() {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.items[key]
	if !ok {
		return nil
	}
	if time.Now().After(item.expireAt) {
		c.removeItem(item)
		return nil
	}
	c.ll.MoveToFront(item.element)
	return item.value
}

func (c *IntentCache) Set(key string, v *bfe_basic.AiIntent) {
	if !c.Enabled() || v == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if item, ok := c.items[key]; ok {
		item.value = v
		item.expireAt = time.Now().Add(c.ttl)
		c.ll.MoveToFront(item.element)
		return
	}

	for c.ll.Len() >= c.capacity {
		back := c.ll.Back()
		if back == nil {
			break
		}
		c.removeElement(back)
	}

	item := &cacheItem{
		key:      key,
		value:    v,
		expireAt: time.Now().Add(c.ttl),
	}
	item.element = c.ll.PushFront(item)
	c.items[key] = item
}

func (c *IntentCache) removeItem(item *cacheItem) {
	delete(c.items, item.key)
	c.ll.Remove(item.element)
}

func (c *IntentCache) removeElement(e *list.Element) {
	if item, ok := e.Value.(*cacheItem); ok {
		c.removeItem(item)
		return
	}
	c.ll.Remove(e)
}

func (c *IntentCache) evictExpired() {
	now := time.Now()
	for e := c.ll.Back(); e != nil; {
		prev := e.Prev()
		if item, ok := e.Value.(*cacheItem); ok && now.After(item.expireAt) {
			c.removeItem(item)
		}
		e = prev
	}
}

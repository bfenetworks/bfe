// Copyright (c) 2019 Baidu, Inc.
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

package lru_cache

import "testing"

var TestKeys = []string{
	"key1",
	"key2",
	"key3",
	"key4",
	"key5",
	"key6",
}

var TestValues = []string{
	"val1",
	"val2",
	"val3",
	"val4",
	"val5",
	"val6",
}

func prepareCache() *LRUCache {
	cache := NewLRUCache(8)
	for i := range TestKeys {
		cache.Add(TestKeys[i], TestValues[i])
	}
	return cache
}

func TestAddAndGet(t *testing.T) {
	cache := prepareCache()
	for i, key := range TestKeys {
		actual, _ := cache.Get(key)
		expect := TestValues[i]
		if actual != expect {
			t.Errorf("Get() error, expect: %s, actual: %s", expect, actual)
		}
	}
}

func TestDel(t *testing.T) {
	cache := prepareCache()
	if cache.Len() != 6 {
		t.Errorf("Wrong element count in cache: %d, expecte: 6", cache.Len())
	}
	// case 1
	cache.Del("key1")
	if _, ok := cache.Get("key1"); ok {
		t.Errorf("Should not found deleted value")
	}
	if cache.Len() != 5 {
		t.Errorf("Wrong element count in cache: %d, expecte: 5", cache.Len())
	}
	// case 2
	cache.Del("key_not_exist")
	if _, ok := cache.Get("key_not_exist"); ok {
		t.Errorf("Should not found item not exist")
	}
	if cache.Len() != 5 {
		t.Errorf("Wrong element count in cache: %d, expecte: 5", cache.Len())
	}
}

func TestEvict(t *testing.T) {
	cache := prepareCache()
	cache.Add("a", "va")
	// case 1
	evict := cache.Add("b", "vb")
	if cache.Len() != 8 {
		t.Errorf("Wrong element count in cache: %d, expecte: 8", cache.Len())
	}
	if evict == true {
		t.Errorf("Should add item without eviction")
	}
	// case 2
	evict = cache.Add("c", "vc")
	if cache.Len() != 8 {
		t.Errorf("Wrong element count in cache: %d, expecte: 8", cache.Len())
	}
	if evict == false {
		t.Errorf("Should add item with eviction")
	}
	// case 3
	evict = cache.Add("c", "vc")
	if cache.Len() != 8 {
		t.Errorf("Wrong element count in cache: %d, expecte: 8", cache.Len())
	}
	if evict == true {
		t.Errorf("Should add item without eviction")
	}
}

func TestAddAndKeys(t *testing.T) {
	cache := prepareCache()
	keys := cache.Keys()
	if len(keys) != len(TestKeys) {
		t.Errorf("Should have same key length")
	}
}

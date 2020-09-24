// Copyright (c) 2019 The BFE Authors.
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

package trie

import (
	"strings"
	"testing"
)

func TestDNSMatch(t *testing.T) {

	trie := NewTrie()
	if trie == nil {
		t.Error()
	}

	trie.Set(strings.Split("com.baidu.www", "."), "1")
	trie.Set(strings.Split("com.baidu.*", "."), "2")
	trie.Set(strings.Split("co.baidu.weidu", "."), "2")

	_, ok := trie.Get(strings.Split("com.baidu.www", "."))
	if !ok {
		t.Error()
	}

	_, ok = trie.Get(strings.Split("com.baidu.100", "."))
	if !ok {
		t.Error()
	}

	match, _ := trie.Get(strings.Split("com.1baidu.100", "."))
	if match != nil {
		t.Error()
	}
}

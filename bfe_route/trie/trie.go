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

// Package trie implements a simple trie data structure that maps "paths" (which
// are slices of strings) to arbitrary data values (type interface{}).
package trie

import "errors"

type trieChildren map[string]*Trie

type Trie struct {
	Entry      interface{}
	SplatEntry interface{} // to match xxx.xxx.*
	Children   trieChildren
}

// NewTrie makes a new empty Trie
func NewTrie() *Trie {
	return &Trie{
		Children: make(trieChildren),
	}
}

// Get retrieves an element from the Trie
//
// Takes a path (which can be empty, to denote the root element of the Trie),
// and returns the object if the path exists in the Trie, or nil and a status of
// false. Example:
//
//     if res, ok := trie.Get([]string{"foo", "bar"}), ok {
//       fmt.Println("Value at /foo/bar was", res)
//     }
func (t *Trie) Get(path []string) (entry interface{}, ok bool) {
	if len(path) == 0 {
		return t.getEntry()
	}

	key := path[0]
	newPath := path[1:]

	res, ok := t.Children[key]
	if ok {
		entry, ok = res.Get(newPath)
	}

	if entry == nil && t.SplatEntry != nil {
		entry = t.SplatEntry
		ok = true
	}

	return
}

// Set creates an element in the Trie
//
// Takes a path (which can be empty, to denote the root element of the Trie),
// and an arbitrary value (interface{}) to use as the leaf data.
func (t *Trie) Set(path []string, value interface{}) error {
	if len(path) == 0 {
		t.setEntry(value)
		return nil
	}

	if path[0] == "*" {
		if len(path) != 1 {
			return errors.New("* should be last element")
		}
		t.SplatEntry = value
	}

	key := path[0]
	newPath := path[1:]

	res, ok := t.Children[key]
	if !ok {
		// Trie node that should hold entry doesn't already exist, so let's create it
		res = NewTrie()
		t.Children[key] = res
	}

	return res.Set(newPath, value)
}

func (t *Trie) setEntry(value interface{}) {
	t.Entry = value
}

func (t *Trie) getEntry() (entry interface{}, ok bool) {
	return t.Entry, t.Entry != nil
}

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

package byte_pool

import (
	"bytes"
	"testing"
)

func TestFixedBytePool(t *testing.T) {
	key := []byte("hello world")
	keyLen := len(key)

	pool := NewFixedBytePool(2, keyLen)

	if pool.MaxElemSize() != keyLen {
		t.Error("t.elemeSize error")
	}

	if err := pool.Set(1, key); err != nil {
		t.Error("set should be success")
	}

	resuItem := pool.Get(1)

	if len(key) != len(resuItem) {
		t.Error("testItem and resuItem not same len")
	}
	if !bytes.Equal(key, resuItem) {
		t.Error("testItem, and resuItem not equal")
	}

	if err := pool.Set(2, key); err == nil {
		t.Error("set should failed")
	}

	key = []byte("large than max ele size")
	if err := pool.Set(1, key); err == nil {
		t.Error("set should failed")
	}

}

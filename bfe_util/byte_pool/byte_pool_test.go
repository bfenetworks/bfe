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

func TestBytePool(t *testing.T) {
	eleNum := 2
	maxElemSize := 13

	pool := NewBytePool(eleNum, maxElemSize)
	if pool.MaxElemSize() != maxElemSize {
		t.Error("t.elemeSize error")
	}

	key := []byte("hello world")
	if err := pool.Set(1, key); err != nil {
		t.Error("set should be success")
	}

	result := pool.Get(1)
	if len(key) != len(result) {
		t.Error("result should keep length")
	}

	if !bytes.Equal(key, result) {
		t.Error("item should keep unchanged")
	}

	if err := pool.Set(2, key); err == nil {
		t.Error("set should failed")
	}

	key = []byte("large than max ele size")
	if err := pool.Set(1, key); err == nil {
		t.Error("set should failed")
	}
}

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

import "fmt"

type FixedBytePool struct {
	buf        []byte
	elemSize   int // element length
	maxElemNum int // max element num
}

// NewFixedBytePool creates a new FixedBytePool
//
// PARAMS:
//   - elemNum: int, the max element num of FixedBytePool
//   - elemSize: int, the max length of each element
//
// RETURNS:
//   - a pointer point to the FixedBytePool
func NewFixedBytePool(elemNum int, elemSize int) *FixedBytePool {
	pool := new(FixedBytePool)
	pool.buf = make([]byte, elemNum*elemSize)
	pool.elemSize = elemSize
	pool.maxElemNum = elemNum

	return pool
}

// Set sets the index node of FixedBytePool with key
//
// PARAMS:
//   - index: index of the byte Pool
//   - key: []byte key
func (pool *FixedBytePool) Set(index int32, key []byte) error {
	if int(index) >= pool.maxElemNum {
		return fmt.Errorf("index out of range %d %d", index, pool.maxElemNum)
	}

	if len(key) != pool.elemSize {
		return fmt.Errorf("length must be %d while %d", pool.elemSize, len(key))
	}
	start := int(index) * pool.elemSize
	copy(pool.buf[start:], key)

	return nil
}

// Get the byte slice of giving index and length
//
// PARAMS:
//   - index: int, index of the FixedBytePool
//
// RETURNS:
//   - key: []byte type store in the FixedBytePool
func (pool *FixedBytePool) Get(index int32) []byte {
	start := int(index) * pool.elemSize
	end := start + pool.elemSize

	return pool.buf[start:end]
}

// MaxElemSize return the space allocate for each element
func (pool *FixedBytePool) MaxElemSize() int {
	return pool.elemSize
}

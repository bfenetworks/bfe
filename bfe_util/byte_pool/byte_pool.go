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

type BytePool struct {
	buf         []byte
	length      []uint32
	maxElemSize int // max length of element
	maxElemNum  int // max element num
}

// NewBytePool creates a new BytePool
//
// PARAMS:
//   - elemNum: int, the max element num of BytePool
//   - maxElemSize: int, the max length of each element
//
// RETURNS:
//   - a pointer point to the BytePool
func NewBytePool(elemNum int, maxElemSize int) *BytePool {
	pool := new(BytePool)
	pool.buf = make([]byte, elemNum*maxElemSize)
	pool.length = make([]uint32, elemNum)
	pool.maxElemSize = maxElemSize
	pool.maxElemNum = elemNum

	return pool
}

// Set sets the index node of BytePool with key
//
// PARAMS:
//   - index: index of the byte Pool
//   - key: []byte key
func (pool *BytePool) Set(index int32, key []byte) error {
	if int(index) >= pool.maxElemNum {
		return fmt.Errorf("index out of range %d %d", index, pool.maxElemNum)
	}

	if len(key) > pool.maxElemSize {
		return fmt.Errorf("elemSize large than maxSize %d %d", len(key), pool.maxElemSize)
	}

	start := int(index) * pool.maxElemSize
	copy(pool.buf[start:], key)

	pool.length[index] = uint32(len(key))

	return nil
}

// Get the byte slice
//
// PARAMS:
//   - index: int, index of the BytePool
//
// RETURNS:
//   - key: []byte type store in the BytePool
func (pool *BytePool) Get(index int32) []byte {
	start := int(index) * pool.maxElemSize
	end := start + int(pool.length[index])

	return pool.buf[start:end]
}

// MaxElemSize returns the space allocate for each element
func (pool *BytePool) MaxElemSize() int {
	return pool.maxElemSize
}

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

package bfe_util

import (
	"sync"
)

type FixedPool struct {
	pool sync.Pool // memory block pool
	size int       // memory block size
}

func NewFixedPool(size int) *FixedPool {
	p := new(FixedPool)
	p.size = size
	return p
}

// GetBlock gets a byte slice from pool
func (p *FixedPool) GetBlock() []byte {
	if v := p.pool.Get(); v != nil {
		return v.([]byte)
	}
	return make([]byte, p.size)
}

// PutBlock releases a byte slice to pool
func (p *FixedPool) PutBlock(block []byte) {
	// just ignore block with mismatched size
	if len(block) != p.size {
		return
	}
	p.pool.Put(block)
}

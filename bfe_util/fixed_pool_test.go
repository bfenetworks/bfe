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
	"testing"
)

func TestFixedPool(t *testing.T) {
	p := NewFixedPool(1024)

	b1 := make([]byte, 1024)
	b2 := make([]byte, 2048)
	p.PutBlock(b1)
	p.PutBlock(b2)

	b3 := p.GetBlock()
	if len(b3) != 1024 {
		t.Errorf("GetBlock() error")
	}
	b4 := p.GetBlock()
	if len(b4) != 1024 {
		t.Errorf("GetBlock() error")
	}
}

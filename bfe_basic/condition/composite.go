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

// composite condition implementation

package condition

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition/parser"
)

// UnaryCond is unary condition for !cond
type UnaryCond struct {
	op   parser.Token
	cond Condition
}

func (uc *UnaryCond) Match(req *bfe_basic.Request) bool {
	switch uc.op {
	case parser.NOT:
		return !uc.cond.Match(req)
	default:
		return false
	}
}

// BinaryCond is binary condition for lc&&rc , lc||rc
type BinaryCond struct {
	op parser.Token
	lc Condition
	rc Condition
}

func (bc *BinaryCond) Match(req *bfe_basic.Request) bool {
	switch bc.op {
	case parser.LAND:
		return bc.lc.Match(req) && bc.rc.Match(req)
	case parser.LOR:
		return bc.lc.Match(req) || bc.rc.Match(req)
	default:
		return false
	}
}

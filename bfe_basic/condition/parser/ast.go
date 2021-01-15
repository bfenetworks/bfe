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

// ast node for condition expression

package parser

import (
	"bytes"
	"go/token"
	"strconv"
	"strings"
)

type Node interface {
	Pos() token.Pos
	End() token.Pos
}

type Expr interface {
	Node
	//    exprNode()
}

type BinaryExpr struct {
	X  Expr
	Op Token
	Y  Expr
}

type UnaryExpr struct {
	X     Expr
	Op    Token
	OpPos token.Pos
}

type Ident struct {
	Name    string
	NamePos token.Pos
}

type BasicLit struct {
	Kind     Token
	Value    string
	ValuePos token.Pos
}

type CallExpr struct {
	Fun    *Ident
	Args   BasicLitList
	Rparen token.Pos
}

type ParenExpr struct {
	X Expr
}

func (c CallExpr) String() string {
	var b bytes.Buffer

	b.WriteString(c.Fun.Name)
	b.WriteString("(")

	var strArgs []string
	for _, arg := range c.Args {
		if arg.Kind == STRING {
			strArgs = append(strArgs, strconv.Quote(arg.Value))
		} else {
			strArgs = append(strArgs, arg.Value)
		}
	}

	b.WriteString(strings.Join(strArgs, ","))
	b.WriteString(")")

	return b.String()

}

type BasicLitList []*BasicLit

func (b *BinaryExpr) Pos() token.Pos {
	return b.X.Pos()
}

func (b *BinaryExpr) End() token.Pos {
	return b.Y.End()
}

func (u *UnaryExpr) Pos() token.Pos {
	return u.OpPos
}

func (u *UnaryExpr) End() token.Pos {
	return u.X.End()
}

func (id *Ident) Pos() token.Pos {
	return id.NamePos
}

func (id *Ident) End() token.Pos {
	return token.Pos(int(id.NamePos) + len(id.Name))
}

func (b *BasicLit) Pos() token.Pos {
	return b.ValuePos
}

func (b *BasicLit) End() token.Pos {
	return token.Pos(int(b.ValuePos) + len(b.Value))
}

func (c *CallExpr) Pos() token.Pos {
	return c.Fun.Pos()
}

func (c *CallExpr) End() token.Pos {
	return c.Rparen
}

func (b BasicLitList) Pos() token.Pos {
	return b[0].Pos()
}

func (b BasicLitList) End() token.Pos {
	return b[len(b)].End()
}

func (p ParenExpr) Pos() token.Pos {
	return p.X.Pos()
}

func (p ParenExpr) End() token.Pos {
	return p.X.End()
}

func (b *BasicLit) ToBool() bool {
	if b.Kind != BOOL {
		return false
	}

	return strings.ToUpper(b.Value) == "TRUE"
}

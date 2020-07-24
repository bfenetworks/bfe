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

// visitor over ast nodes

package parser

// Visitor wraps the Visit method which is invoked for each node encountered by Walk.
// If the result visitor w is not nil, Walk visits each of the children
// of node with the visitor w, followed by a call of w.Visit(nil).
type Visitor interface {
	Visit(node Node) (w Visitor)
}

func Walk(v Visitor, node Node) {
	if v = v.Visit(node); v == nil {
		return
	}

	switch n := node.(type) {
	case *BinaryExpr:
		// semant check for X,Y
		Walk(v, n.X)
		Walk(v, n.Y)
	case *UnaryExpr:
		// semant check for X
		Walk(v, n.X)
	case *Ident:
		// do nothing
	case *CallExpr:
		Walk(v, n.Fun)
		Walk(v, n.Args)
	case *ParenExpr:
		Walk(v, n.X)
	}
}

type inspector func(Node) bool

func (f inspector) Visit(node Node) Visitor {
	if f(node) {
		return f
	}
	return nil
}

// Inspect traverses an AST in depth-first order: It starts by calling
// f(node); node must not be nil. If f returns true, Inspect invokes f
// for all the non-nil children of node, recursively.
//
func Inspect(node Node, f func(Node) bool) {
	Walk(inspector(f), node)
}

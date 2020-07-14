//go:generate goyacc -p cond cond.y

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

// parser for condition expression

package parser

import (
	"fmt"
	"go/token"
	"strings"
	"sync"
)

// ErrorHandler is error handler for scanner and lexer
type ErrorHandler func(pos token.Pos, msg string)

type Parser struct {
	fset    *token.FileSet
	scanner Scanner
	lexer   *condLex

	identList []*Ident
	errors    []Error
	ast       Node
}

type Error struct {
	pos token.Position
	msg string
}

func (e Error) Error() string {
	return fmt.Sprintf("%s %s", e.pos, e.msg)
}

func (p *Parser) Init(src []byte) {
	p.fset = token.NewFileSet()
	p.errors = p.errors[0:0]
	p.identList = p.identList[0:0]

	file := p.fset.AddFile("", p.fset.Base(), len(src))
	p.scanner.Init(file, src, p.addError)
	p.lexer = &condLex{
		s:   &p.scanner,
		err: p.addError,
	}
}

func (p *Parser) addError(pos token.Pos, msg string) {
	p.errors = append(p.errors, Error{pos: p.fset.Position(pos), msg: msg})
}

// Error return first error.
func (p *Parser) Error() error {
	if len(p.errors) == 0 {
		return nil
	}

	return p.errors[0]
}

var parseLock sync.Mutex

func (p *Parser) Parse() {
	parseLock.Lock()
	defer parseLock.Unlock()

	// Note: y.go:condParse() use global variable for parsed nodes.
	condParse(p.lexer)
	p.ast = parseNode

	if len(p.errors) > 0 {
		return
	}

	// colllect all variables
	Inspect(p.ast, p.collectVariable)

	// static check for all call expr
	Inspect(p.ast, p.primitiveCheck)
}

// String returns string representation of parsed variables and errors.
func (p Parser) String() string {
	var variables []string

	for _, ident := range p.identList {
		variables = append(variables, ident.Name)
	}

	var errors []string

	for _, err := range p.errors {
		errors = append(errors, err.Error())
	}

	return "names: " + strings.Join(variables, ",") + "\terrors: " + strings.Join(errors, ",")
}

// Parse parse given condition string.
//
// condStr: input condition expression
// err : err is not nil if parse error(including scan, lexer, prototype check)
// idents: if err is nil, all conditionVariable is list in idents ([]*Ident)
// node: if err is nil, parsed ast is returned by node
func Parse(condStr string) (Node, []*Ident, error) {
	var p Parser

	p.Init([]byte(condStr))
	p.Parse()

	if err := p.Error(); err != nil {
		return nil, nil, err
	}

	return p.ast, p.identList, nil
}

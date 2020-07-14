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

// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.


%{

package parser

import (
	"fmt"
	"go/token"
)

%}

%union {
	Node Node
	str	string
}

%token IDENT LAND LOR LPAREN RPAREN NOT SEMICOLON BASICLIT COMMA BOOL STRING INT FLOAT IMAG COMMENT ILLEGAL

%left LAND
%left LOR
%right NOT	
%%

top:
   	expr
	{
		parseNode = $1.Node
	}
expr:
	LPAREN expr RPAREN
	{
		$$.Node = &ParenExpr{$2.Node.(Expr)}

	}
|	expr LAND expr
	{
		$$.Node = &BinaryExpr{$1.Node.(Expr), LAND, $3.Node.(Expr)}
	}
|   expr LOR expr
	{
		$$.Node = &BinaryExpr{$1.Node.(Expr), LOR, $3.Node.(Expr)}
	}
|	NOT expr
	{
		$$.Node = &UnaryExpr{$2.Node.(Expr), NOT, lastTokenPos}
	}
|	callExpr
	{
		$$.Node = $1.Node
	}
|	IDENT
	{
		$$.Node = $1.Node
	}

callExpr:
	IDENT LPAREN paramlist RPAREN
	{
		$$.Node = &CallExpr{$1.Node.(*Ident), $3.Node.(BasicLitList), lastPos}
	}
|   IDENT LPAREN RPAREN
    {
        $$.Node = &CallExpr{$1.Node.(*Ident), nil, lastPos}
    }

paramlist:
	BASICLIT
	{
		$$.Node = BasicLitList{$1.Node.(*BasicLit)}
	}
|	paramlist COMMA BASICLIT
	{
		$$.Node = append($1.Node.(BasicLitList), $3.Node.(*BasicLit))
	}


%%

// The parser expects the lexer to return 0 on EOF.  Give it a name
// for clarity.
const EOF = 0

var (
    parseNode Node	// save parse node
    lastPos      token.Pos
	lastTokenPos token.Pos
)

// The parser uses the type <prefix>Lex as a lexer.  It must provide
// the methods Lex(*<prefix>SymType) int and Error(string).
type condLex struct {
	s   *Scanner
	err ErrorHandler
}

// The parser calls this method to get each new token.
func (x *condLex) Lex(yylval *condSymType) int {
	for {
		pos, tok, lit := x.s.Scan()

		lastPos = pos

		// fmt.Printf("got token %s %s\n", tok, lit)
		switch tok {
		case EOF:
			return EOF
		case IDENT:
			yylval.Node = &Ident{Name:lit, NamePos: pos}
			return IDENT
		case BOOL, STRING, INT:
			yylval.Node = &BasicLit{Kind:tok, Value:lit, ValuePos: pos}
			return BASICLIT 
		case LPAREN, RPAREN, LAND, LOR, SEMICOLON, COMMA, NOT:
			lastTokenPos = pos
			return int(tok)
		default:
			x.Error(fmt.Sprintf("unrecognized token %d", tok))
			return EOF
		}
	}
}

// The parser calls this method on a parse error.
func (x *condLex) Error(s string) {
	if x.err != nil {
		x.err(lastPos, s)
	}
}


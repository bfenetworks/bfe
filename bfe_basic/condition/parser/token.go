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

// token utility for condition expression

package parser

type Token int

var keywords = []string{
	"break",
	"case",
	"chan",
	"const",
	"continue",

	"default",
	"defer",
	"else",
	"fallthrough",
	"for",

	"func",
	"go",
	"goto",
	"if",
	"import",

	"interface",
	"map",
	"package",
	"range",
	"return",

	"select",
	"struct",
	"switch",
	"type",
	"var",
}

var tokens = map[Token]string{
	IDENT:     "IDENT",
	LAND:      "LAND",
	LOR:       "LOR",
	LPAREN:    "LPAREN",
	RPAREN:    "RPAREN",
	NOT:       "NOT",
	SEMICOLON: "SEMICOLON",
	BASICLIT:  "BASICLIT",
	COMMA:     "COMMA",
	BOOL:      "BOOL",
	STRING:    "STRING",
	INT:       "INT",
	FLOAT:     "FLOAT",
	IMAG:      "IMAG",
	COMMENT:   "COMMENT",
	ILLEGAL:   "ILLEGAL",
	EOF:       "EOF",
}

var symbols = map[Token]string{
	IDENT:     "",
	LAND:      "&&",
	LOR:       "||",
	LPAREN:    "(",
	RPAREN:    ")",
	NOT:       "!",
	SEMICOLON: ";",
	BASICLIT:  "BASICLIT",
	COMMA:     ",",
	BOOL:      "BOOL",
	STRING:    "STRING",
	INT:       "INT",
	FLOAT:     "FLOAT",
	IMAG:      "IMAG",
	COMMENT:   "//",
	ILLEGAL:   "ILLEGAL",
	EOF:       "EOF",
}

func (t Token) Symbol() string {
	return symbols[t]
}

func (t Token) String() string {
	return tokens[t]
}

func Lookup(ident string) Token {
	for _, keyword := range keywords {
		if ident == keyword {
			// reserved
			return ILLEGAL
		}
	}
	return IDENT
}

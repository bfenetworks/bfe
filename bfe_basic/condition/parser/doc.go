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

// package provider parser to parse condition
//
//     node, idents, err := parser.Parse("a && b && c")
//
//     err
//     // condStr: input condition expression: err is not nil if parse error(including scan, lexer, prototype check)
//     idents: if err is nil, all conditionVariable is list in idents ([]*Ident)
//     node: if err is nil, parsed ast is returned by node

package parser

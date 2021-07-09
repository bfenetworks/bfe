// Copyright (c) 2019 Baidu, Inc.
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

package bfe_discovery

import (
	"strings"
)

var DefaultDecorators = []Decorator{DecoToLower, DecoPrefix}

type Decorator func(string) string

func Decorate(key string, ds ...Decorator) string {
	for _, decorator := range ds {
		key = decorator(key)
	}
	return key
}

var DecoToUpper Decorator = strings.ToUpper

var DecoToLower Decorator = strings.ToLower

func DecoPrefix(key string) string {
	if !strings.HasPrefix(key, Prefix) {
		return Prefix + key
	}
	return key
}

func DecoSuffix(key string) string {
	if !strings.HasSuffix(key, Suffix) {
		key += Suffix
	}
	return key
}

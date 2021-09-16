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

package string_reverse

// ReverseFqdnHost reverse host.
// i.e.: www.baidu.com news.baidu.com -> moc.udiab.swen moc.udiab.www will have same prefix
func ReverseFqdnHost(host string) string {
	r := []rune(host)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}

	if len(r) > 0 && r[0] == '.' {
		r = r[1:]
	}

	return string(r)
}

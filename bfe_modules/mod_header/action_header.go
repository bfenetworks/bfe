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

package mod_header

import (
	"github.com/bfenetworks/bfe/bfe_http"
)

// insert or modify existing header
func headerSet(h *bfe_http.Header, key string, value string) {
	h.Set(key, value)
}

// append #value to existing header or insert a new one
func headerAdd(h *bfe_http.Header, key string, value string) {
	h.Add(key, value)
}

// delete header specified by key
func headerDel(h *bfe_http.Header, key string) {
	h.Del(key)
}

// rename header originalKey to newKey
func headerRename(h *bfe_http.Header, originalKey, newKey string) {
	val := h.Get(originalKey)
	h.Set(newKey, val)
	h.Del(originalKey)
}

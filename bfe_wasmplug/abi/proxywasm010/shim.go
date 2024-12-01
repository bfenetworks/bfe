/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package proxywasm010

import (
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/proxy-wasm-go-host/proxywasm/common"
)

// HeaderMapWrapper wraps api.HeaderMap into proxy-wasm-go-host/common.HeaderMap
// implement common.HeaderMap
type HeaderMapWrapper struct {
	bfe_http.Header
}

// Override
func (h HeaderMapWrapper) Get(key string) (string, bool) {
	s := h.Header.Get(key)
	if s == "" {
		return "", false
	} else {
		return s, true
	}
}

func (h HeaderMapWrapper) Range(f func(key, value string) bool) {
	stopped := false
	for k, v := range h.Header {
		if stopped {
			return 
		}
		stopped = !f(k, v[0])
	}
}

func (h HeaderMapWrapper) ByteSize() uint64 {
	// TODO: to implement
	return 0
}

func (h HeaderMapWrapper) Clone() common.HeaderMap {
	return &HeaderMapWrapper{h.Header}
}

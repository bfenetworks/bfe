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

// Copyright 2010 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bfe_server

import (
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_http"
)

func TestNeedsSniff(t *testing.T) {
	// needsSniff returns true with an empty response.
	r := &response{}
	if got, want := r.needsSniff(), true; got != want {
		t.Errorf("needsSniff = %t; want %t", got, want)
	}
	// needsSniff returns false when Content-Type = nil.
	r.handlerHeader = bfe_http.Header{"Content-Type": nil}
	if got, want := r.needsSniff(), false; got != want {
		t.Errorf("needsSniff empty Content-Type = %t; want %t", got, want)
	}
}

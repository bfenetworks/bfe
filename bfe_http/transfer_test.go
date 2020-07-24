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

// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bfe_http

import (
	"strings"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_bufio"
)

func TestBodyReadBadTrailer(t *testing.T) {
	b := &body{
		src: strings.NewReader("foobar"),
		hdr: true, // force reading the trailer
		r:   bfe_bufio.NewReader(strings.NewReader("")),
	}
	buf := make([]byte, 7)
	n, err := b.Read(buf[:3])
	got := string(buf[:n])
	if got != "foo" || err != nil {
		t.Fatalf(`first Read = %d (%q), %v; want 3 ("foo")`, n, got, err)
	}

	n, err = b.Read(buf[:])
	got = string(buf[:n])
	if got != "bar" || err != nil {
		t.Fatalf(`second Read = %d (%q), %v; want 3 ("bar")`, n, got, err)
	}

	n, err = b.Read(buf[:])
	got = string(buf[:n])
	if err == nil {
		t.Errorf("final Read was successful (%q), expected error from trailer read", got)
	}
}

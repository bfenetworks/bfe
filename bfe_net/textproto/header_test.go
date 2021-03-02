// Copyright (c) 2021 The BFE Authors.
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

// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package textproto

import "testing"

type canonicalHeaderKeyTest struct {
	in, out string
}

var canonicalHeaderKeyTests = []canonicalHeaderKeyTest{
	{"a-b-c", "A-B-C"},
	{"a-1-c", "A-1-C"},
	{"User-Agent", "User-Agent"},
	{"uSER-aGENT", "User-Agent"},
	{"user-agent", "User-Agent"},
	{"USER-AGENT", "User-Agent"},

	// Other valid tchar bytes in tokens:
	{"foo-bar_baz", "Foo-Bar_baz"},
	{"foo-bar$baz", "Foo-Bar$baz"},
	{"foo-bar~baz", "Foo-Bar~baz"},
	{"foo-bar*baz", "Foo-Bar*baz"},

	// Non-ASCII or anything with spaces or non-token chars is unchanged:
	{"üser-agenT", "üser-agenT"},
	{"a B", "a B"},

	// This caused a panic due to mishandling of a space:
	{"C Ontent-Transfer-Encoding", "C Ontent-Transfer-Encoding"},
	{"foo bar", "foo bar"},
}

func TestCanonicalMIMEHeaderKey(t *testing.T) {
	for _, tt := range canonicalHeaderKeyTests {
		if s := CanonicalMIMEHeaderKey(tt.in); s != tt.out {
			t.Errorf("CanonicalMIMEHeaderKey(%q) = %q, want %q", tt.in, s, tt.out)
		}
	}
}

// Issue #34799 add a Header method to get multiple values []string, with canonicalized key
func TestMIMEHeaderMultipleValues(t *testing.T) {
	testHeader := MIMEHeader{
		"Set-Cookie": {"cookie 1", "cookie 2"},
	}
	values := testHeader.Values("set-cookie")
	n := len(values)
	if n != 2 {
		t.Errorf("count: %d; want 2", n)
	}
}

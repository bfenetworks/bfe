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

package bfe_http

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

var writeSetCookiesTests = []struct {
	Cookie *Cookie
	Raw    string
}{
	{
		&Cookie{Name: "cookie-1", Value: "v$1"},
		"cookie-1=v$1",
	},
	{
		&Cookie{Name: "cookie-2", Value: "two", MaxAge: 3600},
		"cookie-2=two; Max-Age=3600",
	},
	{
		&Cookie{Name: "cookie-3", Value: "three", Domain: ".example.com"},
		"cookie-3=three; Domain=example.com",
	},
	{
		&Cookie{Name: "cookie-4", Value: "four", Path: "/restricted/"},
		"cookie-4=four; Path=/restricted/",
	},
	{
		&Cookie{Name: "cookie-5", Value: "five", Domain: "wrong;bad.abc"},
		"cookie-5=five",
	},
	{
		&Cookie{Name: "cookie-6", Value: "six", Domain: "bad-.abc"},
		"cookie-6=six",
	},
	{
		&Cookie{Name: "cookie-7", Value: "seven", Domain: "127.0.0.1"},
		"cookie-7=seven; Domain=127.0.0.1",
	},
	{
		&Cookie{Name: "cookie-8", Value: "eight", Domain: "::1"},
		"cookie-8=eight",
	},
	// According to IETF 6265 Section 5.1.1.5, the year cannot be less than 1601
	{
		&Cookie{Name: "cookie-10", Value: "expiring-1601", Expires: time.Date(1601, 1, 1, 1, 1, 1, 1, time.UTC)},
		"cookie-10=expiring-1601; Expires=Mon, 01 Jan 1601 01:01:01 GMT",
	},
	{
		&Cookie{Name: "cookie-11", Value: "invalid-expiry", Expires: time.Date(1600, 1, 1, 1, 1, 1, 1, time.UTC)},
		"cookie-11=invalid-expiry",
	},
	{
		&Cookie{Name: "cookie-12", Value: "samesite-default", SameSite: SameSiteDefaultMode},
		"cookie-12=samesite-default; SameSite",
	},
	{
		&Cookie{Name: "cookie-13", Value: "samesite-lax", SameSite: SameSiteLaxMode},
		"cookie-13=samesite-lax; SameSite=Lax",
	},
	{
		&Cookie{Name: "cookie-14", Value: "samesite-strict", SameSite: SameSiteStrictMode},
		"cookie-14=samesite-strict; SameSite=Strict",
	},
	{
		&Cookie{Name: "cookie-15", Value: "samesite-none", SameSite: SameSiteNoneMode},
		"cookie-15=samesite-none; SameSite=None",
	},
}

func TestWriteSetCookies(t *testing.T) {
	for i, tt := range writeSetCookiesTests {
		if g, e := tt.Cookie.String(), tt.Raw; g != e {
			t.Errorf("Test %d, expecting:\n%s\nGot:\n%s\n", i, e, g)
			continue
		}
	}
}

type headerOnlyResponseWriter Header

func (ho headerOnlyResponseWriter) Header() Header {
	return Header(ho)
}

func (ho headerOnlyResponseWriter) Write([]byte) (int, error) {
	panic("NOIMPL")
}

func (ho headerOnlyResponseWriter) WriteHeader(int) {
	panic("NOIMPL")
}

func TestSetCookie(t *testing.T) {
	m := make(Header)
	SetCookie(headerOnlyResponseWriter(m), &Cookie{Name: "cookie-1", Value: "one", Path: "/restricted/"})
	SetCookie(headerOnlyResponseWriter(m), &Cookie{Name: "cookie-2", Value: "two", MaxAge: 3600})
	if l := len(m["Set-Cookie"]); l != 2 {
		t.Fatalf("expected %d cookies, got %d", 2, l)
	}
	if g, e := m["Set-Cookie"][0], "cookie-1=one; Path=/restricted/"; g != e {
		t.Errorf("cookie #1: want %q, got %q", e, g)
	}
	if g, e := m["Set-Cookie"][1], "cookie-2=two; Max-Age=3600"; g != e {
		t.Errorf("cookie #2: want %q, got %q", e, g)
	}
}

var addCookieTests = []struct {
	Cookies []*Cookie
	Raw     string
}{
	{
		[]*Cookie{},
		"",
	},
	{
		[]*Cookie{{Name: "cookie-1", Value: "v$1"}},
		"cookie-1=v$1",
	},
	{
		[]*Cookie{
			{Name: "cookie-1", Value: "v$1"},
			{Name: "cookie-2", Value: "v$2"},
			{Name: "cookie-3", Value: "v$3"},
		},
		"cookie-1=v$1; cookie-2=v$2; cookie-3=v$3",
	},
}

func TestAddCookie(t *testing.T) {
	for i, tt := range addCookieTests {
		req, _ := NewRequest(MethodGet, "http://example.com/", nil)
		for _, c := range tt.Cookies {
			req.AddCookie(c)
		}
		if g := req.Header.Get("Cookie"); g != tt.Raw {
			t.Errorf("Test %d:\nwant: %s\n got: %s\n", i, tt.Raw, g)
			continue
		}
	}
}

var readSetCookiesTests = []struct {
	Header  Header
	Cookies []*Cookie
}{
	{
		Header{"Set-Cookie": {"Cookie-1=v$1"}},
		[]*Cookie{{Name: "Cookie-1", Value: "v$1", Raw: "Cookie-1=v$1"}},
	},
	{
		Header{"Set-Cookie": {"NID=99=YsDT5i3E-CXax-; expires=Wed, 23-Nov-2011 01:05:03 GMT; path=/; domain=.google.ch; HttpOnly"}},
		[]*Cookie{{
			Name:       "NID",
			Value:      "99=YsDT5i3E-CXax-",
			Path:       "/",
			Domain:     ".google.ch",
			HttpOnly:   true,
			Expires:    time.Date(2011, 11, 23, 1, 5, 3, 0, time.UTC),
			RawExpires: "Wed, 23-Nov-2011 01:05:03 GMT",
			Raw:        "NID=99=YsDT5i3E-CXax-; expires=Wed, 23-Nov-2011 01:05:03 GMT; path=/; domain=.google.ch; HttpOnly",
		}},
	},
	{
		Header{"Set-Cookie": {".ASPXAUTH=7E3AA; expires=Wed, 07-Mar-2012 14:25:06 GMT; path=/; HttpOnly"}},
		[]*Cookie{{
			Name:       ".ASPXAUTH",
			Value:      "7E3AA",
			Path:       "/",
			Expires:    time.Date(2012, 3, 7, 14, 25, 6, 0, time.UTC),
			RawExpires: "Wed, 07-Mar-2012 14:25:06 GMT",
			HttpOnly:   true,
			Raw:        ".ASPXAUTH=7E3AA; expires=Wed, 07-Mar-2012 14:25:06 GMT; path=/; HttpOnly",
		}},
	},
	{
		Header{"Set-Cookie": {"ASP.NET_SessionId=foo; path=/; HttpOnly"}},
		[]*Cookie{{
			Name:     "ASP.NET_SessionId",
			Value:    "foo",
			Path:     "/",
			HttpOnly: true,
			Raw:      "ASP.NET_SessionId=foo; path=/; HttpOnly",
		}},
	},
	{
		Header{"Set-Cookie": {"samesitedefault=foo; SameSite"}},
		[]*Cookie{{
			Name:     "samesitedefault",
			Value:    "foo",
			SameSite: SameSiteDefaultMode,
			Raw:      "samesitedefault=foo; SameSite",
		}},
	},
	{
		Header{"Set-Cookie": {"samesitelax=foo; SameSite=Lax"}},
		[]*Cookie{{
			Name:     "samesitelax",
			Value:    "foo",
			SameSite: SameSiteLaxMode,
			Raw:      "samesitelax=foo; SameSite=Lax",
		}},
	},
	{
		Header{"Set-Cookie": {"samesitestrict=foo; SameSite=Strict"}},
		[]*Cookie{{
			Name:     "samesitestrict",
			Value:    "foo",
			SameSite: SameSiteStrictMode,
			Raw:      "samesitestrict=foo; SameSite=Strict",
		}},
	},
	{
		Header{"Set-Cookie": {"samesitenone=foo; SameSite=None"}},
		[]*Cookie{{
			Name:     "samesitenone",
			Value:    "foo",
			SameSite: SameSiteNoneMode,
			Raw:      "samesitenone=foo; SameSite=None",
		}},
	},
	{
		Header{"Set-Cookie": {"SID=XXX; expires=Thu, 18-Feb-21 06:59:27 GMT; max-age=31536000; path=/; domain=.test.com; version=1"}},
		[]*Cookie{{
			Name:       "SID",
			Value:      "XXX",
			Expires:    time.Date(2021, time.February, 18, 6, 59, 27, 0, time.UTC),
			RawExpires: "Thu, 18-Feb-21 06:59:27 GMT",
			MaxAge:     31536000,
			Path:       "/",
			Domain:     ".test.com",
			Unparsed:   []string{"version=1"},
			Raw:        "SID=XXX; expires=Thu, 18-Feb-21 06:59:27 GMT; max-age=31536000; path=/; domain=.test.com; version=1",
		}},
	},
	{
		Header{"Set-Cookie": {"STOKEN=xxx; expires=Tue, 25-Apr-2028 08:07:15 GMT; path=/; domain=test2.com; secure; httponly"}},
		[]*Cookie{{
			Name:       "STOKEN",
			Value:      "xxx",
			Expires:    time.Date(2028, time.April, 25, 8, 7, 15, 0, time.UTC),
			RawExpires: "Tue, 25-Apr-2028 08:07:15 GMT",
			Path:       "/",
			Domain:     "test2.com",
			HttpOnly:   true,
			Secure:     true,
			Raw:        "STOKEN=xxx; expires=Tue, 25-Apr-2028 08:07:15 GMT; path=/; domain=test2.com; secure; httponly",
		}},
	},
	// TODO(bradfitz): users have reported seeing this in the
	// wild, but do browsers handle it? RFC 6265 just says "don't
	// do that" (section 3) and then never mentions header folding
	// again.
	// Header{"Set-Cookie": {"ASP.NET_SessionId=foo; path=/; HttpOnly, .ASPXAUTH=7E3AA; expires=Wed, 07-Mar-2012 14:25:06 GMT; path=/; HttpOnly"}},
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%#v", v)
	}
	return string(b)
}

func TestReadSetCookies(t *testing.T) {
	for i, tt := range readSetCookiesTests {
		for n := 0; n < 2; n++ { // to verify readSetCookies doesn't mutate its input
			c := readSetCookies(tt.Header)
			if !reflect.DeepEqual(c, tt.Cookies) {
				t.Errorf("#%d readSetCookies: have\n%s\nwant\n%s\n", i, toJSON(c), toJSON(tt.Cookies))
				continue
			}
		}
	}
}

var readCookiesTests = []struct {
	Header  Header
	Filter  string
	Cookies []*Cookie
}{
	{
		Header{"Cookie": {"Cookie-1=v$1", "c2=v2"}},
		"",
		[]*Cookie{
			{Name: "Cookie-1", Value: "v$1"},
			{Name: "c2", Value: "v2"},
		},
	},
	{
		Header{"Cookie": {"Cookie-1=v$1", "c2=v2"}},
		"c2",
		[]*Cookie{
			{Name: "c2", Value: "v2"},
		},
	},
	{
		Header{"Cookie": {"Cookie-1=v$1; c2=v2"}},
		"",
		[]*Cookie{
			{Name: "Cookie-1", Value: "v$1"},
			{Name: "c2", Value: "v2"},
		},
	},
	{
		Header{"Cookie": {"Cookie-1=v$1; c2=v2"}},
		"c2",
		[]*Cookie{
			{Name: "c2", Value: "v2"},
		},
	},
}

func TestReadCookies(t *testing.T) {
	for i, tt := range readCookiesTests {
		for n := 0; n < 2; n++ { // to verify readCookies doesn't mutate its input
			c := readCookies(tt.Header, tt.Filter)
			if !reflect.DeepEqual(c, tt.Cookies) {
				t.Errorf("#%d readCookies:\nhave: %s\nwant: %s\n", i, toJSON(c), toJSON(tt.Cookies))
				continue
			}
		}
	}
}

func TestCookieSanitizeValue(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"foo", "foo"},
		{"foo bar", "foobar"},
		{"\x00\x7e\x7f\x80", "\x7e"},
		{`"withquotes"`, "withquotes"},
	}
	for _, tt := range tests {
		if got := sanitizeCookieValue(tt.in); got != tt.want {
			t.Errorf("sanitizeCookieValue(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func TestCookieSanitizePath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/path", "/path"},
		{"/path with space/", "/path with space/"},
		{"/just;no;semicolon\x00orstuff/", "/justnosemicolonorstuff/"},
	}
	for _, tt := range tests {
		if got := sanitizeCookiePath(tt.in); got != tt.want {
			t.Errorf("sanitizeCookiePath(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func TestDisableSanitize(t *testing.T) {
	SetDisableSanitize(true)
	tests := []struct {
		in, want string
	}{
		{"foo", "foo"},
		{"foo bar", "foo bar"},
		{"\x00\x7e\x7f\x80", "\x00\x7e\x7f\x80"},
		{`"withquotes"`, "\"withquotes\""},
	}
	for _, tt := range tests {
		if got, _ := parseCookieValue(tt.in); got != tt.want {
			t.Errorf("after SetDisableSanitize, parseCookieValue(%q) = %q; want %q", tt.in, got, tt.want)
		}
		if got := sanitizeCookieValue(tt.in); got != tt.want {
			t.Errorf("after SetDisableSanitize, sanitizeCookieValue(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func BenchmarkCookieString(b *testing.B) {
	const wantCookieString = `cookie-9=i3e01nf61b6t23bvfmplnanol3; Path=/restricted/; Domain=example.com; Expires=Tue, 10 Nov 2009 23:00:00 GMT; Max-Age=3600`
	c := &Cookie{
		Name:    "cookie-9",
		Value:   "i3e01nf61b6t23bvfmplnanol3",
		Expires: time.Unix(1257894000, 0),
		Path:    "/restricted/",
		Domain:  ".example.com",
		MaxAge:  3600,
	}
	var benchmarkCookieString string
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkCookieString = c.String()
	}
	if have, want := benchmarkCookieString, wantCookieString; have != want {
		b.Fatalf("Have: %v Want: %v", have, want)
	}
}

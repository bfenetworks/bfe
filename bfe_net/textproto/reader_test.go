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

package textproto

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_bufio"
)

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

func reader(s string) *Reader {
	return NewReader(bfe_bufio.NewReader(strings.NewReader(s)))
}

func TestReadLine(t *testing.T) {
	r := reader("line1\nline2\n")
	s, err := r.ReadLine()
	if s != "line1" || err != nil {
		t.Fatalf("Line 1: %s, %v", s, err)
	}
	s, err = r.ReadLine()
	if s != "line2" || err != nil {
		t.Fatalf("Line 2: %s, %v", s, err)
	}
	s, err = r.ReadLine()
	if s != "" || err != io.EOF {
		t.Fatalf("EOF: %s, %v", s, err)
	}
}

func TestReadContinuedLine(t *testing.T) {
	r := reader("line1\nline\n 2\nline3\n")
	s, err := r.ReadContinuedLine()
	if s != "line1" || err != nil {
		t.Fatalf("Line 1: %s, %v", s, err)
	}
	s, err = r.ReadContinuedLine()
	if s != "line 2" || err != nil {
		t.Fatalf("Line 2: %s, %v", s, err)
	}
	s, err = r.ReadContinuedLine()
	if s != "line3" || err != nil {
		t.Fatalf("Line 3: %s, %v", s, err)
	}
	s, err = r.ReadContinuedLine()
	if s != "" || err != io.EOF {
		t.Fatalf("EOF: %s, %v", s, err)
	}
}

func TestReadCodeLine(t *testing.T) {
	r := reader("123 hi\n234 bye\n345 no way\n")
	code, msg, err := r.ReadCodeLine(0)
	if code != 123 || msg != "hi" || err != nil {
		t.Fatalf("Line 1: %d, %s, %v", code, msg, err)
	}
	code, msg, err = r.ReadCodeLine(23)
	if code != 234 || msg != "bye" || err != nil {
		t.Fatalf("Line 2: %d, %s, %v", code, msg, err)
	}
	code, msg, err = r.ReadCodeLine(346)
	if code != 345 || msg != "no way" || err == nil {
		t.Fatalf("Line 3: %d, %s, %v", code, msg, err)
	}
	if e, ok := err.(*Error); !ok || e.Code != code || e.Msg != msg {
		t.Fatalf("Line 3: wrong error %v\n", err)
	}
	code, msg, err = r.ReadCodeLine(1)
	if code != 0 || msg != "" || err != io.EOF {
		t.Fatalf("EOF: %d, %s, %v", code, msg, err)
	}
}

func TestReadDotLines(t *testing.T) {
	r := reader("dotlines\r\n.foo\r\n..bar\n...baz\nquux\r\n\r\n.\r\nanother\n")
	s, err := r.ReadDotLines()
	want := []string{"dotlines", "foo", ".bar", "..baz", "quux", ""}
	if !reflect.DeepEqual(s, want) || err != nil {
		t.Fatalf("ReadDotLines: %v, %v", s, err)
	}

	s, err = r.ReadDotLines()
	want = []string{"another"}
	if !reflect.DeepEqual(s, want) || err != io.ErrUnexpectedEOF {
		t.Fatalf("ReadDotLines2: %v, %v", s, err)
	}
}

func TestReadDotBytes(t *testing.T) {
	r := reader("dotlines\r\n.foo\r\n..bar\n...baz\nquux\r\n\r\n.\r\nanot.her\r\n")
	b, err := r.ReadDotBytes()
	want := []byte("dotlines\nfoo\n.bar\n..baz\nquux\n\n")
	if !reflect.DeepEqual(b, want) || err != nil {
		t.Fatalf("ReadDotBytes: %q, %v", b, err)
	}

	b, err = r.ReadDotBytes()
	want = []byte("anot.her\n")
	if !reflect.DeepEqual(b, want) || err != io.ErrUnexpectedEOF {
		t.Fatalf("ReadDotBytes2: %q, %v", b, err)
	}
}

func TestReadMIMEHeader(t *testing.T) {
	r := reader("my-key: Value 1  \r\nLong-key: Even \n Longer Value\r\nmy-Key: Value 2\r\n\n")
	m, err := r.ReadMIMEHeader()
	want := MIMEHeader{
		"My-Key":   {"Value 1", "Value 2"},
		"Long-Key": {"Even Longer Value"},
	}
	if !reflect.DeepEqual(m, want) || err != nil {
		t.Fatalf("ReadMIMEHeader: %v, %v; want %v", m, err, want)
	}
}

func TestReadMIMEHeaderSingle(t *testing.T) {
	r := reader("Foo: bar\n\n")
	m, err := r.ReadMIMEHeader()
	want := MIMEHeader{"Foo": {"bar"}}
	if !reflect.DeepEqual(m, want) || err != nil {
		t.Fatalf("ReadMIMEHeader: %v, %v; want %v", m, err, want)
	}
}

func TestReadMIMEHeaderNoKey(t *testing.T) {
	r := reader(": bar\ntest-1: 1\n\n")
	m, err := r.ReadMIMEHeader()
	want := MIMEHeader{"Test-1": {"1"}}
	if !reflect.DeepEqual(m, want) || err != nil {
		t.Fatalf("ReadMIMEHeader: %v, %v; want %v", m, err, want)
	}
}
func TestLargeReadMIMEHeader(t *testing.T) {
	data := make([]byte, 16*1024)
	for i := 0; i < len(data); i++ {
		data[i] = 'x'
	}
	sdata := string(data)
	r := reader("Cookie: " + sdata + "\r\n\n")
	m, err := r.ReadMIMEHeader()
	if err != nil {
		t.Fatalf("ReadMIMEHeader: %v", err)
	}
	cookie := m.Get("Cookie")
	if cookie != sdata {
		t.Fatalf("ReadMIMEHeader: %v bytes, want %v bytes", len(cookie), len(sdata))
	}
}

// TestReadMIMEHeaderNonCompliant checks that we don't normalize headers
// with spaces before colons, and accept spaces in keys.
func TestReadMIMEHeaderNonCompliant(t *testing.T) {
	// These invalid headers will be rejected by net/http according to RFC 7230.
	r := reader("Foo: bar\r\n" +
		"Content-Language: en\r\n" +
		"SID : 0\r\n" +
		"Audio Mode : None\r\n" +
		"Privilege : 127\r\n\r\n")
	m, err := r.ReadMIMEHeader()
	want := MIMEHeader{
		"Foo":              {"bar"},
		"Content-Language": {"en"},
		"SID ":             {"0"},
		"Audio Mode ":      {"None"},
		"Privilege ":       {"127"},
	}
	if !reflect.DeepEqual(m, want) || err != nil {
		t.Fatalf("ReadMIMEHeader =\n%v, %v; want:\n%v", m, err, want)
	}
}

func TestReadMIMEHeaderMalformed(t *testing.T) {
	inputs := []string{
		"No colon first line\r\nFoo: foo\r\n\r\n",
		" No colon first line with leading space\r\nFoo: foo\r\n\r\n",
		"\tNo colon first line with leading tab\r\nFoo: foo\r\n\r\n",
		" First: line with leading space\r\nFoo: foo\r\n\r\n",
		"\tFirst: line with leading tab\r\nFoo: foo\r\n\r\n",
		"Foo: foo\r\nNo colon second line\r\n\r\n",
		"Foo-\n\tBar: foo\r\n\r\n",
		"Foo-\r\n\tBar: foo\r\n\r\n",
		"Foo\r\n\t: foo\r\n\r\n",
		"Foo-\n\tBar",
	}

	for _, input := range inputs {
		r := reader(input)
		if m, err := r.ReadMIMEHeader(); err == nil {
			t.Errorf("ReadMIMEHeader(%q) = %v, %v; want nil, err", input, m, err)
		}
	}
}

// Test that continued lines are properly trimmed. Issue 11204.
func TestReadMIMEHeaderTrimContinued(t *testing.T) {
	// In this header, \n and \r\n terminated lines are mixed on purpose.
	// We expect each line to be trimmed (prefix and suffix) before being concatenated.
	// Keep the spaces as they are.
	r := reader("" + // for code formatting purpose.
		"a:\n" +
		" 0 \r\n" +
		"b:1 \t\r\n" +
		"c: 2\r\n" +
		" 3\t\n" +
		"  \t 4  \r\n\n")
	m, err := r.ReadMIMEHeader()
	if err != nil {
		t.Fatal(err)
	}
	want := MIMEHeader{
		"A": {"0"},
		"B": {"1"},
		"C": {"2 3 4"},
	}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("ReadMIMEHeader mismatch.\n got: %q\nwant: %q", m, want)
	}
}

type readResponseTest struct {
	in       string
	inCode   int
	wantCode int
	wantMsg  string
}

var readResponseTests = []readResponseTest{
	{"230-Anonymous access granted, restrictions apply\n" +
		"Read the file README.txt,\n" +
		"230  please",
		23,
		230,
		"Anonymous access granted, restrictions apply\nRead the file README.txt,\n please",
	},

	{"230 Anonymous access granted, restrictions apply\n",
		23,
		230,
		"Anonymous access granted, restrictions apply",
	},

	{"400-A\n400-B\n400 C",
		4,
		400,
		"A\nB\nC",
	},

	{"400-A\r\n400-B\r\n400 C\r\n",
		4,
		400,
		"A\nB\nC",
	},
}

// See http://www.ietf.org/rfc/rfc959.txt page 36.
func TestRFC959Lines(t *testing.T) {
	for i, tt := range readResponseTests {
		r := reader(tt.in + "\nFOLLOWING DATA")
		code, msg, err := r.ReadResponse(tt.inCode)
		if err != nil {
			t.Errorf("#%d: ReadResponse: %v", i, err)
			continue
		}
		if code != tt.wantCode {
			t.Errorf("#%d: code=%d, want %d", i, code, tt.wantCode)
		}
		if msg != tt.wantMsg {
			t.Errorf("#%d: msg=%q, want %q", i, msg, tt.wantMsg)
		}
	}
}

// Test that multi-line errors are appropriately and fully read. Issue 10230.
func TestReadMultiLineError(t *testing.T) {
	r := reader("550-5.1.1 The email account that you tried to reach does not exist. Please try\n" +
		"550-5.1.1 double-checking the recipient's email address for typos or\n" +
		"550-5.1.1 unnecessary spaces. Learn more at\n" +
		"Unexpected but legal text!\n" +
		"550 5.1.1 https://support.google.com/mail/answer/6596 h20si25154304pfd.166 - gsmtp\n")

	wantMsg := "5.1.1 The email account that you tried to reach does not exist. Please try\n" +
		"5.1.1 double-checking the recipient's email address for typos or\n" +
		"5.1.1 unnecessary spaces. Learn more at\n" +
		"Unexpected but legal text!\n" +
		"5.1.1 https://support.google.com/mail/answer/6596 h20si25154304pfd.166 - gsmtp"

	code, msg, err := r.ReadResponse(250)
	if err == nil {
		t.Errorf("ReadResponse: no error, want error")
	}
	if code != 550 {
		t.Errorf("ReadResponse: code=%d, want %d", code, 550)
	}
	if msg != wantMsg {
		t.Errorf("ReadResponse: msg=%q, want %q", msg, wantMsg)
	}
	if err != nil && err.Error() != "550 "+wantMsg {
		t.Errorf("ReadResponse: error=%q, want %q", err.Error(), "550 "+wantMsg)
	}
}

func TestCommonHeaders(t *testing.T) {
	commonHeaderOnce.Do(initCommonHeader)
	for h := range commonHeader {
		if h != CanonicalMIMEHeaderKey(h) {
			t.Errorf("Non-canonical header %q in commonHeader", h)
		}
	}
	b := []byte("content-Length")
	want := "Content-Length"
	n := testing.AllocsPerRun(200, func() {
		if x := canonicalMIMEHeaderKey(b); x != want {
			t.Fatalf("canonicalMIMEHeaderKey(%q) = %q; want %q", b, x, want)
		}
	})
	if n > 0 {
		t.Errorf("canonicalMIMEHeaderKey allocs = %v; want 0", n)
	}
}

var clientHeaders = strings.Replace(`Host: golang.org
Connection: keep-alive
Cache-Control: max-age=0
Accept: application/xml,application/xhtml+xml,text/html;q=0.9,text/plain;q=0.8,image/png,*/*;q=0.5
User-Agent: Mozilla/5.0 (X11; U; Linux x86_64; en-US) AppleWebKit/534.3 (KHTML, like Gecko) Chrome/6.0.472.63 Safari/534.3
Accept-Encoding: gzip,deflate,sdch
Accept-Language: en-US,en;q=0.8,fr-CH;q=0.6
Accept-Charset: ISO-8859-1,utf-8;q=0.7,*;q=0.3
COOKIE: __utma=000000000.0000000000.0000000000.0000000000.0000000000.00; __utmb=000000000.0.00.0000000000; __utmc=000000000; __utmz=000000000.0000000000.00.0.utmcsr=code.google.com|utmccn=(referral)|utmcmd=referral|utmcct=/p/go/issues/detail
Non-Interned: test

`, "\n", "\r\n", -1)

var serverHeaders = strings.Replace(`Content-Type: text/html; charset=utf-8
Content-Encoding: gzip
Date: Thu, 27 Sep 2012 09:03:33 GMT
Server: Google Frontend
Cache-Control: private
Content-Length: 2298
VIA: 1.1 proxy.example.com:80 (XXX/n.n.n-nnn)
Connection: Close
Non-Interned: test

`, "\n", "\r\n", -1)

func BenchmarkReadMIMEHeader(b *testing.B) {
	b.ReportAllocs()
	for _, set := range []struct {
		name    string
		headers string
	}{
		{"client_headers", clientHeaders},
		{"server_headers", serverHeaders},
	} {
		b.Run(set.name, func(b *testing.B) {
			var buf bytes.Buffer
			br := bfe_bufio.NewReader(&buf)
			r := NewReader(br)

			for i := 0; i < b.N; i++ {
				buf.WriteString(set.headers)
				if _, err := r.ReadMIMEHeader(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkUncommon(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	br := bfe_bufio.NewReader(&buf)
	r := NewReader(br)
	for i := 0; i < b.N; i++ {
		buf.WriteString("uncommon-header-for-benchmark: foo\r\n\r\n")
		h, err := r.ReadMIMEHeader()
		if err != nil {
			b.Fatal(err)
		}
		if _, ok := h["Uncommon-Header-For-Benchmark"]; !ok {
			b.Fatal("Missing result header.")
		}
	}
}

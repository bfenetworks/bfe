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

// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bfe_http2

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_http2/hpack"
)

func TestNewFingerprintWithCalculate(t *testing.T) {
	fp := newFingerprint()
	if got, want := fp.Calculate(), "|00|0|"; got != want {
		t.Errorf("Calculate result = %s; want %s", got, want)
	}

	fr, _ := testFramer()
	settings := []Setting{{1, 2}, {3, 4}}
	fr.WriteSettings(settings...)
	f, err := fr.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	fp.ProcessFrame(readFrameResult{f, nil, func() {}})
	if got, want := fp.Calculate(), "|00|0|"; got != want {
		t.Errorf("Calculate result = %s; want %s", got, want)
	}
}

func TestNewFingerprintSettingsFrame(t *testing.T) {
	fp := newFingerprint()
	fr, _ := testFramer()
	settings := []Setting{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}, {5, 6}, {6, 7}, {7, 8}, {8, 9}}
	fr.WriteSettings(settings...)
	f, err := fr.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	fp.ProcessFrame(readFrameResult{f, nil, func() {}})
	if got, want := fp.Calculate(), "1:2;2:3;3:4;4:5;5:6;6:7|00|0|"; got != want {
		t.Errorf("Calculate result = %s; want %s", got, want)
	}
}

func TestNewFingerprintWindowUpdateFrame(t *testing.T) {
	fp := newFingerprint()
	fr, _ := testFramer()
	var i uint32
	for i = 1; i < 3; i++ {
		fr.WriteWindowUpdate(0, i)
	}
	for i = 1; i < 3; i++ {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatal(err)
		}
		fp.ProcessFrame(readFrameResult{f, nil, func() {}})
	}
	if got, want := fp.Calculate(), "|1|0|"; got != want {
		t.Errorf("Calculate result = %s; want %s", got, want)
	}
}

func TestNewFingerprintPriorityFrame(t *testing.T) {
	fp := newFingerprint()
	fr, _ := testFramer()
	priorities := []string{}
	var i uint32
	for i = 1; i < 4; i++ {
		priorities = append(priorities, fmt.Sprintf("%d:%d:%d:%d", i, func() uint8 {
			if i%2 == 1 {
				return 1
			}
			return 0
		}(), i-1, uint8(i)*10))
		fr.WritePriority(i, PriorityParam{
			StreamDep: i - 1,
			Exclusive: i%2 == 1,
			Weight:    uint8(i) * 10,
		})
	}
	for i = 1; i < 4; i++ {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatal(err)
		}
		fp.ProcessFrame(readFrameResult{f, nil, func() {}})
	}
	if got, want := fp.Calculate(), fmt.Sprintf("|00|%s|", strings.Join(priorities, ",")); got != want {
		t.Errorf("Calculate result = %s; want %s", got, want)
	}
}

func TestNewFingerprintMetaHeadersFrame(t *testing.T) {
	write := func(f *Framer, priority PriorityParam, frags ...[]byte) {
		for i, frag := range frags {
			end := (i == len(frags)-1)
			if i == 0 {
				f.WriteHeaders(HeadersFrameParam{
					StreamID:      1,
					BlockFragment: frag,
					EndHeaders:    end,
					Priority:      priority,
				})
			} else {
				f.WriteContinuation(1, end, frag)
			}
		}
	}

	tests := [...]struct {
		name string
		w    func(*Framer)
		want string
	}{
		{
			name: "firefox headers",
			w: func(f *Framer) {
				var he hpackEncoder
				all := he.encodeHeaderRaw(t,
					":method", "GET", ":path", "/", ":authority", "", ":scheme", "https")
				write(f, PriorityParam{
					StreamDep: 10,
					Exclusive: true,
					Weight:    11,
				}, all)
			},
			want: "|00|1:1:10:11|m,p,a,s",
		},
		{
			name: "chrome headers",
			w: func(f *Framer) {
				var he hpackEncoder
				all := he.encodeHeaderRaw(t,
					":method", "GET", ":authority", "", ":scheme", "https", ":path", "/")
				write(f, PriorityParam{}, all)
			},
			want: "|00|0|m,a,s,p",
		},
		{
			name: "safari headers",
			w: func(f *Framer) {
				var he hpackEncoder
				all := he.encodeHeaderRaw(t,
					":method", "GET", ":scheme", "https", ":path", "/", ":authority", "")
				write(f, PriorityParam{
					StreamDep: 2,
					Exclusive: false,
					Weight:    22,
				}, all)
			},
			want: "|00|1:0:2:22|m,s,p,a",
		},
	}
	for _, tt := range tests {
		buf := new(bytes.Buffer)
		f := NewFramer(buf, buf)
		f.ReadMetaHeaders = hpack.NewDecoder(initialHeaderTableSize, nil)
		tt.w(f)

		got, err := f.ReadFrame()
		if err != nil {
			t.Fatal(err)
			t.Errorf("%s: %v\n", tt.name, err)
		}

		fp := newFingerprint()
		fp.ProcessFrame(readFrameResult{got, nil, func() {}})
		if got, want := fp.Calculate(), tt.want; got != want {
			t.Errorf("Calculate result = %s; want %s", got, want)
		}
	}
}

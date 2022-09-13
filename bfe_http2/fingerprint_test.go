// Copyright (c) 2022 The BFE Authors.
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
	settings := []Setting{
		{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}, {5, 6}, {6, 7}, {7, 8}, {1, 10},
	}
	fr.WriteSettings(settings...)
	f, err := fr.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	fp.ProcessFrame(readFrameResult{f, nil, func() {}})
	if got, want := fp.Calculate(), "1:10;2:3;3:4;4:5;5:6;6:7|00|0|"; got != want {
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
		exclusive := 0
		if i%2 == 1 {
			exclusive = 1
		}
		priorities = append(priorities, fmt.Sprintf("%d:%d:%d:%d", i, exclusive, i-1, uint8(i)*10))
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
		name   string
		w      func(*Framer)
		want   string
		hasErr bool
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
		{
			name: "safari headers with illegal Pseudo Heade",
			w: func(f *Framer) {
				var he hpackEncoder
				all := he.encodeHeaderRaw(t,
					":method", "GET", ":scheme", "https", ":path", "/", ":authority", "", ":auth", "empty")
				write(f, PriorityParam{
					StreamDep: 2,
					Exclusive: false,
					Weight:    22,
				}, all)
			},
			want:   "|00|0|",
			hasErr: true,
		},
	}
	for _, tt := range tests {
		buf := new(bytes.Buffer)
		f := NewFramer(buf, buf)
		f.ReadMetaHeaders = hpack.NewDecoder(initialHeaderTableSize, nil)
		tt.w(f)

		got, err := f.ReadFrame()
		if err != nil && !tt.hasErr {
			t.Fatal(err)
			t.Errorf("%s: %v\n", tt.name, err)
		}

		fp := newFingerprint()
		fp.ProcessFrame(readFrameResult{got, err, func() {}})
		if got, want := fp.Calculate(), tt.want; got != want {
			t.Errorf("Calculate result = %s; want %s", got, want)
		}
	}
}

func TestNewFingerprintWithFakeBrowsers(t *testing.T) {
	writeHeaders := func(
		f *Framer, streamId uint32, priority PriorityParam, frags ...[]byte,
	) {
		for i, frag := range frags {
			end := (i == len(frags)-1)
			if i == 0 {
				err := f.WriteHeaders(HeadersFrameParam{
					StreamID:      streamId,
					BlockFragment: frag,
					EndHeaders:    end,
					Priority:      priority,
				})
				if err != nil {
					t.Errorf("%s", err)
				}
			} else {
				if err := f.WriteContinuation(1, end, frag); err != nil {
					t.Errorf("%s", err)
				}
			}
		}
	}

	newSetting := func(settings []Setting) readFrameResult {
		fr, _ := testFramer()
		fr.WriteSettings(settings...)
		f, err := fr.ReadFrame()
		return readFrameResult{f, err, func() {}}
	}

	newWindowUpdate := func(streamID uint32, incr uint32) readFrameResult {
		fr, _ := testFramer()
		fr.WriteWindowUpdate(streamID, incr)
		f, err := fr.ReadFrame()
		return readFrameResult{f, err, func() {}}
	}

	newPriority := func(streamID, streamDep uint32, exclusive bool, weight uint8) readFrameResult {
		fr, _ := testFramer()
		fr.WritePriority(streamID, PriorityParam{
			StreamDep: streamDep,
			Exclusive: exclusive,
			Weight:    weight,
		})
		f, err := fr.ReadFrame()
		return readFrameResult{f, err, func() {}}
	}

	newHeader := func(
		streamID uint32, headers []string, priorityParam PriorityParam,
	) readFrameResult {
		buf := new(bytes.Buffer)
		f := NewFramer(buf, buf)
		f.AllowIllegalWrites = true
		f.ReadMetaHeaders = hpack.NewDecoder(initialHeaderTableSize, nil)

		var he hpackEncoder
		all := he.encodeHeaderRaw(t, headers...)
		writeHeaders(f, streamID, priorityParam, all)

		got, err := f.ReadFrame()
		return readFrameResult{got, err, func() {}}
	}

	tests := [...]struct {
		name   string
		frames []readFrameResult
		want   string
		hasErr bool
	}{
		{
			name: "firefox",
			frames: []readFrameResult{
				newSetting([]Setting{{1, 65536}, {4, 131072}, {5, 16384}}),
				newWindowUpdate(0, 12517377),
				newPriority(3, 0, false, 200),
				newPriority(5, 0, false, 100),
				newPriority(7, 0, false, 0),
				newPriority(9, 7, false, 0),
				newPriority(11, 3, false, 0),
				newPriority(13, 0, false, 240),
				newHeader(
					15,
					[]string{":method", "GET", ":path", "/", ":authority", "", ":scheme", "https"},
					PriorityParam{
						StreamDep: 13,
						Exclusive: false,
						Weight:    41,
					}),
			},
			want: "1:65536;4:131072;5:16384|12517377|3:0:0:200,5:0:0:100,7:0:0:0,9:0:7:0,11:0:3:0,13:0:0:240,15:0:13:41|m,p,a,s",
		},
		{
			name: "edge",
			frames: []readFrameResult{
				newSetting([]Setting{{1, 65536}, {3, 1000}, {4, 6291456}, {6, 262144}}),
				newWindowUpdate(0, 15663105),
				newHeader(
					1,
					[]string{":method", "GET", ":authority", "", ":scheme", "https", ":path", "/"},
					PriorityParam{
						StreamDep: 0,
						Exclusive: true,
						Weight:    255,
					}),
			},
			want: "1:65536;3:1000;4:6291456;6:262144|15663105|1:1:0:255|m,a,s,p",
		},
	}
	for _, tt := range tests {
		fp := newFingerprint()
		for _, f := range tt.frames {
			fp.ProcessFrame(f)
		}
		if got, want := fp.Calculate(), tt.want; got != want {
			t.Errorf("Calculate (%s) result = %s; want %s", tt.name, got, want)
		}
	}
}

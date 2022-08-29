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
	"sync"
)

type fingerprint struct {
	lock sync.RWMutex

	// serverConn are reused in stream and needs to prevent duplicate parsing.
	calculated   bool
	windowUpdate uint32

	settingKeys   []SettingID
	settings      map[SettingID]uint32
	priorities    []string
	pseudoHeaders []byte

	// the final value of the fingerprint.
	value string
}

func newFingerprint() *fingerprint {
	return &fingerprint{
		// the average number of settings here may be 6.
		settingKeys: make([]SettingID, 0, 6),
		settings:    make(map[SettingID]uint32, 6),
		// the average number of priority frame here may be 5.
		priorities: make([]string, 0, 5),
		// any legitimate request will have 3-4 headers.
		pseudoHeaders: make([]byte, 0, 4),
	}
}

// the readFrameResult will no longer exist if readFrames again,
// so it is necessary to save the fingerprint information with plain value.
func (fp *fingerprint) ProcessFrame(res readFrameResult) {
	fp.lock.Lock()
	defer fp.lock.Unlock()

	// once the fingerprint is used, we should not process frame again.
	if fp.calculated {
		return
	}

	// if error occured, the frame will also discard by h2.
	if res.err != nil {
		return
	}

	switch f := res.f.(type) {
	case *SettingsFrame:
		var sk SettingID
		for sk = 1; sk <= 6; sk++ {
			sv, ok := f.Value(sk)
			if !ok {
				continue
			}
			// if there are multiple occurrences,
			// we only take the first as the order of the setting key.
			if _, ok = fp.settings[sk]; !ok {
				fp.settingKeys = append(fp.settingKeys, sk)
			}
			// use the final setting value as the fingerprint.
			fp.settings[sk] = sv
		}
	case *WindowUpdateFrame:
		if fp.windowUpdate > 0 {
			break
		}
		fp.windowUpdate = f.Increment
	case *PriorityFrame:
		fp.processPriority(f.StreamID, f.PriorityParam)
	case *MetaHeadersFrame:
		if f.HasPriority() {
			fp.processPriority(f.StreamID, f.Priority)
		}
		for _, field := range f.Fields {
			switch field.Name {
			case ":method", ":path", ":scheme", ":authority":
				fp.pseudoHeaders = append(fp.pseudoHeaders, field.Name[1])
			default:
				continue
			}
		}
	default:
		return
	}
}

func (fp *fingerprint) processPriority(sid uint32, f PriorityParam) {
	exclusive := 0
	if f.Exclusive {
		exclusive = 1
	}

	fp.priorities = append(
		fp.priorities,
		fmt.Sprintf("%d:%d:%d:%d", sid, exclusive, f.StreamDep, f.Weight),
	)
}

func (fp *fingerprint) Calculate() string {
	fp.lock.Lock()
	defer fp.lock.Unlock()

	if fp.calculated {
		return fp.value
	}

	buf := bytes.NewBuffer([]byte{})
	var sk SettingID
	for _, sk = range fp.settingKeys {
		if sv, ok := fp.settings[sk]; ok {
			fmt.Fprintf(buf, "%d:%d;", sk, sv)
		}
	}
	if len(fp.settings) > 0 {
		buf.Truncate(buf.Len() - 1)
	}

	buf.WriteByte('|')
	if fp.windowUpdate == 0 {
		buf.WriteString("00")
	} else {
		fmt.Fprintf(buf, "%d", fp.windowUpdate)
	}

	buf.WriteByte('|')
	if len(fp.priorities) == 0 {
		buf.WriteByte('0')
	} else {
		buf.WriteString(strings.Join(fp.priorities, ","))
	}

	buf.WriteByte('|')
	for k, v := range fp.pseudoHeaders {
		buf.WriteByte(v)
		if k < len(fp.pseudoHeaders)-1 {
			buf.WriteByte(',')
		}
	}

	fp.calculated = true
	fp.value = buf.String()
	return fp.value
}

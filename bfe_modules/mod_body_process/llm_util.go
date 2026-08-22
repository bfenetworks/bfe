// Copyright (c) 2026 The BFE Authors.
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

package mod_body_process

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const UnknownModel = "unknown"

type QuotaUsage struct {
	//return from reponse
	PromptTokens      int64 // number of tokens in the prompt
	CompletionTokens  int64 // number of tokens in the completion
	CacheReadTokens   int64 // usage.cache_read_tokens, already included in PromptTokens
	CacheWriteTokens  int64 // usage.cache_write_tokens, independent add-on item
	AudioInputTokens  int64 // usage.audio_input_tokens, already included in PromptTokens
	AudioOutputTokens int64 // usage.audio_output_tokens, already included in CompletionTokens
	ImageCount        int64 // number of generated images for image generation models
	UsedQuota         int64 // used quota for this request

	//estimate for current response
	CurrentTokens int64 //effect when IsGuess is true
	IsGuess       bool  //true = is estimate
}

type SSEEvent struct {
	ID        *string
	Event     *string
	DataLines [][]byte
	Retry     *int
	Comments  [][]byte
	RawLines  [][]byte

	//raw data
	raw       []byte
	dirty     bool
	truncated bool
	endstyle  string
}

func (e *SSEEvent) hasContent() bool {
	return e.ID != nil ||
		e.Event != nil ||
		len(e.DataLines) > 0 ||
		len(e.Comments) > 0 ||
		len(e.RawLines) > 0 ||
		e.Retry != nil
}

func (e *SSEEvent) SetID(v *string) {
	if e.ID == v {
		return
	}
	if e.ID != nil && v != nil && *e.ID == *v {
		return
	}

	e.ID = v
	e.dirty = true
}

func (e *SSEEvent) SetEvent(v *string) {
	if e.Event == v {
	}
	if e.Event != nil && v != nil && *e.Event == *v {
		return
	}

	e.Event = v
	e.dirty = true
}

func (e *SSEEvent) SetData(v []byte) {
	e.DataLines = [][]byte{v}
	e.dirty = true
}

func (e *SSEEvent) AppendDataLine(b []byte) {
	e.DataLines = append(e.DataLines, b)
	e.dirty = true
}

func (e *SSEEvent) SetJsonField(jpath string, newValue string) error {
	data := e.GetData()
	ndata, err := sjson.SetBytes(data, jpath, newValue)
	if err != nil {
		return err
	}
	e.SetData([]byte(ndata))
	e.dirty = true
	return nil
}

func (e *SSEEvent) GetData() []byte {
	return bytes.Join(e.DataLines, []byte("\n"))
}

func (e *SSEEvent) GetAuditData() []byte {
	return e.GetData()
}

func (e *SSEEvent) GetQuotaUsage() QuotaUsage {
	data := e.GetData()
	used := gjson.GetBytes(data, "usage.total_tokens").Int()
	prompt := gjson.GetBytes(data, "usage.prompt_tokens").Int()
	completion := gjson.GetBytes(data, "usage.completion_tokens").Int()
	cacheRead := gjson.GetBytes(data, "usage.cache_read_tokens").Int()
	cacheWrite := gjson.GetBytes(data, "usage.cache_write_tokens").Int()
	audioInput := gjson.GetBytes(data, "usage.audio_input_tokens").Int()
	audioOutput := gjson.GetBytes(data, "usage.audio_output_tokens").Int()
	imageCount := gjson.GetBytes(data, "usage.image_count").Int()
	if imageCount == 0 {
		imageCount = gjson.GetBytes(data, "data.#").Int()
	}

	curtoken := int64(0)
	isguess := true
	if used > 0 || imageCount > 0 {
		isguess = false
	} else {
		curtoken = EstimateContentToken(string(data))
	}

	return QuotaUsage{
		PromptTokens:      prompt,
		CompletionTokens:  completion,
		CacheReadTokens:   cacheRead,
		CacheWriteTokens:  cacheWrite,
		AudioInputTokens:  audioInput,
		AudioOutputTokens: audioOutput,
		ImageCount:        imageCount,
		UsedQuota:         used,
		CurrentTokens:     curtoken,
		IsGuess:           isguess,
	}
}

func (e *SSEEvent) ToBytes() []byte {
	if !e.dirty && e.raw != nil {
		return e.raw
	}

	var buf bytes.Buffer
	for _, c := range e.Comments {
		buf.Write(c)
		buf.WriteString(e.endstyle)
	}

	if e.ID != nil {
		buf.WriteString("id: ")
		buf.WriteString(*e.ID)
		buf.WriteString(e.endstyle)
	}

	if e.Event != nil {
		buf.WriteString("event: ")
		buf.WriteString(*e.Event)
		buf.WriteString(e.endstyle)
	}

	for _, d := range e.DataLines {
		buf.WriteString("data: ")
		buf.Write(d)
		buf.WriteString(e.endstyle)
	}

	if e.Retry != nil {
		buf.WriteString("retry: ")
		buf.WriteString(strconv.Itoa(*e.Retry))
		buf.WriteString(e.endstyle)
	}

	for _, r := range e.RawLines {
		buf.Write(r)
		buf.WriteString(e.endstyle)
	}

	if !e.truncated {
		buf.WriteString(e.endstyle)
	}

	return buf.Bytes()
}

type SSEEventDecoder struct {
	r *bufio.Reader
}

func NewSSEEventDecoder(source io.Reader) (EventDecoder, error) {
	return &SSEEventDecoder{
		r: bufio.NewReader(source),
	}, nil
}

func (d *SSEEventDecoder) Decode() ([]Event, error) {
	var (
		ev     SSEEvent
		rawBuf bytes.Buffer
	)
	ev.endstyle = "\n"

	for {
		line, err := d.r.ReadString('\n')
		//fmt.Printf("----:%s,%+v\n", line, err)
		if err != nil && len(line) == 0 {
			if ev.hasContent() {
				ev.truncated = true
				ev.raw = rawBuf.Bytes()
				return []Event{&ev}, nil
			}

			if err == io.EOF {
				return []Event{}, nil
			}

			return nil, err
		}

		rawBuf.WriteString(line)

		trimmed := strings.TrimSuffix(line, "\n")
		trimmed = strings.TrimSuffix(trimmed, "\r")

		if trimmed == "" {
			if ev.hasContent() {
				ev.raw = rawBuf.Bytes()
				if strings.HasSuffix(line, "\r\n") {
					ev.endstyle = "\r\n"
				}
				return []Event{&ev}, nil
			}
			continue
		}

		if strings.HasPrefix(trimmed, ":") {
			ev.Comments = append(ev.Comments, []byte(trimmed))
			continue
		}

		field, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			ev.RawLines = append(ev.RawLines, []byte(trimmed))
			continue
		}

		if strings.HasPrefix(value, " ") {
			value = value[1:]
		}

		switch field {
		case "event":
			ev.Event = &value
		case "id":
			ev.ID = &value
		case "data":
			ev.DataLines = append(ev.DataLines, []byte(value))
		case "retry":
			if v, err := strconv.Atoi(value); err == nil {
				ev.Retry = &v
			}
		default:
			ev.RawLines = append(ev.RawLines, []byte(trimmed))
		}
	}
}

func remarshal(src any, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func EstimateContentToken(val string) int64 {
	return int64(len(val)) / 4
}

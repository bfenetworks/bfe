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

	"github.com/tidwall/sjson"
)

import (
	modelprotocol "github.com/bfenetworks/bfe/bfe_model_protocol"
	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

const UnknownModel = "unknown"

type QuotaUsage struct {
	//return from reponse
	PromptTokens      int64 // number of tokens in the prompt
	CompletionTokens  int64 // number of tokens in the completion
	CacheReadTokens   int64 // usage.cache_read_tokens, already included in PromptTokens
	CacheWriteTokens  int64 // usage.cache_write_tokens, already included in PromptTokens (normalized for Anthropic)
	AudioInputTokens  int64 // usage.audio_input_tokens, already included in PromptTokens
	AudioOutputTokens int64 // usage.audio_output_tokens, already included in CompletionTokens
	ImageInputTokens  int64 // usage.image_input_tokens / input_token_details.image_tokens, already included in PromptTokens
	VideoCount        int64 // number of generated videos for video generation models
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

// extractUsageFields extracts usage fields from one response body by
// composing the protocol adapters: the openai adapter chain first, then
// the anthropic (Claude) chain when no OpenAI-style prompt/completion
// tokens are present. This mirrors the legacy all-chain extraction
// field-for-field.
func extractUsageFields(data []byte) modelprotocol.UsageFields {
	fields := modelprotocol.Get(modelprotocol.ProtocolOpenAI).ExtractUsageFields(data)
	if fields.PromptTokens == 0 && fields.CompletionTokens == 0 {
		claude := modelprotocol.Get(modelprotocol.ProtocolAnthropic).ExtractUsageFields(data)
		fields.PromptTokens = claude.PromptTokens
		fields.CompletionTokens = claude.CompletionTokens
		if fields.CacheReadTokens == 0 {
			fields.CacheReadTokens = claude.CacheReadTokens
		}
		if fields.CacheWriteTokens == 0 {
			fields.CacheWriteTokens = claude.CacheWriteTokens
		}
		if fields.UsedQuota == 0 {
			fields.UsedQuota = claude.UsedQuota
		}
	}
	return fields
}

func (e *SSEEvent) GetQuotaUsage() QuotaUsage {
	data := e.GetData()
	fields := extractUsageFields(data)

	curtoken := int64(0)
	isguess := true
	if fields.UsedQuota > 0 || fields.ImageCount > 0 || fields.VideoCount > 0 {
		isguess = false
	} else {
		curtoken = EstimateContentToken(string(data))
	}

	return QuotaUsage{
		PromptTokens:      fields.PromptTokens,
		CompletionTokens:  fields.CompletionTokens,
		CacheReadTokens:   fields.CacheReadTokens,
		CacheWriteTokens:  fields.CacheWriteTokens,
		AudioInputTokens:  fields.AudioInputTokens,
		AudioOutputTokens: fields.AudioOutputTokens,
		ImageInputTokens:  fields.ImageInputTokens,
		VideoCount:        fields.VideoCount,
		ImageCount:        fields.ImageCount,
		UsedQuota:         fields.UsedQuota,
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

// EstimateContentToken estimates the token count of a response body chunk
// (roughly 4 bytes per token). The implementation lives in
// bfe_model_protocol/utils and is kept re-exported here for compatibility.
func EstimateContentToken(val string) int64 {
	return utils.EstimateContentToken(val)
}

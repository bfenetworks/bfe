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
	"bytes"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func TestNewBodyProcessor(t *testing.T) {
	source := io.NopCloser(bytes.NewBufferString("hello"))
	bp := NewBodyProcessor(source)
	if bp == nil {
		t.Fatal("NewBodyProcessor should not return nil")
	}
	if bp.GetSource() != source {
		t.Error("GetSource should return original source")
	}
}

func TestBodyProcessorReadWithLineDecoder(t *testing.T) {
	source := io.NopCloser(bytes.NewBufferString("hello\nworld\n"))
	bp := NewBodyProcessor(source)
	bp.CreateEventDecoder(NewLineDecoder)
	bp.CreateEventEncoder(NewGeneralEncoder)

	buf, err := ioutil.ReadAll(bp)
	if err != nil {
		t.Fatalf("ReadAll failed: %s", err)
	}
	if string(buf) != "hello\nworld\n" {
		t.Errorf("unexpected output: %s", string(buf))
	}
}

func TestBodyProcessorReadWithJsonDecoder(t *testing.T) {
	source := io.NopCloser(bytes.NewBufferString(`{"a":1}{"b":2}`))
	bp := NewBodyProcessor(source)
	bp.CreateEventDecoder(NewJsonDecoder)
	bp.CreateEventEncoder(NewGeneralEncoder)

	buf, err := ioutil.ReadAll(bp)
	if err != nil {
		t.Fatalf("ReadAll failed: %s", err)
	}
	if !strings.Contains(string(buf), `"a":1`) || !strings.Contains(string(buf), `"b":2`) {
		t.Errorf("unexpected output: %s", string(buf))
	}
}

func TestBodyProcessorReadWithSSEDecoder(t *testing.T) {
	source := io.NopCloser(bytes.NewBufferString("data: hello\n\ndata: world\n\n"))
	bp := NewBodyProcessor(source)
	bp.CreateEventDecoder(NewSSEEventDecoder)
	bp.CreateEventEncoder(NewGeneralEncoder)

	buf, err := ioutil.ReadAll(bp)
	if err != nil {
		t.Fatalf("ReadAll failed: %s", err)
	}
	if !strings.Contains(string(buf), "data: hello") {
		t.Errorf("unexpected output: %s", string(buf))
	}
}

func TestBodyProcessorReadWithContentTypeDecoderJSON(t *testing.T) {
	source := io.NopCloser(bytes.NewBufferString(`{"a":1}{"b":2}`))
	bp := NewBodyProcessor(source)
	bp.CreateEventDecoder(func(r io.Reader) (EventDecoder, error) {
		return NewContentTypeDecoder(r, "application/json")
	})
	bp.CreateEventEncoder(NewGeneralEncoder)

	buf, err := ioutil.ReadAll(bp)
	if err != nil {
		t.Fatalf("ReadAll failed: %s", err)
	}
	if !strings.Contains(string(buf), `"a":1`) {
		t.Errorf("unexpected output: %s", string(buf))
	}
}

func TestBodyProcessorReadWithContentTypeDecoderSSE(t *testing.T) {
	source := io.NopCloser(bytes.NewBufferString("data: hello\n\n"))
	bp := NewBodyProcessor(source)
	bp.CreateEventDecoder(func(r io.Reader) (EventDecoder, error) {
		return NewContentTypeDecoder(r, "text/event-stream")
	})
	bp.CreateEventEncoder(NewGeneralEncoder)

	buf, err := ioutil.ReadAll(bp)
	if err != nil {
		t.Fatalf("ReadAll failed: %s", err)
	}
	if !strings.Contains(string(buf), "data: hello") {
		t.Errorf("unexpected output: %s", string(buf))
	}
}

func TestBodyProcessorWithProcessor(t *testing.T) {
	source := io.NopCloser(bytes.NewBufferString("hello\nworld\n"))
	bp := NewBodyProcessor(source)
	bp.CreateEventDecoder(NewLineDecoder)
	bp.CreateEventEncoder(NewGeneralEncoder)
	bp.AddProcessor(EventProcessorFunc(func(events []Event) ([]Event, error) {
		return events, nil
	}))

	buf, err := ioutil.ReadAll(bp)
	if err != nil {
		t.Fatalf("ReadAll failed: %s", err)
	}
	if string(buf) != "hello\nworld\n" {
		t.Errorf("unexpected output: %s", string(buf))
	}
}

func TestBodyProcessorRejection(t *testing.T) {
	source := io.NopCloser(bytes.NewBufferString("hello\n"))
	bp := NewBodyProcessor(source)
	bp.CreateEventDecoder(NewLineDecoder)
	bp.CreateEventEncoder(NewGeneralEncoder)
	bp.AddProcessor(EventProcessorFunc(func(events []Event) ([]Event, error) {
		return nil, &RejectionError{Message: "rejected", StatusCode: 400}
	}))

	_, err := ioutil.ReadAll(bp)
	if err == nil {
		t.Fatal("expected rejection error")
	}
	if bp.RejectionResponse() == nil {
		t.Fatal("expected RejectionResponse")
	}
}

func TestBodyProcessorClose(t *testing.T) {
	source := io.NopCloser(bytes.NewBufferString("hello"))
	bp := NewBodyProcessor(source)
	if err := bp.Close(); err != nil {
		t.Fatalf("Close failed: %s", err)
	}
}

func TestGeneralEncoder(t *testing.T) {
	var buf bytes.Buffer
	enc, err := NewGeneralEncoder(&buf)
	if err != nil {
		t.Fatalf("NewGeneralEncoder failed: %s", err)
	}
	events := []Event{newRawEvent("hello"), newRawEvent("world")}
	if _, err := enc.Encode(events); err != nil {
		t.Fatalf("Encode failed: %s", err)
	}
	if buf.String() != "helloworld" {
		t.Errorf("unexpected encoded output: %s", buf.String())
	}
}

func TestRawEventToBytes(t *testing.T) {
	e := newRawEvent("hello")
	if string(e.ToBytes()) != "hello" {
		t.Errorf("ToBytes returned %s", string(e.ToBytes()))
	}
}

func TestRawEventGetQuotaUsage(t *testing.T) {
	e := newRawEvent(`{"usage":{"total_tokens":10,"prompt_tokens":3,"completion_tokens":7}}`)
	q := e.GetQuotaUsage()
	if q.UsedQuota != 10 {
		t.Errorf("expected UsedQuota 10, got %d", q.UsedQuota)
	}
	if q.PromptTokens != 3 {
		t.Errorf("expected PromptTokens 3, got %d", q.PromptTokens)
	}
	if q.CompletionTokens != 7 {
		t.Errorf("expected CompletionTokens 7, got %d", q.CompletionTokens)
	}
	if q.IsGuess {
		t.Error("expected IsGuess false when usage is present")
	}
}

func TestRawEventGetQuotaUsageEstimate(t *testing.T) {
	e := newRawEvent(`{"text":"hello world"}`)
	q := e.GetQuotaUsage()
	if q.IsGuess != true {
		t.Error("expected IsGuess true when usage absent")
	}
	if q.CurrentTokens <= 0 {
		t.Errorf("expected positive CurrentTokens, got %d", q.CurrentTokens)
	}
}

func TestLineDecoder(t *testing.T) {
	dec, err := NewLineDecoder(bytes.NewBufferString("line1\nline2"))
	if err != nil {
		t.Fatalf("NewLineDecoder failed: %s", err)
	}
	events, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %s", err)
	}
	if len(events) != 1 || string(events[0].ToBytes()) != "line1\n" {
		t.Errorf("unexpected first decode: %v", events)
	}
	events, err = dec.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %s", err)
	}
	if len(events) != 1 || string(events[0].ToBytes()) != "line2" {
		t.Errorf("unexpected second decode: %v", events)
	}
	events, err = dec.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %s", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty events at EOF, got %v", events)
	}
}

func TestJsonDecoder(t *testing.T) {
	dec, err := NewJsonDecoder(bytes.NewBufferString(`{"a":1}`))
	if err != nil {
		t.Fatalf("NewJsonDecoder failed: %s", err)
	}
	events, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %s", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	events, err = dec.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %s", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty events at EOF, got %v", events)
	}
}

func TestJsonDecoderError(t *testing.T) {
	dec, err := NewJsonDecoder(bytes.NewBufferString(`{invalid`))
	if err != nil {
		t.Fatalf("NewJsonDecoder failed: %s", err)
	}
	_, err = dec.Decode()
	if err == nil {
		t.Error("expected decode error for invalid JSON")
	}
}

func TestContentTypeDecoderJSON(t *testing.T) {
	dec, err := NewContentTypeDecoder(bytes.NewBufferString(`{"a":1}`), "application/json")
	if err != nil {
		t.Fatalf("NewContentTypeDecoder failed: %s", err)
	}
	events, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %s", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestContentTypeDecoderSSE(t *testing.T) {
	dec, err := NewContentTypeDecoder(bytes.NewBufferString("data: hello\n\n"), "text/event-stream")
	if err != nil {
		t.Fatalf("NewContentTypeDecoder failed: %s", err)
	}
	events, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %s", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestContentTypeDecoderDefault(t *testing.T) {
	dec, err := NewContentTypeDecoder(bytes.NewBufferString("hello\n"), "text/plain")
	if err != nil {
		t.Fatalf("NewContentTypeDecoder failed: %s", err)
	}
	events, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %s", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestDoRequestProcess(t *testing.T) {
	m := NewModuleBodyProcess()
	req := newTestRequestWithBody("AI_product", `{"prompt":"hello"}`)
	conf := &BodyProcessConfig{Dec: "json", Proc: []ProcConf{}}

	bp := m.DoRequestProcess(req, conf)
	if bp == nil {
		t.Fatal("expected BodyProcessor")
	}
	if req.HttpRequest.Body != bp {
		t.Error("expected request body replaced by BodyProcessor")
	}
}

func TestDoRequestProcessNilConf(t *testing.T) {
	m := NewModuleBodyProcess()
	req := newTestRequestWithBody("AI_product", `{"prompt":"hello"}`)

	bp := m.DoRequestProcess(req, nil)
	if bp != nil {
		t.Error("expected nil BodyProcessor for nil config")
	}
}

func TestDoRequestProcessLine(t *testing.T) {
	m := NewModuleBodyProcess()
	req := newTestRequestWithBody("AI_product", "hello\nworld\n")
	conf := &BodyProcessConfig{Dec: "line"}

	bp := m.DoRequestProcess(req, conf)
	if bp == nil {
		t.Fatal("expected BodyProcessor")
	}
}

func TestDoResponseProcess(t *testing.T) {
	m := NewModuleBodyProcess()
	req := newTestRequest("AI_product")
	req.InitAiBasicInfo()
	res := &bfe_http.Response{
		StatusCode: bfe_http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("hello\n")),
		Header:     make(bfe_http.Header),
	}
	conf := &BodyProcessConfig{Dec: "line"}

	bp := m.DoResponseProcess(req, res, conf)
	if bp == nil {
		t.Fatal("expected BodyProcessor")
	}
	if res.Body != bp {
		t.Error("expected response body replaced by BodyProcessor")
	}
}

func TestDoResponseProcessNilConfAndQuota(t *testing.T) {
	m := NewModuleBodyProcess()
	req := newTestRequest("AI_product")
	res := &bfe_http.Response{
		StatusCode: bfe_http.StatusBadRequest,
		Body:       io.NopCloser(bytes.NewBufferString("error")),
	}

	bp := m.DoResponseProcess(req, res, nil)
	if bp != nil {
		t.Error("expected nil BodyProcessor when no config and non-OK status")
	}
}

func TestDoResponseProcessSSE(t *testing.T) {
	m := NewModuleBodyProcess()
	req := newTestRequest("AI_product")
	req.InitAiBasicInfo()
	res := &bfe_http.Response{
		StatusCode: bfe_http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("data: hello\n\n")),
		Header:     make(bfe_http.Header),
	}
	conf := &BodyProcessConfig{Dec: "sse"}

	bp := m.DoResponseProcess(req, res, conf)
	if bp == nil {
		t.Fatal("expected BodyProcessor")
	}
}

func TestDoResponseProcessQuota(t *testing.T) {
	m := NewModuleBodyProcess()
	req := newTestRequest("AI_product")
	ai := req.InitAiBasicInfo()
	ai.SetAllowEstimateToken(true)
	res := &bfe_http.Response{
		StatusCode: bfe_http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"usage":{"total_tokens":10}}`)),
		Header:     make(bfe_http.Header),
	}

	bp := m.DoResponseProcess(req, res, nil)
	if bp == nil {
		t.Fatal("expected BodyProcessor for quota processing")
	}
}

func TestEventProcessorFunc(t *testing.T) {
	f := EventProcessorFunc(func(events []Event) ([]Event, error) {
		return events, nil
	})
	if _, err := f.Process(nil); err != nil {
		t.Fatalf("Process failed: %s", err)
	}
}

func TestBPError(t *testing.T) {
	err := &BPError{Err: io.EOF}
	if err.Unwrap() != io.EOF {
		t.Error("Unwrap should return original error")
	}
	if !strings.Contains(err.Error(), "BodyProcessError") {
		t.Error("Error should contain BodyProcessError prefix")
	}
}

// Issue #1352: the quota usage processor tracks response completion so that
// aborted requests are not billed by full-request estimation.
func TestQuotaUsageProcessorMarksResponseCompletion(t *testing.T) {
	sseEvent := func(data string) Event {
		return &SSEEvent{DataLines: [][]byte{[]byte(data)}}
	}

	newProcessor := func() (*QuotaUsageProcessor, *bfe_basic.AiBasicInfo) {
		httpReq, _ := bfe_http.NewRequest("POST", "http://example.com/v1/chat/completions", nil)
		req := bfe_basic.NewRequest(httpReq, nil, nil, nil, nil)
		ai := req.InitAiBasicInfo()
		res := &bfe_http.Response{StatusCode: 200}
		return NewQuotaUsageProcessor(req, res), ai
	}

	// Stream cut right after message_start (client abort): neither the final
	// usage nor stream completion was seen.
	p, ai := newProcessor()
	events := []Event{
		sseEvent(`{"type":"message_start","usage":{"input_tokens":320,"output_tokens":0}}`),
	}
	if _, err := p.Process(events); err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if ai.IsFinalUsageSeen() {
		t.Error("message_start usage must not be treated as final usage")
	}
	if ai.IsResponseCompleted() {
		t.Error("stream without message_stop must not be treated as completed")
	}

	// Full stream: message_delta carries the final usage, message_stop
	// terminates the stream.
	p, ai = newProcessor()
	events = []Event{
		sseEvent(`{"type":"message_start","usage":{"input_tokens":320,"output_tokens":0}}`),
		sseEvent(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":15}}`),
		sseEvent(`{"type":"message_stop"}`),
	}
	if _, err := p.Process(events); err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if !ai.IsFinalUsageSeen() {
		t.Error("expected final usage seen after message_delta")
	}
	if !ai.IsResponseCompleted() {
		t.Error("expected response completed after message_stop")
	}
	// message_delta carries only output tokens; the prompt tokens parsed
	// from message_start must be preserved in the merged usage.
	usage := ai.GetTokenUsage()
	if usage.PromptTokens != 320 || usage.CompletionTokens != 15 || usage.UsedQuota != 335 {
		t.Errorf("expected merged usage prompt=320 completion=15 quota=335, got %d/%d/%d",
			usage.PromptTokens, usage.CompletionTokens, usage.UsedQuota)
	}

	// OpenAI stream with [DONE] terminator.
	p, ai = newProcessor()
	events = []Event{
		sseEvent(`{"id":"chatcmpl-1","choices":[{"delta":{"content":"hi"}}]}`),
		sseEvent(`{"id":"chatcmpl-1","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`),
		sseEvent(`[DONE]`),
	}
	if _, err := p.Process(events); err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if !ai.IsFinalUsageSeen() {
		t.Error("expected final usage seen after OpenAI final usage chunk")
	}
	if !ai.IsResponseCompleted() {
		t.Error("expected response completed after [DONE]")
	}
}

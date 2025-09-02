// Copyright (c) 2025 The BFE Authors.
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
	"fmt"
	"io"
	"strings"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_modules/mod_ai_token_auth"
)

// BodyProcessor 扩展中断支持
type BodyProcessor struct {
	source     io.ReadCloser
	buffer     *bytes.Buffer
	decoder    EventDecoder
	processors []EventProcessor
	encoder	    EventEncoder
	// mu         sync.Mutex
	// closed     bool
	err        error
	rejection  *RejectionError // 中断时存储的违规信息
	
	// 中断时回调
	onReject func(error, *BodyProcessor)
}

// NewBodyProcessor 创建处理器
func NewBodyProcessor(source io.ReadCloser) *BodyProcessor {
	return &BodyProcessor{
		source: source,
		buffer: bytes.NewBuffer(nil),
	}
}

func (bp *BodyProcessor) GetSource() io.ReadCloser {
	// bp.mu.Lock()
	// defer bp.mu.Unlock()
	return bp.source
}

// 注册中断回调
func (bp *BodyProcessor) OnReject(fn func(error, *BodyProcessor)) {
	// bp.mu.Lock()
	// defer bp.mu.Unlock()
	bp.onReject = fn
}

// RejectionError 自定义错误类型
type RejectionError struct {
	Message    string
	StatusCode int
	// RejectionResponse func(http.ResponseWriter) // 自定义响应生成器
}

func (e *RejectionError) Error() string {
	return e.Message
}

type Event interface {
	// GetType() string
	// GetData() []byte
	ToBytes() []byte // 转换为字节数组
}

type EventDecoder interface {
	// return:
	//  events, nil - len(events) > 0, success
	//  events, nil - len(events) = 0, 没有更多数据, eof
	//  nil, error - 发生错误
	Decode() ([]Event, error)
}

type EventDecoderFac func(source io.Reader) (EventDecoder, error)
type EventDecoderFacWithReq func(source io.Reader, req bfe_basic.Request) (EventDecoder, error)

type EventEncoder interface {
	Encode(events []Event) (int, error)
}

type EventEncoderFac func(dest io.Writer) (EventEncoder, error)

type EventProcessor interface {
	Process(events []Event) ([]Event, error)
}

func (bp *BodyProcessor) CreateEventDecoder(fac EventDecoderFac) {
	// bp.mu.Lock()
	// defer bp.mu.Unlock()
	dec, err := fac(bp.source)
	if err != nil {
		bp.err = fmt.Errorf("create event decoder: %w", err)
		return
	}
	bp.decoder = dec
}

func (bp *BodyProcessor) CreateEventEncoder(fac EventEncoderFac) {
	// bp.mu.Lock()
	// defer bp.mu.Unlock()
	enc, err := fac(bp.buffer)
	if err != nil {
		bp.err = fmt.Errorf("create event encoder: %w", err)
		return
	}
	bp.encoder = enc
}

func (bp *BodyProcessor) AddProcessor(p EventProcessor) {
	// bp.mu.Lock()
	// defer bp.mu.Unlock()
	
	bp.processors = append(bp.processors, p)
}

// ProcessorFunc 简化处理器实现
type EventProcessorFunc func([]Event) ([]Event, error)

func (f EventProcessorFunc) Process(events []Event) ([]Event, error) {
	return f(events)
}

// Read 实现io.Reader接口（支持中断）
func (bp *BodyProcessor) Read(p []byte) (n int, err error) {
	// bp.mu.Lock()
	// defer bp.mu.Unlock()
	
	// if bp.rejection != nil {
	// 	return 0, bp.rejection // 返回违规错误
	// }
	
	if bp.err != nil && bp.err != io.EOF {
		return 0, bp.err
	}
	
	// 检查缓冲区是否足够
	if bp.buffer.Len() < len(p) && bp.err != io.EOF {
		if err := bp.fillBuffer(); err != nil {
			return 0, err
		}
	}
	
	return bp.buffer.Read(p)
}

// fillBuffer 实现内容审查和中断
func (bp *BodyProcessor) fillBuffer() error {
	for {
		events, decodeErr := bp.decoder.Decode()
		if decodeErr != nil {
			bp.err = decodeErr
			return decodeErr
		}
		if len(events) == 0 {
			bp.err = io.EOF
			// eof is not an error for fillbuffer, just break the loop
			break
		}
		// 处理事件
		for _, processor := range bp.processors {
			if len(events) == 0 {
				break // 没有事件可处理
			}
			var processErr error
			events, processErr = processor.Process(events)
			if processErr != nil {
				bp.err = processErr
				// 检查是否为中断错误
				if cvErr, ok := processErr.(*RejectionError); ok {
					bp.handleRejection(cvErr)
					return cvErr
				}
				return processErr
			}
		}
		// 编码事件
		n, encodeErr := bp.encoder.Encode(events)
		if encodeErr != nil {
			bp.err = encodeErr
			return encodeErr
		}
		if n > 0 {
			break // 至少有一个事件被处理
		}
	}
	return nil
}

// handleRejection 处理内容违规事件
func (bp *BodyProcessor) handleRejection(err *RejectionError) {
	bp.rejection = err
	
	// 触发回调
	if bp.onReject != nil {
		bp.onReject(err, bp)
	}
}

// RejectionResponse 获取中断响应（如在反向代理中使用）
func (bp *BodyProcessor) RejectionResponse() *RejectionError {
	// bp.mu.Lock()
	// defer bp.mu.Unlock()
	return bp.rejection
}

// Close 实现io.Closer接口
func (bp *BodyProcessor) Close() error {
	// bp.mu.Lock()
	// defer bp.mu.Unlock()
	
	bp.buffer.Reset()
	return bp.source.Close()
}

// FillBuffer 公开的缓冲区填充方法
// 安全地从源读取并处理一个数据块
func (bp *BodyProcessor) FillBuffer() error {
	// bp.mu.Lock()
	// defer bp.mu.Unlock()
	
	if bp.err != nil {
		return bp.err
	}
	
	return bp.fillBuffer()
}

func (m *ModuleBodyProcess) DoRequestProcess(req *bfe_basic.Request, conf *BodyProcessConfig) *BodyProcessor {
	if conf == nil {
		return nil // 没有配置，直接返回
	}

	m.state.ReqProcess.Inc(1)

	bp := NewBodyProcessor(req.HttpRequest.Body)
	switch conf.Dec {
	// case "sse":  // sse is not available for request body
	// 	bp.CreateEventDecoder(NewSSEEventDecoder)
	case "line":
		bp.CreateEventDecoder(NewLineDecoder)
	case "json":
		bp.CreateEventDecoder(NewJsonDecoder)
	default:
		contentType := req.HttpRequest.Header.Get("Content-Type")
		bp.CreateEventDecoder(func(source io.Reader)(EventDecoder, error) {
			return NewContentTypeDecoder(source, contentType)} ) // 使用ContentTypeDecoder根据Content-Type自动选择解码器
		// bp.CreateEventDecoder(NewJsonDecoder) // 默认使用ndJson解码
	}
	bp.CreateEventEncoder(NewGeneralEncoder)
	for _, proc := range conf.Proc {
		switch proc.Name {
		case "textfilter":
			caf, _ := NewContentAudit(proc.Params[0], false)
			bp.AddProcessor(caf)
		}
	}
	
	req.HttpRequest.Body = bp
	req.HttpRequest.ContentLength = -1 // 设置为-1表示不确定长度
	req.HttpRequest.Header.Del("Content-Length")
	return bp
}

func (m *ModuleBodyProcess) DoResponseProcess(req *bfe_basic.Request, res *bfe_http.Response, conf *BodyProcessConfig) *BodyProcessor {
	// 检查是否需要处理streamcompletion
	ccq := NewCalcCompletionQuota(req)

	if conf == nil && ccq == nil {
		return nil // 没有配置，直接返回
	}

	m.state.ResProcess.Inc(1)
	
	bp := NewBodyProcessor(res.Body)
	// 缺省添加streamcompletion处理器
	if ccq != nil {
		bp.AddProcessor(ccq)
	}

	var dec string
	if conf != nil {
		dec = conf.Dec
	}

	switch dec {
	case "sse":  // sse is not available for request body
		bp.CreateEventDecoder(NewSSEEventDecoder)
	case "line":
		bp.CreateEventDecoder(NewLineDecoder)
	case "json":
		bp.CreateEventDecoder(NewJsonDecoder)
	default:
		contentType := res.Header.Get("Content-Type")
		bp.CreateEventDecoder(func(source io.Reader)(EventDecoder, error) {
			return NewContentTypeDecoder(source, contentType)} ) // 使用ContentTypeDecoder根据Content-Type自动选择解码器
		// bp.CreateEventDecoder(NewJsonDecoder) // 默认使用ndJson解码
	}

	bp.CreateEventEncoder(NewGeneralEncoder)

	if conf != nil {
		for _, proc := range conf.Proc {
			switch proc.Name {
			case "textfilter":
				caf, _ := NewContentAudit(proc.Params[0], true)
				bp.AddProcessor(caf)
			}
		}
	}

	res.Body = bp
	res.ContentLength = -1 // 设置为-1表示不确定长度
	res.Header.Del("Content-Length")
	return bp
}
/*
func (m *ModuleBodyProcess) DoResponseProcess(req *bfe_basic.Request, res *bfe_http.Response, conf *BodyProcessConfig) *BodyProcessor {
	if conf == nil {
		return nil // 没有配置，直接返回
	}

	m.state.ResProcess.Inc(1)

	bp := NewBodyProcessor(res.Body)
	switch conf.Dec {
	case "sse":  // sse is not available for request body
		bp.CreateEventDecoder(NewSSEEventDecoder)
	case "line":
		bp.CreateEventDecoder(NewLineDecoder)
	case "json":
		bp.CreateEventDecoder(NewJsonDecoder)
	default:
		contentType := res.Header.Get("Content-Type")
		bp.CreateEventDecoder(func(source io.Reader)(EventDecoder, error) {
			return NewContentTypeDecoder(source, contentType)} ) // 使用ContentTypeDecoder根据Content-Type自动选择解码器
		// bp.CreateEventDecoder(NewJsonDecoder) // 默认使用ndJson解码
	}
	
	bp.CreateEventEncoder(NewGeneralEncoder)

	// 缺省添加streamcompletion处理器
	p := NewCalcCompletionQuota(req)
	if p != nil {
		bp.AddProcessor(p)
	}

	for _, proc := range conf.Proc {
		switch proc.Name {
		case "textfilter":
			caf, _ := NewContentAudit(proc.Params[0], true)
			bp.AddProcessor(caf)
		}
	}

	res.Body = bp
	res.ContentLength = -1 // 设置为-1表示不确定长度
	res.Header.Del("Content-Length")
	return bp
}
*/
// SSEEvent 表示一个SSE事件
type SSEEvent struct {
	ID    string
	Event string
	Data  []byte
	Retry int
	// raw   []byte // 原始事件数据
	truncated bool // 是否被截断
}

// ToBytes 将事件转换为SSE格式
func (e *SSEEvent) ToBytes() []byte {
	var buf bytes.Buffer
	if e.ID != "" {
		buf.WriteString("id: " + e.ID + "\n")
	}
	if e.Event != "" {
		buf.WriteString("event: " + e.Event + "\n")
	}
	if len(e.Data) > 0 {
		lines := strings.Split(string(e.Data), "\n")
		for _, line := range lines {
			buf.WriteString("data: " + line + "\n")
		}
	}
	if e.Retry > 0 {
		buf.WriteString(fmt.Sprintf("retry: %d\n", e.Retry))
	}
	if !e.truncated {
		buf.WriteString("\n")
	}
	return buf.Bytes()
}

type GeneralEncoder struct {
	dest io.Writer
}

func NewGeneralEncoder(dest io.Writer) (EventEncoder, error) {
	return &GeneralEncoder{dest: dest}, nil
}

func (enc *GeneralEncoder) Encode(events []Event) (int, error) {
	var total int
	for _, event := range events {
		data := event.ToBytes()
		n, err := enc.dest.Write(data)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

type SSEEventDecoder struct {
	scanner *bufio.Scanner
}

func NewSSEEventDecoder(source io.Reader) (EventDecoder, error) {
	scanner := bufio.NewScanner(source)
	return &SSEEventDecoder{scanner: scanner}, nil
}

func (dec *SSEEventDecoder) Decode() ([]Event, error) {
	var current SSEEvent
	dataLines := []string{}
	for dec.scanner.Scan() {
		line := dec.scanner.Text()
		if line == "" {
			// 空行表示一个完整的事件结束
			if len(dataLines) == 0 && current.Event == "" && current.ID == "" && len(current.Data) == 0 {
				continue // 跳过空事件
			}
			current.Data = []byte(strings.Join(dataLines, "\n"))
			return []Event{&current}, nil
		}

		// 解析SSE事件
		if strings.HasPrefix(line, "event:") {
			current.Event = strings.TrimSpace(line[6:])
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(line[5:]))
		} else if strings.HasPrefix(line, "id:") {
			current.ID = strings.TrimSpace(line[3:])
		} else if strings.HasPrefix(line, "retry:") {
			var retry int
			_, err := fmt.Sscanf(line[6:], "%d", &retry)
			if err != nil {
				return nil, fmt.Errorf("invalid retry value: %s", line[6:])
			}
			current.Retry = retry
		} else {
			// 未知的SSE行，可能需要处理或忽略
			return nil, fmt.Errorf("unknown SSE line: %s", line)
		}
	}
	// 检查是否有未完成的事件
	if len(dataLines) > 0 || current.Event != "" || current.ID != "" {
		current.Data = []byte(strings.Join(dataLines, "\n"))
		current.truncated = true // 标记为被截断
		return []Event{&current}, nil
	}

	return []Event{}, dec.scanner.Err()
}

type RawEvent []byte

func (e *RawEvent) ToBytes() []byte {
	return *e
}

type LineDecoder struct {
	reader *bufio.Reader
}

func NewLineDecoder(source io.Reader) (EventDecoder, error) {
	reader := bufio.NewReader(source)
	return &LineDecoder{reader: reader}, nil
}

func (dec *LineDecoder) Decode() ([]Event, error) {
	line, err := dec.reader.ReadBytes('\n')
	if len(line) != 0 {
		re := RawEvent(line)
		return []Event{&re}, nil
	}
	if err == io.EOF {
		return []Event{}, nil // 没有更多数据
	}
	return nil, fmt.Errorf("line decode error: %w", err)
}

type JsonDecoder struct {
	dec *json.Decoder
}

func NewJsonDecoder(source io.Reader) (EventDecoder, error) {
	dec := json.NewDecoder(source)
	return &JsonDecoder{dec: dec}, nil
}

func (dec *JsonDecoder) Decode() ([]Event, error) {
	var event json.RawMessage
	// 尝试解码一个JSON对象
	if err := dec.dec.Decode(&event); err != nil {
		if err == io.EOF {
			return []Event{}, nil // 没有更多数据
		}
		return nil, fmt.Errorf("json decode error: %w", err)
	}
	re := RawEvent(event)
	return []Event{&re}, nil
}

type ContentTypeDecoder struct {
	contentType string
	dec         EventDecoder
}

func NewContentTypeDecoder(source io.Reader, contentType string) (EventDecoder, error) {
	var dec EventDecoder
	switch contentType {
	case "application/sse", "text/event-stream", "application/x-sse":
		dec, _ = NewSSEEventDecoder(source)
	case "application/json", "application/ndjson", "application/x-ndjson":
		dec, _ = NewJsonDecoder(source) // ndjson is a line-delimited JSON, can use JsonDecoder
	default:
		dec, _ = NewLineDecoder(source) // 默认使用行解码器
	}

	return &ContentTypeDecoder{contentType: contentType, dec: dec}, nil
}

func (ctd *ContentTypeDecoder) Decode() ([]Event, error) {
	return ctd.dec.Decode()
}

func GetEventTokens(ev Event) int64 {
	if ev == nil {
		return 0
	}

	switch e := ev.(type) {
	case *RawEvent:
		return int64(len(*e)/4)
	case *SSEEvent:
		return int64(len(e.Data)/4)
	default:
		return 0
	}
}

func NewCalcCompletionQuota(req *bfe_basic.Request) EventProcessorFunc {
	ctx := mod_ai_token_auth.GetTokenAuthContext(req)
	if ctx == nil || ctx.CompletionTokens != -1 {
		return nil // 没有token上下文，或 CompletionTokens 已知，无需计算
	}
	return func(events []Event) ([]Event, error) {
		for _, ev := range events {
			if ctx.CompletionTokens == -1 {
				ctx.CompletionTokens = 0 // 初始化为0
			}
			// 累加事件的token数
			ctx.CompletionTokens += GetEventTokens(ev)
		}
		return events, nil // 没有事件，直接返回
	}
}

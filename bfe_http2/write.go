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
	"log"
	"time"
)

import (
	http "github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_http2/hpack"
)

// writeFramer is implemented by any type that is used to write frames.
type writeFramer interface {
	writeFrame(writeContext) error
}

// writeContext is the interface needed by the various frame writer
// types below. All the writeFrame methods below are scheduled via the
// frame writing scheduler (see writeScheduler in writesched.go).
//
// This interface is implemented by *serverConn.
//
// TODO: decide whether to a) use this in the client code (which didn't
// end up using this yet, because it has a simpler design, not
// currently implementing priorities), or b) delete this and
// make the server code a bit more concrete.
type writeContext interface {
	Framer() *Framer
	Flush() error
	CloseConn() error
	// HeaderEncoder returns an HPACK encoder that writes to the
	// returned buffer.
	HeaderEncoder() (*hpack.Encoder, *bytes.Buffer)
}

// endsStream reports whether the given frame writer w will locally
// close the stream.
func endsStream(w writeFramer) bool {
	switch v := w.(type) {
	case *writeData:
		return v.endStream
	case *writeResHeaders:
		return v.endStream
	case nil:
		// This can only happen if the caller reuses w after it's
		// been intentionally nil'ed out to prevent use. Keep this
		// here to catch future refactoring breaking it.
		panic("endsStream called on nil writeFramer")
	}
	return false
}

type flushFrameWriter struct{}

func (flushFrameWriter) writeFrame(ctx writeContext) error {
	return ctx.Flush()
}

type closeFrameWriter struct{}

func (closeFrameWriter) writeFrame(ctx writeContext) error {
	return ctx.CloseConn()
}

type writeSettings []Setting

func (s writeSettings) writeFrame(ctx writeContext) error {
	return ctx.Framer().WriteSettings([]Setting(s)...)
}

type writeGoAway struct {
	maxStreamID uint32
	code        ErrCode
}

func (p *writeGoAway) writeFrame(ctx writeContext) error {
	err := ctx.Framer().WriteGoAway(p.maxStreamID, p.code, nil)
	if p.code != 0 {
		ctx.Flush() // ignore error: we're hanging up on them anyway
		time.Sleep(50 * time.Millisecond)
		ctx.CloseConn()
	}
	return err
}

type writeData struct {
	streamID  uint32
	p         []byte
	endStream bool
}

func (w *writeData) String() string {
	return fmt.Sprintf("writeData(stream=%d, p=%d, endStream=%v)", w.streamID, len(w.p), w.endStream)
}

func (w *writeData) writeFrame(ctx writeContext) error {
	return ctx.Framer().WriteData(w.streamID, w.endStream, w.p)
}

// handlerPanicRST is the message sent from handler goroutines when
// the handler panics.
type handlerPanicRST struct {
	StreamID uint32
}

func (hp handlerPanicRST) writeFrame(ctx writeContext) error {
	return ctx.Framer().WriteRSTStream(hp.StreamID, ErrCodeInternal)
}

func (se StreamError) writeFrame(ctx writeContext) error {
	return ctx.Framer().WriteRSTStream(se.StreamID, se.Code)
}

type writePingAck struct{ pf *PingFrame }

func (w writePingAck) writeFrame(ctx writeContext) error {
	return ctx.Framer().WritePing(true, w.pf.Data)
}

type writeSettingsAck struct{}

func (writeSettingsAck) writeFrame(ctx writeContext) error {
	return ctx.Framer().WriteSettingsAck()
}

// writeResHeaders is a request to write a HEADERS and 0+ CONTINUATION frames
// for HTTP response headers or trailers from a server handler.
type writeResHeaders struct {
	streamID    uint32
	httpResCode int         // 0 means no ":status" line
	h           http.Header // may be nil
	trailers    []string    // if non-nil, which keys of h to write. nil means all.
	endStream   bool

	date          string
	contentType   string
	contentLength string
}

func encKV(enc *hpack.Encoder, k, v string) int {
	if VerboseLogs {
		log.Printf("http2: server encoding header %q = %q", k, v)
	}
	enc.WriteField(hpack.HeaderField{Name: k, Value: v})
	return len(k) + len(v) // size of original header field
}

func (w *writeResHeaders) writeFrame(ctx writeContext) error {
	headerSize := 0 // original header size
	enc, buf := ctx.HeaderEncoder()
	buf.Reset()

	if w.httpResCode != 0 {
		encKV(enc, ":status", httpCodeString(w.httpResCode))
	}

	headerSize += encodeHeaders(enc, w.h, w.trailers)

	if w.contentType != "" {
		headerSize += encKV(enc, "content-type", w.contentType)
	}
	if w.contentLength != "" {
		headerSize += encKV(enc, "content-length", w.contentLength)
	}
	if w.date != "" {
		headerSize += encKV(enc, "date", w.date)
	}

	headerBlock := buf.Bytes()
	if len(headerBlock) == 0 && w.trailers == nil {
		panic("unexpected empty hpack")
	}

	state.H2ResHeaderOriginalSize.Inc(uint(headerSize))
	state.H2ResHeaderCompressSize.Inc(uint(len(headerBlock)))

	// For now we're lazy and just pick the minimum MAX_FRAME_SIZE
	// that all peers must support (16KB). Later we could care
	// more and send larger frames if the peer advertised it, but
	// there's little point. Most headers are small anyway (so we
	// generally won't have CONTINUATION frames), and extra frames
	// only waste 9 bytes anyway.
	const maxFrameSize = 16384

	first := true
	for len(headerBlock) > 0 {
		frag := headerBlock
		if len(frag) > maxFrameSize {
			frag = frag[:maxFrameSize]
		}
		headerBlock = headerBlock[len(frag):]
		endHeaders := len(headerBlock) == 0
		var err error
		if first {
			first = false
			err = ctx.Framer().WriteHeaders(HeadersFrameParam{
				StreamID:      w.streamID,
				BlockFragment: frag,
				EndStream:     w.endStream,
				EndHeaders:    endHeaders,
			})
		} else {
			err = ctx.Framer().WriteContinuation(w.streamID, endHeaders, frag)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

type write100ContinueHeadersFrame struct {
	streamID uint32
}

func (w write100ContinueHeadersFrame) writeFrame(ctx writeContext) error {
	enc, buf := ctx.HeaderEncoder()
	buf.Reset()
	encKV(enc, ":status", "100")
	return ctx.Framer().WriteHeaders(HeadersFrameParam{
		StreamID:      w.streamID,
		BlockFragment: buf.Bytes(),
		EndStream:     false,
		EndHeaders:    true,
	})
}

type writeWindowUpdate struct {
	streamID uint32 // or 0 for conn-level
	n        uint32
}

func (wu writeWindowUpdate) writeFrame(ctx writeContext) error {
	return ctx.Framer().WriteWindowUpdate(wu.streamID, wu.n)
}

func (wu *writeWindowUpdate) String() string {
	return fmt.Sprintf("writeWindowUpdate(stream=%d, n=%d)", wu.streamID, wu.n)
}

func encodeHeaders(enc *hpack.Encoder, h http.Header, keys []string) int {
	headerSize := 0 // original header size
	if keys == nil {
		sorter := sorterPool.Get().(*sorter)
		// Using defer here, since the returned keys from the
		// sorter.Keys method is only valid until the sorter
		// is returned:
		defer sorterPool.Put(sorter)
		keys = sorter.Keys(h)
	}
	for _, k := range keys {
		vv := h[k]
		k = lowerHeader(k)
		if !validHeaderFieldName(k) {
			// TODO: return an error? golang.org/issue/14048
			// For now just omit it.
			continue
		}
		isTE := k == "transfer-encoding"
		for _, v := range vv {
			if !validHeaderFieldValue(v) {
				// TODO: return an error? golang.org/issue/14048
				// For now just omit it.
				continue
			}
			// TODO: more of "8.1.2.2 Connection-Specific Header Fields"
			if isTE && v != "trailers" {
				continue
			}
			headerSize += encKV(enc, k, v)
		}
	}
	return headerSize
}

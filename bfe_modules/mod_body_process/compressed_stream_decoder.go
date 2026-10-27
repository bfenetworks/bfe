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
	"errors"
	"io"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_util"
)

// maxPlaintextPerResponse bounds the decompressed bytes parsed for usage in one
// response (issue #1468). A compressed stream can expand arbitrarily
// (decompression bomb) while the usage chunk only arrives at the end, so the
// budget is deliberately generous: going past it stops the usage parse and
// leaves the request to the estimate fallback, never blocking the forward.
const maxPlaintextPerResponse int64 = 64 << 20

// tapEvent carries two views of one stream position: raw is the still
// compressed byte range read from the upstream and forwarded verbatim, plain is
// the plaintext event produced by decompressing that range and used only for
// usage parsing. The plaintext is never re-encoded into the forwarded stream
// (issue #1468).
type tapEvent struct {
	raw   []byte
	plain Event // nil for flush / raw-only events
}

func (e *tapEvent) ToBytes() []byte {
	return e.raw
}

func (e *tapEvent) GetQuotaUsage(authStyle string) QuotaUsage {
	if e.plain == nil {
		// A raw-only event carries no plaintext, so it neither reports usage
		// nor contributes to the estimate.
		return QuotaUsage{IsGuess: true}
	}
	return e.plain.GetQuotaUsage(authStyle)
}

// CompressedStreamDecoder parses a compressed streaming response without
// changing the bytes forwarded to the client (issue #1468).
//
// The source is teed into a decompressor used only for parsing: the inner
// decoder reads plaintext events from it, while every byte the decompressor
// consumed is emitted verbatim through tapEvent.ToBytes(). Each upstream byte is
// therefore forwarded exactly once and in order; the plaintext only feeds the
// usage parser.
//
// Any decompression or parse error, or exceeding maxPlaintextPerResponse,
// switches the decoder to a raw-only pass-through (LineDecoder semantics), so
// the forward is never truncated and the request simply falls back to the
// estimate path.
type CompressedStreamDecoder struct {
	src     io.Reader
	raw     bytes.Buffer // bytes consumed from src but not forwarded yet
	dec     io.ReadCloser
	inner   EventDecoder
	reader  *bufio.Reader // lazily created raw pass-through reader
	plain   int64         // decompressed bytes parsed so far
	rawOnly bool
	done    bool

	// onRawOnly is called once when the decoder degrades to pass-through.
	onRawOnly func()
}

// NewCompressedStreamDecoder builds a tap decoder for the given Content-Encoding
// value. fac builds the plaintext event decoder (e.g. NewSSEEventDecoder).
func NewCompressedStreamDecoder(source io.Reader, encoding string, fac EventDecoderFac) (*CompressedStreamDecoder, error) {
	d := &CompressedStreamDecoder{src: source}

	dec, err := bfe_util.NewContentDecoder(encoding, io.TeeReader(source, &d.raw))
	if err != nil {
		return nil, err
	}
	d.dec = dec

	inner, err := fac(dec)
	if err != nil {
		dec.Close()
		return nil, err
	}
	d.inner = inner

	return d, nil
}

// SetRawOnlyCallback registers the callback invoked when the decoder degrades
// to a raw pass-through, so the caller can count the degradation.
func (d *CompressedStreamDecoder) SetRawOnlyCallback(fn func()) {
	d.onRawOnly = fn
}

// Decode returns the next forwarded chunk, wrapping the plaintext event it was
// decompressed from when one is available.
func (d *CompressedStreamDecoder) Decode() ([]Event, error) {
	if d.done {
		return []Event{}, nil
	}

	if d.rawOnly {
		return d.decodeRaw()
	}

	if d.plain > maxPlaintextPerResponse {
		d.enterRawOnly(nil)
		return d.flushOrRaw()
	}

	for {
		events, err := d.inner.Decode()
		raw := d.drainRaw()

		if len(events) > 0 {
			for _, ev := range events {
				d.plain += int64(len(ev.ToBytes()))
			}
			return wrapTapEvents(events, raw), nil
		}

		// The BodyProcessor contract treats an empty event batch as end of
		// stream (fillBuffer sets io.EOF on it), and SSEEventDecoder returns
		// exactly that at EOF — so this is the only exit besides a real error.
		if err != nil && !errors.Is(err, io.EOF) {
			d.enterRawOnly(err)
			raw = append(raw, d.drainRaw()...)
			if len(raw) > 0 {
				return []Event{&tapEvent{raw: raw}}, nil
			}
			return d.decodeRaw()
		}

		// The plaintext stream ended: pull whatever the decompressor still
		// holds, then pass the rest of the source through verbatim so the gzip
		// trailer and any trailing bytes reach the client. A clean end is not a
		// degradation, hence no callback and no warning.
		d.drainDecoder()
		raw = append(raw, d.drainRaw()...)
		d.rawOnly = true
		if len(raw) > 0 {
			return []Event{&tapEvent{raw: raw}}, nil
		}
		return d.decodeRaw()
	}
}

// Close releases the decompressor. The source itself is closed by
// BodyProcessor.Close.
func (d *CompressedStreamDecoder) Close() error {
	if d.dec != nil {
		return d.dec.Close()
	}
	return nil
}

// enterRawOnly records the degradation once and reports it to the caller.
func (d *CompressedStreamDecoder) enterRawOnly(cause error) {
	if d.rawOnly {
		return
	}
	d.rawOnly = true
	if cause != nil {
		log.Logger.Warn("mod_body_process: decompress stream body failed, "+
			"falling back to pass-through: %v", cause)
	} else {
		log.Logger.Warn("mod_body_process: decompressed body exceeds %d bytes, "+
			"usage parse stopped", maxPlaintextPerResponse)
	}
	if d.onRawOnly != nil {
		d.onRawOnly()
	}
}

// flushOrRaw forwards the bytes already consumed, otherwise continues in
// pass-through mode.
func (d *CompressedStreamDecoder) flushOrRaw() ([]Event, error) {
	if raw := d.drainRaw(); len(raw) > 0 {
		return []Event{&tapEvent{raw: raw}}, nil
	}
	return d.decodeRaw()
}

// decodeRaw forwards the remaining source bytes verbatim, one line at a time,
// which is exactly the pre-existing LineDecoder behaviour.
func (d *CompressedStreamDecoder) decodeRaw() ([]Event, error) {
	if d.reader == nil {
		d.reader = bufio.NewReader(d.src)
	}

	line, err := d.reader.ReadBytes('\n')
	if len(line) != 0 {
		return []Event{&tapEvent{raw: line}}, nil
	}
	if err == io.EOF {
		d.done = true
		return []Event{}, nil
	}
	if err != nil {
		return nil, err
	}
	return []Event{}, nil
}

// drainRaw moves the bytes consumed since the last call out of the tee buffer.
func (d *CompressedStreamDecoder) drainRaw() []byte {
	if d.raw.Len() == 0 {
		return nil
	}
	out := make([]byte, d.raw.Len())
	copy(out, d.raw.Bytes())
	d.raw.Reset()
	return out
}

// drainDecoder reads the decompressor to its end so its internal buffer is
// pushed through the tee (and thus forwarded), bounded by the plaintext budget.
func (d *CompressedStreamDecoder) drainDecoder() {
	if d.dec == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(d.dec, d.plainRemaining()))
}

func (d *CompressedStreamDecoder) plainRemaining() int64 {
	remaining := maxPlaintextPerResponse - d.plain
	if remaining < 0 {
		return 0
	}
	return remaining
}

// wrapTapEvents attaches the forwarded (compressed) bytes to the first plaintext
// event of the batch.
func wrapTapEvents(events []Event, raw []byte) []Event {
	out := make([]Event, 0, len(events))
	for i, ev := range events {
		te := &tapEvent{plain: ev}
		if i == 0 {
			te.raw = raw
		}
		out = append(out, te)
	}
	return out
}

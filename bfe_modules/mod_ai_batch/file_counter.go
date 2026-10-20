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

package mod_ai_batch

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"

	"github.com/bfenetworks/bfe/bfe_basic"
)

// errUploadLimitExceeded aborts a streamed upload whose true file size exceeds
// the effective file limits (only known while streaming). The client observes a
// truncated/aborted request; the counter records it.
var errUploadLimitExceeded = errors.New("mod_ai_batch: upload exceeds effective file limits")

// uploadPartStats describes the `file` part of a buffered multipart upload.
// Bytes/lines always describe the FILE CONTENT, never the envelope (part
// headers, boundary lines and CRLFs are excluded): the envelope inflates a
// one-line 206-byte jsonl to 11 lines / 509 bytes, which then also inflates
// est_lines and the reserved amount.
type uploadPartStats struct {
	Filename string
	Lines    int64
	Bytes    int64
}

// multipartFilePartStats extracts the first `file` part of a (possibly
// truncated) multipart/form-data body. ok=false means the body could not be
// parsed as multipart or the `file` part content is not complete within body:
// callers must fall back to approximate counting and mark the binding
// accordingly.
func multipartFilePartStats(boundary string, body []byte) (stats uploadPartStats, ok bool) {
	if len(body) == 0 || boundary == "" {
		return uploadPartStats{}, false
	}

	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, perr := mr.NextPart()
		if perr != nil {
			return uploadPartStats{}, false
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		filename := part.FileName()
		n, lines, cerr := countPartContent(part)
		part.Close()
		if cerr != nil {
			return uploadPartStats{}, false
		}
		return uploadPartStats{Filename: filename, Lines: lines, Bytes: n}, true
	}
}

// countPartContent counts the bytes and newlines of a part's content with
// O(1) memory.
func countPartContent(r io.Reader) (bytesCount, lines int64, err error) {
	buf := make([]byte, 32*1024)
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			bytesCount += int64(n)
			lines += int64(bytes.Count(buf[:n], []byte{'\n'}))
		}
		if rerr != nil {
			if rerr == io.EOF {
				return bytesCount, lines, nil
			}
			return bytesCount, lines, rerr
		}
	}
}

// newUploadLimitBody installs the outgoing-body limit gate. Byte/line limits
// are defined on the `file` part content (issue #1434): a multipart envelope is
// measured on that part only, everything else on the whole body.
func newUploadLimitBody(source io.ReadCloser, boundary string, ctx *batchContext,
	aiMeta *bfe_basic.AiBasicInfo) io.ReadCloser {

	if boundary == "" {
		return newEnvelopeCountingBody(source, ctx, aiMeta)
	}
	return &multipartLimitBody{
		src:    source,
		ctx:    ctx,
		aiMeta: aiMeta,
		bound:  []byte("--" + boundary),
		delim:  []byte("\r\n--" + boundary),
	}
}

// envelopeCountingBody wraps the outgoing request body and counts the WHOLE
// body while streaming to the upstream, with O(1) memory. It is the degraded
// path of newUploadLimitBody: used for uploads that are not multipart
// envelopes, or whose boundary is missing, where the envelope IS the file.
//
// When the effective limits (already resolved into AiBasicInfo by
// mod_ai_rate_limit) are exceeded, reads start failing so the upload aborts.
type envelopeCountingBody struct {
	source io.ReadCloser
	ctx    *batchContext
	aiMeta *bfe_basic.AiBasicInfo
}

func newEnvelopeCountingBody(source io.ReadCloser, ctx *batchContext,
	aiMeta *bfe_basic.AiBasicInfo) *envelopeCountingBody {

	return &envelopeCountingBody{source: source, ctx: ctx, aiMeta: aiMeta}
}

func (c *envelopeCountingBody) Read(p []byte) (int, error) {
	if c.ctx.uploadAborted {
		return 0, errUploadLimitExceeded
	}
	n, err := c.source.Read(p)
	if n > 0 {
		c.ctx.uploadBytes += int64(n)
		for _, b := range p[:n] {
			if b == '\n' {
				c.ctx.uploadLines++
			}
		}
		if c.exceedsLimits() {
			c.ctx.uploadAborted = true
			return n, errUploadLimitExceeded
		}
	}
	return n, err
}

func (c *envelopeCountingBody) exceedsLimits() bool {
	if lim := c.aiMeta.BatchEffMaxFileBytes; lim > 0 && c.ctx.uploadBytes > lim {
		return true
	}
	if lim := c.aiMeta.BatchEffMaxFileLines; lim > 0 && c.ctx.uploadLines > lim {
		return true
	}
	return false
}

func (c *envelopeCountingBody) Close() error {
	return c.source.Close()
}

// multipart parsing states of multipartLimitBody.
const (
	mpPreamble = iota // looking for the first "--<boundary>"
	mpHeaders         // collecting one part's headers (until CRLFCRLF)
	mpBody            // forwarding one part's content
	mpEpilogue        // final "--<boundary>--": nothing left to measure
)

// maxPartHeaderBytes caps the headers of a single part; a larger header block
// is treated as unparsable and the gate degrades to whole-body counting.
const maxPartHeaderBytes = 8 * 1024

var crlfcrlf = []byte("\r\n\r\n")

// multipartLimitBody forwards an outgoing multipart upload byte-for-byte while
// counting only the `file` part content against the effective file limits.
// It is the streaming counterpart of multipartFilePartStats: without it a
// legal 1-line/138-byte file in a 11-line/502-byte envelope was aborted as
// soon as the envelope crossed max_file_lines (issue #1434).
//
// Bytes already handed to the transport but not yet classified (the tail of a
// possible "\r\n--<boundary>" delimiter) are kept in pend, so the parser
// survives delimiters and headers split across reads. Counting a few bytes
// late is harmless: the abort threshold is off by at most one delimiter.
type multipartLimitBody struct {
	src    io.ReadCloser
	ctx    *batchContext
	aiMeta *bfe_basic.AiBasicInfo

	bound []byte // "--" + boundary
	delim []byte // "\r\n--" + boundary
	state int

	pend []byte // emitted downstream, not yet classified

	counting bool // payload bytes currently count as file content
	fileSeen bool // the first `file` part has been seen
	finished bool // the `file` part content is complete

	partBytes int64
	partLines int64

	aborted bool
}

func (b *multipartLimitBody) Read(p []byte) (int, error) {
	if b.aborted {
		return 0, errUploadLimitExceeded
	}
	n, err := b.src.Read(p)
	if n > 0 {
		// envelope counters stay whole-body: they are the approximate
		// binding fallback when the `file` part cannot be located
		b.ctx.uploadBytes += int64(n)
		b.ctx.uploadLines += int64(bytes.Count(p[:n], []byte{'\n'}))
		b.feed(p[:n])
		if b.overLimit() {
			b.aborted = true
			b.ctx.uploadAborted = true
			return n, errUploadLimitExceeded
		}
	}
	if err == io.EOF {
		b.finalize()
	}
	return n, err
}

func (b *multipartLimitBody) Close() error {
	return b.src.Close()
}

func (b *multipartLimitBody) feed(data []byte) {
	b.pend = append(b.pend, data...)
	b.processPending()
}

func (b *multipartLimitBody) finalize() {
	if b.state == mpBody {
		b.processBody(true)
	}
	b.pend = b.pend[:0]
}

func (b *multipartLimitBody) overLimit() bool {
	// no `finished` shortcut: the file part may complete inside the very chunk
	// that exceeds a limit, and the numbers stop growing after it completes
	if lim := b.aiMeta.BatchEffMaxFileBytes; lim > 0 && b.partBytes > lim {
		return true
	}
	if lim := b.aiMeta.BatchEffMaxFileLines; lim > 0 && b.partLines > lim {
		return true
	}
	return false
}

// processPending advances the parser as far as the buffered bytes allow.
func (b *multipartLimitBody) processPending() {
	for {
		switch b.state {
		case mpPreamble:
			idx := bytes.Index(b.pend, b.bound)
			if idx < 0 {
				if len(b.pend) > maxPartHeaderBytes {
					b.degrade()
					return
				}
				// nothing before the first boundary can be file content, but a
				// boundary split across reads must survive the trim
				if keep := len(b.bound) - 1; len(b.pend) > keep {
					b.pend = append(b.pend[:0], b.pend[len(b.pend)-keep:]...)
				}
				return
			}
			rest := b.pend[idx+len(b.bound):]
			if len(rest) < 2 {
				// keep the boundary marker until it can be classified
				b.pend = append(b.pend[:0], b.pend[idx:]...)
				return
			}
			if rest[0] == '-' && rest[1] == '-' {
				b.state = mpEpilogue
				b.pend = b.pend[:0]
				return
			}
			if rest[0] == '\r' && rest[1] == '\n' {
				b.state = mpHeaders
				b.pend = append(b.pend[:0], rest[2:]...)
				continue
			}
			// "--<boundary>" that is not a delimiter: rescan right after it
			b.pend = append(b.pend[:0], b.pend[idx+1:]...)

		case mpHeaders:
			idx := bytes.Index(b.pend, crlfcrlf)
			if idx < 0 {
				if len(b.pend) > maxPartHeaderBytes {
					b.degrade()
					return
				}
				return
			}
			b.beginPart(b.pend[:idx])
			// beginPart may degrade; the header block is consumed either way
			b.pend = append(b.pend[:0], b.pend[idx+4:]...)
			if b.state != mpEpilogue {
				b.state = mpBody
			}

		case mpBody:
			if !b.processBody(false) {
				return
			}

		case mpEpilogue:
			if b.counting {
				// degraded parse: count the rest conservatively as file
				// content so the limits cannot be bypassed
				b.countPayload(b.pend)
			}
			b.pend = b.pend[:0]
			return
		}
	}
}

// processBody consumes the part content up to the next delimiter. It returns
// true when the parser moved to another state and the caller must continue.
// With final set (EOF) the whole pending buffer is classified, which lets the
// trailing delimiter be excluded from the file content.
func (b *multipartLimitBody) processBody(final bool) bool {
	keep := len(b.delim) + 2
	limit := len(b.pend) - keep
	if final {
		limit = len(b.pend)
	}
	if limit <= 0 {
		if final {
			b.pend = b.pend[:0]
		}
		return false
	}

	i := 0
	for i < limit {
		// search the whole remainder: a delimiter may start before limit and
		// end after it, and such a prefix must never be counted as content
		idx := bytes.Index(b.pend[i:], b.delim)
		if idx < 0 || i+idx >= limit {
			b.countPayload(b.pend[i:limit])
			i = limit
			break
		}
		start := i + idx
		b.countPayload(b.pend[i:start])

		after := b.pend[start+len(b.delim):]
		if len(after) < 2 {
			// EOF right after the delimiter: end of the multipart body
			b.finishPart()
			b.state = mpEpilogue
			b.pend = b.pend[:0]
			return false
		}
		if after[0] == '-' && after[1] == '-' {
			b.finishPart()
			b.state = mpEpilogue
			b.pend = b.pend[:0]
			return false
		}
		if after[0] == '\r' && after[1] == '\n' {
			b.finishPart()
			b.state = mpHeaders
			b.pend = append(b.pend[:0], after[2:]...)
			return true
		}
		// the delimiter prefix sits inside the content: it is payload
		b.countPayload(b.pend[start : start+1])
		i = start + 1
	}

	b.pend = append(b.pend[:0], b.pend[i:]...)
	return false
}

// beginPart classifies a part header block. Only the first `file` part is
// measured (matching multipartFilePartStats).
func (b *multipartLimitBody) beginPart(hdr []byte) {
	b.counting = false
	for _, line := range bytes.Split(hdr, []byte("\r\n")) {
		idx := bytes.IndexByte(line, ':')
		if idx <= 0 {
			continue
		}
		if !bytes.EqualFold(bytes.TrimSpace(line[:idx]), []byte("Content-Disposition")) {
			continue
		}
		_, params, err := mime.ParseMediaType(string(bytes.TrimSpace(line[idx+1:])))
		if err != nil {
			b.degrade()
			return
		}
		if params["name"] != "file" || b.fileSeen {
			return
		}
		b.fileSeen = true
		b.counting = true
		b.partBytes, b.partLines = 0, 0
		b.ctx.uploadFilename = params["filename"]
		return
	}
}

// finishPart publishes the exact file statistics once the `file` part content
// ends, so the binding does not fall back to the approximate envelope count.
func (b *multipartLimitBody) finishPart() {
	if b.counting {
		b.finished = true
		b.ctx.uploadPartOK = true
		b.ctx.uploadPartBytes = b.partBytes
		b.ctx.uploadPartLines = b.partLines
	}
	b.counting = false
}

func (b *multipartLimitBody) countPayload(sub []byte) {
	if !b.counting || len(sub) == 0 {
		return
	}
	b.partBytes += int64(len(sub))
	b.partLines += int64(bytes.Count(sub, []byte{'\n'}))
}

// degrade abandons structural parsing. When the `file` part already completed
// its numbers are authoritative and nothing else is measured; otherwise every
// remaining byte is counted as file content (conservative: a false reject is
// preferable to a limit bypass, see sys_design/ai_batch.md).
func (b *multipartLimitBody) degrade() {
	b.state = mpEpilogue
	if b.finished {
		b.counting = false
		return
	}
	b.counting = true
}

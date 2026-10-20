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

// errUploadLimitExceeded aborts a chunked upload whose true size exceeds the
// effective file limits (only known while streaming). The client observes a
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

// multipartFilePartStats extracts the `file` part of a buffered
// multipart/form-data body. ok=false means the body could not be parsed as
// multipart (not buffered, unexpected encoding, or no file part): callers
// must fall back to approximate counting and mark the binding accordingly.
func multipartFilePartStats(contentType string, body []byte) (stats uploadPartStats, ok bool) {
	if len(body) == 0 || contentType == "" {
		return uploadPartStats{}, false
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return uploadPartStats{}, false
	}
	boundary := params["boundary"]
	if boundary == "" {
		return uploadPartStats{}, false
	}

	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, perr := mr.NextPart()
		if perr == io.EOF {
			return uploadPartStats{}, false
		}
		if perr != nil {
			return uploadPartStats{}, false
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		n, lines, cerr := countPartContent(part)
		part.Close()
		if cerr != nil {
			return uploadPartStats{}, false
		}
		return uploadPartStats{Filename: part.FileName(), Lines: lines, Bytes: n}, true
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

// countingBody wraps the outgoing request body (basicReq.OutRequest.Body)
// to count bytes and jsonl lines while streaming to the upstream, with O(1)
// memory. When the effective limits (already resolved into AiBasicInfo by
// mod_ai_rate_limit) are exceeded, reads start failing so the upload aborts.
//
// This counter sees the WHOLE multipart envelope, so its numbers are only
// used to abort oversized streams and as a fallback when the file part could
// not be located (batchContext.uploadPartStats). Binding bytes/lines come
// from that part statistics / the provider File object instead.
type countingBody struct {
	source io.ReadCloser
	ctx    *batchContext
	aiMeta *bfe_basic.AiBasicInfo
}

func newCountingBody(source io.ReadCloser, ctx *batchContext, aiMeta *bfe_basic.AiBasicInfo) *countingBody {
	return &countingBody{source: source, ctx: ctx, aiMeta: aiMeta}
}

func (c *countingBody) Read(p []byte) (int, error) {
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

func (c *countingBody) exceedsLimits() bool {
	if lim := c.aiMeta.BatchEffMaxFileBytes; lim > 0 && c.ctx.uploadBytes > lim {
		return true
	}
	if lim := c.aiMeta.BatchEffMaxFileLines; lim > 0 && c.ctx.uploadLines > lim {
		return true
	}
	return false
}

func (c *countingBody) Close() error {
	return c.source.Close()
}

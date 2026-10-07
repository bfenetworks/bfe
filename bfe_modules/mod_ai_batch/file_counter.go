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
	"errors"
	"io"

	"github.com/bfenetworks/bfe/bfe_basic"
)

// errUploadLimitExceeded aborts a chunked upload whose true size exceeds the
// effective file limits (only known while streaming). The client observes a
// truncated/aborted request; the counter records it.
var errUploadLimitExceeded = errors.New("mod_ai_batch: upload exceeds effective file limits")

// countingBody wraps the outgoing request body (basicReq.OutRequest.Body)
// to count bytes and jsonl lines while streaming to the upstream, with O(1)
// memory. When the effective limits (already resolved into AiBasicInfo by
// mod_ai_rate_limit) are exceeded, reads start failing so the upload aborts.
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

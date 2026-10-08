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
	"github.com/bfenetworks/bfe/bfe_basic"
)

// batchContext carries the per-request batch state between this module's
// callbacks. It is created at HandleAfterAITargetModel (operation
// classification + pre-checks), enriched while the body streams (upload
// counting, response capture/usage parsing), and consumed at
// HandleRequestFinish (bookkeeping, settle contract handoff).
type batchContext struct {
	op string // bfe_basic.BatchOp*

	// identifiers
	fileId       string // path file id (get/download) or response id (upload)
	batchId      string // path batch id (get/cancel) or response id (create)
	inputFileId  string // create request body
	outputFileId string // create/get response body

	// routing snapshot, used for affinity bindings and BATCH_* key scoping
	clusterName string
	keyName     string // selected upstream key (last ClusterKeyName)
	provider    string

	// upload counting (OpUpload)
	uploadBytes     int64
	uploadLines     int64
	uploadAborted   bool // stream aborted because a limit was hit
	contentLength   int64
	knownContentLen bool

	// response capture (small JSON ops)
	respBufOwner *captureBody

	// download usage parsing (OpDownload)
	usageByModel map[string]*bfe_basic.TokenUsage
	usageParsed  bool
	parsedLines  int64

	// quota bookkeeping
	estLines     int64 // lines of the input file, looked up at create
	rmbUnits     int64 // reserve mirror in 1e-8 RMB units
	tokenUnits   int64 // reserve mirror in total_token units
	reserved     bool  // pre-check passed / reserve written at create
	released     bool  // reserve released (terminal status / cancel / settle)
	overSized    bool  // upload/download hit effective limits
	ownerChecked bool
	ownerDenied  bool
}

// getBatchContext returns the per-request batch context, or nil when the
// request was not classified as a batch operation.
func getBatchContext(req *bfe_basic.Request) *batchContext {
	if v := req.GetContext(ctxBatchKey); v != nil {
		if ctx, ok := v.(*batchContext); ok {
			return ctx
		}
	}
	return nil
}

// setBatchContext attaches the context to the request.
func setBatchContext(req *bfe_basic.Request, ctx *batchContext) {
	req.SetContext(ctxBatchKey, ctx)
}

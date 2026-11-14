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

package bal_gslb

import (
	"errors"
	"fmt"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

// EPP admission rejection (issue #1482).
//
// llm-d EPP refuses a request with an ext_proc ImmediateResponse when flow
// control cannot admit it: 429 for backpressure (queue TTL expired / pool
// saturated) and 503 when the pool has no capacity at all. That response is a
// decision, not a failure: bfe must surface it to the client instead of
// retrying another EPP address or falling back to local balancing (which would
// silently forward the request into the saturated pool).
//
// Only 429 and 503 are treated as decisions; any other ImmediateResponse status
// stays on the transport error path so an unexpected EPP reply cannot change
// retry/fallback behaviour.

// EPPRejected carries the EPP admission decision.
type EPPRejected struct {
	// StatusCode is the HTTP status the EPP asked the proxy to return (429/503).
	StatusCode int
	// Code is an optional machine-readable code the EPP put in x-error-code.
	Code string
	// RetryAfter is the optional Retry-After hint from the ImmediateResponse.
	RetryAfter string
	// Body is the truncated EPP body, for logs only: it may carry internal
	// pool/topology details and is never echoed to the client.
	Body string
}

func (e *EPPRejected) Error() string {
	return fmt.Sprintf("%s: status=%d code=%q body=%q", bfe_basic.ErrEppRejected,
		e.StatusCode, e.Code, e.Body)
}

// IsEPPRejected reports whether err is an EPP admission decision.
func IsEPPRejected(err error) bool {
	return rejectionOf(err) != nil
}

// rejectionOf returns the EPP admission decision carried by err, or nil.
func rejectionOf(err error) *EPPRejected {
	if err == nil {
		return nil
	}
	var rej *EPPRejected
	if errors.As(err, &rej) {
		return rej
	}
	return nil
}

// newEppRejected builds an EPPRejected from an ext_proc ImmediateResponse,
// returning nil when the status is not an admission decision (429/503) so the
// caller keeps the legacy error path.
func newEppRejected(ir *extprocv3.ImmediateResponse) *EPPRejected {
	if ir == nil {
		return nil
	}

	rej := &EPPRejected{Body: truncateBody(string(ir.GetBody()))}
	if st := ir.GetStatus(); st != nil {
		rej.StatusCode = int(st.GetCode())
	}
	// Retry-After / x-error-code ride on the local-reply header mutation, not
	// on the HttpStatus (go-control-plane's HttpStatus only carries the code).
	for _, h := range ir.GetHeaders().GetSetHeaders() {
		key := h.GetHeader().GetKey()
		value := string(h.GetHeader().GetRawValue())
		if value == "" {
			value = h.GetHeader().GetValue()
		}
		switch {
		case equalFoldASCII(key, "retry-after"):
			rej.RetryAfter = value
		case equalFoldASCII(key, "x-error-code"):
			rej.Code = value
		}
	}
	if rej.Code == "" {
		rej.Code = ir.GetDetails()
	}
	if rej.StatusCode == 0 {
		rej.StatusCode = bfe_http.StatusTooManyRequests
	}
	if rej.StatusCode != bfe_http.StatusTooManyRequests &&
		rej.StatusCode != bfe_http.StatusServiceUnavailable {
		return nil
	}
	return rej
}

// EPPRejectedDetail returns the EPP decision recorded on the request, if any.
func EPPRejectedDetail(req *bfe_basic.Request) *EPPRejected {
	if req == nil {
		return nil
	}
	detail, ok := req.GetContext(REQ_CTX_EPP_REJECT).(*EPPRejected)
	if !ok {
		return nil
	}
	return detail
}

// eppRejectMaxBodyLen bounds the body kept for logs.
const eppRejectMaxBodyLen = 512

func truncateBody(body string) string {
	if len(body) <= eppRejectMaxBodyLen {
		return body
	}
	return body[:eppRejectMaxBodyLen] + "..."
}

// equalFoldASCII is a small, allocation-free ASCII case-insensitive compare for
// header names (avoids pulling strings.ToLower into every IR).
func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

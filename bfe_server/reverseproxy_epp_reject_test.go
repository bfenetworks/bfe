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

package bfe_server

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/bfenetworks/bfe/bfe_balance/bal_gslb"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

// TestAIGatewayFailureResponseEppRejected locks the client-visible contract of
// issue #1482: an EPP admission refusal keeps the status the EPP decided
// (429 EPP_OVERLOAD / 503 EPP_NO_CAPACITY), forwards the EPP Retry-After hint,
// keeps the gateway-generated marker, and never echoes the EPP body (which may
// carry pool topology).
func TestAIGatewayFailureResponseEppRejected(t *testing.T) {
	decode := func(t *testing.T, res *bfe_http.Response) bfe_basic.AiErrorBody {
		t.Helper()
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var aiErrBody bfe_basic.AiErrorBody
		if jsonErr := json.Unmarshal(body, &aiErrBody); jsonErr != nil {
			t.Fatalf("body is not a unified AiError JSON: %v, body: %s", jsonErr, body)
		}
		return aiErrBody
	}

	t.Run("429 overload", func(t *testing.T) {
		basicReq := &bfe_basic.Request{
			HttpRequest: &bfe_http.Request{Header: bfe_http.Header{}},
			ErrCode:     bfe_basic.ErrEppRejected,
			Context:     make(map[interface{}]interface{}),
		}
		basicReq.SetContext(bal_gslb.REQ_CTX_EPP_REJECT, &bal_gslb.EPPRejected{
			StatusCode: bfe_http.StatusTooManyRequests,
			Code:       "QUEUE_TTL_EXPIRED",
			RetryAfter: "7",
			Body:       "secret internal pool details",
		})

		res := aiGatewayFailureResponse(basicReq)
		if res.StatusCode != bfe_http.StatusTooManyRequests {
			t.Fatalf("expected status 429, got %d", res.StatusCode)
		}
		if got := res.Header.Get("Retry-After"); got != "7" {
			t.Errorf("expected the EPP Retry-After to be forwarded, got %q", got)
		}
		if got := res.Header.Get(bfe_basic.HeaderBfeGwError); got != "1" {
			t.Errorf("expected gateway-generated marker, got %q", got)
		}
		aiErrBody := decode(t, res)
		if aiErrBody.Error.Code != bfe_basic.CodeEppOverload {
			t.Errorf("expected error.code %q, got %q", bfe_basic.CodeEppOverload, aiErrBody.Error.Code)
		}
		if aiErrBody.Error.Type != bfe_basic.TypeRateLimitError {
			t.Errorf("expected error.type %q, got %q", bfe_basic.TypeRateLimitError, aiErrBody.Error.Type)
		}
		if msg := aiErrBody.Error.Message; msg == "" ||
			msg == "secret internal pool details" {
			t.Errorf("the EPP body must never be echoed, got %q", msg)
		}
	})

	t.Run("503 no capacity", func(t *testing.T) {
		basicReq := &bfe_basic.Request{
			HttpRequest: &bfe_http.Request{Header: bfe_http.Header{}},
			ErrCode:     bfe_basic.ErrEppRejected,
			Context:     make(map[interface{}]interface{}),
		}
		basicReq.SetContext(bal_gslb.REQ_CTX_EPP_REJECT, &bal_gslb.EPPRejected{
			StatusCode: bfe_http.StatusServiceUnavailable,
		})

		res := aiGatewayFailureResponse(basicReq)
		if res.StatusCode != bfe_http.StatusServiceUnavailable {
			t.Fatalf("expected status 503, got %d", res.StatusCode)
		}
		if got := res.Header.Get("Retry-After"); got != "" {
			t.Errorf("503 must not carry Retry-After, got %q", got)
		}
		aiErrBody := decode(t, res)
		if aiErrBody.Error.Code != bfe_basic.CodeEppNoCapacity {
			t.Errorf("expected error.code %q, got %q", bfe_basic.CodeEppNoCapacity, aiErrBody.Error.Code)
		}
		if aiErrBody.Error.Type != bfe_basic.TypeInternalError {
			t.Errorf("expected error.type %q, got %q", bfe_basic.TypeInternalError, aiErrBody.Error.Type)
		}
	})

	t.Run("missing detail still maps by ErrCode", func(t *testing.T) {
		basicReq := &bfe_basic.Request{
			HttpRequest: &bfe_http.Request{Header: bfe_http.Header{}},
			ErrCode:     bfe_basic.ErrEppRejected,
			Context:     make(map[interface{}]interface{}),
		}
		res := aiGatewayFailureResponse(basicReq)
		if res.StatusCode != bfe_http.StatusTooManyRequests {
			t.Fatalf("expected the overload default (429), got %d", res.StatusCode)
		}
	})
}

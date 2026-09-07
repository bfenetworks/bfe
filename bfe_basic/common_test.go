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

package bfe_basic

import (
	"io/ioutil"
	"testing"
)

// TestCreateSpecifiedContentRespAttachesHttpResponse is a regression test for
// bfenetworks/bfe#1359: the response built by CreateSpecifiedContentResp must
// be attached to request.HttpResponse, otherwise HandleReadResponse callbacks
// (e.g. mod_header) dereference a nil HttpResponse and panic.
func TestCreateSpecifiedContentRespAttachesHttpResponse(t *testing.T) {
	req := new(Request)
	resp := CreateSpecifiedContentResp(req, 404, "text/plain", "AI route not found")

	if req.HttpResponse != resp {
		t.Fatal("CreateSpecifiedContentResp should attach the response to request.HttpResponse")
	}
	if resp.StatusCode != 404 {
		t.Fatalf("StatusCode = %d, want 404", resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	if string(body) != "AI route not found" {
		t.Fatalf("body = %q, want %q", body, "AI route not found")
	}
}

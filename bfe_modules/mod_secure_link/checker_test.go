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

package mod_secure_link

import (
	"net/url"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func TestExpression(t *testing.T) {
	expression, err := NewExpression(&CheckerConfig{
		ChecksumKey: "md5",
		ExpiresKey:  "expires",
		ExpressionNodes: []ExpressionNodeFile{
			{Type: "query", Param: "expires"},
			{Type: "uri"},
			{Type: "remote_addr"},
			{Type: "label", Param: " secret"},
		},
	})
	if err != nil {
		t.Errorf("want nil, got: %v", err)
		return
	}

	got := expression.Value(&bfe_basic.Request{
		HttpRequest: &bfe_http.Request{
			RemoteAddr: "127.0.0.1",
			RequestURI: "/a/b",
		},
		Query: url.Values{
			"md5":     []string{""},
			"expires": []string{"9999"},
		},
	})
	{
		want := "9999/a/b127.0.0.1 secret"
		if want != got {
			t.Errorf("want: %v, got: %v", want, got)
		}
	}
}

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

package mod_secure_link

import (
	"net/url"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func TestNewCheckerError(t *testing.T) {
	_, err := NewChecker(&CheckerConfig{
		ChecksumKey: "md5",
		ExpressionNodes: []ExpressionNodeFile{
			{Type: "unknown"},
		},
	})
	if err == nil {
		t.Errorf("NewChecker() want error, got nil")
	}
}

func TestNewNodeTypes(t *testing.T) {
	expr, err := NewExpression(&CheckerConfig{
		ChecksumKey: "md5",
		ExpressionNodes: []ExpressionNodeFile{
			{Type: "label", Param: "prefix "},
			{Type: "query", Param: "q"},
			{Type: "header", Param: "X-H"},
			{Type: "host"},
			{Type: "uri"},
			{Type: "remote_addr"},
		},
	})
	if err != nil {
		t.Fatalf("NewExpression() error: %v", err)
	}

	req := &bfe_basic.Request{
		HttpRequest: &bfe_http.Request{
			Host:       "example.com",
			RequestURI: "/path",
			RemoteAddr: "1.2.3.4",
			Header:     bfe_http.Header{},
		},
		Query: url.Values{
			"q": []string{"foo"},
		},
	}
	req.HttpRequest.Header.Set("X-H", "bar")

	want := "prefix foobarexample.com/path1.2.3.4"
	if got := expr.Value(req); got != want {
		t.Errorf("Expression.Value() want %q, got %q", want, got)
	}
}

func TestNewNodeBadType(t *testing.T) {
	_, err := NewNode(ExpressionNodeFile{Type: "bad_type"})
	if err == nil {
		t.Errorf("NewNode() want error, got nil")
	}
}

func TestExpressionEmptyNodes(t *testing.T) {
	expr, err := NewExpression(&CheckerConfig{})
	if err != nil {
		t.Fatalf("NewExpression() error: %v", err)
	}
	if got := expr.Value(&bfe_basic.Request{}); got != "" {
		t.Errorf("Expression.Value() want empty string, got %q", got)
	}
}

func TestCheckWithoutExpiresKey(t *testing.T) {
	checker, err := NewChecker(&CheckerConfig{
		ChecksumKey: "md5",
		ExpiresKey:  "expires",
		ExpressionNodes: []ExpressionNodeFile{
			{Type: "label", Param: "secret"},
		},
	})
	if err != nil {
		t.Fatalf("NewChecker() error: %v", err)
	}

	req := &bfe_basic.Request{
		HttpRequest: &bfe_http.Request{},
		Query: url.Values{
			"md5": []string{"x"},
		},
	}
	if got := checker.Check(req); got != ErrReqWithoutExpiresKey {
		t.Errorf("Check() want %v, got %v", ErrReqWithoutExpiresKey, got)
	}
}

func TestCheckInvalidExpiresValue(t *testing.T) {
	checker, err := NewChecker(&CheckerConfig{
		ChecksumKey: "md5",
		ExpiresKey:  "expires",
		ExpressionNodes: []ExpressionNodeFile{
			{Type: "label", Param: "secret"},
		},
	})
	if err != nil {
		t.Fatalf("NewChecker() error: %v", err)
	}

	req := &bfe_basic.Request{
		HttpRequest: &bfe_http.Request{},
		Query: url.Values{
			"expires": []string{"abc"},
			"md5":     []string{"x"},
		},
	}
	if got := checker.Check(req); got != ErrReqInvalidExpiresValue {
		t.Errorf("Check() want %v, got %v", ErrReqInvalidExpiresValue, got)
	}
}

func TestCheckWithoutChecksumKey(t *testing.T) {
	checker, err := NewChecker(&CheckerConfig{
		ChecksumKey: "md5",
		ExpressionNodes: []ExpressionNodeFile{
			{Type: "label", Param: "secret"},
		},
	})
	if err != nil {
		t.Fatalf("NewChecker() error: %v", err)
	}

	req := &bfe_basic.Request{
		HttpRequest: &bfe_http.Request{},
		Query:       url.Values{},
	}
	if got := checker.Check(req); got != ErrReqWithoutChecksumKey {
		t.Errorf("Check() want %v, got %v", ErrReqWithoutChecksumKey, got)
	}
}

func TestCheckValidWithoutExpires(t *testing.T) {
	checker, err := NewChecker(&CheckerConfig{
		ChecksumKey: "md5",
		ExpressionNodes: []ExpressionNodeFile{
			{Type: "uri"},
			{Type: "label", Param: " secret"},
		},
	})
	if err != nil {
		t.Fatalf("NewChecker() error: %v", err)
	}

	want := checker.encode("/a/b secret")
	req := &bfe_basic.Request{
		HttpRequest: &bfe_http.Request{RequestURI: "/a/b"},
		Query: url.Values{
			"md5": []string{want},
		},
	}
	if got := checker.Check(req); got != nil {
		t.Errorf("Check() want nil, got %v", got)
	}
}

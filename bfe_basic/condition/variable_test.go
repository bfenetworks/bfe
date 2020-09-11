// Copyright (c) 2020 The BFE Authors.
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

package condition

import (
	"reflect"
	"regexp"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func Test_pathVariableMatcher_Match(t *testing.T) {
	type args struct {
		rule string
		path string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "normal test",
			args: args{
				rule: "/teams/{teamId}/users/{userId}",
				path: "/teams/1/users/3",
			},
			want: true,
		},
		{
			name: "invalid path",
			args: args{
				rule: "/teams/{teamId}/users/{userId}",
				path: "/teams/1/users_test/3",
			},
			want: false,
		},
		{
			name: "path with /",
			args: args{
				rule: "/teams/{teamId}/users/{userId}",
				path: "/teams/1/users/3/",
			},
			want: false,
		},
		{
			name: "rule with /",
			args: args{
				rule: "/teams/{teamId}/users/{userId}/",
				path: "/teams/1/users/3",
			},
			want: false,
		},
		{
			name: "rule and path with /",
			args: args{
				rule: "/teams/{teamId}/users/{userId}/",
				path: "/teams/1/users/3/",
			},
			want: true,
		},
		{
			name: "not start with rule",
			args: args{
				rule: "/teams/{teamId}/users/{userId}",
				path: "/api/v1/teams/1/users/3",
			},
			want: false,
		},
		{
			name: "not end with rule",
			args: args{
				rule: "/teams/{teamId}/users/{userId}",
				path: "/teams/1/users/3/info/3",
			},
			want: false,
		},
		{
			name: "not end with rule",
			args: args{
				rule: "/teams/teamId/users/userId",
				path: "/teams/teamId/users/userId",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := new(pathVariableMatcher)
			p.Init(tt.args.rule, false)
			if got := p.Match(tt.args.path); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_braceIndices(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name    string
		args    args
		want    []int
		wantErr bool
	}{
		{
			name: "test normal",
			args: args{
				s: "/user/{userId}",
			},
			want:    []int{6, 13},
			wantErr: false,
		},
		{
			name: "test no brace",
			args: args{
				s: "/user/userId",
			},
			want:    []int{},
			wantErr: false,
		},
		{
			name: "test no left brace",
			args: args{
				s: "/user/{userId",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "test no right brace",
			args: args{
				s: "/user/userId}",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "test no err seq brace",
			args: args{
				s: "/user/u}serId{",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "test multi left brace",
			args: args{
				s: "/user/{{serId}",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "test no multi brace",
			args: args{
				s: "/user/{{serId}}",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := braceIndices(tt.args.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("braceIndices() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("braceIndices() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func getExp(tpl string) *regexp.Regexp {
	ret, _ := regexp.Compile(tpl)
	return ret
}

func Test_buildPathVariableRegexp(t *testing.T) {
	type args struct {
		tpl          string
		isPrefixType bool
	}
	tests := []struct {
		name    string
		args    args
		want    *variableRegexp
		wantErr bool
	}{
		{
			name: "test no prefix",
			args: args{
				tpl:          "/user/{userId}",
				isPrefixType: false,
			},
			want: &variableRegexp{
				originalStr: "/user/{userId}",
				patternStr:  "^/user/(?P<v0>[^/]+)$",
				varNames:    []string{"userId"},
				regexp:      getExp("^/user/(?P<v0>[^/]+)$"),
			},
			wantErr: false,
		},
		{
			name: "test prefix",
			args: args{
				tpl:          "/user/{userId}",
				isPrefixType: true,
			},
			want: &variableRegexp{
				originalStr: "/user/{userId}",
				patternStr:  "^/user/(?P<v0>[^/]+)",
				varNames:    []string{"userId"},
				regexp:      getExp("^/user/(?P<v0>[^/]+)"),
			},
			wantErr: false,
		},
		{
			name: "test invalid params",
			args: args{
				tpl:          "/user/{userId",
				isPrefixType: false,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "test invalid params",
			args: args{
				tpl:          "/user/{userId}/info/{ss",
				isPrefixType: false,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "test invalid params",
			args: args{
				tpl:          "/user/{userId}/info/{",
				isPrefixType: false,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "test invalid params",
			args: args{
				tpl:          "/user/{userId}/info/}",
				isPrefixType: false,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "test invalid params",
			args: args{
				tpl:          "/user/{userId}/info/{",
				isPrefixType: true,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "test with regexp in raw str",
			args: args{
				tpl:          "/u.*/{userId}/in.*/{infoId}",
				isPrefixType: true,
			},
			want: &variableRegexp{
				originalStr: "/u.*/{userId}/in.*/{infoId}",
				patternStr:  "^/u\\.\\*/(?P<v0>[^/]+)/in\\.\\*/(?P<v1>[^/]+)",
				varNames:    []string{"userId", "infoId"},
				regexp:      getExp("^/u\\.\\*/(?P<v0>[^/]+)/in\\.\\*/(?P<v1>[^/]+)"),
			},
			wantErr: false,
		},
		{
			name: "test without params",
			args: args{
				tpl:          "/user/userId/info/infoId",
				isPrefixType: true,
			},
			want: &variableRegexp{
				originalStr: "/user/userId/info/infoId",
				patternStr:  "^/user/userId/info/infoId",
				varNames:    []string{},
				regexp:      getExp("^/user/userId/info/infoId"),
			},
			wantErr: false,
		},
		{
			name: "test without params with regexp",
			args: args{
				tpl:          "/user/userId/info/infoId",
				isPrefixType: true,
			},
			want: &variableRegexp{
				originalStr: "/user/userId/info/infoId",
				patternStr:  "^/user/userId/info/infoId",
				varNames:    []string{},
				regexp:      getExp("^/user/userId/info/infoId"),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildPathVariableRegexp(tt.args.tpl, tt.args.isPrefixType)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildPathVariableRegexp() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildPathVariableRegexp() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_pathVariableMatcher_ExtractVariable(t *testing.T) {
	type args struct {
		rule string
		path string
	}
	tests := []struct {
		name string
		args args
		want map[string]interface{}
	}{
		{
			name: "test normal",
			args: args{
				rule: "/user/{userId}/info/{infoId}",
				path: "/user/123/info/456",
			},
			want: map[string]interface{}{
				"userId": "123",
				"infoId": "456",
			},
		},
		{
			name: "test no vars",
			args: args{
				rule: "/user/123/info/456",
				path: "/user/123/info/456",
			},
			want: map[string]interface{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewPathVariableMatcher(tt.args.rule, false)
			if err != nil {
				t.Errorf("ExtractVariable() err= %v", err)
				return
			}
			if got := p.ExtractVariable(tt.args.path); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractVariable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_VariableCond_Match(t *testing.T) {
	matcher, _ := NewPathVariableMatcher("/user/{userId}/info/{infoId}", false)
	needVars := map[string]interface{}{
		"userId": "123",
		"infoId": "456",
	}
	vc := VariableCond{
		name:    "",
		node:    nil,
		fetcher: &PathFetcher{},
		matcher: matcher,
	}
	// not prefix type
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://example.org/user/123/info/456", nil)
	req.Context = make(map[interface{}]interface{})

	matched := vc.Match(req)
	if !matched {
		t.Errorf("request should match")
	} else {
		if !reflect.DeepEqual(needVars, req.Context[bfe_basic.BfeVarsKey]) {
			t.Errorf("Match() got=%v, want=%v", req.Context[bfe_basic.BfeVarsKey], needVars)
		}
		for key, value := range needVars {
			if value != req.GetVar(key) {
				t.Errorf("Match() got=%v, want=%v", req.GetVar(key), value)
			}
		}
	}

	// prefix path
	req = new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://example.org/user/123/info/456/path", nil)
	req.Context = make(map[interface{}]interface{})

	matched = vc.Match(req)
	if matched {
		t.Errorf("request should not match")
	}

	// prefix mod
	matcher, _ = NewPathVariableMatcher("/user/{userId}/info/{infoId}", true)
	needVars = map[string]interface{}{
		"userId": "123",
		"infoId": "456",
	}
	vc = VariableCond{
		name:    "",
		node:    nil,
		fetcher: &PathFetcher{},
		matcher: matcher,
	}
	req = new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://example.org/user/123/info/456", nil)
	req.Context = make(map[interface{}]interface{})

	matched = vc.Match(req)
	if !matched {
		t.Errorf("request should match")
	} else {
		if !reflect.DeepEqual(needVars, req.Context[bfe_basic.BfeVarsKey]) {
			t.Errorf("Match() got=%v, want=%v", req.Context[bfe_basic.BfeVarsKey], needVars)
		}
	}

	req = new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://example.org/user/123/info/456/", nil)
	req.Context = make(map[interface{}]interface{})

	matched = vc.Match(req)
	if !matched {
		t.Errorf("request should match")
	} else {
		if !reflect.DeepEqual(needVars, req.Context[bfe_basic.BfeVarsKey]) {
			t.Errorf("Match() got=%v, want=%v", req.Context[bfe_basic.BfeVarsKey], needVars)
		}
	}

	req = new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://example.org/user/123/info/456/path", nil)
	req.Context = make(map[interface{}]interface{})

	matched = vc.Match(req)
	if !matched {
		t.Errorf("request should match")
	} else {
		if !reflect.DeepEqual(needVars, req.Context[bfe_basic.BfeVarsKey]) {
			t.Errorf("Match() got=%v, want=%v", req.Context[bfe_basic.BfeVarsKey], needVars)
		}
	}

	// no varibale
	matcher, _ = NewPathVariableMatcher("/user/userId/info/infoId", true)
	vc = VariableCond{
		name:    "",
		node:    nil,
		fetcher: &PathFetcher{},
		matcher: matcher,
	}
	req = new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://example.org/user/userId/info/infoId", nil)
	req.Context = make(map[interface{}]interface{})

	matched = vc.Match(req)
	if !matched {
		t.Errorf("request should match")
	} else {
		if !reflect.DeepEqual(nil, req.Context[bfe_basic.BfeVarsKey]) {
			t.Errorf("Match() got=%v, want=%v", req.Context[bfe_basic.BfeVarsKey], needVars)
		}
	}

	// match raw str with reg
	matcher, _ = NewPathVariableMatcher("/u.*/{userId}/i.*/{infoId}", true)
	needVars = map[string]interface{}{
		"userId": "123",
		"infoId": "456",
	}
	vc = VariableCond{
		name:    "",
		node:    nil,
		fetcher: &PathFetcher{},
		matcher: matcher,
	}
	req = new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://example.org/u.*/123/i.*/456", nil)
	req.Context = make(map[interface{}]interface{})

	matched = vc.Match(req)
	if !matched {
		t.Errorf("request should match")
	} else {
		if !reflect.DeepEqual(needVars, req.Context[bfe_basic.BfeVarsKey]) {
			t.Errorf("Match() got=%v, want=%v", req.Context[bfe_basic.BfeVarsKey], needVars)
		}
	}

	req = new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://example.org/user/123/info/456", nil)
	req.Context = make(map[interface{}]interface{})

	matched = vc.Match(req)
	if matched {
		t.Errorf("request should not match")
	}

	// not with context
	matcher, _ = NewPathVariableMatcher("/u.*/{userId}/i.*/{infoId}", true)
	vc = VariableCond{
		name:    "",
		node:    nil,
		fetcher: &PathFetcher{},
		matcher: matcher,
	}
	req = new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://example.org/u.*/123/i.*/456", nil)

	matched = vc.Match(req)
	if matched {
		t.Errorf("request should not match")
	}
}

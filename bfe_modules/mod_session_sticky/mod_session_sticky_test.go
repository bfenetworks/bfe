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

package mod_session_sticky

import (
	"fmt"
	"net/url"
	"reflect"
	"testing"

	"github.com/baidu/go-lib/lru_cache"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/bfenetworks/bfe/bfe_balance/backend"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const product = "unittest"

func GetModuleSessionSticky() *ModuleSessionSticky {
	m := NewModuleSessionSticky()
	m.configPath = "./test_data/mod_session_sticky.data"
	m.loadConfData(url.Values{})
	return m
}

func GetModuleSessionStickyByFilename(filename string) *ModuleSessionSticky {
	m := NewModuleSessionSticky()
	m.configPath = filename
	err := m.loadConfData(url.Values{})
	if err != nil {
		fmt.Printf("failed to load file:%s, err:%s\n", filename, err)
	}
	return m
}

func prepareRequest(product, path string) *bfe_basic.Request {
	req := new(bfe_basic.Request)
	req.HttpRequest = new(bfe_http.Request)
	req.HttpRequest.Header = make(bfe_http.Header)
	req.Route.Product = product
	req.Session = new(bfe_basic.Session)
	req.Context = make(map[interface{}]interface{})
	req.HttpRequest.URL = &url.URL{}
	req.HttpRequest.URL.Path = path
	req.Trans = bfe_basic.RequestTransport{
		Backend: &backend.BfeBackend{},
	}

	return req
}

func prepareResponse() *bfe_http.Response {
	res := new(bfe_http.Response)
	res.StatusCode = 200
	res.Header = make(bfe_http.Header)

	return res
}

func prepareResponseJsession() *bfe_http.Response {
	res := new(bfe_http.Response)
	res.StatusCode = 200
	res.Header = make(bfe_http.Header)
	cookie := bfe_http.Cookie{
		Name:   "JSessionID",
		Value:  "testsession",
		MaxAge: 120,
	}

	res.Header.Add("Set-Cookie", cookie.String())

	return res
}

func TestModuleSessionSticky_FindAndCacheStickyRule(t *testing.T) {
	m := GetModuleSessionSticky()
	productRule, exists := m.ruleTable.Search(product)
	if !exists {
		t.FailNow()
	}
	type args struct {
		request *bfe_basic.Request
		rules   *StickyRuleList
	}
	tests := []struct {
		name string
		args args
		want *StickyRule
	}{
		// TODO: Add test cases.
		{
			name: "no rules",
			args: args{
				request: prepareRequest(bfe_basic.GlobalProduct, "/"),
				rules:   nil,
			},
			want: nil,
		},
		{
			name: "no rule",
			args: args{
				request: nil,
				rules:   nil,
			},
			want: nil,
		},
		{
			name: "without rule",
			args: args{
				request: prepareRequest(product, "/unittest"),
				rules:   productRule,
			},
			want: &(*productRule)[0],
		},
		{
			name: "without rule",
			args: args{
				request: prepareRequest(product, "/t"),
				rules:   productRule,
			},
			want: nil,
		},
		{
			name: "withCache",
			args: args{
				request: func() *bfe_basic.Request {
					r := prepareRequest(product, "/unittest")
					r.Context[ModSessionStickyKey] = (*productRule)[0]
					return r
				}(),
				rules: productRule,
			},
			want: &(*productRule)[0],
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := GetModuleSessionSticky()

			if got := m.FindAndCacheStickyRule(tt.args.request, tt.args.rules); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ModuleSessionSticky.FindAndCacheStickyRule() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModuleSessionSticky_encodeHandler(t *testing.T) {
	m := GetModuleSessionSticky()
	type args struct {
		request *bfe_basic.Request
		res     *bfe_http.Response
	}
	tests := []struct {
		name    string
		args    args
		want    int
		wantStr string
	}{
		// TODO: Add test cases.
		{
			name: "normal",
			args: args{
				request: func() *bfe_basic.Request {
					r := prepareRequest(product, "/unittest")
					r.Trans.Backend.Addr = "127.0.0.1"
					r.Trans.Backend.Port = 80
					r.Backend.SubclusterName = "unittest"
					productRule, _ := m.ruleTable.Search(product)
					r.Context[ModSessionStickyKey] = SessionStickyData{rule: &(*productRule)[0]}
					return r
				}(),
				res: prepareResponse(),
			},
			want:    1,
			wantStr: "bfe_ssbl=GhNYQRMLEwBQVEpRHwEfABMdExIMFhUTCwkBHRNCFwEHDURCRVRDEwtAFgoIRUVUQkUTHUARAQ9URkVYXFQTWA0RDV1M; Max-Age=12000",
		},
		{
			name: "without context",
			args: args{
				request: func() *bfe_basic.Request {
					r := prepareRequest(product, "/unittest")
					r.Trans.Backend.Addr = "127.0.0.1"
					r.Trans.Backend.Port = 80
					r.Backend.SubclusterName = "unittest"
					return r
				}(),
				res: prepareResponse(),
			},
			want:    1,
			wantStr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.encodeHandler(tt.args.request, tt.args.res); got != tt.want {
				t.Errorf("ModuleSessionSticky.encodeHandler() = %v, want %v", got, tt.want)
			}
			if tt.args.res.Header.Get("Set-Cookie") != tt.wantStr {
				t.Errorf("ModuleSessionSticky.encodeHandler() = %v, want %v",
					tt.args.res.Header.Get("Set-Cookie"), tt.wantStr)
			}
		})
	}
}

func TestModuleSessionSticky_encodeHandler_domainAndPath(t *testing.T) {
	m := GetModuleSessionStickyByFilename("./test_data/mod_session_sticky_2.data")
	type args struct {
		request *bfe_basic.Request
		res     *bfe_http.Response
	}
	tests := []struct {
		name    string
		args    args
		want    int
		wantStr string
	}{
		// TODO: Add test cases.
		{
			name: "normal",
			args: args{
				request: func() *bfe_basic.Request {
					r := prepareRequest(product, "/unittest")
					r.Trans.Backend.Addr = "127.0.0.1"
					r.Trans.Backend.Port = 80
					r.Backend.SubclusterName = "unittest"
					productRule, _ := m.ruleTable.Search(product)
					r.Context[ModSessionStickyKey] = SessionStickyData{rule: &(*productRule)[0]}
					return r
				}(),
				res: prepareResponse(),
			},
			want:    1,
			wantStr: "bfe_ssbl=GhNYQRMLEwBQVEpRHwEfABMdExIMFhUTCwkBHRNCFwEHDURCRVRDEwtAFgoIRUVUQkUTHUARAQ9URkVYXFQTWA0RDV1M; Path=/; Domain=test.com; Max-Age=12000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.encodeHandler(tt.args.request, tt.args.res); got != tt.want {
				t.Errorf("ModuleSessionSticky.encodeHandler() = %v, want %v", got, tt.want)
			}
			if tt.args.res.Header.Get("Set-Cookie") != tt.wantStr {
				t.Errorf("ModuleSessionSticky.encodeHandler() = %v, want %v",
					tt.args.res.Header.Get("Set-Cookie"), tt.wantStr)
			}
		})
	}
}

func TestModuleSessionSticky_encodeHandler_Path(t *testing.T) {
	m := GetModuleSessionStickyByFilename("./test_data/mod_session_sticky_3.data")
	type args struct {
		request *bfe_basic.Request
		res     *bfe_http.Response
	}
	tests := []struct {
		name    string
		args    args
		want    int
		wantStr string
	}{
		// TODO: Add test cases.
		{
			name: "normal",
			args: args{
				request: func() *bfe_basic.Request {
					r := prepareRequest(product, "/unittest")
					r.Trans.Backend.Addr = "127.0.0.1"
					r.Trans.Backend.Port = 80
					r.Backend.SubclusterName = "unittest"
					productRule, _ := m.ruleTable.Search(product)
					r.Context[ModSessionStickyKey] = SessionStickyData{rule: &(*productRule)[0]}
					return r
				}(),
				res: prepareResponse(),
			},
			want:    1,
			wantStr: "bfe_ssbl=GhNYQRMLEwBQVEpRHwEfABMdExIMFhUTCwkBHRNCFwEHDURCRVRDEwtAFgoIRUVUQkUTHUARAQ9URkVYXFQTWA0RDV1M; Path=/; Max-Age=12000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.encodeHandler(tt.args.request, tt.args.res); got != tt.want {
				t.Errorf("ModuleSessionSticky.encodeHandler() = %v, want %v", got, tt.want)
			}
			if tt.args.res.Header.Get("Set-Cookie") != tt.wantStr {
				t.Errorf("ModuleSessionSticky.encodeHandler() = %v, want %v",
					tt.args.res.Header.Get("Set-Cookie"), tt.wantStr)
			}
		})
	}
}

func TestModuleSessionStickyWithNilBackend_encodeHandler(t *testing.T) {
	m := GetModuleSessionSticky()
	type args struct {
		request *bfe_basic.Request
		res     *bfe_http.Response
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "normal",
			args: args{
				request: func() *bfe_basic.Request {
					r := prepareRequest(product, "/unittest")
					r.Trans.Backend = nil
					r.Backend.SubclusterName = "unittest"
					productRule, _ := m.ruleTable.Search(product)
					r.Context[ModSessionStickyKey] = SessionStickyData{rule: &(*productRule)[0]}
					return r
				}(),
				res: prepareResponse(),
			},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.encodeHandler(tt.args.request, tt.args.res); got != tt.want {
				t.Errorf("ModuleSessionSticky.encodeHandler() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModuleSessionSticky_init(t *testing.T) {
	m := GetModuleSessionSticky()
	type args struct {
		cbs     *bfe_module.BfeCallbacks
		whs     *web_monitor.WebHandlers
		cr      string
		wantErr bool
	}

	case0 := args{
		cbs:     bfe_module.NewBfeCallbacks(),
		whs:     web_monitor.NewWebHandlers(),
		cr:      "./test_data",
		wantErr: false,
	}
	if err := m.Init(case0.cbs, case0.whs, case0.cr); (err != nil) != case0.wantErr {
		t.Errorf("ModuleMarkdown.Init() error = %v, wantErr %v", err, case0.wantErr)
	}

}

/*
	func Test_processDecode(t *testing.T) {
		m := NewModuleSessionSticky()
		m.Init(nil, nil, "./test_data/mod_session.data")
		type args struct {
			req  *bfe_basic.Request
			rule StickyRule
		}
		tests := []struct {
			name string
			args args
		}{
			// TODO: Add test cases.

		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				m.processDecode(tt.args.req, tt.args.rule)
			})
		}
	}
*/
func Test_getStickyBackend(t *testing.T) {
	addr := "127.0.0.1"
	port := 80
	sub := "test"
	type args struct {
		code string
	}
	tests := []struct {
		name    string
		args    args
		want    *bfe_basic.SessionStickyBackend
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "normal",
			args: args{
				code: `{"ip":"127.0.0.1","port":80,"subcluster":"test"}`,
			},
			want: &bfe_basic.SessionStickyBackend{
				Addr:       &addr,
				Port:       &port,
				SubCluster: &sub,
			},
			wantErr: false,
		},
		{
			name: "empty fields",
			args: args{
				code: `{"ip":"127.0.0.1","port":80,"subcluster123":"test"}`,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty fields",
			args: args{
				code: `{"ip":"127.0.0.1","port":"80","subcluster":"test"}`,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getStickyBackend(tt.args.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("getStickyBackend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getStickyBackend() = %v, want %v", got, tt.want)
			}
		})
	}
}

/*
	func Test_processEncode(t *testing.T) {
		type args struct {
			req  *bfe_basic.Request
			res  *bfe_http.Response
			rule StickyRule
		}
		tests := []struct {
			name string
			args args
		}{
			// TODO: Add test cases.
		}
		m := GetModuleSessionSticky()
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				m.processEncode(tt.args.req, tt.args.res, tt.args.rule)
			})
		}
	}
*/
func Test_getStickyBackendStr(t *testing.T) {
	addr := "127.0.0.1"
	port := 80
	sub := "test"
	type args struct {
		bk *bfe_basic.SessionStickyBackend
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "normal",
			args: args{
				bk: &bfe_basic.SessionStickyBackend{
					Addr:       &addr,
					Port:       &port,
					SubCluster: &sub,
				},
			},
			want:    `{"ip":"127.0.0.1","port":80,"subcluster":"test","renewtime":null}`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getStickyBackendStr(tt.args.bk)
			if (err != nil) != tt.wantErr {
				t.Errorf("getStickyBackendStr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getStickyBackendStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_doMask(t *testing.T) {
	type args struct {
		maskCode []byte
		val      string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.
		{
			name: "normal",
			args: args{
				maskCode: []byte{'F', 'F', 'F', 'F'},
				val:      "123",
			},
			want: "wtu",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := doMask(tt.args.maskCode, tt.args.val); got != tt.want {
				t.Errorf("doMask() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_doEncode(t *testing.T) {
	type args struct {
		src      string
		maskCode []byte
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.
		{
			name: "normal",
			args: args{
				src:      "123",
				maskCode: []byte{'F', 'F', 'F', 'F'},
			},
			want: "d3R1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := doEncode(tt.args.src, tt.args.maskCode); got != tt.want {
				t.Errorf("doEncode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_doDecode(t *testing.T) {
	type args struct {
		src      string
		maskCode []byte
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "normal",
			args: args{
				src:      "d3R1",
				maskCode: []byte{'F', 'F', 'F', 'F'},
			},
			want:    "123",
			wantErr: false,
		},
		{
			name: "normal",
			args: args{
				src:      "d",
				maskCode: []byte{'F', 'F', 'F', 'F'},
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := doDecode(tt.args.src, tt.args.maskCode)
			if (err != nil) != tt.wantErr {
				t.Errorf("doDecode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("doDecode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModuleSessionSticky_decodeHandler(t *testing.T) {
	m := GetModuleSessionSticky()
	type args struct {
		request *bfe_basic.Request
	}
	tests := []struct {
		name   string
		args   args
		want   int
		want1  *bfe_http.Response
		wantBk *bfe_basic.SessionStickyBackend
	}{
		// TODO: Add test cases.
		{
			name: "",
			args: args{
				request: func() *bfe_basic.Request {
					r := prepareRequest(product, "/unittest")
					r.HttpRequest.AddCookie(&bfe_http.Cookie{
						Name:  "bfe_ssbl",
						Value: "GhNYQRMLEwBQVEpRHwEfABMdExIMFhUTCwkBHRNCFwEHDURCRVRDEwtAFgoIRUVUQkUTTA",
					})
					productRule, _ := m.ruleTable.Search(product)
					r.Context[ModSessionStickyKey] = SessionStickyData{rule: &(*productRule)[0]}
					return r
				}(),
			},
			want:  1,
			want1: nil,
			wantBk: &bfe_basic.SessionStickyBackend{
				Addr: func() *string {
					r := "127.0.0.1"
					return &r
				}(),
				Port: func() *int {
					r := 80
					return &r
				}(),
				SubCluster: func() *string {
					r := "unittest"
					return &r
				}(),
			},
		},
		{
			name: "",
			args: args{
				request: func() *bfe_basic.Request {
					r := prepareRequest(product, "/unittest")
					r.HttpRequest.AddCookie(&bfe_http.Cookie{
						Name:  "bfe_ssbl",
						Value: "GhNYQRMLEwBQVEpRwEHDURCRVRDEwtAFgoIRUVUQkUTTA",
					})
					productRule, _ := m.ruleTable.Search(product)
					r.Context[ModSessionStickyKey] = SessionStickyData{rule: &(*productRule)[0]}
					return r
				}(),
			},
			want:   1,
			want1:  nil,
			wantBk: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := m.decodeHandler(tt.args.request)
			if got != tt.want {
				t.Errorf("decodeHandler() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("decodeHandler() got1 = %v, want %v", got1, tt.want1)
			}
			if val, ok := tt.args.request.Context[bfe_basic.SessionStickyBackendKey]; ok {
				oval, _ := val.(*bfe_basic.SessionStickyBackend)
				if !reflect.DeepEqual(oval, tt.wantBk) {
					t.Errorf("decodeHandler() got1 = %v, want %v", oval, tt.wantBk)
				}
			}
		})
	}
}

func TestModuleSessionStickyJsession(t *testing.T) {
	m := GetModuleSessionStickyByFilename("./test_data/mod_session_sticky_jsession.data")
	m.jsessioncache = lru_cache.NewLRUCache(defaultCacheSize)

	req := func() *bfe_basic.Request {
		r := prepareRequest(product, "/unittest")
		r.Trans.Backend.Addr = "127.0.0.1"
		r.Trans.Backend.Port = 80
		r.Backend.SubclusterName = "unittest"
		productRule, _ := m.ruleTable.Search(product)
		r.Context[ModSessionStickyKey] = SessionStickyData{rule: &(*productRule)[0]}
		return r
	}()
	res := prepareResponseJsession()

	m.encodeHandler(req, res)

	val, ok := m.jsessioncache.Get("testsession")
	if !ok {
		t.Errorf("encodeHandler() did not save bk for %s", "testsession")
		return
	}
	bk, ok := val.(*bfe_basic.SessionStickyBackend)
	if !ok {
		t.Errorf("encodeHandler() bk saved with wrong type: %v", val)
		return
	}
	if bk.Addr != &req.Trans.Backend.Addr || bk.Port != &req.Trans.Backend.Port || bk.SubCluster != &req.Backend.SubclusterName {
		t.Errorf("encodeHandler() saved wrong bk %v", bk)
		return
	}

	cookie := bfe_http.Cookie{
		Name:   "JSessionID",
		Value:  "testsession",
		MaxAge: 120,
	}

	//req.HttpRequest.AddCookie(&cookie)
	req.CachedCookie()["JSessionID"] = &cookie

	m.decodeHandler(req)
	if req.Context[bfe_basic.SessionStickyBackendKey] != bk {
		t.Errorf("decodeHandler got error context: %v, not: %v", req.Context[bfe_basic.SessionStickyBackendKey], bk)
		return
	}
}

func TestModuleSessionStickySecure(t *testing.T) {
	m := GetModuleSessionStickyByFilename("./test_data/mod_session_sticky_secure.data")

	req := func() *bfe_basic.Request {
		r := prepareRequest(product, "/unittest")
		r.Trans.Backend.Addr = "127.0.0.1"
		r.Trans.Backend.Port = 80
		r.Backend.SubclusterName = "unittest"
		productRule, _ := m.ruleTable.Search(product)
		r.Context[ModSessionStickyKey] = SessionStickyData{rule: &(*productRule)[0]}
		return r
	}()
	res := prepareResponse()

	m.encodeHandler(req, res)
	wantStr := "bfe_ssbl=FkMaG0FVRlRfVl1bTV9KVE9NURsMHRBHV1lDR0EcEQcODQYYFwoWR1dDBgUKGxAAHhVRR0EdAQsIFgcCDgpGXwMUHwce; Max-Age=12000; HttpOnly; Secure"
	if res.Header.Get("Set-Cookie") != wantStr {
		t.Errorf("ModuleSessionSticky.encodeHandler() = %v, want %v",
			res.Header.Get("Set-Cookie"), wantStr)
	}

	_, err := m.decodeHandler(req)
	if err != nil {
		t.Errorf("decodeHandler: %v, err:%v", req, err)
	}
}

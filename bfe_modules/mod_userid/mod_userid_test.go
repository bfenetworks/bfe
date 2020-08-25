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

// Package mod_userid generate user identity to trace one user in deffient request
// this mod will auto set user id for request if user id not exited in cookie to cookie
package mod_userid

import (
	"net/url"
	"reflect"
	"testing"
	"time"
)

import (
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func TestModuleUserIDName(t *testing.T) {
	m := NewModuleUserID()
	if m.Name() != "mod_userid" {
		t.Errorf("module name is wrong, Expect \"%s\"", "mod_userid")
	}
}

func modFactory() *ModuleUserID {
	m := NewModuleUserID()
	m.loadConfData(url.Values{
		"path": []string{"./testdata/mod_userid/userid_rule.data"},
	})
	return m
}

var (
	bfeUIDCookie = &bfe_http.Cookie{
		Name:  "bfe_userid",
		Value: "bfe_userid",
	}

	productUIDCookie = &bfe_http.Cookie{
		Name:  "bfe_userid",
		Value: "product_userid",
	}

	productOtherCookie = &bfe_http.Cookie{
		Name:  "product_id",
		Value: "product_id",
	}
)

func TestModuleUserIDSetUID2Response(t *testing.T) {
	type args struct {
		requestFactory func() *bfe_basic.Request
		resFactory     func() *bfe_http.Response
	}
	tests := []struct {
		name          string
		moduleFactory func() *ModuleUserID
		args          args
		want          int
		judge         func(request *bfe_basic.Request, res *bfe_http.Response, t *testing.T) bool
	}{
		{
			name:          "case: succ, without userid",
			moduleFactory: modFactory,
			args: args{
				requestFactory: func() *bfe_basic.Request {
					req := &bfe_basic.Request{
						Context: map[interface{}]interface{}{},
					}

					return req
				},
				resFactory: func() *bfe_http.Response {
					rsp := &bfe_http.Response{
						Header: make(bfe_http.Header),
					}

					return rsp
				},
			},
			want: bfe_module.BfeHandlerGoOn,
			judge: func(request *bfe_basic.Request, res *bfe_http.Response, t *testing.T) bool {
				return len(res.Cookies()) == 0
			},
		},
		{
			name:          "case: succ, with userid, backend not modify",
			moduleFactory: modFactory,
			args: args{
				requestFactory: func() *bfe_basic.Request {
					req := &bfe_basic.Request{
						Context: map[interface{}]interface{}{
							UidCtxKey: bfeUIDCookie,
						},
					}

					return req
				},
				resFactory: func() *bfe_http.Response {
					rsp := &bfe_http.Response{
						Header: make(bfe_http.Header),
					}

					rsp.Header.Add("Set-Cookie", bfeUIDCookie.String())
					rsp.Header.Add("Set-Cookie", productOtherCookie.String())

					return rsp
				},
			},
			want: bfe_module.BfeHandlerGoOn,
			judge: func(request *bfe_basic.Request, res *bfe_http.Response, t *testing.T) bool {
				cs := res.Cookies()
				if l := len(cs); l != 3 {
					t.Errorf("len(cookies) = %v, want %v", l, 3)
					return false
				}

				if cs[0].String() != bfeUIDCookie.String() {
					t.Errorf("bfe cookie not match")
					return false
				}

				if cs[1].String() != productOtherCookie.String() {
					t.Errorf("bfe cookie not match")
					return false
				}

				return false
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.moduleFactory()
			request, rsp := tt.args.requestFactory(), tt.args.resFactory()

			if got := m.rspSetUid(request, rsp); got != tt.want {
				t.Errorf("ModuleUserID.rspSetUid() = %v, want %v", got, tt.want)
				return
			}

			if tt.judge != nil {
				tt.judge(request, rsp, t)
			}
		})
	}
}

func TestModuleUserIDSetUID2Request(t *testing.T) {
	tests := []struct {
		name           string
		requestFactory func() *bfe_basic.Request
		moduleFactory  func() *ModuleUserID
		want           int
		want1          *bfe_http.Response

		requestJudge func(*bfe_basic.Request, *testing.T)
	}{
		{
			name:          "succ: existed userid",
			moduleFactory: modFactory,
			requestFactory: func() *bfe_basic.Request {
				req := &bfe_basic.Request{
					Context: map[interface{}]interface{}{
						UidCtxKey: bfeUIDCookie,
					},
					HttpRequest: &bfe_http.Request{
						Header: make(bfe_http.Header),
					},
				}

				req.Route.Product = "example.org"
				req.HttpRequest.AddCookie(productOtherCookie)
				req.HttpRequest.AddCookie(productUIDCookie)

				return req
			},

			requestJudge: func(req *bfe_basic.Request, t *testing.T) {
				c, ok := req.Cookie("bfe_userid")
				if !ok {
					t.Errorf("want get cookie")
					return
				}
				if c.MaxAge == 0 {
					t.Errorf("max age want not be 0")
					return
				}

				if data := req.GetContext(UidCtxKey); data == nil {
					t.Errorf("got nil, want not nil")
					return
				}
			},
			want: bfe_module.BfeHandlerGoOn,
		},
		{
			name:          "succ: not exist userid",
			moduleFactory: modFactory,
			requestFactory: func() *bfe_basic.Request {
				req := &bfe_basic.Request{
					Context: map[interface{}]interface{}{
						UidCtxKey: bfeUIDCookie,
					},
					HttpRequest: &bfe_http.Request{
						Header: make(bfe_http.Header),
					},
				}

				req.Route.Product = "example.org"
				req.HttpRequest.AddCookie(productOtherCookie)
				// req.HttpRequest.AddCookie(productUIDCookie)

				return req
			},

			requestJudge: func(req *bfe_basic.Request, t *testing.T) {
				c, ok := req.Cookie("bfe_userid")
				if !ok {
					t.Errorf("want get cookie")
					return
				}
				if c.MaxAge == 0 {
					t.Errorf("max age want not be 0")
					return
				}

				if data := req.GetContext(UidCtxKey); data == nil {
					t.Errorf("got nil, want not nil")
					return
				}
			},
			want: bfe_module.BfeHandlerGoOn,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.moduleFactory()

			request := tt.requestFactory()

			got, got1 := m.reqSetUid(request)

			if got != tt.want {
				t.Errorf("ModuleUserID.reqSetUid() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("ModuleUserID.reqSetUid() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestModuleUserIDSetConfig(t *testing.T) {
	m := NewModuleUserID()
	for i := 0; i < 100; i++ {
		go m.setConfig(nil)
		go m.getConfig()
	}

	time.Sleep(time.Second)
}

func TestModuleUserIDInit(t *testing.T) {
	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()

	m := NewModuleUserID()
	if err := m.Init(cbs, whs, "./testdata"); err != nil {
		t.Errorf("want nil, got: %v", err)
	}
}

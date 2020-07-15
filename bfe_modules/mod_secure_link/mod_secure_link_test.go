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
	"github.com/bfenetworks/bfe/bfe_module"
)

func TestModuleSecureLink(t *testing.T) {
	mod := NewModuleSecureLink()
	err := mod.loadConfData(url.Values{
		"path": {"testdata/mod_secure_link/secure_link_rule.data"},
	})
	if err != nil {
		t.Error(err)
		return
	}

	cases := []struct {
		name       string
		reqFactory func() *bfe_basic.Request
		wantReturn int
		wantCode   int
	}{
		{
			name: "ok",
			reqFactory: func() *bfe_basic.Request {
				return &bfe_basic.Request{
					HttpRequest: &bfe_http.Request{
						RemoteAddr: "127.0.0.1",
						RequestURI: "/a/b",
					},
					Query: url.Values{
						"sign": []string{"3U03NuohsM1xZjF6XPS0vg"},
						"time": []string{"9999999999"},
					},
					Route: bfe_basic.RequestRoute{
						Product: "p1",
					},
				}
			},
			wantReturn: bfe_module.BfeHandlerGoOn,
			wantCode:   0,
		},

		{
			name: "overdue",
			reqFactory: func() *bfe_basic.Request {
				return &bfe_basic.Request{
					HttpRequest: &bfe_http.Request{
						RemoteAddr: "127.0.0.1",
						RequestURI: "/a/b",
					},
					Query: url.Values{
						"sign": []string{"3U03NuohsM1xZjF6XPS0vg"},
						"time": []string{"0"},
					},
					Route: bfe_basic.RequestRoute{
						Product: "p1",
					},
				}
			},
			wantReturn: bfe_module.BfeHandlerResponse,
			wantCode:   403,
		},

		{
			name: "no sign",
			reqFactory: func() *bfe_basic.Request {
				return &bfe_basic.Request{
					HttpRequest: &bfe_http.Request{
						RemoteAddr: "127.0.0.1",
						RequestURI: "/a/b",
					},
					Query: url.Values{
						"sign": []string{""},
						"time": []string{"9999999999"},
					},
					Route: bfe_basic.RequestRoute{
						Product: "p1",
					},
				}
			},
			wantReturn: bfe_module.BfeHandlerResponse,
			wantCode:   403,
		},

		{
			name: "bad sign",
			reqFactory: func() *bfe_basic.Request {
				return &bfe_basic.Request{
					HttpRequest: &bfe_http.Request{
						RemoteAddr: "127.0.0.1",
						RequestURI: "/a/b",
					},
					Query: url.Values{
						"sign": []string{"---"},
						"time": []string{"9999999999"},
					},
					Route: bfe_basic.RequestRoute{
						Product: "p1",
					},
				}
			},
			wantReturn: bfe_module.BfeHandlerResponse,
			wantCode:   403,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.reqFactory()
			ret, rsp := mod.validateHandler(req)
			if tt.wantReturn != ret {
				t.Errorf("want: %v, got: %v", tt.wantReturn, ret)
				return
			}
			if tt.wantCode != 0 {
				if tt.wantCode != rsp.StatusCode {
					t.Errorf("want: %v, got: %v", tt.wantCode, req.Redirect.Code)
				}
			}
		})
	}
}

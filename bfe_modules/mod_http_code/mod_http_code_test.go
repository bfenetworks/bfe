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

package mod_http_code

import (
	"testing"
)

import (
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func TestNewModuleHttpCode(t *testing.T) {
	m := NewModuleHttpCode()
	if m == nil {
		t.Fatal("NewModuleHttpCode() should not return nil")
	}
	if m.Name() != ModHttpCode {
		t.Errorf("Name() = %s, want %s", m.Name(), ModHttpCode)
	}
}

func TestModuleHttpCodeName(t *testing.T) {
	m := NewModuleHttpCode()
	if got := m.Name(); got != ModHttpCode {
		t.Errorf("Name() = %s, want %s", got, ModHttpCode)
	}
}

func TestRequestFinish(t *testing.T) {
	m := NewModuleHttpCode()
	var req bfe_basic.Request
	req.HttpRequest = new(bfe_http.Request)
	req.HttpResponse = new(bfe_http.Response)
	req.HttpResponse.StatusCode = 201
	req.Session = new(bfe_basic.Session)

	m.requestFinish(&req, nil)
	if m.state.All2XX.Get() != 1 {
		t.Errorf("counter All2XX should be 1")
	}

	m.requestFinish(&req, nil)

	if m.state.All2XX.Get() != 2 {
		t.Errorf("counter All2XX should be 2")
	}
}

func TestRequestFinishStatusCodes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		want2XX int64
		want3XX int64
		want4XX int64
		want5XX int64
	}{
		{"2xx lower boundary", 200, 1, 0, 0, 0},
		{"2xx upper boundary", 299, 1, 0, 0, 0},
		{"3xx lower boundary", 300, 0, 1, 0, 0},
		{"3xx upper boundary", 399, 0, 1, 0, 0},
		{"4xx lower boundary", 400, 0, 0, 1, 0},
		{"4xx upper boundary", 499, 0, 0, 1, 0},
		{"5xx lower boundary", 500, 0, 0, 0, 1},
		{"5xx upper boundary", 599, 0, 0, 0, 1},
		{"1xx not counted", 100, 0, 0, 0, 0},
		{"6xx not counted", 600, 0, 0, 0, 0},
		{"0 not counted", 0, 0, 0, 0, 0},
		{"2xx mid", 250, 1, 0, 0, 0},
		{"3xx mid", 350, 0, 1, 0, 0},
		{"4xx mid", 404, 0, 0, 1, 0},
		{"5xx mid", 503, 0, 0, 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModuleHttpCode()
			req := &bfe_basic.Request{
				HttpRequest:  new(bfe_http.Request),
				HttpResponse: &bfe_http.Response{StatusCode: tt.status},
				Session:      new(bfe_basic.Session),
			}

			ret := m.requestFinish(req, nil)
			if ret != bfe_module.BfeHandlerGoOn {
				t.Errorf("requestFinish() = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
			}

			if got := m.state.All2XX.Get(); got != tt.want2XX {
				t.Errorf("All2XX = %d, want %d", got, tt.want2XX)
			}
			if got := m.state.All3XX.Get(); got != tt.want3XX {
				t.Errorf("All3XX = %d, want %d", got, tt.want3XX)
			}
			if got := m.state.All4XX.Get(); got != tt.want4XX {
				t.Errorf("All4XX = %d, want %d", got, tt.want4XX)
			}
			if got := m.state.All5XX.Get(); got != tt.want5XX {
				t.Errorf("All5XX = %d, want %d", got, tt.want5XX)
			}
		})
	}
}

func TestRequestFinishNilResponse(t *testing.T) {
	m := NewModuleHttpCode()
	req := &bfe_basic.Request{
		HttpRequest:  new(bfe_http.Request),
		HttpResponse: nil,
		Session:      new(bfe_basic.Session),
	}

	ret := m.requestFinish(req, nil)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("requestFinish() = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}

	if got := m.state.All2XX.Get(); got != 0 {
		t.Errorf("All2XX = %d, want 0", got)
	}
	if got := m.state.All3XX.Get(); got != 0 {
		t.Errorf("All3XX = %d, want 0", got)
	}
	if got := m.state.All4XX.Get(); got != 0 {
		t.Errorf("All4XX = %d, want 0", got)
	}
	if got := m.state.All5XX.Get(); got != 0 {
		t.Errorf("All5XX = %d, want 0", got)
	}
}

func TestRequestFinishCounterState(t *testing.T) {
	m := NewModuleHttpCode()
	codes := []int{200, 200, 301, 404, 500, 200, 302, 403, 502}
	want2XX := int64(3)
	want3XX := int64(2)
	want4XX := int64(2)
	want5XX := int64(2)

	for _, code := range codes {
		req := &bfe_basic.Request{
			HttpRequest:  new(bfe_http.Request),
			HttpResponse: &bfe_http.Response{StatusCode: code},
			Session:      new(bfe_basic.Session),
		}
		m.requestFinish(req, nil)
	}

	if got := m.state.All2XX.Get(); got != want2XX {
		t.Errorf("All2XX = %d, want %d", got, want2XX)
	}
	if got := m.state.All3XX.Get(); got != want3XX {
		t.Errorf("All3XX = %d, want %d", got, want3XX)
	}
	if got := m.state.All4XX.Get(); got != want4XX {
		t.Errorf("All4XX = %d, want %d", got, want4XX)
	}
	if got := m.state.All5XX.Get(); got != want5XX {
		t.Errorf("All5XX = %d, want %d", got, want5XX)
	}
}

func TestInit(t *testing.T) {
	m := NewModuleHttpCode()
	if m.Name() != ModHttpCode {
		t.Errorf("ModHttpCode Name() should be %s", ModHttpCode)
	}

	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()
	err := m.Init(cbs, whs, "test")
	if err != nil {
		t.Errorf("ModHttpCode Init() error: %v", err)
	}
}

func TestInitFailure(t *testing.T) {
	tests := []struct {
		name    string
		cbs     *bfe_module.BfeCallbacks
		whs     *web_monitor.WebHandlers
		cr      string
		wantErr bool
	}{
		{
			name:    "nil web handlers",
			cbs:     bfe_module.NewBfeCallbacks(),
			whs:     nil,
			cr:      "test",
			wantErr: true,
		},
		{
			name:    "empty callbacks",
			cbs:     &bfe_module.BfeCallbacks{},
			whs:     web_monitor.NewWebHandlers(),
			cr:      "test",
			wantErr: true,
		},
		{
			name:    "empty callbacks and nil web handlers",
			cbs:     &bfe_module.BfeCallbacks{},
			whs:     nil,
			cr:      "test",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModuleHttpCode()
			err := m.Init(tt.cbs, tt.whs, tt.cr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Init() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

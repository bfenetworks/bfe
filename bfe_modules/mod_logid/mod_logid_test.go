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

package mod_logid

import (
	"io/ioutil"
	"os"
	"strconv"
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

// maxUint63 is the upper bound of a log id (bfenetworks/bfe#1417): logid is
// stored as a signed BIGINT on MySQL / Doris / StarRocks, so values above
// 2^63-1 would be truncated (Doris, non-strict) or rejected (MySQL, strict).
const maxUint63 = uint64(1)<<63 - 1

func TestNewModuleLogId(t *testing.T) {
	m := NewModuleLogId()
	if m == nil {
		t.Fatal("NewModuleLogId() returned nil")
	}
	if m.Name() != ModLogId {
		t.Errorf("Name() = %q, want %q", m.Name(), ModLogId)
	}
}

func TestModuleLogIdInit(t *testing.T) {
	m := NewModuleLogId()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	tmpDir, err := ioutil.TempDir("", "mod_logid")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := m.Init(cb, wh, tmpDir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// handler should be registered in monitor handlers
	if _, err := wh.GetHandler(web_monitor.WebHandleMonitor, ModLogId); err != nil {
		t.Errorf("monitor handler not registered: %v", err)
	}
}

func TestModuleLogIdInitFailure(t *testing.T) {
	m := NewModuleLogId()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	tmpDir, err := ioutil.TempDir("", "mod_logid")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := m.Init(cb, wh, tmpDir); err != nil {
		t.Fatalf("first Init() error: %v", err)
	}

	// second Init should fail because monitor handler is already registered
	if err := m.Init(cb, wh, tmpDir); err == nil {
		t.Errorf("second Init() should fail")
	}
}

func TestSessionIdHandler(t *testing.T) {
	m := NewModuleLogId()
	session := bfe_basic.NewSession(nil)

	ret := m.sessionIdHandler(session)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("sessionIdHandler() = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
	if !isDecimalLogId(session.SessionId) {
		t.Errorf("SessionId = %q, want decimal log id (<= 2^63-1)", session.SessionId)
	}
}

func TestRequestIdHandler(t *testing.T) {
	tests := []struct {
		name          string
		trustSource   bool
		headerLogId   string
		expectSame    bool // expect request to keep headerLogId unchanged
		expectCounter int
	}{
		{
			name: "not trust source",
		},
		{
			name:        "trust source with header",
			trustSource: true,
			headerLogId: "existing-log-id",
			expectSame:  true,
		},
		{
			name:          "trust source without header",
			trustSource:   true,
			expectSame:    false,
			expectCounter: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModuleLogId()
			session := bfe_basic.NewSession(nil)
			session.SetTrustSource(tt.trustSource)

			httpReq, err := bfe_http.NewRequest("GET", "http://example.org/", nil)
			if err != nil {
				t.Fatalf("NewRequest() error: %v", err)
			}
			httpReq.Header = make(bfe_http.Header)
			if tt.headerLogId != "" {
				httpReq.Header.Set(bfe_basic.HeaderBfeLogId, tt.headerLogId)
			}

			req := bfe_basic.NewRequest(httpReq, nil, nil, session, nil)

			ret, resp := m.requestIdHandler(req)
			if ret != bfe_module.BfeHandlerGoOn {
				t.Errorf("requestIdHandler() = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
			}
			if resp != nil {
				t.Errorf("requestIdHandler() response = %v, want nil", resp)
			}

			if tt.expectSame {
				if req.LogId != "" {
					t.Errorf("LogId = %q, want empty", req.LogId)
				}
				if got := httpReq.Header.Get(bfe_basic.HeaderBfeLogId); got != tt.headerLogId {
					t.Errorf("header %s = %q, want %q", bfe_basic.HeaderBfeLogId, got, tt.headerLogId)
				}
			} else {
				if !isDecimalLogId(req.LogId) {
					t.Errorf("LogId = %q, want decimal log id (<= 2^63-1)", req.LogId)
				}
				if got := httpReq.Header.Get(bfe_basic.HeaderBfeLogId); got != req.LogId {
					t.Errorf("header %s = %q, want %q", bfe_basic.HeaderBfeLogId, got, req.LogId)
				}
			}

			if int(m.state.NoLogidFromUpperBfe.Get()) != tt.expectCounter {
				t.Errorf("NoLogidFromUpperBfe counter = %d, want %d",
					m.state.NoLogidFromUpperBfe.Get(), tt.expectCounter)
			}
		})
	}
}

func TestGenLogId(t *testing.T) {
	// generator moved to bfe_basic.GenLogId (bfenetworks/bfe#1441 round 2);
	// mod_logid keeps a thin conformance test on the shared implementation.
	logId := bfe_basic.GenLogId()
	if !isDecimalLogId(logId) {
		t.Errorf("GenLogId() = %q, want decimal log id (<= 2^63-1)", logId)
	}

	// the conversion into the uint64 access-log field must be lossless
	// (bfenetworks/bfe#1417): parse and re-format on the same value
	v, err := strconv.ParseUint(logId, 10, 64)
	if err != nil {
		t.Fatalf("ParseUint(%q) error: %v", logId, err)
	}
	if v == 0 {
		t.Errorf("GenLogId() = %q, want non-zero value", logId)
	}
	if strconv.FormatUint(v, 10) != logId {
		t.Errorf("FormatUint(ParseUint(%q)) = %q, want identity", logId, strconv.FormatUint(v, 10))
	}

	// two generated ids should differ with overwhelming probability
	another := bfe_basic.GenLogId()
	if another == logId {
		t.Errorf("GenLogId() returned duplicate value %q", logId)
	}
}

func TestGetState(t *testing.T) {
	m := NewModuleLogId()
	_, err := m.getState(nil)
	if err != nil {
		t.Errorf("getState() error: %v", err)
	}
}

// isDecimalLogId reports whether s is a log id in the current format
// (bfenetworks/bfe#1417): a decimal string denoting a value in [1, 2^63-1].
func isDecimalLogId(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	v, err := strconv.ParseUint(s, 10, 64)
	return err == nil && v > 0 && v <= maxUint63
}

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

package mod_trace

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

import (
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
	"github.com/opentracing/opentracing-go"
	jaegercli "github.com/uber/jaeger-client-go"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func initModule(t *testing.T) *ModuleTrace {
	t.Helper()

	m := NewModuleTrace()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	return m
}

func prepareTraceRequest(host, product string) *bfe_basic.Request {
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = product
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://"+host, nil)
	req.HttpRequest.Header = make(bfe_http.Header)
	req.Context = make(map[interface{}]interface{})
	return req
}

func writeTempConfig(t *testing.T, content string, dataFiles map[string]string) string {
	t.Helper()

	tmpDir, err := ioutil.TempDir("", "mod_trace")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	modDir := filepath.Join(tmpDir, ModTrace)
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	confPath := filepath.Join(modDir, ModTrace+".conf")
	if err := ioutil.WriteFile(confPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	dataDir := filepath.Join(tmpDir, ModTrace)
	for name, content := range dataFiles {
		path := filepath.Join(dataDir, name)
		if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}
	}

	return tmpDir
}

func TestNewModuleTrace(t *testing.T) {
	m := NewModuleTrace()
	if m == nil {
		t.Fatal("NewModuleTrace() returned nil")
	}
	if m.Name() != ModTrace {
		t.Errorf("Name() = %q, want %q", m.Name(), ModTrace)
	}
}

func TestModuleTraceInit(t *testing.T) {
	m := NewModuleTrace()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if globalTrace == nil || !globalTrace.IsEnabled() {
		t.Errorf("global trace is not enabled after Init()")
	}

	if _, err := wh.GetHandler(web_monitor.WebHandleMonitor, ModTrace); err != nil {
		t.Errorf("monitor handler not registered: %v", err)
	}
	if _, err := wh.GetHandler(web_monitor.WebHandleMonitor, ModTrace+".diff"); err != nil {
		t.Errorf("monitor diff handler not registered: %v", err)
	}
	if _, err := wh.GetHandler(web_monitor.WebHandleReload, ModTrace); err != nil {
		t.Errorf("reload handler not registered: %v", err)
	}

	if _, err := m.getState(nil); err != nil {
		t.Errorf("getState() error: %v", err)
	}
	if _, err := m.getStateDiff(nil); err != nil {
		t.Errorf("getStateDiff() error: %v", err)
	}
}

func TestModuleTraceInitFailureBadConf(t *testing.T) {
	conf := `[Basic]
ServiceName = bfe
DataPath = mod_trace/trace_rule.data
`
	tmpDir := writeTempConfig(t, conf, nil)

	m := NewModuleTrace()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, tmpDir); err == nil {
		t.Errorf("Init() should fail with missing TraceAgent")
	}
}

func TestModuleTraceInitFailureTraceSetup(t *testing.T) {
	conf := `[Basic]
ServiceName = bfe
DataPath = mod_trace/trace_rule.data
TraceAgent = jaeger

[Jaeger]
Propagation = unknown
`
	data := `{
    "Version": "2026",
    "Config": {
        "example_product": [
             {
                 "Cond": "req_host_in(\"example.org\")",
                 "Enable": true
             }
        ]
    }
}
`
	tmpDir := writeTempConfig(t, conf, map[string]string{"trace_rule.data": data})

	m := NewModuleTrace()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, tmpDir); err == nil {
		t.Errorf("Init() should fail when trace agent setup fails")
	}
}

func TestModuleTraceInitFailureDuplicateHandler(t *testing.T) {
	m := NewModuleTrace()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("first Init() error: %v", err)
	}

	if err := m.Init(cb, wh, "./testdata"); err == nil {
		t.Errorf("second Init() should fail because handlers are already registered")
	}
}

func TestStartTraceNoMatch(t *testing.T) {
	m := initModule(t)

	req := prepareTraceRequest("example.org", "unknown_product")
	ret, resp := m.startTrace(req)

	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("startTrace() = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
	if resp != nil {
		t.Errorf("startTrace() response = %v, want nil", resp)
	}
	if req.GetContext(CtxSpan) != nil {
		t.Errorf("CtxSpan should be nil when product does not match")
	}
}

func TestStartTraceDisabled(t *testing.T) {
	m := initModule(t)

	data := `{
    "Version": "2026",
    "Config": {
        "example_product": [
             {
                 "Cond": "req_host_in(\"example.org\")",
                 "Enable": false
             }
        ]
    }
}
`
	f, err := ioutil.TempFile("", "trace_rule_disabled_*.data")
	if err != nil {
		t.Fatalf("TempFile() error: %v", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(data); err != nil {
		t.Fatalf("WriteString() error: %v", err)
	}
	f.Close()

	if _, err := m.loadRuleData(url.Values{"path": []string{f.Name()}}); err != nil {
		t.Fatalf("loadRuleData() error: %v", err)
	}

	req := prepareTraceRequest("example.org", "example_product")
	m.startTrace(req)

	if req.GetContext(CtxSpan) != nil {
		t.Errorf("CtxSpan should be nil when rule is disabled")
	}
}

func TestStartTraceCreatesSpan(t *testing.T) {
	m := initModule(t)

	before := m.state.StartSpanCount.Get()
	req := prepareTraceRequest("example.org", "example_product")
	ret, resp := m.startTrace(req)

	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("startTrace() = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
	if resp != nil {
		t.Errorf("startTrace() response = %v, want nil", resp)
	}

	value := req.GetContext(CtxSpan)
	if value == nil {
		t.Fatalf("CtxSpan should not be nil")
	}
	if span, ok := value.(opentracing.Span); !ok || span == nil {
		t.Errorf("CtxSpan should contain a non-nil span")
	}

	if got := m.state.StartSpanCount.Get(); got != before+1 {
		t.Errorf("StartSpanCount = %d, want %d", got, before+1)
	}

	if req.HttpRequest.Header.Get("Uber-Trace-Id") == "" {
		t.Errorf("Uber-Trace-Id header should be injected")
	}
}

func TestFinishTraceNoSpan(t *testing.T) {
	m := initModule(t)

	req := prepareTraceRequest("example.org", "example_product")
	before := m.state.FinishSpanCount.Get()

	ret := m.finishTrace(req, nil)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("finishTrace() = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
	if got := m.state.FinishSpanCount.Get(); got != before {
		t.Errorf("FinishSpanCount = %d, want %d", got, before)
	}
}

func TestFinishTraceWithSpan(t *testing.T) {
	m := initModule(t)

	req := prepareTraceRequest("example.org", "example_product")
	m.startTrace(req)

	req.HttpResponse = &bfe_http.Response{StatusCode: 500}
	req.ErrMsg = "backend error"
	req.Backend = bfe_basic.BackendInfo{
		ClusterName:    "test_cluster",
		SubclusterName: "test_subcluster",
		BackendAddr:    "127.0.0.1",
		BackendPort:    8080,
	}

	before := m.state.FinishSpanCount.Get()
	ret := m.finishTrace(req, nil)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("finishTrace() = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
	if got := m.state.FinishSpanCount.Get(); got != before+1 {
		t.Errorf("FinishSpanCount = %d, want %d", got, before+1)
	}
}

func TestTraceIDPropagation(t *testing.T) {
	_ = initModule(t)

	traceID := "1234567890abcdef"
	parentSpanID := "fedcba0987654321"
	headerValue := fmt.Sprintf("%s:%s:0:1", traceID, parentSpanID)

	req := prepareTraceRequest("example.org", "example_product")
	req.HttpRequest.Header.Set("Uber-Trace-Id", headerValue)

	span := StartSpan(req.HttpRequest)
	if span == nil {
		t.Fatal("StartSpan() returned nil")
	}

	spanCtx, ok := span.Context().(jaegercli.SpanContext)
	if !ok {
		t.Fatalf("span context should be jaeger.SpanContext, got %T", span.Context())
	}
	if spanCtx.TraceID().String() != traceID {
		t.Errorf("TraceID = %q, want %q", spanCtx.TraceID().String(), traceID)
	}

	InjectRequestHeader(span, req.HttpRequest)
	injected := req.HttpRequest.Header.Get("Uber-Trace-Id")
	if injected == "" {
		t.Fatalf("Uber-Trace-Id header should be injected")
	}

	parts := strings.SplitN(injected, ":", 2)
	if len(parts) < 2 {
		t.Fatalf("injected Uber-Trace-Id has invalid format: %q", injected)
	}
	if parts[0] != traceID {
		t.Errorf("injected trace id = %q, want %q", parts[0], traceID)
	}
}

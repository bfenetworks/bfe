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

package mod_ai_route_integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_modules"
	"github.com/bfenetworks/bfe/bfe_server"
)

var modulesOnce sync.Once

func TestMain(m *testing.M) {
	modulesOnce.Do(bfe_modules.SetModules)
	os.Exit(m.Run())
}

// fakeConn implements net.Conn for test environment.
type fakeConn struct{}

func (c *fakeConn) Read(b []byte) (int, error)       { return 0, nil }
func (c *fakeConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *fakeConn) Close() error                     { return nil }
func (c *fakeConn) LocalAddr() net.Addr              { return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0} }
func (c *fakeConn) RemoteAddr() net.Addr             { return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0} }
func (c *fakeConn) SetDeadline(t time.Time) error    { return nil }
func (c *fakeConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(t time.Time) error { return nil }

const (
	apiHost         = "api.example.org"
	otherHost       = "other.example.org"
	entityHost      = "entity.example.org"
	unknownHost     = "unknown.example.org"
	apiPath         = "/v1/chat/completions"
	apiKeyUserA     = "ak_user_a"
	apiKeyUserB     = "ak_user_b"
	apiKeyNoBinding = "ak_no_binding"
)

var clusterNames = []string{
	"cluster_primary_a",
	"cluster_primary_b",
	"cluster_primary_c",
	"cluster_fallback_1",
	"cluster_fallback_2",
	"cluster_entity_default",
	"cluster_global_default",
}

// responseRecorder implements bfe_http.ResponseWriter for test verification.
type responseRecorder struct {
	statusCode int
	header     bfe_http.Header
	body       *bytes.Buffer
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{
		statusCode: 200,
		header:     make(bfe_http.Header),
		body:       new(bytes.Buffer),
	}
}

func (r *responseRecorder) Header() bfe_http.Header {
	return r.header
}

func (r *responseRecorder) Write(p []byte) (int, error) {
	return r.body.Write(p)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}

// backendServer wraps an httptest.Server and records request metadata.
type backendServer struct {
	server      *httptest.Server
	clusterName string
	response    int
	body        string
	hits        int
	mu          sync.Mutex
	models      []string
}

func newBackendServer(clusterName string, response int, body string) *backendServer {
	b := &backendServer{
		clusterName: clusterName,
		response:    response,
		body:        body,
	}
	b.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		b.hits++
		if r.Body != nil {
			bodyBytes, _ := io.ReadAll(r.Body)
			var reqBody map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &reqBody); err == nil {
				if model, ok := reqBody["model"].(string); ok {
					b.models = append(b.models, model)
				}
			}
		}
		b.mu.Unlock()
		w.WriteHeader(b.response)
		if b.body != "" {
			w.Write([]byte(b.body))
		}
	}))
	return b
}

func (b *backendServer) Hits() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.hits
}

func (b *backendServer) Models() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.models...)
}

func (b *backendServer) Close() {
	b.server.Close()
}

func (b *backendServer) Addr() string {
	u, _ := url.Parse(b.server.URL)
	return u.Host
}

func (b *backendServer) HostPort() (string, int) {
	u, _ := url.Parse(b.server.URL)
	host := u.Hostname()
	port := 80
	if u.Port() != "" {
		fmt.Sscanf(u.Port(), "%d", &port)
	}
	return host, port
}

// testEnv holds all resources for a single integration test.
type testEnv struct {
	t        *testing.T
	srv      *bfe_server.BfeServer
	backends map[string]*backendServer
	tempDir  string
}

func newTestEnv(t *testing.T, responseMap map[string]int) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*backendServer),
	}

	// start mock backends
	for _, name := range clusterNames {
		resp, ok := responseMap[name]
		if !ok {
			resp = http.StatusOK
		}
		e.backends[name] = newBackendServer(name, resp, fmt.Sprintf("response from %s", name))
	}

	// prepare temp conf dir
	e.tempDir = t.TempDir()
	if err := copyDir("testdata", e.tempDir); err != nil {
		t.Fatalf("copy testdata failed: %v", err)
	}

	// generate cluster_table.data with actual backend addresses
	if err := e.generateClusterTable(); err != nil {
		t.Fatalf("generate cluster table failed: %v", err)
	}

	// load bfe config
	confPath := filepath.Join(e.tempDir, "bfe.conf")
	cfg, err := bfe_conf.BfeConfigLoad(confPath, e.tempDir)
	if err != nil {
		t.Fatalf("load bfe config failed: %v", err)
	}

	// create server
	e.srv = bfe_server.NewBfeServer(cfg, e.tempDir, "test")

	// init web monitor (needed by InitModules)
	if err := e.srv.InitWebMonitor(0); err != nil {
		t.Fatalf("init web monitor failed: %v", err)
	}

	// register and init modules
	if err := e.srv.RegisterModules(cfg.Server.Modules); err != nil {
		t.Fatalf("register modules failed: %v", err)
	}
	if err := e.srv.InitModules(); err != nil {
		t.Fatalf("init modules failed: %v", err)
	}

	// load server data conf (cluster, host, route, etc.)
	if err := e.srv.InitDataLoad(); err != nil {
		t.Fatalf("init data load failed: %v", err)
	}

	return e
}

func (e *testEnv) Close() {
	for _, b := range e.backends {
		b.Close()
	}
}

func (e *testEnv) generateClusterTable() error {
	clusterTable := map[string]interface{}{
		"Version": "20260720150000",
		"Config":  map[string]interface{}{},
	}
	config := clusterTable["Config"].(map[string]interface{})

	for _, name := range clusterNames {
		b := e.backends[name]
		host, port := b.HostPort()
		config[name] = map[string]interface{}{
			"sub_" + clusterSubName(name): []map[string]interface{}{
				{
					"name":   name + "-backend-0",
					"addr":   host,
					"port":   port,
					"weight": 100,
				},
			},
		}
	}

	data, err := json.MarshalIndent(clusterTable, "", "    ")
	if err != nil {
		return err
	}

	path := filepath.Join(e.tempDir, "cluster_conf", "cluster_table.data")
	return ioutil.WriteFile(path, data, 0644)
}

func clusterSubName(clusterName string) string {
	switch clusterName {
	case "cluster_primary_a":
		return "a"
	case "cluster_primary_b":
		return "b"
	case "cluster_primary_c":
		return "c"
	case "cluster_fallback_1":
		return "fb1"
	case "cluster_fallback_2":
		return "fb2"
	case "cluster_entity_default":
		return "entity"
	case "cluster_global_default":
		return "global"
	}
	return "sub"
}

func (e *testEnv) newRequest(host, apiKey string, body []byte) *bfe_basic.Request {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := bfe_http.NewRequest(http.MethodPost, "http://"+host+apiPath, bodyReader)
	if err != nil {
		e.t.Fatalf("new request failed: %v", err)
	}
	req.Host = host
	req.State = &bfe_http.RequestState{}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	basicReq := bfe_basic.NewRequest(req, &fakeConn{}, bfe_basic.NewRequestStat(time.Now()),
		bfe_basic.NewSession(&fakeConn{}), e.srv.GetServerConf())
	basicReq.Route.Product = "ai_product"

	aiMeta := basicReq.InitAiBasicInfo()
	aiMeta.ClientApiKey = apiKey

	return basicReq
}

var emptyJSONBody = []byte("{}")

func (e *testEnv) callServeHTTPForAI(host, apiKey string, body []byte) *responseRecorder {
	rec := newResponseRecorder()
	basicReq := e.newRequest(host, apiKey, body)
	e.srv.ReverseProxy.ServeHTTPForAI(rec, basicReq)
	if rec.statusCode != http.StatusOK {
		e.t.Logf("ServeHTTPForAI returned status %d, body: %q", rec.statusCode, rec.body.String())
	}
	return rec
}

func copyDir(src, dst string) error {
	entries, err := ioutil.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return err
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := ioutil.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := ioutil.WriteFile(dstPath, data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

// TestMultiRouteTablesApikeyHit verifies that a specific apikey rule is
// matched before falling through to subsequent route tables.
func TestMultiRouteTablesApikeyHit(t *testing.T) {
	e := newTestEnv(t, map[string]int{
		"cluster_primary_a": http.StatusOK,
		"cluster_primary_b": http.StatusOK,
		"cluster_primary_c": http.StatusOK,
	})
	defer e.Close()

	rec := e.callServeHTTPForAI(apiHost, apiKeyUserA, emptyJSONBody)
	if rec.statusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", rec.statusCode, rec.body.String())
	}

	// one of the primary clusters should be hit
	hits := e.backends["cluster_primary_a"].Hits() +
		e.backends["cluster_primary_b"].Hits() +
		e.backends["cluster_primary_c"].Hits()
	if hits != 1 {
		t.Fatalf("expected exactly one primary cluster hit, got %d", hits)
	}
}

// TestMultiRouteTablesEntityFallback verifies that multiple route tables are
// searched in binding order when the apikey table has no matching rule.
func TestMultiRouteTablesEntityFallback(t *testing.T) {
	e := newTestEnv(t, map[string]int{
		"cluster_entity_default": http.StatusOK,
	})
	defer e.Close()

	rec := e.callServeHTTPForAI(otherHost, apiKeyUserA, nil)
	if rec.statusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", rec.statusCode, rec.body.String())
	}

	if e.backends["cluster_entity_default"].Hits() != 1 {
		t.Fatalf("expected cluster_entity_default hit once, got %d", e.backends["cluster_entity_default"].Hits())
	}
}

// TestMultiRouteTablesNoBinding verifies that an apikey without binding
// results in 404.
func TestMultiRouteTablesNoBinding(t *testing.T) {
	e := newTestEnv(t, map[string]int{})
	defer e.Close()

	rec := e.callServeHTTPForAI(apiHost, apiKeyNoBinding, nil)
	if rec.statusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.statusCode)
	}
	body := rec.body.String()
	if !strings.Contains(body, "AI route not found") {
		t.Fatalf("expected 'AI route not found' in body, got %q", body)
	}
}

// TestMultiTargetsWeightedSelection verifies weighted random target selection.
func TestMultiTargetsWeightedSelection(t *testing.T) {
	e := newTestEnv(t, map[string]int{
		"cluster_primary_a": http.StatusOK,
		"cluster_primary_b": http.StatusOK,
		"cluster_primary_c": http.StatusOK,
	})
	defer e.Close()

	const total = 1000
	for i := 0; i < total; i++ {
		rec := e.callServeHTTPForAI(apiHost, apiKeyUserA, emptyJSONBody)
		if rec.statusCode != http.StatusOK {
			t.Fatalf("request %d: expected status 200, got %d", i, rec.statusCode)
		}
	}

	hitsA := e.backends["cluster_primary_a"].Hits()
	hitsB := e.backends["cluster_primary_b"].Hits()
	hitsC := e.backends["cluster_primary_c"].Hits()

	t.Logf("hits distribution: a=%d b=%d c=%d", hitsA, hitsB, hitsC)

	if hitsA+hitsB+hitsC != total {
		t.Fatalf("expected total hits %d, got %d", total, hitsA+hitsB+hitsC)
	}

	// weights: 60 / 30 / 10, allow ±50 tolerance
	if hitsA < 550 || hitsA > 650 {
		t.Fatalf("expected hitsA around 600, got %d", hitsA)
	}
	if hitsB < 250 || hitsB > 350 {
		t.Fatalf("expected hitsB around 300, got %d", hitsB)
	}
	if hitsC < 50 || hitsC > 150 {
		t.Fatalf("expected hitsC around 100, got %d", hitsC)
	}
}

// TestMultiFallbacksSuccess verifies that fallbacks are tried until one
// returns success.
func TestMultiFallbacksSuccess(t *testing.T) {
	e := newTestEnv(t, map[string]int{
		"cluster_primary_a":  http.StatusInternalServerError,
		"cluster_primary_b":  http.StatusInternalServerError,
		"cluster_primary_c":  http.StatusInternalServerError,
		"cluster_fallback_1": http.StatusBadGateway,
		"cluster_fallback_2": http.StatusOK,
	})
	defer e.Close()

	// force deterministic target selection for this test: select primary_a
	// by setting its weight to 100 and others to 0 through route data is not
	// possible at runtime, so we make all primaries fail and verify fallback.
	rec := e.callServeHTTPForAI(apiHost, apiKeyUserA, emptyJSONBody)
	if rec.statusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.statusCode)
	}

	// all three primaries may be selected; at least one primary and both
	// fallbacks should be attempted across retries
	primaryHits := e.backends["cluster_primary_a"].Hits() +
		e.backends["cluster_primary_b"].Hits() +
		e.backends["cluster_primary_c"].Hits()
	if primaryHits < 1 {
		t.Fatalf("expected at least one primary hit, got %d", primaryHits)
	}
	if e.backends["cluster_fallback_1"].Hits() != 1 {
		t.Fatalf("expected cluster_fallback_1 hit once, got %d", e.backends["cluster_fallback_1"].Hits())
	}
	if e.backends["cluster_fallback_2"].Hits() != 1 {
		t.Fatalf("expected cluster_fallback_2 hit once, got %d", e.backends["cluster_fallback_2"].Hits())
	}
}

// TestMultiFallbacksAllFail verifies that all fallbacks are exhausted when
// every attempt fails.
func TestMultiFallbacksAllFail(t *testing.T) {
	e := newTestEnv(t, map[string]int{
		"cluster_primary_a":  http.StatusInternalServerError,
		"cluster_primary_b":  http.StatusInternalServerError,
		"cluster_primary_c":  http.StatusInternalServerError,
		"cluster_fallback_1": http.StatusInternalServerError,
		"cluster_fallback_2": http.StatusInternalServerError,
	})
	defer e.Close()

	rec := e.callServeHTTPForAI(apiHost, apiKeyUserA, emptyJSONBody)
	if rec.statusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.statusCode)
	}

	if e.backends["cluster_fallback_2"].Hits() != 1 {
		t.Fatalf("expected cluster_fallback_2 hit once, got %d", e.backends["cluster_fallback_2"].Hits())
	}
}

// TestModelOverrideAndFallback verifies model override for target and fallback.
func TestModelOverrideAndFallback(t *testing.T) {
	e := newTestEnv(t, map[string]int{
		"cluster_primary_a":  http.StatusInternalServerError,
		"cluster_fallback_1": http.StatusOK,
	})
	defer e.Close()

	body := []byte(`{"model":"origin-model"}`)
	rec := e.callServeHTTPForAI(apiHost, apiKeyUserA, body)
	if rec.statusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.statusCode)
	}

	// force deterministic target selection is hard; verify that at least one
	// primary backend received target model and fallback received fallback model
	var primaryGotTargetModel bool
	for _, name := range []string{"cluster_primary_a", "cluster_primary_b", "cluster_primary_c"} {
		models := e.backends[name].Models()
		for _, m := range models {
			if strings.Contains(m, "target-model-") {
				primaryGotTargetModel = true
			}
		}
	}
	if !primaryGotTargetModel {
		t.Fatalf("expected at least one primary backend to receive target model")
	}

	fallbackModels := e.backends["cluster_fallback_1"].Models()
	if len(fallbackModels) != 1 || fallbackModels[0] != "fallback-model-1" {
		t.Fatalf("expected fallback model 'fallback-model-1', got %v", fallbackModels)
	}
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

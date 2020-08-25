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

package backend

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
)

// test CheckConnect, AnyStatusCode case
func TestCheckConnect_1(t *testing.T) {
	// mock backend
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	// prepare input
	backend := BfeBackend{
		AddrInfo: strings.TrimPrefix(ts.URL, "http://"),
	}
	schem := "http"
	statusCode := cluster_conf.AnyStatusCode
	uri := ""
	succNum := 1
	checkConf := cluster_conf.BackendCheck{
		Schem:      &schem,
		StatusCode: &statusCode,
		Uri:        &uri,
		SuccNum:    &succNum,
	}

	// CheckConnect
	isHealthy, err := CheckConnect(&backend, &checkConf)
	if err != nil {
		t.Errorf("should have no err: %v", err)
	}

	// check
	if !isHealthy {
		t.Errorf("backend should be healthy")
	}
}

// test CheckConnect, 200 status code case
func TestCheckConnect_2(t *testing.T) {
	// mock backend
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	// prepare input
	backend := BfeBackend{
		AddrInfo: strings.TrimPrefix(ts.URL, "http://"),
	}
	schem := "http"
	statusCode := 200
	uri := ""
	succNum := 1
	checkConf := cluster_conf.BackendCheck{
		Schem:      &schem,
		StatusCode: &statusCode,
		Uri:        &uri,
		SuccNum:    &succNum,
	}

	// CheckConnect
	isHealthy, err := CheckConnect(&backend, &checkConf)
	if err != nil {
		t.Errorf("should have no err: %v", err)
	}

	// check
	if !isHealthy {
		t.Errorf("backend should be healthy")
	}
}

// test CheckConnect, wrong status code case
func TestCheckConnect_3(t *testing.T) {
	// mock backend
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	// prepare input
	backend := BfeBackend{
		AddrInfo: strings.TrimPrefix(ts.URL, "http://"),
	}
	schem := "http"
	statusCode := 302
	uri := ""
	succNum := 1
	checkConf := cluster_conf.BackendCheck{
		Schem:      &schem,
		StatusCode: &statusCode,
		Uri:        &uri,
		SuccNum:    &succNum,
	}

	// CheckConnect
	_, err := CheckConnect(&backend, &checkConf)
	if err == nil {
		t.Errorf("should have err")
	}
}

// test CheckConnect, tcp schem
func TestCheckConnect_4(t *testing.T) {
	// mock backend
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	// prepare input
	backend := BfeBackend{
		AddrInfo: strings.TrimPrefix(ts.URL, "http://"),
	}
	schem := "tcp"
	succNum := 1
	checkConf := cluster_conf.BackendCheck{
		Schem:   &schem,
		SuccNum: &succNum,
	}

	// CheckConnect
	_, err := CheckConnect(&backend, &checkConf)
	if err != nil {
		t.Errorf("should have no err: %v", err)
	}
}

// test CheckConnect, wrong schem, processing as http schem
func TestCheckConnect_5(t *testing.T) {
	// mock backend
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	// prepare input
	backend := BfeBackend{
		AddrInfo: strings.TrimPrefix(ts.URL, "http://"),
	}
	schem := "udp"
	statusCode := 200
	uri := ""
	succNum := 1
	checkConf := cluster_conf.BackendCheck{
		Schem:      &schem,
		StatusCode: &statusCode,
		Uri:        &uri,
		SuccNum:    &succNum,
	}

	// CheckConnect
	_, err := CheckConnect(&backend, &checkConf)
	if err != nil {
		t.Errorf("should have no err: %v", err)
	}
}

// test CheckConnect, AnyStatusCode, independent health check port
func TestCheckConnect_6(t *testing.T) {
	// mock backend
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	addrInfo := strings.Split(host, ":")
	addr := addrInfo[0]

	// prepare input
	backend := BfeBackend{
		Addr:     addr,
		AddrInfo: fmt.Sprintf("%s:%d", addr, 80),
	}
	schem := "http"
	statusCode := cluster_conf.AnyStatusCode
	uri := ""
	succNum := 1

	checkConf := cluster_conf.BackendCheck{
		Schem:      &schem,
		Host:       &host,
		StatusCode: &statusCode,
		Uri:        &uri,
		SuccNum:    &succNum,
	}

	// CheckConnect
	isHealthy, err := CheckConnect(&backend, &checkConf)
	if err != nil {
		t.Errorf("should have no err: %v", err)
	}

	// check
	if !isHealthy {
		t.Errorf("backend should be healthy")
	}
}

// test CheckConnect, http AnyStatusCode, dial timeout not nil
func TestCheckConnect_7(t *testing.T) {
	// mock backend
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	// prepare input
	backend := BfeBackend{
		AddrInfo: strings.TrimPrefix(ts.URL, "http://"),
	}
	schem := "http"
	statusCode := cluster_conf.AnyStatusCode
	uri := ""
	succNum := 1
	timeout := 100

	checkConf := cluster_conf.BackendCheck{
		Schem:        &schem,
		StatusCode:   &statusCode,
		Uri:          &uri,
		SuccNum:      &succNum,
		CheckTimeout: &timeout,
	}

	// CheckConnect
	isHealthy, err := CheckConnect(&backend, &checkConf)
	if err != nil {
		t.Errorf("should have no err: %v", err)
	}

	// check
	if !isHealthy {
		t.Errorf("backend should be healthy")
	}
}

// test CheckConnect, tcp, dial timeout not nil
func TestCheckConnect_8(t *testing.T) {
	// mock backend
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	// prepare input
	backend := BfeBackend{
		AddrInfo: strings.TrimPrefix(ts.URL, "http://"),
	}
	schem := "tcp"
	succNum := 1
	timeout := 100

	checkConf := cluster_conf.BackendCheck{
		Schem:        &schem,
		SuccNum:      &succNum,
		CheckTimeout: &timeout,
	}

	// CheckConnect
	_, err := CheckConnect(&backend, &checkConf)
	if err != nil {
		t.Errorf("should have no err: %v", err)
	}
}

// test CheckConnect, 2XX case
func TestCheckConnect_9(t *testing.T) {
	// mock backend
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	// prepare input
	backend := BfeBackend{
		AddrInfo: strings.TrimPrefix(ts.URL, "http://"),
	}
	schem := "http"
	statusCode := 0x02
	uri := ""
	succNum := 1
	checkConf := cluster_conf.BackendCheck{
		Schem:      &schem,
		StatusCode: &statusCode,
		Uri:        &uri,
		SuccNum:    &succNum,
	}

	// CheckConnect
	isHealthy, err := CheckConnect(&backend, &checkConf)
	if err != nil {
		t.Errorf("should have no err: %v", err)
	}

	// check
	if !isHealthy {
		t.Errorf("backend should be healthy")
	}
}

// test CheckConnect, 2XX and 3XX case
func TestCheckConnect_10(t *testing.T) {
	// mock backend
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	// prepare input
	backend := BfeBackend{
		AddrInfo: strings.TrimPrefix(ts.URL, "http://"),
	}
	schem := "http"
	statusCode := 0x06
	uri := ""
	succNum := 1
	checkConf := cluster_conf.BackendCheck{
		Schem:      &schem,
		StatusCode: &statusCode,
		Uri:        &uri,
		SuccNum:    &succNum,
	}

	// CheckConnect
	isHealthy, err := CheckConnect(&backend, &checkConf)
	if err != nil {
		t.Errorf("should have no err: %v", err)
	}

	// check
	if !isHealthy {
		t.Errorf("backend should be healthy")
	}
}

// test CheckConnect, 302 status code case
func TestCheckConnect_11(t *testing.T) {
	// mock backend
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "xxx")
		w.WriteHeader(302)
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	// prepare input
	backend := BfeBackend{
		AddrInfo: strings.TrimPrefix(ts.URL, "http://"),
	}
	schem := "http"
	statusCode := 302
	uri := ""
	succNum := 1
	checkConf := cluster_conf.BackendCheck{
		Schem:      &schem,
		StatusCode: &statusCode,
		Uri:        &uri,
		SuccNum:    &succNum,
	}

	// CheckConnect
	isHealthy, err := CheckConnect(&backend, &checkConf)
	if err != nil {
		t.Errorf("should have no err: %v", err)
	}

	// check
	if !isHealthy {
		t.Errorf("backend should be healthy")
	}
}

// test check, AnyStatusCode, SuccNum bigger than 1
func TestCheck_1(t *testing.T) {
	// mock backend
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	// prepare input
	backend := BfeBackend{
		AddrInfo: strings.TrimPrefix(ts.URL, "http://"),
	}
	schem := "http"
	statusCode := cluster_conf.AnyStatusCode
	uri := ""
	succNum := 2
	checkInterval := 1

	checkConf := cluster_conf.BackendCheck{
		Schem:         &schem,
		StatusCode:    &statusCode,
		Uri:           &uri,
		SuccNum:       &succNum,
		CheckInterval: &checkInterval,
	}

	mockCheckConfFetcher := func(cluster string) *cluster_conf.BackendCheck {
		return &checkConf
	}
	checkConfFetcher = mockCheckConfFetcher

	// check func
	check(&backend, "")

	if backend.SuccNum() != 0 {
		t.Errorf("recover num should be 0")
	}
}

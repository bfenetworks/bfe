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
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/baidu/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_tls"
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
	isHealthy, err := CheckConnect(&backend, &checkConf, nil)
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
	isHealthy, err := CheckConnect(&backend, &checkConf, nil)
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
	_, err := CheckConnect(&backend, &checkConf, nil)
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
	_, err := CheckConnect(&backend, &checkConf, nil)
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
	_, err := CheckConnect(&backend, &checkConf, nil)
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
	isHealthy, err := CheckConnect(&backend, &checkConf, nil)
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
	isHealthy, err := CheckConnect(&backend, &checkConf, nil)
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
	_, err := CheckConnect(&backend, &checkConf, nil)
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
	isHealthy, err := CheckConnect(&backend, &checkConf, nil)
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
	isHealthy, err := CheckConnect(&backend, &checkConf, nil)
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
	isHealthy, err := CheckConnect(&backend, &checkConf, nil)
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

	mockCheckConfFetcher := func(cluster string) (*cluster_conf.BackendCheck, *cluster_conf.BackendHTTPS) {
		return &checkConf, nil
	}
	checkConfFetcher = mockCheckConfFetcher

	// check func
	check(&backend, "", nil)

	if backend.SuccNum() != 0 {
		t.Errorf("recover num should be 0")
	}
}

// test CheckConnect->checkHTTPSConnect >>>>>>>>>>>>>>>>>>>>
// test CheckConnect->checkHTTPSConnect >>>>>>>>>>>>>>>>>>>>
func TestCheckConnect_checkHTTPSConnect(t *testing.T) {
	_ = log.Init(fmt.Sprintf("test_%s", time.Now().String()), "DEBUG", "/tmp", true, "M", 5)
	type confArg struct {
		Schem           string // protocol for health check (HTTP/TCP)
		Uri             string // uri used in health check
		Host            string // if check request use special host header
		StatusCode      int    // default value is 200
		FailNum         int    // unhealthy threshold (consecutive failures of check request)
		SuccNum         int    // healthy threshold (consecutive successes of normal request)
		CheckTimeout    int    // timeout for health check, in ms
		CheckInterval   int    // interval of health check, in ms
		StatusCodeRange string // #issue-14

		//-----------------------
		addrInfo                   string
		requireAndVerifyClientCert bool
		clientInsecure             bool
		assertOk, assertNoErr      bool

		__id string // uuid for mock cluster name
	}
	type mockConf struct {
		check *cluster_conf.BackendCheck
		https *cluster_conf.BackendHTTPS
	}
	var (
		testCase = map[string]confArg{
			"https_xxx_connect_fail": {
				assertOk:                   false,
				assertNoErr:                false,
				Schem:                      "https",
				StatusCodeRange:            "1xx|20x|301|302|4xx",
				Uri:                        "/foo/bar",
				SuccNum:                    1,
				clientInsecure:             false,
				requireAndVerifyClientCert: false,
				addrInfo:                   "1.1.1.1:1111",
			},

			"https_xxx": {
				assertOk:                   true,
				assertNoErr:                true,
				Schem:                      "https",
				StatusCodeRange:            "1xx|20x|301|302|4xx",
				Uri:                        "/foo/bar",
				SuccNum:                    1,
				clientInsecure:             false,
				requireAndVerifyClientCert: false,
			},
			"https_xxx_two_way_authc": {
				assertOk:                   false,
				assertNoErr:                false,
				Schem:                      "https",
				StatusCodeRange:            "1xx|301|4xx",
				Uri:                        "/",
				SuccNum:                    1,
				clientInsecure:             false,
				requireAndVerifyClientCert: true,
			},
			"https_xxx_wrong_host": {
				assertOk:                   false,
				assertNoErr:                false,
				Schem:                      "https",
				StatusCodeRange:            "1xx|20x|301|302|4xx",
				Uri:                        "/foo/bar",
				SuccNum:                    1,
				clientInsecure:             false,
				requireAndVerifyClientCert: false,
				Host:                       "www.foobar.org",
			},
			"https_404": {
				assertOk:                   true,
				assertNoErr:                true,
				Schem:                      "https",
				StatusCode:                 404,
				Uri:                        "/foobar",
				SuccNum:                    1,
				clientInsecure:             false,
				requireAndVerifyClientCert: false,
			},

			"https_404_two_way_authc": {
				assertOk:                   true,
				assertNoErr:                true,
				Schem:                      "https",
				StatusCode:                 404,
				Uri:                        "/foobar",
				SuccNum:                    1,
				clientInsecure:             false,
				requireAndVerifyClientCert: true,
			},

			"https_200": {
				assertOk:                   true,
				assertNoErr:                true,
				Schem:                      "https",
				StatusCode:                 200,
				Uri:                        "/",
				SuccNum:                    1,
				clientInsecure:             false,
				requireAndVerifyClientCert: false,
			},
			"httos_200_two_way_authc": {
				assertOk:                   true,
				assertNoErr:                true,
				Schem:                      "https",
				StatusCode:                 200,
				Uri:                        "/",
				SuccNum:                    1,
				clientInsecure:             false,
				requireAndVerifyClientCert: true,
			},

			"tls_connect_fail": {
				assertOk:                   false,
				assertNoErr:                false,
				Schem:                      "tls",
				clientInsecure:             false,
				requireAndVerifyClientCert: true,
				addrInfo:                   "1.1.1.1:1111",
			},

			"tls": {
				assertOk:                   true,
				assertNoErr:                true,
				Schem:                      "tls",
				clientInsecure:             true,
				requireAndVerifyClientCert: false,
			},

			"tls_two_way_authc": {
				assertOk:                   true,
				assertNoErr:                true,
				Schem:                      "tls",
				clientInsecure:             false,
				requireAndVerifyClientCert: true,
			},
		}
	)

	var (
		confMap     = make(map[string]mockConf)
		confMapLock = new(sync.RWMutex)
		serverHost  = "foobar.org"
		//serverTimeout = 3 * time.Second
		cert, _ = tls.X509KeyPair(bfe_tls.I_CN_FOOBAR_ORG_CRT.Bytes(), bfe_tls.I_CN_FOOBAR_ORG_PRV.Bytes())
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.String() != "/" {
				w.WriteHeader(404)
			} else {
				w.WriteHeader(200)
				w.Header().Set("Content-Type", "text/plain")
				w.Header().Set("Host", serverHost)
				_, _ = w.Write([]byte("Hello BFE https"))
			}
		})

		clusterNameFn = func() string {
			rand.Seed(time.Now().UnixNano())
			return hex.EncodeToString(big.NewInt(time.Now().UnixNano() + rand.Int63()).Bytes())
		}
		casFn = func() *x509.CertPool {
			cas := x509.NewCertPool()
			cas.AppendCertsFromPEM(bfe_tls.BFE_R_CA_CRT.Bytes())
			cas.AppendCertsFromPEM(bfe_tls.BFE_I_CA_CRT.Bytes())
			return cas
		}
		cliCertFn = func() *bfe_tls.Certificate {
			ccert, _ := bfe_tls.X509KeyPair(bfe_tls.I_BFE_DEV_CRT.Bytes(), bfe_tls.I_BFE_DEV_PRV.Bytes())
			return &ccert
		}
		checkConfFn = func(c confArg) (*cluster_conf.BackendCheck, *cluster_conf.BackendHTTPS) {
			var (
				cas       = casFn()
				httpsConf = new(cluster_conf.BackendHTTPS)
				checkConf = &cluster_conf.BackendCheck{
					Schem:           &c.Schem,
					StatusCode:      &c.StatusCode,
					Uri:             &c.Uri,
					SuccNum:         &c.SuccNum,
					StatusCodeRange: &c.StatusCodeRange,
					CheckTimeout:    &c.CheckTimeout,
					Host:            &c.Host,
				}
				// for httpsConf >>>>
				rscalist = []string{}
				insecure = c.clientInsecure
			)
			if c.requireAndVerifyClientCert {
				// client >>>>
				httpsConf.SetBFECert(cliCertFn())
				insecure = false
				// client <<<<
			}
			httpsConf.SetRSCAList(cas)
			httpsConf.RSHost = &c.Host
			httpsConf.RSCAList = &rscalist
			httpsConf.RSInsecureSkipVerify = &insecure
			confMapLock.Lock()
			confMap[c.__id] = mockConf{checkConf, httpsConf}
			confMapLock.Unlock()
			return checkConf, httpsConf
		}

		buildConfAndStartServerFn = func(ctx context.Context, c confArg) *BfeBackend {
			var (
				cas     = casFn()
				backend = &BfeBackend{}
				// 创建TLS配置
				tlsConfig = &tls.Config{
					Certificates: []tls.Certificate{cert},
					ClientAuth:   tls.NoClientCert, // default
				}
			)
			// Two-way authc
			if c.requireAndVerifyClientCert {
				// server
				tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
				tlsConfig.ClientCAs = cas
				tlsConfig.RootCAs = cas
			}

			listener, err := net.Listen("tcp", ":0")
			if err != nil {
				t.Errorf("failed to listen: %v", err)
			}

			server := &http.Server{
				TLSConfig: tlsConfig,
				Handler:   handler,
			}
			go func() {
				if err := server.ServeTLS(listener, "", ""); err != nil && err != http.ErrServerClosed {
					t.Errorf("HTTPS test server fail : %v", err)
				}
			}()
			time.Sleep(100 * time.Millisecond)
			port := listener.Addr().(*net.TCPAddr).Port
			backend.AddrInfo = fmt.Sprintf("127.0.0.1:%d", port)
			if c.addrInfo != "" {
				backend.AddrInfo = c.addrInfo
			}
			// 等待ctx取消信号
			go func() {
				<-ctx.Done()
				if err := server.Close(); err != nil {
					t.Errorf("HTTPS test server Close failed: %v", err)
				}
			}()
			time.Sleep(100 * time.Millisecond)
			return backend
		}

		startlinkAndWaitRtnFn = func(args confArg) func(t *testing.T) {
			args.__id = clusterNameFn()
			if args.Host == "" {
				args.Host = serverHost
			}
			if args.CheckTimeout <= 0 {
				args.CheckTimeout = 2000 // ms
			}
			log.Logger.Debug("cid=%s, test-host=%s", args.__id, args.Host)
			return func(t *testing.T) {
				defer func() {
				}()
				var rtnCh = make(chan checkRtn, 1)
				conf, httpsConf := checkConfFn(args)
				ctx, cancelFn := context.WithCancel(context.Background())
				go func(ctx context.Context, conf *cluster_conf.BackendCheck, httpsConf *cluster_conf.BackendHTTPS) {
					backend := buildConfAndStartServerFn(ctx, args)

					isHealthy, err := CheckConnect(backend, conf, httpsConf)
					rtnCh <- checkRtn{ok: isHealthy, err: err}
				}(ctx, conf, httpsConf)
				select {
				case rtn := <-rtnCh:
					defer cancelFn()
					if rtn.err != nil {
						if args.assertNoErr {
							t.Errorf("assertNoErr=%v, err=%v", args.assertNoErr, rtn.err)
						} else {
							t.Logf("assertNoErr=%v, err=%v", args.assertNoErr, rtn.err)
						}
					}
					if !rtn.ok {
						if args.assertOk {
							t.Errorf("backend assertOk=%v, rtnOk=%v", args.assertOk, rtn.ok)
						} else {
							t.Logf("backend assertOk=%v, rtnOk=%v", args.assertOk, rtn.ok)
						}
					}
				}
			}
		}
	)

	SetCheckConfFetcher(func(name string) (*cluster_conf.BackendCheck, *cluster_conf.BackendHTTPS) {
		confMapLock.Lock()
		defer confMapLock.Unlock()
		cnf := confMap[name]
		return cnf.check, cnf.https
	})
	for k, v := range testCase {
		t.Run(k, startlinkAndWaitRtnFn(v))
	}
	time.Sleep(1 * time.Second)
}

// test CheckConnect->checkHTTPSConnect <<<<<<<<<<<<<<<<<<<<
// test CheckConnect->checkHTTPSConnect <<<<<<<<<<<<<<<<<<<<

func TestExtractIP(t *testing.T) {
	rsAddr1 := "192.168.1.1:8888"
	rsAddr2 := "[fe80::d450:2dc5:d]:8888"

	ip1 := extractIP(rsAddr1)
	if ip1 != "192.168.1.1" {
		t.Error("expect: 192.168.1.1, got :", ip1)
	}

	ip2 := extractIP(rsAddr2)
	if ip2 != "[fe80::d450:2dc5:d]" {
		t.Error("expect: [fe80::d450:2dc5:d], got :", ip2)
	}
}

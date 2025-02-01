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

// health check for backend

package backend

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/baidu/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_debug"
	"github.com/bfenetworks/bfe/bfe_tls"
)

type checkRtn struct {
	ok  bool
	err error
}

func UpdateStatus(backend *BfeBackend, cluster string) bool {
	var (
		checkConf *cluster_conf.BackendCheck
		httpsConf *cluster_conf.BackendHTTPS
	)
	// get conf of health check, which is separately stored for each cluster
	checkConf, httpsConf = getCheckConf(cluster)
	if checkConf == nil {
		// just ignore if not found health check conf
		return false
	}

	// UpdateStatus update backend status.
	// if backend's status become fail, start healthcheck.
	// at most start 1 check goroutine for each backend.
	if backend.UpdateStatus(*checkConf.FailNum) {
		go check(backend, cluster, httpsConf)
		return true
	}

	return false
}

func check(backend *BfeBackend, cluster string, httpsConf *cluster_conf.BackendHTTPS) {

	log.Logger.Info("start healthcheck for %s", backend.Name)

	// backend close chan
	c := backend.CloseChan()

loop:
	for {
		select {
		case <-c: // backend deleted
			break loop
		default:
		}

		// get the latest conf to do health check
		checkConf, _ := getCheckConf(cluster)
		if checkConf == nil {
			// never come here
			time.Sleep(time.Second)
			continue
		}
		checkInterval := time.Duration(*checkConf.CheckInterval) * time.Millisecond

		// health check
		if ok, err := CheckConnect(backend, checkConf, httpsConf); !ok {
			backend.ResetSuccNum()
			if bfe_debug.DebugHealthCheck {
				log.Logger.Debug("backend %s still not avail (check failure: %s)", backend.Name, err)
			}
			time.Sleep(checkInterval)
			continue
		}

		// check whether backend becomes available
		backend.AddSuccNum()
		if !backend.CheckAvail(*checkConf.SuccNum) {
			if bfe_debug.DebugHealthCheck {
				log.Logger.Debug("backend %s still not avail (check success, waiting for more checks)", backend.Name)
			}
			time.Sleep(checkInterval)
			continue
		}

		log.Logger.Info("backend %s back to Normal", backend.Name)
		backend.SetRestart(true)
		backend.SetAvail(true)
		break loop
	}
}

func getHealthCheckAddrInfo(backend *BfeBackend, checkConf *cluster_conf.BackendCheck) string {
	if checkConf.Host != nil {
		// if port for health check is configured, use it instead of backend port
		hostInfo := strings.Split(*checkConf.Host, ":")
		if len(hostInfo) == 2 {
			port := hostInfo[1]
			return fmt.Sprintf("%s:%s", backend.GetAddr(), port)
		}
	}
	return backend.GetAddrInfo()
}

func checkTCPConnect(backend *BfeBackend, checkConf *cluster_conf.BackendCheck) (bool, error) {
	addrInfo := getHealthCheckAddrInfo(backend, checkConf)

	var conn net.Conn
	var err error
	if checkConf.CheckTimeout != nil {
		conn, err = net.DialTimeout("tcp", addrInfo,
			time.Duration(*checkConf.CheckTimeout)*time.Millisecond)
	} else {
		conn, err = net.Dial("tcp", addrInfo)
	}

	if err != nil {
		return false, err
	}
	conn.Close()
	return true, nil
}

func doHTTPHealthCheck(request *http.Request, timeout time.Duration) (int, error) {
	client := &http.Client{
		// Note: disable following an HTTP redirect
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		// Note: timeout of zero means no timeout
		Timeout: timeout,
	}

	response, err := client.Do(request)
	if err != nil {
		return -1, err
	}
	defer response.Body.Close()

	return response.StatusCode, nil
}

// extractIP extract ip address
func extractIP(rsAddr string) string {
	if strings.HasPrefix(rsAddr, "[") {
		// IPv6
		endIndex := strings.LastIndex(rsAddr, "]")
		if endIndex == -1 {
			return ""
		}
		ip := rsAddr[:endIndex+1]
		if net.ParseIP(ip[1:endIndex]) == nil {
			return ""
		}
		return ip
	} else {
		// IPv4
		ip := strings.Split(rsAddr, ":")[0]
		if net.ParseIP(ip) == nil {
			return ""
		}
		return ip
	}
}

func getHostByType(host, rsAddr, hostType *string, def string) string {
	if hostType == nil {
		ht := cluster_conf.HostType_HOST
		hostType = &ht
	}
	switch *hostType {
	case cluster_conf.HostType_Instance_IP:
		if rsAddr != nil {
			return extractIP(*rsAddr)
		}
	default:
		if host != nil {
			return *host
		}
	}
	return def
}

func checkHTTPSConnect(backend *BfeBackend, checkConf *cluster_conf.BackendCheck, httpsConf *cluster_conf.BackendHTTPS) (bool, error) {
	var (
		err          error
		conn         net.Conn
		addrInfo     = getHealthCheckAddrInfo(backend, checkConf)
		checkTimeout = 30 * time.Second
		statusCode   = 0
		host         string
		rootCAs      *x509.CertPool        = nil
		certs        []bfe_tls.Certificate = nil
		cert         bfe_tls.Certificate
		insecure     = false
		uri          = "/"
		checkRtnCh   = make(chan checkRtn, 1)
		rtn          checkRtn
	)

	var (
		getStatusCodeFn = func(statusLine string) (int, error) {
			// "HTTP/1.1 200 OK"
			re, err := regexp.Compile(`\s(\d{3})\s`)
			if err != nil {
				return 0, err
			}
			matches := re.FindStringSubmatch(statusLine)
			if len(matches) == 2 {
				statusCode := matches[1]
				log.Logger.Debug("StatusCode = %s, raw = %s", statusCode, statusLine)
				return strconv.Atoi(statusCode)
			} else {
				return 0, fmt.Errorf("Status code not found: %s", statusLine)
			}
		}

		doCheckFn = func(conn net.Conn) checkRtn {
			// Set timeout
			timeout := 3 * time.Second
			err = conn.SetDeadline(time.Now().Add(timeout))
			if err != nil {
				return checkRtn{false, err}
			}

			// TLS Check
			if err = conn.(*bfe_tls.Conn).Handshake(); err != nil {
				log.Logger.Debug("debug_https err=%s", err.Error())
				return checkRtn{false, err}
			}
			if *checkConf.Schem == "tls" {
				return checkRtn{true, nil}
			}

			// HTTPS Check
			if checkConf.Uri != nil && *checkConf.Uri != "" {
				uri = *checkConf.Uri
			}
			request := fmt.Sprintf("GET %s HTTP/1.1\r\n"+
				"Host: %s\r\n"+
				"User-Agent: BFE-Health-Check\r\n"+
				"\r\n", uri, host)
			_, err = conn.Write([]byte(request))
			if err != nil {
				log.Logger.Debug("debug_https err=%s", err.Error())
				return checkRtn{false, err}
			}
			var (
				response = ""
				ok       bool
				err      error
				data     = make([]byte, 0)
				bufSz    = 128
				buf      = make([]byte, bufSz)
				total    = 0
			)

			for {
				total, err = conn.Read(buf)
				if err != nil {
					break
				}
				data = append(data, buf[:total]...)
				if total < bufSz {
					break
				}
			}

			if err != nil {
				log.Logger.Debug("debug_https err=%s", err.Error())
				return checkRtn{false, err}
			}
			response = string(data)
			log.Logger.Debug("<- Request:\n%s", request)
			log.Logger.Debug("-> Response:\n%s", response)
			if checkConf.StatusCode != nil { // check status code
				var (
					s   string
					arr = strings.Split(response, "\n")
				)
				if len(arr) > 0 {
					s = strings.ToUpper(arr[0])
					statusCode, err = getStatusCodeFn(s)
					if err != nil {
						return checkRtn{false, err}
					}
					if checkConf.StatusCodeRange != nil && *checkConf.StatusCodeRange != "" {
						log.Logger.Debug("statusCode=%d, statusCodeRange=%s", statusCode, *checkConf.StatusCodeRange)
						ok, err := cluster_conf.MatchStatusCodeRange(fmt.Sprintf("%d", statusCode), *checkConf.StatusCodeRange)
						return checkRtn{ok, err}
					}
				}
				ok, err = cluster_conf.MatchStatusCode(statusCode, *checkConf.StatusCode)
			}
			return checkRtn{ok, err}
		}

		toStringFn = func(o interface{}) string {
			b, err := json.Marshal(o)
			if err != nil {
				return err.Error()
			}
			return string(b)
		}
	)

	if checkConf.CheckTimeout != nil {
		checkTimeout = time.Duration(*checkConf.CheckTimeout) * time.Millisecond
	}
	conn, err = net.DialTimeout("tcp", addrInfo, checkTimeout)

	if err != nil {
		log.Logger.Debug("debug_https err=%v", err)
		return false, err
	}

	defer func() {
		if r := recover(); r != nil {
			log.Logger.Debug("recover_panic = %v", r)
		}
		_ = conn.Close()
	}()

	_, err = url.Parse(fmt.Sprintf("%s://%s%s", "https", addrInfo, *checkConf.Uri))
	if err != nil {
		log.Logger.Debug("debug_https err=%v", err)
		return false, err
	}

	serverName := ""
	if httpsConf.RSHost != nil {
		serverName = *httpsConf.RSHost
	} else if checkConf.Host != nil {
		serverName = *checkConf.Host
	}
	host = getHostByType(checkConf.Host, &addrInfo, checkConf.HostType, serverName)

	rootCAs, err = httpsConf.GetRSCAList()

	if cert, err = httpsConf.GetBFECert(); err == nil {
		certs = []bfe_tls.Certificate{cert}
	}

	if httpsConf.RSInsecureSkipVerify != nil {
		insecure = *httpsConf.RSInsecureSkipVerify
	}

	conn = bfe_tls.Client(conn, &bfe_tls.Config{
		Certificates:          certs,
		InsecureSkipVerify:    true,
		ServerName:            host,
		RootCAs:               rootCAs,
		VerifyPeerCertificate: bfe_tls.NewVerifyPeerCertHooks(insecure, host, rootCAs).Ready(),
	})

	log.Logger.Debug("httpsCheck conf=%s", toStringFn(checkConf))
	go func(conn net.Conn, rtnCh chan checkRtn) {
		rtnCh <- doCheckFn(conn)
	}(conn, checkRtnCh)

	if checkTimeout > 0 {
		select {
		case rtn = <-checkRtnCh:
			return rtn.ok, rtn.err
		case <-time.Tick(checkTimeout):
			return false, fmt.Errorf("https checkTimeout %dms", checkTimeout/time.Millisecond)
		}
	} else {
		rtn = <-checkRtnCh
	}
	return rtn.ok, rtn.err
}

func checkHTTPConnect(backend *BfeBackend, checkConf *cluster_conf.BackendCheck) (bool, error) {
	// prepare health check request
	addrInfo := getHealthCheckAddrInfo(backend, checkConf)
	urlStr := fmt.Sprintf("%s://%s%s", "http", addrInfo, *checkConf.Uri)
	request, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return false, err
	}

	// modify http host header if needed
	if checkConf.Host != nil {
		request.Host = *checkConf.Host
	}

	// add headers required by downstream servers
	request.Header.Set("Accept", "*/*")

	// do http health check
	checkTimeout := time.Duration(0)
	if checkConf.CheckTimeout != nil {
		checkTimeout = time.Duration(*checkConf.CheckTimeout) * time.Millisecond
	}

	statusCode, err := doHTTPHealthCheck(request, checkTimeout)
	if err != nil {
		return false, err
	}

	return cluster_conf.MatchStatusCode(statusCode, *checkConf.StatusCode)
}

// CheckConnect checks whether backend server become available.
func CheckConnect(backend *BfeBackend, checkConf *cluster_conf.BackendCheck, httpsConf *cluster_conf.BackendHTTPS) (bool, error) {
	switch *checkConf.Schem {
	case "http":
		return checkHTTPConnect(backend, checkConf)
	case "tcp":
		return checkTCPConnect(backend, checkConf)
	case "https", "tls":
		return checkHTTPSConnect(backend, checkConf, httpsConf)
	default:
		// never come here
		return checkHTTPConnect(backend, checkConf)
	}
}

// CheckConfFetcher returns current health check conf for cluster.
type CheckConfFetcher func(name string) (*cluster_conf.BackendCheck, *cluster_conf.BackendHTTPS)

var checkConfFetcher CheckConfFetcher

func getCheckConf(cluster string) (*cluster_conf.BackendCheck, *cluster_conf.BackendHTTPS) {
	if checkConfFetcher == nil {
		return nil, nil
	}
	return checkConfFetcher(cluster)
}

// SetCheckConfFetcher initializes CheckConfFetcher handler.
func SetCheckConfFetcher(confFetcher CheckConfFetcher) {
	checkConfFetcher = confFetcher
}

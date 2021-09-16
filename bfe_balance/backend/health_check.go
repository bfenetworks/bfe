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
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_debug"
)

func UpdateStatus(backend *BfeBackend, cluster string) bool {
	// get conf of health check, which is separately stored for each cluster
	checkConf := getCheckConf(cluster)
	if checkConf == nil {
		// just ignore if not found health check conf
		return false
	}

	// UpdateStatus update backend status.
	// if backend's status become fail, start healthcheck.
	// at most start 1 check goroutine for each backend.
	if backend.UpdateStatus(*checkConf.FailNum) {
		go check(backend, cluster)
		return true
	}

	return false
}

func check(backend *BfeBackend, cluster string) {
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
		checkConf := getCheckConf(cluster)
		if checkConf == nil {
			// never come here
			time.Sleep(time.Second)
			continue
		}
		checkInterval := time.Duration(*checkConf.CheckInterval) * time.Millisecond

		// health check
		if ok, err := CheckConnect(backend, checkConf); !ok {
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
func CheckConnect(backend *BfeBackend, checkConf *cluster_conf.BackendCheck) (bool, error) {
	switch *checkConf.Schem {
	case "http":
		return checkHTTPConnect(backend, checkConf)
	case "tcp":
		return checkTCPConnect(backend, checkConf)
	default:
		// never come here
		return checkHTTPConnect(backend, checkConf)
	}
}

// CheckConfFetcher returns current health check conf for cluster.
type CheckConfFetcher func(cluster string) *cluster_conf.BackendCheck

var checkConfFetcher CheckConfFetcher

func getCheckConf(cluster string) *cluster_conf.BackendCheck {
	if checkConfFetcher == nil {
		return nil
	}
	return checkConfFetcher(cluster)
}

// SetCheckConfFetcher initializes CheckConfFetcher handler.
func SetCheckConfFetcher(confFetcher CheckConfFetcher) {
	checkConfFetcher = confFetcher
}

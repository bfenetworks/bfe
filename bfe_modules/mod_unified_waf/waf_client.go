// Copyright (c) 2025 The BFE Authors.
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

package mod_unified_waf

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/baidu/go-lib/gotrack"
	"github.com/baidu/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_modules/mod_unified_waf/waf_impl"
	"github.com/bfenetworks/bwi/bwi"
)

var (
	ERR_WAF_FORBIDDEN = errors.New("FORBIDDEN_BY_WAF") // request forbidden by waf
)

type WafDetectResult struct {
	Result bwi.WafResult
	Error  error
}

func (obj *WafDetectResult) getWafEventId() string {
	if obj.Result == nil {
		return ""
	}
	return obj.Result.GetEventId()
}

func (obj *WafDetectResult) Passed() bool {
	return obj.Result != nil && obj.Result.GetResultFlag() == bwi.WAF_RESULT_PASS
}

func (obj *WafDetectResult) Blocked() bool {
	return obj.Result != nil && obj.Result.GetResultFlag() == bwi.WAF_RESULT_BLOCK
}

type HealthCheckerConf struct {
	UnavailableFailedThres int64 //unavailable failed threshold
	HealthCheckInterval    int64 //health check interval(ms)
}

type WafClient struct {
	wafEntries    *waf_impl.WafImplMethodBundle
	client        bwi.WafServer // chang-ting sdk client
	serverAddress string        // waf server instance address
	serverIP      string
	hcPort        atomic.Uint32

	connectTimeout time.Duration // connection timeout
	// reqTimeout     time.Duration  // detection timeout for a request
	// retryMax       int // detection retry for a request
	globalWafParam *GlobalParam

	concurrency     int      // how many concurrency goroutine call waf-server
	concurrencyChan chan int // concurrency pool

	monitor *MonitorStates

	refCount int  // reference counter
	toDelete bool // if toDelete set true, it indicates current waf client is going to be deleted

	lock sync.RWMutex

	errCounter   atomic.Int64 // gosnserver err counter
	available    atomic.Bool  // if errCounter >= AVAILABLE_THRESHOLD, let available = false
	maxWaitCount atomic.Int64
	curWaitCount atomic.Int64

	HCConf HealthCheckerConf

	exitCh chan struct{}
}

type Peeker interface {
	Peek(n int) ([]byte, error)
}

func NewWafClient(wafEntries *waf_impl.WafImplMethodBundle, addr string, instConf *WafInstance, wafParam *GlobalParam, poolSize int, m *MonitorStates) (*WafClient, error) {
	connectTimeout := time.Duration(wafParam.WafClient.ConnectTimeout * int(time.Millisecond))

	c := new(WafClient)
	c.monitor = m
	c.serverAddress = addr
	c.serverIP = instConf.IpAddr
	c.UpdateInstanceConf(instConf)

	c.connectTimeout = connectTimeout
	c.wafEntries = wafEntries
	c.client = c.wafEntries.NewWafServerWithPoolSize(func() (net.Conn, error) {
		conn, err := net.DialTimeout("tcp", c.serverAddress, c.connectTimeout)
		if err != nil {
			c.monitor.state.Inc(bfe_basic.NET_ERR, 1)
		}
		return conn, err
	}, poolSize)

	c.globalWafParam = wafParam

	c.concurrency = wafParam.WafClient.Concurrency
	c.concurrencyChan = make(chan int, c.concurrency)
	for i := 0; i < c.concurrency; i++ {
		c.concurrencyChan <- 1
	}
	log.Logger.Info("Set waf client: %s concurrency = %d", c.serverAddress, c.concurrency)

	c.errCounter.Store(0)
	c.available.Store(true)

	c.updateHCConf(&wafParam.HealthChecker)

	c.curWaitCount.Store(0)
	c.maxWaitCount.Store(int64(wafParam.WafClient.MaxWaitCount))

	c.monitor.state.Set("waf_client_available_"+c.serverAddress, "true")

	c.exitCh = make(chan struct{}, 1)
	go c.checkWafServer() // start health check task

	return c, nil
}

func (c *WafClient) Detect(req *bfe_basic.Request, wafReq *http.Request, param *WafParam) (bool, string) {
	c.monitor.state.Inc("waf_client_detect_"+c.serverAddress, 1)

	reqTimeout, retryMax := c.GetDetectParam(wafReq.ContentLength)
	var startTime, endTime, finTime time.Time
	startTime = time.Now()
	finTime = startTime.Add(reqTimeout)

	if c.curWaitCount.Load() > c.maxWaitCount.Load() {
		c.monitor.state.Inc(bfe_basic.REQ_NO_CHECK, 1)
		c.monitor.state.Inc("waf_client_detect_closed_skip_"+c.serverAddress, 1)
		setWafStatus(req, (int)(bfe_basic.WAF_NO_CHECK))
		if openDebug {
			log.Logger.Debug("waf instance is closed, but skip, instance = %s, logid = %s",
				c.serverAddress, req.LogId)
		}
		return false, ""
	}

	isGetToken := c.getToken(req, reqTimeout, req.LogId)
	endTime = time.Now()
	if isGetToken {
		c.monitor.delayCallComp.AddBySub(startTime, endTime)
	} else {
		c.monitor.delayCallComp.AddBySub(startTime, endTime)
		c.monitor.state.Inc(bfe_basic.REQ_TIMEOUT, 1)
		c.monitor.state.Inc("waf_client_detect_concurrency_timout_"+c.serverAddress, 1)
		setWafStatus(req, (int)(bfe_basic.WAF_TIMEOUT))
		setWafSpentTime(req, startTime, endTime)

		if openDebug {
			log.Logger.Debug("time out for concurrency control, logid = %s, start = %d, end = %d",
				req.LogId, startTime.UnixNano(), endTime.UnixNano())
		}
		return false, ""
	}

	// get remaining time
	diff := finTime.Sub(endTime)
	if diff <= 0 {
		diff = time.Duration(1 * time.Millisecond)
	}

	leftTimer := time.NewTicker(diff)
	defer leftTimer.Stop()

	// call waf server
	done := make(chan *WafDetectResult, 1)
	go c.detect(wafReq, done, retryMax, req.LogId)

	// wait result
	select {
	case res := <-done:
		if res.Error != nil {
			endTime = time.Now()
			c.monitor.delay.AddBySub(startTime, endTime)
			c.monitor.state.Inc(bfe_basic.REQ_OTHER, 1)
			c.monitor.state.Inc("waf_client_detect_other_"+c.serverAddress, 1)
			setWafSpentTime(req, startTime, endTime)
			setWafStatus(req, int(bfe_basic.WAF_ERROR))

			// pass, go on
			log.Logger.Warn("waf-server detect pass with error: %s, logid = %s, start = %d, end = %d",
				res.Error.Error(), req.LogId, startTime.UnixNano(), endTime.UnixNano())

			return false, ""
		}

		if res.Blocked() {
			endTime = time.Now()
			c.monitor.delay.AddBySub(startTime, endTime)
			c.monitor.state.Inc(bfe_basic.REQ_FORBIDDEN, 1)
			c.monitor.state.Inc("waf_client_detect_forbidden_"+c.serverAddress, 1)
			setWafSpentTime(req, startTime, endTime)
			setWafStatus(req, int(bfe_basic.WAF_FORBIDDEN))
			setWafRuleName(req, "-")
			req.ErrCode = ERR_WAF_FORBIDDEN

			if openDebug {
				log.Logger.Debug("waf-server detect block, logid = %s, start = %d, end = %d",
					req.LogId, startTime.UnixNano(), endTime.UnixNano())
			}
			return true, res.getWafEventId()
		}

		// res.Result.Passed
		endTime = time.Now()
		c.monitor.delay.AddBySub(startTime, endTime)
		c.monitor.state.Inc(bfe_basic.REQ_OK, 1)
		c.monitor.state.Inc("waf_client_detect_ok_"+c.serverAddress, 1)
		setWafSpentTime(req, startTime, endTime)
		setWafStatus(req, int(bfe_basic.WAF_PASS))

		// pass, go on
		if openDebug {
			log.Logger.Debug("waf-server detect pass, logid = %s, start = %d, end = %d",
				req.LogId, startTime.UnixNano(), endTime.UnixNano())
		}
		return false, ""

	case <-leftTimer.C: // use time.Ticker instead of time.After()
		endTime = time.Now()
		c.monitor.delay.AddBySub(startTime, endTime)
		c.monitor.state.Inc(bfe_basic.REQ_TIMEOUT, 1)
		c.monitor.state.Inc("waf_client_detect_timeout_"+c.serverAddress, 1)
		setWafStatus(req, (int)(bfe_basic.WAF_TIMEOUT))
		setWafSpentTime(req, startTime, endTime)

		if openDebug {
			log.Logger.Debug("time out for waiting waf-server, logid = %s, start = %d, end = %d",
				req.LogId, startTime.UnixNano(), endTime.UnixNano())
		}
	}

	return false, ""
}

func (c *WafClient) getToken(req *bfe_basic.Request, reqTimeout time.Duration, logId string) bool {
	// concurrency control: concurrencyChan is used as a pool
	ok := false

	c.curWaitCount.Add(1)
	defer c.curWaitCount.Add(-1)

	ticker := time.NewTicker(reqTimeout)
	defer ticker.Stop()

	select {
	case <-c.concurrencyChan:
		ok = true
	case <-ticker.C: // use time.Ticker instead of time.After()
		ok = false
	}

	if ok && openDebug {
		log.Logger.Debug("get concurrencyChan, logid = %s", logId)
	}

	return ok
}

func (c *WafClient) getHcServerStr() string {
	addr := fmt.Sprintf("%s:%d", c.serverIP, c.hcPort.Load())
	return addr
}

func (c *WafClient) detect(req *http.Request, done chan *WafDetectResult, retryMax int, logId string) {
	var res bwi.WafResult
	var err error
	defer func() {
		// release
		c.concurrencyChan <- 1
		if openDebug {
			log.Logger.Debug("release concurrencyChan, logid = %s", logId)
		}
		if err := recover(); err != nil {
			log.Logger.Warn("waf client detect panic, logid = %s. err:%v\n%s", logId, err, gotrack.CurrentStackTrace(0))
		}
	}()

	//set circuit status in use side
	maxRunCount := retryMax + 1
	for i := 0; i < maxRunCount; i++ {
		res, err = c.client.DetectRequest(req, logId)
		if err == nil {
			c.instanceHeathJudge(true, false)
			break
		}

		if openDebug {
			log.Logger.Debug("c.client.Detect(dc) failed: %s, retry = %d, logid = %s", err.Error(), i, logId)
		}
		c.instanceHeathJudge(false, false)
	}

	// send result
	done <- &WafDetectResult{Result: res, Error: err}
	if openDebug {
		log.Logger.Debug("waf detect done, logid = %s", logId)
	}
}

func (c *WafClient) IsAvailable() bool {
	return c.available.Load()
}

func (c *WafClient) instanceHeathJudge(isOpSucc bool, isHcOp bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if isOpSucc {
		c.errCounter.Store(0)
		c.available.Store(true)
	} else {
		c.errCounter.Add(1)
		// set only once

		if c.errCounter.Load() > atomic.LoadInt64(&c.HCConf.UnavailableFailedThres) && c.available.Load() {
			c.available.Store(false)
			log.Logger.Info("Waf client: %s available set to false", c.serverAddress)
		}
	}

	if c.available.Load() {
		c.monitor.state.Set("waf_client_available_"+c.serverAddress, "true")
	} else {
		c.monitor.state.Set("waf_client_available_"+c.serverAddress, "false")
	}
}

// check waf server health
func doCheck(wafEntries *waf_impl.WafImplMethodBundle, addr string) bool {
	defer func() {
		if err := recover(); err != nil {
			log.Logger.Warn("waf client:%s doCheck: panic serving :%v\n%s",
				addr, err, gotrack.CurrentStackTrace(0))
		}
	}()

	log.Logger.Info("doCheck(): start check: %s", addr)

	// connect to waf server
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		log.Logger.Info("doCheck(): DialTimeout(): %s", err)
		return false
	}

	// using DoHeartbeat() as headlth checking
	err = wafEntries.HealthCheck(conn)
	if err != nil {
		log.Logger.Info("doCheck(): DoHeartbeat(): %s", err)

		err := conn.Close()
		if err != nil {
			log.Logger.Warn("doCheck(): Heart beat conn.Close(): %s", err)
		}
		return false
	}

	// if conn close failed, still has some problems.
	err = conn.Close()
	if err != nil {
		log.Logger.Warn("doCheck(): Heart beat conn.Close(): %s", err)
		return false
	}

	return true
}

func (c *WafClient) checkWafServer() {
	keySuccess := fmt.Sprintf("waf_client_check_success_%s", c.serverAddress)
	keyFailed := fmt.Sprintf("waf_client_check_failed_%s", c.serverAddress)

	for {
		interval := time.Duration(atomic.LoadInt64(&c.HCConf.HealthCheckInterval)) * time.Millisecond
		select {
		// check waf server every second
		case <-time.After(interval):
			success := doCheck(c.wafEntries, c.getHcServerStr())
			//success := true
			c.instanceHeathJudge(success, true)

			if success {
				log.Logger.Debug("checkWafServer(): %s doCheck() success", c.serverAddress)
				c.monitor.state.Inc(keySuccess, 1)
			} else {
				log.Logger.Info("checkWafServer(): %s doCheck() failed", c.serverAddress)
				c.monitor.state.Inc(keyFailed, 1)
			}
		case <-c.exitCh:
			log.Logger.Info("checkWafServer(): %s get exit signal", c.serverAddress)
			return
		}
	}
}

// generate request for remote call
func generateHeaders(headers bfe_http.Header) http.Header {
	newHeaders := http.Header{}
	for k, v := range headers {
		newHeaders[k] = v
	}

	return newHeaders
}

// body data with http method: POST/PUT/PATCH will be checked
func checkBodyWithHttpMethod(method string) bool {
	switch method {
	case http.MethodPost:
		return true
	case http.MethodPatch:
		return true
	case http.MethodPut:
		return true
	}

	return false
}

// set waf spent time
func setWafSpentTime(req *bfe_basic.Request, start time.Time, end time.Time) {
	info := bfe_basic.GetWafInfo(req)
	info.WafSpentTime = end.Sub(start).Nanoseconds() / 1000000
}

// set waf status
func setWafStatus(req *bfe_basic.Request, status int) {
	info := bfe_basic.GetWafInfo(req)
	info.WafStatus = status
}

// set waf rule
func setWafRuleName(req *bfe_basic.Request, ruleName string) {
	info := bfe_basic.GetWafInfo(req)
	info.WafRuleName = ruleName
}

func (c *WafClient) WafServerAddress() string {
	return c.serverAddress
}

func (c *WafClient) UpdateInstanceConf(instConf *WafInstance) {
	c.lock.Lock()
	c.hcPort.Store(uint32(instConf.HealthCheckPort))
	c.lock.Unlock()
}

func (c *WafClient) UpdateWafGlobalParam(wafGlobalParam *GlobalParam) {
	c.lock.Lock()
	c.globalWafParam = wafGlobalParam
	c.lock.Unlock()

	t := time.Duration(wafGlobalParam.WafClient.ConnectTimeout * int(time.Millisecond))
	c.updateConnTimeout(t, int64(wafGlobalParam.WafClient.MaxWaitCount))

	c.updateHCConf(&wafGlobalParam.HealthChecker)
}

func (c *WafClient) updateConnTimeout(timeout time.Duration, maxWaitCount int64) {
	c.lock.Lock()
	if c.connectTimeout != timeout {
		c.connectTimeout = timeout

		// reset socket factory
		c.client.UpdateSockFactory(func() (net.Conn, error) {
			conn, err := net.DialTimeout("tcp", c.serverAddress, c.connectTimeout)
			if err != nil {
				c.monitor.state.Inc(bfe_basic.NET_ERR, 1)
			}
			return conn, err
		})
	}
	c.lock.Unlock()

	c.maxWaitCount.Store(maxWaitCount)
}

func (c *WafClient) updateHCConf(hcconff *HealthCheckerConf) {
	c.lock.Lock()
	c.HCConf.HealthCheckInterval = hcconff.HealthCheckInterval
	c.HCConf.UnavailableFailedThres = hcconff.UnavailableFailedThres
	c.lock.Unlock()
}

func (c *WafClient) GetDetectParam(bodySize int64) (time.Duration, int) {
	c.lock.Lock()
	timeout := time.Duration(c.globalWafParam.GetReqTimeout(int(bodySize)) * int(time.Millisecond))
	retryMax := c.globalWafParam.WafDetect.RetryMax
	c.lock.Unlock()

	return timeout, retryMax
}

func (c *WafClient) GetRefCount() int {
	c.lock.RLock()
	counter := c.refCount
	c.lock.RUnlock()

	return counter
}

func (c *WafClient) AddRefCount() {
	c.lock.Lock()
	c.refCount = c.refCount + 1
	c.lock.Unlock()
}

func (c *WafClient) DecRefCount() {
	c.lock.Lock()

	c.refCount = c.refCount - 1
	if c.refCount < 0 {
		c.refCount = 0
		log.Logger.Warn("WafClient ref counter error: refCount < 0")
	}

	c.lock.Unlock()
}

func (c *WafClient) SetDeleteTag() {
	c.lock.Lock()
	c.toDelete = true
	c.available.Store(false)
	c.monitor.state.Set("waf_client_available_"+c.serverAddress+"_delete_tag", "true")
	c.lock.Unlock()
}

func (c *WafClient) WillBeDeleted() bool {
	c.lock.RLock()
	toDelete := c.toDelete
	c.lock.RUnlock()

	return toDelete
}

func (c *WafClient) Close() error {
	del := c.WillBeDeleted()
	rc := c.GetRefCount()

	if del && rc == 0 {
		c.client.Close()

		// tell checkWafServer() to exit
		c.exitCh <- struct{}{}

		log.Logger.Info("Waf client: %s close", c.serverAddress)
		c.monitor.state.Delete("waf_client_available_" + c.serverAddress)
		c.monitor.state.Delete("waf_client_available_" + c.serverAddress + "_delete_tag")
		c.monitor.state.Inc(DELETED_CLIENTS, 1)

		return nil
	}

	return fmt.Errorf("WafClient.Close(): toDelete = %v, RefCounter = %d", del, rc)
}

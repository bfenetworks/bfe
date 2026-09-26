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

package mod_traffic_mirror

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_balance"
)

// mirrorTargetScheme: mirror requests dial backends over plain HTTP, same as
// the main forwarding path (see setBackendAddr in bfe_server/reverseproxy.go)
const mirrorTargetScheme = "http"

// mirrorSender dispatches mirror tasks asynchronously. A fixed worker pool
// (MaxConcurrent workers draining a bounded queue) keeps goroutine usage
// flat under load: submitting never blocks the main request, and tasks that
// arrive while the queue is full are dropped and counted.
type mirrorSender struct {
	conf    *ConfModTrafficMirror
	client  *http.Client
	queue   chan *mirrorTask
	table   *mirrorRuleTable
	breaker *mirrorCircuitBreaker
	pms     *PrometheusStates

	connectTimeout time.Duration
	ttfbTimeout    time.Duration
	totalTimeout   time.Duration
}

func newMirrorSender(conf *ConfModTrafficMirror, table *mirrorRuleTable,
	breaker *mirrorCircuitBreaker, pms *PrometheusStates) *mirrorSender {
	s := &mirrorSender{
		conf:    conf,
		queue:   make(chan *mirrorTask, conf.Basic.QueueCapacity),
		table:   table,
		breaker: breaker,
		pms:     pms,

		connectTimeout: time.Duration(conf.Basic.ConnectTimeoutMs) * time.Millisecond,
		ttfbTimeout:    time.Duration(conf.Basic.TTFBTimeoutMs) * time.Millisecond,
		totalTimeout:   time.Duration(conf.Basic.TotalTimeoutMs) * time.Millisecond,
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   s.connectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          conf.Basic.MaxConcurrent,
		MaxIdleConnsPerHost:   conf.Basic.MaxConcurrent,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   s.connectTimeout,
		ResponseHeaderTimeout: 0, // TTFB is bounded by the context/timer below
	}

	s.client = &http.Client{
		Transport: transport,
		// never follow redirects for mirror sub-requests
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return s
}

// start launches the worker pool; it never returns and the workers live for
// the process lifetime (BFE modules have no shutdown hook).
func (s *mirrorSender) start(workers int) {
	for i := 0; i < workers; i++ {
		go s.worker()
	}
}

func (s *mirrorSender) worker() {
	for task := range s.queue {
		s.exec(task)
	}
}

// Submit enqueues a mirror task without blocking; it returns false when the
// queue is full and the task is dropped.
func (s *mirrorSender) Submit(task *mirrorTask) bool {
	select {
	case s.queue <- task:
		return true
	default:
		return false
	}
}

// exec runs one mirror request end to end: circuit check, backend pick,
// layered-timeout send, full response drain and metrics. It never panics up
// to the worker; any failure is counted and swallowed.
func (s *mirrorSender) exec(task *mirrorTask) {
	s.pms.incInflight(task.Cluster)
	defer s.pms.decInflight(task.Cluster)

	// FR-12: drop fast when the mirror target is circuit-open
	if s.breaker.Open(task.Cluster) {
		s.table.incCircuitOpen()
		s.pms.reportCircuitOpen(task.Cluster)
		return
	}

	// resolve a mirror backend from the cluster balance table (async side
	// only; never touches the original request)
	target, err := s.resolveTarget(task)
	if err != nil {
		s.onFail(task, "resolve", err)
		return
	}

	resp, ttfb, err := s.doWithLayeredTimeout(task, target)
	if err != nil {
		s.onFail(task, "send", err)
		return
	}
	s.breaker.OnSuccess(task.Cluster)

	// FR-8: drain the response completely (SSE to [DONE]) while parsing
	result := drainAndParseBody(resp.Body, resp.Header.Get("Content-Type"),
		s.conf.Basic.MaxResponseBodyBytes)
	resp.Body.Close()

	s.table.incReqTotal()
	s.pms.reportRequest(task.Product, task.Cluster, task.Model)
	s.pms.reportResponse(task.Cluster, resp.StatusCode, result)
	if result.Truncated {
		s.table.incRespTruncated()
		s.pms.reportTruncated(task.Cluster)
	}
	s.pms.reportLatency(task.Cluster, ttfb, time.Since(task.StartAt))
}

// onFail counts and logs a failed mirror attempt
func (s *mirrorSender) onFail(task *mirrorTask, reason string, err error) {
	s.table.incSendFail()
	s.pms.reportFailure(task.Cluster, reason)
	s.pms.reportLatency(task.Cluster, 0, time.Since(task.StartAt))
	s.breaker.OnFail(task.Cluster)
	if openDebug {
		log.Logger.Info("mod_traffic_mirror: mirror failed cluster[%s] reason[%s]: %v",
			task.Cluster, reason, err)
	}
}

// resolveTarget picks a healthy backend for the mirror cluster and builds
// the request URL. Resolution happens here (not on the main path) so the
// mirror decision stays free of balance-table locks.
func (s *mirrorSender) resolveTarget(task *mirrorTask) (string, error) {
	balTable := bfe_balance.GetGlobalBalTable()
	if balTable == nil {
		return "", fmt.Errorf("balance table not registered")
	}
	bal, err := balTable.Lookup(task.Cluster)
	if err != nil {
		return "", err
	}
	backend, err := bal.PickBackend()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s://%s%s", mirrorTargetScheme, backend.GetAddrInfo(), task.URI), nil
}

// doWithLayeredTimeout sends the mirror request with three layered timeouts
// (FR-9): connect (dialer), first byte (hard cancel timer) and total
// (context). It returns the response and the observed time to first byte.
func (s *mirrorSender) doWithLayeredTimeout(task *mirrorTask, target string) (*http.Response, time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.totalTimeout)
	defer cancel()

	start := time.Now()
	var ttfb time.Duration
	var ttfbOnce sync.Once
	gotByte := make(chan struct{})

	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			ttfbOnce.Do(func() {
				ttfb = time.Since(start)
				close(gotByte)
			})
		},
	}
	ctx = httptrace.WithClientTrace(ctx, trace)

	// hard TTFB limit: cancel when no first byte arrives in time
	if s.ttfbTimeout > 0 {
		timer := time.AfterFunc(s.ttfbTimeout, func() {
			select {
			case <-gotByte:
				// first byte already received
			default:
				cancel()
			}
		})
		defer timer.Stop()
	}

	req, err := http.NewRequestWithContext(ctx, task.Method, target, bytes.NewReader(task.Body))
	if err != nil {
		return nil, 0, err
	}
	req.Header = task.Header
	if task.Host != "" {
		req.Host = task.Host
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, ttfb, err
	}
	return resp, ttfb, nil
}

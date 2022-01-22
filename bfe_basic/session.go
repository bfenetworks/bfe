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

// session is one incoming http connection

package bfe_basic

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_tls"
)

const (
	SessionNotTrustSource int32 = 0
	SessionTrustSource    int32 = 1
)

type Session struct {
	SessionId string    // session id
	StartTime time.Time // time of accept the connection
	EndTime   time.Time // time of close connection
	Overhead  time.Duration

	Connection net.Conn
	RemoteAddr *net.TCPAddr // client address

	Use100Continue bool // "expect 100-continue" is used?

	Proto    string                   // protocol for the connection
	IsSecure bool                     // over tls connection?
	TlsState *bfe_tls.ConnectionState // tls state when using TLS

	Vip     net.IP // the virtual ip visited
	Vport   int    // vip virtual port visited
	Product string // product name of vip
	Rtt     uint32 // smoothed RTT for current connection (us)

	lock          sync.RWMutex                // lock for session
	reqNum        int64                       // number of total request
	reqNumActive  int64                       // number of active request
	readTotal     int64                       // total bytes read from client socket
	writeTotal    int64                       // total bytes write to client socket
	isTrustSource int32                       // from Trust source or not
	errCode       error                       // err of the connection
	errMsg        string                      // message of error
	context       map[interface{}]interface{} // special session state
}

// NewSession creates and initializes a new session
func NewSession(conn net.Conn) *Session {
	s := new(Session)

	s.StartTime = time.Now()

	s.Connection = conn
	if conn != nil {
		s.RemoteAddr = conn.RemoteAddr().(*net.TCPAddr)
	}

	s.Use100Continue = false
	s.context = make(map[interface{}]interface{})
	return s
}

func (s *Session) GetVip() net.IP {
	return s.Vip
}

func (s *Session) Finish() {
	s.EndTime = time.Now()
	s.Overhead = s.EndTime.Sub(s.StartTime)
}

func (s *Session) IncReqNum(count int) int64 {
	return atomic.AddInt64(&s.reqNum, int64(count))
}

func (s *Session) ReqNum() int64 {
	return atomic.LoadInt64(&s.reqNum)
}

func (s *Session) SetReqNum(count int) {
	atomic.StoreInt64(&s.reqNum, int64(count))
}

func (s *Session) IncReqNumActive(count int) int64 {
	return atomic.AddInt64(&s.reqNumActive, int64(count))
}

func (s *Session) ReqNumActive() int64 {
	return atomic.LoadInt64(&s.reqNumActive)
}

func (s *Session) SetReqNumActive(count int) {
	atomic.StoreInt64(&s.reqNumActive, int64(count))
}

func (s *Session) UpdateReadTotal(total int) int {
	ntotal := int64(total)
	rtotal := atomic.SwapInt64(&s.readTotal, ntotal)

	// return diff with last value
	if ntotal >= rtotal {
		return int(ntotal - rtotal)
	}
	return 0
}

func (s *Session) ReadTotal() int {
	return int(atomic.LoadInt64(&s.readTotal))
}

func (s *Session) UpdateWriteTotal(total int) int {
	ntotal := int64(total)
	wtotal := atomic.SwapInt64(&s.writeTotal, ntotal)

	// return diff with last value
	if ntotal >= wtotal {
		return int(ntotal - wtotal)
	}
	return 0
}

func (s *Session) WriteTotal() int {
	return int(atomic.LoadInt64(&s.writeTotal))
}

func (s *Session) SetError(errCode error, errMsg string) {
	s.lock.Lock()
	s.errCode = errCode
	s.errMsg = errMsg
	s.lock.Unlock()
}

func (s *Session) GetError() (string, error) {
	s.lock.RLock()
	errCode := s.errCode
	errMsg := s.errMsg
	s.lock.RUnlock()

	return errMsg, errCode
}

// ClearContext clears the old context and makes a new one.
func (s *Session) ClearContext() {
	s.context = make(map[interface{}]interface{})
}

func (s *Session) SetContext(key, val interface{}) {
	s.lock.Lock()
	s.context[key] = val
	s.lock.Unlock()
}

func (s *Session) GetContext(key interface{}) interface{} {
	s.lock.RLock()
	val := s.context[key]
	s.lock.RUnlock()
	return val
}

func (s *Session) TrustSource() bool {
	val := atomic.LoadInt32(&s.isTrustSource)
	return val == SessionTrustSource
}

func (s *Session) SetTrustSource(isTrustSource bool) {
	val := SessionNotTrustSource
	if isTrustSource {
		val = SessionTrustSource
	}
	atomic.StoreInt32(&s.isTrustSource, val)
}

func (s *Session) String() string {
	s.lock.RLock()
	val := s.SessionId
	s.lock.RUnlock()
	return fmt.Sprintf("session id: %s", val)
}

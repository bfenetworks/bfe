// Copyright (c) 2019 Baidu, Inc.
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
	"net"
	"sync"
	"sync/atomic"
	"time"
	"fmt"
)

import (
	"github.com/baidu/bfe/bfe_tls"
)

type Session struct {
	SessionId string    // session id
	StartTime time.Time // time of accept the connection
	EndTime   time.Time // time of close connection
	Overhead  time.Duration

	Connection net.Conn
	RemoteAddr *net.TCPAddr // client address

	Use100Continue bool // "expect 100-continue" is used?
	IsTrustIP      bool // from Trust IP?

	Proto    string                   // protocol for the connection
	IsSecure bool                     // over tls connection?
	TlsState *bfe_tls.ConnectionState // tls state when using TLS

	Vip     net.IP // the virtual ip visited
	Vport   int    // vip virtual port visited
	Product string // product name of vip
	Rtt     uint32 // smoothed RTT for current connection (us)

	lock         sync.Mutex                  // lock for session
	ReqNum       int64                       // number of total request
	ReqNumActive int64                       // number of active request
	ReadTotal    int64                       // total bytes read from client socket
	WriteTotal   int64                       // total bytes write to client socket
	ErrCode      error                       // err of the connection
	ErrMsg       string                      // message of error
	Context      map[interface{}]interface{} // special session state
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
	s.Context = make(map[interface{}]interface{})
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
	return atomic.AddInt64(&s.ReqNum, int64(count))
}

func (s *Session) IncReqNumActive(count int) int64 {
	return atomic.AddInt64(&s.ReqNumActive, int64(count))
}

func (s *Session) UpdateReadTotal(total int) int {
	ntotal := int64(total)
	rtotal := atomic.SwapInt64(&s.ReadTotal, ntotal)

	// return diff with last value
	if ntotal >= rtotal {
		return int(ntotal - rtotal)
	}
	return 0
}

func (s *Session) UpdateWriteTotal(total int) int {
	ntotal := int64(total)
	wtotal := atomic.SwapInt64(&s.WriteTotal, ntotal)

	// return diff with last value
	if ntotal >= wtotal {
		return int(ntotal - wtotal)
	}
	return 0
}

func (s *Session) SetError(errCode error, errMsg string) {
	s.lock.Lock()
	s.ErrCode = errCode
	s.ErrMsg = errMsg
	s.lock.Unlock()
}

func (s *Session) GetError() (error, string) {
	s.lock.Lock()
	errCode := s.ErrCode
	errMsg := s.ErrMsg
	s.lock.Unlock()

	return errCode, errMsg
}

func (s *Session) SetContext(key, val interface{}) {
	s.lock.Lock()
	s.Context[key] = val
	s.lock.Unlock()
}

func (s *Session) GetContext(key interface{}) interface{} {
	s.lock.Lock()
	val := s.Context[key]
	s.lock.Unlock()
	return val
}

func (s *Session) String() string {
	s.lock.Lock()
	val := s.SessionId
	s.lock.Unlock()
	return fmt.Sprintf("session id: %s", val)
}

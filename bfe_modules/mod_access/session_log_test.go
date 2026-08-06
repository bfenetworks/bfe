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

package mod_access

import (
	"bytes"
	"errors"
	"net"
	"testing"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_tls"
)

func prepareSessionLogTest(t *testing.T) (*bfe_basic.Session, *bytes.Buffer) {
	conn := newTestConn()

	session := bfe_basic.NewSession(conn)
	session.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}
	session.StartTime = time.Now().Add(-time.Second)
	session.EndTime = time.Now()
	session.Overhead = session.EndTime.Sub(session.StartTime)
	session.IsSecure = true
	session.Use100Continue = true
	session.SetReqNum(5)
	session.UpdateReadTotal(1024)
	session.UpdateWriteTotal(2048)
	session.SetError(errors.New("ses_err"), "session timeout")
	session.TlsState = &bfe_tls.ConnectionState{
		ClientRandom: []byte{0x01, 0x02, 0x03},
		ServerRandom: []byte{0x04, 0x05, 0x06},
	}

	return session, bytes.NewBuffer(nil)
}

func TestBuildErrorMsg(t *testing.T) {
	if msg := buildErrorMsg(nil, ""); msg != "-" {
		t.Errorf("buildErrorMsg(nil) got: %s, want: -", msg)
	}

	if msg := buildErrorMsg(errors.New("code1"), ""); msg != "code1" {
		t.Errorf("buildErrorMsg(code1) got: %s, want: code1", msg)
	}

	if msg := buildErrorMsg(errors.New("code1"), "timeout"); msg != "code1,timeout" {
		t.Errorf("buildErrorMsg(code1,timeout) got: %s, want: code1,timeout", msg)
	}
}

func TestOnLogFmtSesClientIp(t *testing.T) {
	session, buff := prepareSessionLogTest(t)
	err := onLogFmtSesClientIp(nil, &LogFmtItem{}, buff, session)
	if err != nil {
		t.Errorf("onLogFmtSesClientIp() error: %v", err)
	}
	if buff.String() != "192.168.1.1:12345" {
		t.Errorf("onLogFmtSesClientIp() got: %s", buff.String())
	}
}

func TestOnLogFmtSesStartTime(t *testing.T) {
	session, buff := prepareSessionLogTest(t)
	err := onLogFmtSesStartTime(nil, &LogFmtItem{}, buff, session)
	if err != nil {
		t.Errorf("onLogFmtSesStartTime() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtSesStartTime() output is empty")
	}
}

func TestOnLogFmtSesEndTime(t *testing.T) {
	session, buff := prepareSessionLogTest(t)
	err := onLogFmtSesEndTime(nil, &LogFmtItem{}, buff, session)
	if err != nil {
		t.Errorf("onLogFmtSesEndTime() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtSesEndTime() output is empty")
	}
}

func TestOnLogFmtSesOverhead(t *testing.T) {
	session, buff := prepareSessionLogTest(t)
	err := onLogFmtSesOverhead(nil, &LogFmtItem{}, buff, session)
	if err != nil {
		t.Errorf("onLogFmtSesOverhead() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtSesOverhead() output is empty")
	}
}

func TestOnLogFmtSesIsSecure(t *testing.T) {
	session, buff := prepareSessionLogTest(t)
	err := onLogFmtSesIsSecure(nil, &LogFmtItem{}, buff, session)
	if err != nil {
		t.Errorf("onLogFmtSesIsSecure() error: %v", err)
	}
	if buff.String() != "true" {
		t.Errorf("onLogFmtSesIsSecure() got: %s", buff.String())
	}
}

func TestOnLogFmtSesUse100(t *testing.T) {
	session, buff := prepareSessionLogTest(t)
	err := onLogFmtSesUse100(nil, &LogFmtItem{}, buff, session)
	if err != nil {
		t.Errorf("onLogFmtSesUse100() error: %v", err)
	}
	if buff.String() != "true" {
		t.Errorf("onLogFmtSesUse100() got: %s", buff.String())
	}
}

func TestOnLogFmtSesKeepAliveNum(t *testing.T) {
	session, buff := prepareSessionLogTest(t)
	err := onLogFmtSesKeepAliveNum(nil, &LogFmtItem{}, buff, session)
	if err != nil {
		t.Errorf("onLogFmtSesKeepAliveNum() error: %v", err)
	}
	if buff.String() != "5" {
		t.Errorf("onLogFmtSesKeepAliveNum() got: %s", buff.String())
	}
}

func TestOnLogFmtSesReadTotal(t *testing.T) {
	session, buff := prepareSessionLogTest(t)
	err := onLogFmtSesReadTotal(nil, &LogFmtItem{}, buff, session)
	if err != nil {
		t.Errorf("onLogFmtSesReadTotal() error: %v", err)
	}
	if buff.String() != "1024" {
		t.Errorf("onLogFmtSesReadTotal() got: %s", buff.String())
	}
}

func TestOnLogFmtSesWriteTotal(t *testing.T) {
	session, buff := prepareSessionLogTest(t)
	err := onLogFmtSesWriteTotal(nil, &LogFmtItem{}, buff, session)
	if err != nil {
		t.Errorf("onLogFmtSesWriteTotal() error: %v", err)
	}
	if buff.String() != "2048" {
		t.Errorf("onLogFmtSesWriteTotal() got: %s", buff.String())
	}
}

func TestOnLogFmtSesErrorCode(t *testing.T) {
	session, buff := prepareSessionLogTest(t)
	err := onLogFmtSesErrorCode(nil, &LogFmtItem{}, buff, session)
	if err != nil {
		t.Errorf("onLogFmtSesErrorCode() error: %v", err)
	}
	want := "ses_err,session timeout"
	if buff.String() != want {
		t.Errorf("onLogFmtSesErrorCode() got: %s, want: %s", buff.String(), want)
	}
}

func TestOnLogFmtSesTLSClientRandom(t *testing.T) {
	session, buff := prepareSessionLogTest(t)
	err := onLogFmtSesTLSClientRandom(nil, &LogFmtItem{}, buff, session)
	if err != nil {
		t.Errorf("onLogFmtSesTLSClientRandom() error: %v", err)
	}
	if buff.String() != "010203" {
		t.Errorf("onLogFmtSesTLSClientRandom() got: %s", buff.String())
	}
}

func TestOnLogFmtSesTLSServerRandom(t *testing.T) {
	session, buff := prepareSessionLogTest(t)
	err := onLogFmtSesTLSServerRandom(nil, &LogFmtItem{}, buff, session)
	if err != nil {
		t.Errorf("onLogFmtSesTLSServerRandom() error: %v", err)
	}
	if buff.String() != "040506" {
		t.Errorf("onLogFmtSesTLSServerRandom() got: %s", buff.String())
	}
}

func TestOnLogFmtSesTLSRandomNil(t *testing.T) {
	conn := newTestConn()
	session := bfe_basic.NewSession(conn)
	session.TlsState = nil

	buff := bytes.NewBuffer(nil)
	err := onLogFmtSesTLSClientRandom(nil, &LogFmtItem{}, buff, session)
	if err != nil {
		t.Errorf("onLogFmtSesTLSClientRandom() error: %v", err)
	}
	if buff.String() != "-" {
		t.Errorf("onLogFmtSesTLSClientRandom() nil tls got: %s, want: -", buff.String())
	}
}

func TestOnLogFmtSesNilSession(t *testing.T) {
	buff := bytes.NewBuffer(nil)

	tests := []struct {
		name string
		fn   func(*ModuleAccess, *LogFmtItem, *bytes.Buffer, *bfe_basic.Session) error
	}{
		{"ClientIp", onLogFmtSesClientIp},
		{"EndTime", onLogFmtSesEndTime},
		{"ErrorCode", onLogFmtSesErrorCode},
		{"IsSecure", onLogFmtSesIsSecure},
		{"KeepAliveNum", onLogFmtSesKeepAliveNum},
		{"Overhead", onLogFmtSesOverhead},
		{"ReadTotal", onLogFmtSesReadTotal},
		{"TLSClientRandom", onLogFmtSesTLSClientRandom},
		{"TLSServerRandom", onLogFmtSesTLSServerRandom},
		{"Use100", onLogFmtSesUse100},
		{"WriteTotal", onLogFmtSesWriteTotal},
		{"StartTime", onLogFmtSesStartTime},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buff.Reset()
			err := tt.fn(nil, &LogFmtItem{}, buff, nil)
			if err == nil {
				t.Errorf("onLogFmtSes%s() should return error when session is nil", tt.name)
			}
		})
	}
}

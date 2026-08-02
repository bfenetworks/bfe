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

package mod_access_pb3

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_tls"

	bfe_access_pb3 "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
)

func makeSessionLogTest(t *testing.T) (*bfe_basic.Session, *bfe_access_pb3.SessionLog) {
	conn := newTestConn()
	session := bfe_basic.NewSession(conn)
	session.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}
	session.Vip = net.ParseIP("10.0.0.1")
	session.SessionId = "100"
	session.StartTime = time.Now().Add(-time.Second)
	session.EndTime = time.Now()
	session.Overhead = session.EndTime.Sub(session.StartTime)
	session.Proto = "http"
	session.Product = "unittest"
	session.SetReqNum(5)
	session.UpdateReadTotal(1024)
	session.UpdateWriteTotal(2048)
	session.SetError(errors.New("ses_err"), "session timeout")
	session.TlsState = &bfe_tls.ConnectionState{
		Version:       0x0303,
		CipherSuite:   0x002f,
		DidResume:     true,
		OcspStaple:    true,
		HandshakeTime: 100 * time.Millisecond,
	}

	return session, &bfe_access_pb3.SessionLog{}
}

func TestSessionLogGen(t *testing.T) {
	session, _ := makeSessionLogTest(t)
	bfeLog := sessionLogGen(session)

	if bfeLog == nil {
		t.Fatal("sessionLogGen() returns nil")
	}
	if bfeLog.Product == nil || *bfeLog.Product != bfe_access_pb3.ProductID_BFE {
		t.Error("Product error")
	}
	if bfeLog.LogType == nil || *bfeLog.LogType != bfe_access_pb3.BfeLogType_Session {
		t.Error("LogType error")
	}
	if bfeLog.SessionLog == nil {
		t.Fatal("SessionLog is nil")
	}
}

func TestSessionLogTagGen(t *testing.T) {
	tests := []struct {
		product string
		errCode error
		want    string
	}{
		{"unittest", nil, "session_unittest"},
		{"", nil, "session_bfe"},
		{"unit-test", nil, "session_unit_test"},
		{"unittest", errors.New("err"), "session_err_unittest"},
		{"", errors.New("err"), "session_err_bfe"},
	}

	for _, tt := range tests {
		session := &bfe_basic.Session{Product: tt.product}
		if tt.errCode != nil {
			session.SetError(tt.errCode, "errmsg")
		}
		got := sessionLogTagGen(session)
		if got != tt.want {
			t.Errorf("sessionLogTagGen(%s, %v) got: %s, want: %s", tt.product, tt.errCode, got, tt.want)
		}
	}
}

func TestSesBasicInfoGen(t *testing.T) {
	session, sesLog := makeSessionLogTest(t)
	sesBasicInfoGen(sesLog, session)

	if sesLog.ErrCode == nil || *sesLog.ErrCode != "ses_err" {
		t.Error("ErrCode error")
	}
	if sesLog.ErrMsg == nil || *sesLog.ErrMsg != "session timeout" {
		t.Error("ErrMsg error")
	}
	if sesLog.ReqNum == nil || *sesLog.ReqNum != 5 {
		t.Error("ReqNum error")
	}
	if sesLog.ReadLen == nil || *sesLog.ReadLen != 1024 {
		t.Error("ReadLen error")
	}
	if sesLog.WriteLen == nil || *sesLog.WriteLen != 2048 {
		t.Error("WriteLen error")
	}
	if sesLog.AddrInfo == nil {
		t.Error("AddrInfo is nil")
	}
	if sesLog.Proto == nil || *sesLog.Proto != "http" {
		t.Error("Proto error")
	}
	if sesLog.Product == nil || *sesLog.Product != "unittest" {
		t.Error("Product error")
	}
}

func TestSesTimeInfoGen(t *testing.T) {
	session, sesLog := makeSessionLogTest(t)
	sesTimeInfoGen(sesLog, session)

	if sesLog.StartTime == nil || *sesLog.StartTime == 0 {
		t.Error("StartTime error")
	}
	if sesLog.AllTime == nil || *sesLog.AllTime == 0 {
		t.Error("AllTime error")
	}
}

func TestSesTlsInfoGen(t *testing.T) {
	session, sesLog := makeSessionLogTest(t)
	sesTlsInfoGen(sesLog, session)

	if sesLog.TlsVersion == nil || *sesLog.TlsVersion != 0x0303 {
		t.Error("TlsVersion error")
	}
	if sesLog.CipherSuite == nil || *sesLog.CipherSuite != 0x002f {
		t.Error("CipherSuite error")
	}
	if sesLog.SessionResume == nil || !*sesLog.SessionResume {
		t.Error("SessionResume error")
	}
	if sesLog.OcspStaple == nil || !*sesLog.OcspStaple {
		t.Error("OcspStaple error")
	}
	if sesLog.HandshakeTime == nil || *sesLog.HandshakeTime == 0 {
		t.Error("HandshakeTime error")
	}
}

func TestSesTlsInfoGenNil(t *testing.T) {
	conn := newTestConn()
	session := bfe_basic.NewSession(conn)
	session.TlsState = nil
	sesLog := &bfe_access_pb3.SessionLog{}

	sesTlsInfoGen(sesLog, session)
	if sesLog.TlsVersion != nil {
		t.Error("TlsVersion should be nil when TlsState is nil")
	}
}

func TestSesLostTypeGen(t *testing.T) {
	conn := newTestConn()
	session := bfe_basic.NewSession(conn)
	session.SetError(errors.New("err"), "mod_crypto RsaDecrypt failed")
	sesLog := &bfe_access_pb3.SessionLog{}

	sesLostTypeGen(sesLog, session)
	if sesLog.LostType == nil || *sesLog.LostType != bfe_access_pb3.LostType_Bfe {
		t.Error("LostType should be Bfe")
	}
}

func TestSesLostTypeGenNoMatch(t *testing.T) {
	conn := newTestConn()
	session := bfe_basic.NewSession(conn)
	session.SetError(errors.New("err"), "some other error")
	sesLog := &bfe_access_pb3.SessionLog{}

	sesLostTypeGen(sesLog, session)
	if sesLog.LostType != nil {
		t.Error("LostType should be nil when no pattern matches")
	}
}

func TestSesLostTypeGenNoError(t *testing.T) {
	conn := newTestConn()
	session := bfe_basic.NewSession(conn)
	sesLog := &bfe_access_pb3.SessionLog{}

	sesLostTypeGen(sesLog, session)
	if sesLog.LostType != nil {
		t.Error("LostType should be nil when no error msg")
	}
}

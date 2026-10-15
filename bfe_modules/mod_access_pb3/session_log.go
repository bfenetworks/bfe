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
	"strconv"
	"strings"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"google.golang.org/protobuf/proto"

	bfe_access_pb3 "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
)

var (
	// If session error contains one of the following errors, indicates that the handshake error is caused by bfe
	handshakeErrorPatterns = []string{
		"mod_crypto RsaDecrypt failed",
	}
)

// prepare pb log for session
func sessionLogGen(session *bfe_basic.Session) *bfe_access_pb3.BfeLog {
	// create session log
	bfeLog := &bfe_access_pb3.BfeLog{}
	bfeLog.Product = BFE_PRODUCT_ID
	bfeLog.LogType = LOG_TYPE_SESSION

	// fill log tag
	bfeLog.LogTag = proto.String(sessionLogTagGen(session))

	// log id
	sessionId, _ := strconv.ParseUint(session.SessionId, 10, 64)
	bfeLog.Logid = proto.Uint64(sessionId)

	// timestamp
	now := time.Now()
	bfeLog.Timestamp = proto.Uint64(uint64(now.Unix()))

	sessionLog := new(bfe_access_pb3.SessionLog)
	bfeLog.SessionLog = sessionLog

	// basic info
	sesBasicInfoGen(sessionLog, session)

	// time info
	sesTimeInfoGen(sessionLog, session)

	// tls/ssl info
	sesTlsInfoGen(sessionLog, session)

	// tls handshake error reason
	sesLostTypeGen(sessionLog, session)

	return bfeLog
}

/*
generate log-tag for session log

- for none-error session, log_tag will be session_<product name>
- for none-error session, if product is empty, log_tag will be session_bfe
- for error session, log_tag will be session_err_<product name>
- for error session, if product is empty, log_tag will be session_err_bfe
*/
func sessionLogTagGen(ses *bfe_basic.Session) string {
	product := ses.Product
	if product == "" {
		product = "bfe"
	}

	// Note: Possible characters in product name are a-z, 0-9, _, -.
	// According to name convertion in biglog platform, valid characters
	// for log tag are a-z, 0-9, _.  We replace "-" with "_" for pb log tag.
	product = strings.Replace(product, "-", "_", -1)

	errCode, _ := ses.GetError()
	if errCode == "" {
		return "session_" + product
	}

	return "session_err_" + product
}

// basic info
func sesBasicInfoGen(sesLog *bfe_access_pb3.SessionLog, session *bfe_basic.Session) {
	// err code for session
	// for successful session, err_code should be ""
	errMsg, errCode := session.GetError()
	if errCode != nil {
		sesLog.ErrCode = proto.String(errCode.Error())
	} else {
		sesLog.ErrCode = proto.String("")
	}
	sesLog.ErrMsg = proto.String(errMsg)

	// number of requests served in this session/connection
	sesLog.ReqNum = proto.Uint32(uint32(session.ReqNum()))

	// total bytes read from client socket
	sesLog.ReadLen = proto.Uint32(uint32(session.ReadTotal()))

	// total bytes write to client socket
	sesLog.WriteLen = proto.Uint32(uint32(session.WriteTotal()))

	// connection addr info
	sesLog.AddrInfo = ConnAddrInfoGen(session)

	// protocol
	sesLog.Proto = proto.String(session.Proto)

	// product
	sesLog.Product = proto.String(session.Product)
}

// time info
func sesTimeInfoGen(sesLog *bfe_access_pb3.SessionLog, session *bfe_basic.Session) {
	// timestamp of session start
	sesLog.StartTime = proto.Uint64(uint64(session.StartTime.Unix()))

	// time duration of session (in ms)
	ms := session.EndTime.Sub(session.StartTime).Nanoseconds() / 1000000
	sesLog.AllTime = proto.Uint32(uint32(ms))

	// smoothed round tripper time between src sock ip and bfe(in ms)
	sesLog.Rtt = proto.Uint32(session.Rtt / 1000)
}

// tls/ssl info
func sesTlsInfoGen(sesLog *bfe_access_pb3.SessionLog, session *bfe_basic.Session) {
	if session.TlsState == nil {
		return
	}

	state := session.TlsState
	sesLog.TlsVersion = proto.Uint32(uint32(state.Version))
	sesLog.CipherSuite = proto.Uint32(uint32(state.CipherSuite))
	sesLog.SessionResume = proto.Bool(state.DidResume)
	sesLog.OcspStaple = proto.Bool(state.OcspStaple)
	sesLog.HandshakeTime = proto.Uint32(uint32(state.HandshakeTime / time.Millisecond))
}

func sesLostTypeGen(sesLog *bfe_access_pb3.SessionLog, ses *bfe_basic.Session) {
	errMsg, _ := ses.GetError()
	if len(errMsg) == 0 {
		return
	}

	for _, errPattern := range handshakeErrorPatterns {
		if strings.Contains(errMsg, errPattern) {
			lostType := bfe_access_pb3.LostType_Bfe
			sesLog.LostType = &lostType
			return
		}
	}
}

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

package mod_access

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
)

import (
	"github.com/baidu/bfe/bfe_basic"
)

// FormatSesClientIP
func onLogFmtSesClientIp(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	buff.WriteString(session.RemoteAddr.String())
	return nil
}

// FormatSesEndTime
func onLogFmtSesEndTime(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	buff.WriteString(session.EndTime.String())

	return nil
}

func buildErrorMsg(err error, errMsg string) string {
	var msg string
	if err == nil {
		msg = "-"
	} else {
		msg = err.Error()
		if len(errMsg) != 0 {
			msg += "," + errMsg
		}
	}

	return msg
}

// FormatSesErrorCode
func onLogFmtSesErrorCode(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	errCode, errMsg := session.GetError()
	msg := buildErrorMsg(errCode, errMsg)
	buff.WriteString(msg)

	return nil
}

// FormatSesIsSecure
func onLogFmtSesIsSecure(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	msg := fmt.Sprintf("%v", session.IsSecure)
	buff.WriteString(msg)

	return nil
}

// FormatSesKeepaliveNum
func onLogFmtSesKeepAliveNum(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	msg := fmt.Sprintf("%d", session.ReqNum)
	buff.WriteString(msg)

	return nil
}

// FormatSesOverHead
func onLogFmtSesOverhead(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	msg := fmt.Sprintf("%s", session.Overhead.String())
	buff.WriteString(msg)

	return nil
}

// FormatSesReadTotal
func onLogFmtSesReadTotal(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	msg := fmt.Sprintf("%d", session.ReadTotal)
	buff.WriteString(msg)

	return nil
}

// FormatSesTLSClientRandom
func onLogFmtSesTLSClientRandom(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	if session.TlsState != nil {
		buff.WriteString(hex.EncodeToString(session.TlsState.ClientRandom))
	} else {
		buff.WriteString("-")
	}

	return nil
}

// FormatSesTLSServerRandom
func onLogFmtSesTLSServerRandom(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	if session.TlsState != nil {
		buff.WriteString(hex.EncodeToString(session.TlsState.ServerRandom))
	} else {
		buff.WriteString("-")
	}

	return nil
}

// FormatSesUse100
func onLogFmtSesUse100(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	if session.Use100Continue {
		buff.WriteString("1")
	} else {
		buff.WriteString("0")
	}

	return nil
}

// FormatSesWriteTotal
func onLogFmtSesWriteTotal(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	msg := fmt.Sprintf("%d", session.WriteTotal)
	buff.WriteString(msg)

	return nil
}

// FormatSesStartTime
func onLogFmtSesStartTime(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	buff.WriteString(session.StartTime.String())

	return nil
}

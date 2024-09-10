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

package mod_access

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
)

func onLogFmtSesClientIp(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	buff.WriteString(session.RemoteAddr.String())

	return nil
}

func onLogFmtSesEndTime(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	buff.WriteString(session.EndTime.String())

	return nil
}

func buildErrorMsg(err error, errMsg string) string {
	msg := "-"
	if err != nil {
		msg = err.Error()
		if len(errMsg) != 0 {
			msg += "," + errMsg
		}
	}

	return msg
}

func onLogFmtSesErrorCode(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	errMsg, errCode := session.GetError()
	msg := buildErrorMsg(errCode, errMsg)
	buff.WriteString(msg)

	return nil
}

func onLogFmtSesIsSecure(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	msg := fmt.Sprintf("%v", session.IsSecure)
	buff.WriteString(msg)

	return nil
}

func onLogFmtSesKeepAliveNum(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	msg := fmt.Sprintf("%d", session.ReqNum())
	buff.WriteString(msg)

	return nil
}

func onLogFmtSesOverhead(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	msg := session.Overhead.String()
	buff.WriteString(msg)

	return nil
}

func onLogFmtSesReadTotal(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	msg := fmt.Sprintf("%d", session.ReadTotal())
	buff.WriteString(msg)

	return nil
}

func onLogFmtSesTLSClientRandom(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	msg := "-"
	if session.TlsState != nil {
		msg = hex.EncodeToString(session.TlsState.ClientRandom)
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtSesTLSServerRandom(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	msg := "-"
	if session.TlsState != nil {
		msg = hex.EncodeToString(session.TlsState.ServerRandom)
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtSesUse100(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	msg := fmt.Sprintf("%v", session.Use100Continue)
	buff.WriteString(msg)

	return nil
}

func onLogFmtSesWriteTotal(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	msg := fmt.Sprintf("%d", session.WriteTotal())
	buff.WriteString(msg)

	return nil
}

func onLogFmtSesStartTime(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	session *bfe_basic.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	buff.WriteString(session.StartTime.String())

	return nil
}

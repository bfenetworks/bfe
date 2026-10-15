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

// module for bfe unified log id generation

package mod_logid

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strconv"
)

import (
	"github.com/bfenetworks/go-lib/web-monitor/metrics"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	ModLogId = "mod_logid"
)

type ModuleLogIdState struct {
	NoLogidFromUpperBfe *metrics.Counter // counter for no logid cases when requests come from trusted ip
}

type ModuleLogId struct {
	name    string           // name of module
	state   ModuleLogIdState // module state
	metrics metrics.Metrics
}

func NewModuleLogId() *ModuleLogId {
	m := new(ModuleLogId)
	m.name = ModLogId
	m.metrics.Init(&m.state, ModLogId, 0)

	return m
}

func (m *ModuleLogId) Name() string {
	return m.name
}

func (m *ModuleLogId) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	// register handler
	err := cbs.AddFilter(bfe_module.HandleAccept, m.sessionIdHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.sessionIdHandler): %s", m.name, err.Error())
	}

	err = cbs.AddFilter(bfe_module.HandleBeforeLocation, m.requestIdHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.requestIdHandler): %s", m.name, err.Error())
	}

	// register web handler
	err = whs.RegisterHandler(web_monitor.WebHandleMonitor, m.name, m.getState)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.getState): %s", m.name, err.Error())
	}

	return nil
}

func (m *ModuleLogId) sessionIdHandler(session *bfe_basic.Session) int {
	session.SessionId = genLogId()

	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleLogId) requestIdHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	// check if request comes from trusted ip
	if req.Session.TrustSource() {
		logId := req.HttpRequest.Header.Get(bfe_basic.HeaderBfeLogId)
		if logId != "" {
			return bfe_module.BfeHandlerGoOn, nil
		} else {
			// trust ip, should has a logid
			m.state.NoLogidFromUpperBfe.Inc(1)
		}
	}

	// generate a new log id
	req.LogId = genLogId()
	req.HttpRequest.Header.Set(bfe_basic.HeaderBfeLogId, req.LogId)
	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleLogId) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

// genLogId generates the unified log id shared by the session id, the request
// log id and the X-Bfe-Log-Id header.
//
// Format (bfenetworks/bfe#1417): a decimal string denoting a value in
// [1, 2^63-1]. The access log carries the same value as a uint64
// (`BfeLog.logid`), so the string must be convertible without truncation; the
// previous 32-char hex format (128 bits) could not fit that field and was
// silently written as 0, which turned the report table's dedup key into a
// same-second overwrite (n-1 of n concurrent rows lost).
//
// The value is masked to 63 bits on purpose: logid is stored as a signed
// BIGINT on MySQL / Doris / StarRocks, and an out-of-range value would be
// truncated to the boundary value by Doris (non-strict), making every high-bit
// row collide, or reject the whole batch on MySQL (Error 1264).
func genLogId() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}
	return strconv.FormatUint(binary.LittleEndian.Uint64(b)&0x7FFFFFFFFFFFFFFF, 10)
}

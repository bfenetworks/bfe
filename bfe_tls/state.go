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

package bfe_tls

import (
	"github.com/baidu/go-lib/web-monitor/metrics"
)

type TlsState struct {
	TlsHandshakeReadClientHelloErr        *metrics.Counter
	TlsHandshakeFullAll                   *metrics.Counter
	TlsHandshakeFullSucc                  *metrics.Counter
	TlsHandshakeResumeAll                 *metrics.Counter
	TlsHandshakeResumeSucc                *metrics.Counter
	TlsHandshakeCheckResumeSessionTicket  *metrics.Counter
	TlsHandshakeShouldResumeSessionTicket *metrics.Counter
	TlsHandshakeCheckResumeSessionCache   *metrics.Counter
	TlsHandshakeShouldResumeSessionCache  *metrics.Counter
	TlsHandshakeAcceptSslv2ClientHello    *metrics.Counter
	TlsHandshakeAcceptEcdheWithoutExt     *metrics.Counter
	TlsHandshakeNoSharedCipherSuite       *metrics.Counter
	TlsHandshakeSslv2NotSupport           *metrics.Counter
	TlsHandshakeOcspTimeErr               *metrics.Counter
	TlsStatusRequestExtCount              *metrics.Counter
	TlsHandshakeZeroData                  *metrics.Counter
}

var state TlsState

func GetTlsState() *TlsState {
	return &state
}

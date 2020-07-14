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

package bfe_http2

import (
	"github.com/baidu/go-lib/web-monitor/metrics"
)

type Http2State struct {
	H2TimeoutPreface                   *metrics.Counter
	H2TimeoutSetting                   *metrics.Counter
	H2TimeoutConn                      *metrics.Counter
	H2TimeoutReadStream                *metrics.Counter
	H2TimeoutWriteStream               *metrics.Counter
	H2ErrGotReset                      *metrics.Counter
	H2ErrMaxStreamPerConn              *metrics.Counter
	H2ErrMaxHeaderListSize             *metrics.Counter
	H2ErrMaxHeaderUriSize              *metrics.Counter
	H2PanicConn                        *metrics.Counter
	H2PanicStream                      *metrics.Counter
	H2ReqHeaderOriginalSize            *metrics.Counter
	H2ReqHeaderCompressSize            *metrics.Counter
	H2ResHeaderOriginalSize            *metrics.Counter
	H2ResHeaderCompressSize            *metrics.Counter
	H2ConnOverload                     *metrics.Counter
	H2ReqOverload                      *metrics.Counter
	H2ConnExceedMaxQueuedControlFrames *metrics.Counter
}

var state Http2State

func GetHttp2State() *Http2State {
	return &state
}

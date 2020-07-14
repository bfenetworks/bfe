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

package bfe_spdy

import (
	"github.com/baidu/go-lib/web-monitor/metrics"
)

type SpdyState struct {
	SpdyTimeoutConn            *metrics.Counter
	SpdyTimeoutReadStream      *metrics.Counter
	SpdyTimeoutWriteStream     *metrics.Counter
	SpdyErrInvalidSynStream    *metrics.Counter
	SpdyErrInvalidDataStream   *metrics.Counter
	SpdyErrFlowControl         *metrics.Counter
	SpdyErrBadRequest          *metrics.Counter
	SpdyErrStreamAlreadyClosed *metrics.Counter
	SpdyErrStreamCancel        *metrics.Counter
	SpdyErrMaxStreamPerConn    *metrics.Counter
	SpdyErrGotReset            *metrics.Counter
	SpdyErrNewFramer           *metrics.Counter
	SpdyUnknownFrame           *metrics.Counter
	SpdyPanicConn              *metrics.Counter
	SpdyPanicStream            *metrics.Counter
	SpdyReqHeaderCompressSize  *metrics.Counter
	SpdyReqHeaderOriginalSize  *metrics.Counter
	SpdyResHeaderCompressSize  *metrics.Counter
	SpdyResHeaderOriginalSize  *metrics.Counter
	SpdyReqOverload            *metrics.Counter
	SpdyConnOverload           *metrics.Counter
}

var state SpdyState

func GetSpdyState() *SpdyState {
	return &state
}

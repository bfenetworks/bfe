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

package server_cert_conf

import (
	"time"
)

import (
	"golang.org/x/crypto/ocsp"
)

// OcspTimeRangeCheck checks whether the OCSP response is within its validity
// time range at current server time.
func OcspTimeRangeCheck(parse *ocsp.Response) bool {
	serverTime := time.Now()
	nextUpdate := parse.NextUpdate
	thisUpdate := parse.ThisUpdate

	// default tolerant time, one hour
	deltaTime := time.Duration(3600) * time.Second

	// serverTime should be [thisUpdate+deltaTime, nextUpdate-deltaTime]
	if serverTime.Sub(thisUpdate) < deltaTime || nextUpdate.Sub(serverTime) < deltaTime {
		return false
	}

	return true
}

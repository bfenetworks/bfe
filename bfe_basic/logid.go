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

package bfe_basic

import (
	"crypto/rand"
	"encoding/binary"
	"strconv"
)

// GenLogId generates the unified log id shared by the session id, the request
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
//
// Shared since bfenetworks/bfe#1441 (round 2): gateway-generated AI error
// responses also use it as the last fallback of the X-Bfe-Log-Id correlation
// header, so the generator lives here (bfe_basic) instead of mod_logid.
func GenLogId() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}
	return strconv.FormatUint(binary.LittleEndian.Uint64(b)&0x7FFFFFFFFFFFFFFF, 10)
}

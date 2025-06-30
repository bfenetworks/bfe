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

// basic waf info

package bfe_basic

const (
	REQ_CHECK_ONLY = "CheckOnly"
	REQ_NO_CHECK   = "NoCheck"
	REQ_FORBIDDEN  = "Forbidden"
	REQ_OK         = "WaitResponse.Pass.Ok"
	REQ_TIMEOUT    = "WaitResponse.Pass.Timeout"
	REQ_OTHER      = "WaitResponse.Pass.Other"
	NET_ERR        = "Net.Error" // net error between go-bfe and waf-server
)

const (
	WAF_NO_CHECK  = 0 // no check for request
	WAF_CHECKONLY = 1 // check only; from mod_waf_client, not used now
	WAF_FORBIDDEN = 2 // check and forbidden
	WAF_PASS      = 3 // check and pass
	WAF_DEGRADE   = 4 // check, but pass with degraded
	WAF_TIMEOUT   = 5 // check, but pass with timeout
	WAF_ERROR     = 6 // check and pass with error happened
)

const (
	REQ_CTX_WAF_INFO = "waf_client.waf_info"
)

// support old waf info struct
type WafInfo struct {
	WafSpentTime int64  // in ms.
	WafStatus    int    // waf status, see bfe proto file for detail
	WafRuleName  string // not used
}

func GetWafInfo(req *Request) *WafInfo {
	var info *WafInfo

	val := req.GetContext(REQ_CTX_WAF_INFO)
	if val != nil {
		info = val.(*WafInfo)
	} else {
		info = new(WafInfo)
		req.SetContext(REQ_CTX_WAF_INFO, info)
	}
	return info
}

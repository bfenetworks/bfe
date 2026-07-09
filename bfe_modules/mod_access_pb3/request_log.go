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
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_util/net_util"
	"google.golang.org/protobuf/proto"

	bfe_access_pb3 "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
)

var (
	BFE_PRODUCT_ID = (*bfe_access_pb3.ProductID)(
		proto.Int32(int32(bfe_access_pb3.ProductID_BFE)))
	LOG_TYPE_REQ = (*bfe_access_pb3.BfeLogType)(
		proto.Int32(int32(bfe_access_pb3.BfeLogType_Request)))
	LOG_TYPE_SESSION = (*bfe_access_pb3.BfeLogType)(
		proto.Int32(int32(bfe_access_pb3.BfeLogType_Session)))
)

// prepare pb log for request
func (m *ModuleAccessPb3) requestLogGen(req *bfe_basic.Request, res *bfe_http.Response) *bfe_access_pb3.BfeLog {
	// create request log
	bfeLog := &bfe_access_pb3.BfeLog{}
	bfeLog.Product = BFE_PRODUCT_ID
	bfeLog.LogType = LOG_TYPE_REQ

	// fill log tag
	bfeLog.LogTag = proto.String(reqLogTagGen(req))

	// log id
	logId, _ := strconv.ParseUint(req.LogId, 10, 64)
	bfeLog.Logid = proto.Uint64(logId)

	// timestamp
	now := time.Now()
	bfeLog.Timestamp = proto.Uint64(uint64(now.Unix()))

	requestLog := new(bfe_access_pb3.RequestLog)
	bfeLog.RequestLog = requestLog

	// basic info
	reqBasicInfoGen(requestLog, req)

	// request header info
	reqReqHeaderInfoGen(requestLog, req)

	// find location info
	reqFindLocationInfoGen(requestLog, req)

	// response info
	reqResponseInfoGen(requestLog, req, res)

	// time info
	reqTimeInfoGen(requestLog, req)

	// AI info
	reqAiInfoGen(requestLog, req, res)

	return bfeLog
}

/*
generate log-tag for request log

- for none-error req, log_tag will be req_<product name>
- for error req, log_tag will be req_err_<product name>
- for error req, if product is empty, log_tag will be req_err_bfe
*/
func reqLogTagGen(req *bfe_basic.Request) string {
	product := req.Route.Product

	// Note: Possible characters in product name are a-z, 0-9, _, -.
	// According to name convertion in biglog platform, valid characters
	// for log tag are a-z, 0-9, _.  We replace "-" with "_" for pb log tag.
	product = strings.Replace(product, "-", "_", -1)

	if req.ErrCode == nil {
		// none error request
		if product != "" {
			return "req_" + product
		} else {
			return "req_bfe"
		}
	} else {
		// error request
		if product != "" {
			return "req_err_" + product
		} else {
			return "req_err_bfe"
		}
	}
}

// basic info
func reqBasicInfoGen(reqLog *bfe_access_pb3.RequestLog, req *bfe_basic.Request) {
	// err coder, err msg
	if req.ErrCode != nil {
		reqLog.ErrCode = proto.String(req.ErrCode.Error())
	} else {
		reqLog.ErrCode = proto.String("")
	}
	reqLog.ErrMsg = proto.String(req.ErrMsg)

	// length
	reqLog.ReqHeaderLen = proto.Uint32(uint32(req.Stat.HeaderLenIn))
	reqLog.ReqBodyLen = proto.Uint32(uint32(req.Stat.BodyLenIn))

	// session id
	sessionId, _ := strconv.ParseUint(req.Session.SessionId, 10, 64)
	reqLog.SessionId = proto.Uint64(sessionId)

	// connection addr info
	reqLog.AddrInfo = ConnAddrInfoGen(req.Session)

	// client src ip
	reqLog.ClientIp = proto.Uint32(0)
	if req.ClientAddr != nil {
		cip4 := req.ClientAddr.IP.To4()
		if cip4 != nil {
			if cip, err := net_util.IPv4ToUint32(cip4); err == nil {
				reqLog.ClientIp = proto.Uint32(cip)
			}
		} else {
			reqLog.ClientIp6 = proto.String(req.ClientAddr.IP.String())
		}
	}

	reqClientNetworkGen(reqLog, req)

	// sequence number of request in the session. start from 1
	reqNum := uint32(0) // unknown sequence number
	if req.HttpRequest.State != nil {
		reqNum = req.HttpRequest.State.SerialNumber
	}
	reqLog.ReqNum = proto.Uint32(reqNum)
}

func reqClientNetworkGen(reqLog *bfe_access_pb3.RequestLog, req *bfe_basic.Request) {
	reqLog.ClientNetwork = (*bfe_access_pb3.NetType)(proto.Int32(int32(bfe_access_pb3.NetType_Ipv4)))
	if req.Session.TrustSource() {
		if req.HttpRequest.Header.Get("Clientip6") != "" {
			reqLog.ClientNetwork = (*bfe_access_pb3.NetType)(proto.Int32(int32(bfe_access_pb3.NetType_Ipv6)))
		}

	} else {
		if isV6Addr(req.RemoteAddr.IP) {
			reqLog.ClientNetwork = (*bfe_access_pb3.NetType)(proto.Int32(int32(bfe_access_pb3.NetType_Ipv6)))
		}
	}
}

func isV6Addr(ip net.IP) bool {
	return ip.To4() == nil
}

// request header info
func reqReqHeaderInfoGen(reqLog *bfe_access_pb3.RequestLog, req *bfe_basic.Request) {
	// protocol
	reqLog.Proto = proto.String(req.Protocol())
	if req.Session.TrustSource() {
		if p := req.HttpRequest.Header.Get("X-Bfe-Proto"); len(p) > 0 {
			reqLog.Proto = proto.String(p)
		}
	}

	// header host
	reqLog.HeaderHost = proto.String(req.HttpRequest.Host)

	// origin uri
	reqLog.OriginUri = proto.String(req.HttpRequest.RequestURI)

	// final uri
	uri := req.HttpRequest.URL.RequestURI()
	if uri != req.HttpRequest.RequestURI {
		// save to final uri, only when uri is not equal to origin uri
		reqLog.FinalUri = proto.String(uri)
	}

	// referrer
	values, found := req.HttpRequest.Header["Referer"]
	if found {
		data := strings.Join(values, ",")
		reqLog.Referrer = proto.String(data)
	}

	// X-Forwarded-For
	values, found = req.HttpRequest.Header["X-Forwarded-For"]
	if found {
		data := strings.Join(values, ",")
		reqLog.XForwardFor = proto.String(data)
	}

	// Accept-Language
	values, found = req.HttpRequest.Header["Accept-Language"]
	if found {
		data := strings.Join(values, ",")
		reqLog.AcceptLanguage = proto.String(data)
	}

	// Authorization
	values, found = req.HttpRequest.Header["Authorization"]
	if found {
		data := strings.Join(values, ",")
		reqLog.Authorization = proto.String(data)
	}

	// User-Agent
	values, found = req.HttpRequest.Header["User-Agent"]
	if found {
		data := strings.Join(values, ",")
		reqLog.UserAgent = proto.String(data)
	}

	// Transfer-Encoding
	if values, found := req.HttpRequest.Header["Transfer-Encoding"]; found {
		data := strings.Join(values, ",")
		reqLog.TransferEncoding = proto.String(data)
	}

	// Method
	reqLog.Method = proto.String(req.HttpRequest.Method)
}

// find location info
func reqFindLocationInfoGen(reqLog *bfe_access_pb3.RequestLog, req *bfe_basic.Request) {
	// product
	reqLog.Product = proto.String(req.Route.Product)

	// cluster
	reqLog.Cluster = proto.String(req.Backend.ClusterName)

	// sub-cluster
	reqLog.SubCluster = proto.String(req.Backend.SubclusterName)

	// backend server ip
	// convert addr from string(e.g., 127.0.0.1) to uint32
	addr := net.ParseIP(req.Backend.BackendAddr)
	if addr != nil {
		ip, err := net_util.IPv4ToUint32(addr.To4())
		if err == nil {
			// backend info
			info := new(bfe_access_pb3.InstanceInfo)

			info.IpAddr = proto.Uint32(ip)
			info.Port = proto.Uint32(req.Backend.BackendPort)

			reqLog.BackendInfo = info
		}
	}

	// time of retry
	reqLog.BackendRetry = proto.Uint32(uint32(req.RetryTime))
}

// response info
func reqResponseInfoGen(reqLog *bfe_access_pb3.RequestLog, req *bfe_basic.Request,
	res *bfe_http.Response) {
	if res == nil {
		return
	}

	// http status code of response
	reqLog.ResStatusCode = proto.Uint32(uint32(res.StatusCode))

	// length of response header
	reqLog.ResHeaderLen = proto.Uint32(uint32(req.Stat.HeaderLenOut))

	// length of response body
	reqLog.ResBodyLen = proto.Uint32(uint32(req.Stat.BodyLenOut))

	// Content-Type in response header
	if values, found := res.Header["Content-Type"]; found {
		data := strings.Join(values, ",")
		reqLog.ResContentType = proto.String(data)
	}

	// Location in response header
	if values, found := res.Header["Location"]; found {
		data := strings.Join(values, ",")
		reqLog.ResLocation = proto.String(data)
	}

	// Transfer-Encoding in response header
	if values, found := res.Header["Transfer-Encoding"]; found {
		data := strings.Join(values, ",")
		reqLog.ResTransferEncoding = proto.String(data)
	}
}

// time info
func reqTimeInfoGen(reqLog *bfe_access_pb3.RequestLog, req *bfe_basic.Request) {
	now := time.Now()

	// time duration of request (in ms)
	// start: start read request, end: finish send response
	ms := now.Sub(req.Stat.ReadReqStart).Nanoseconds() / 1000000
	reqLog.AllTime = proto.Uint32(uint32(ms))

	// time duration of read request from client(in ms)
	// start: start read request, end: finish read request
	ms = req.Stat.ReadReqEnd.Sub(req.Stat.ReadReqStart).Nanoseconds() / 1000000
	reqLog.ReadClientTime = proto.Uint32(uint32(ms))

	// time duration of serve request by cluster (in ms) (include retry time)
	// start: connect to cluster, end: get response from cluster
	ms = req.Stat.ClusterEnd.Sub(req.Stat.ClusterStart).Nanoseconds() / 1000000
	reqLog.ClusterServeTime = proto.Uint32(uint32(ms))

	// time duration of serve request by backend (in ms) (if retry many times, it is last retry)
	// start: connect to backend, end: get response from backend
	ms = req.Stat.BackendEnd.Sub(req.Stat.BackendStart).Nanoseconds() / 1000000
	reqLog.BackendServeTime = proto.Uint32(uint32(ms))

	// time duration of write response to client(in ms)
	// start: start send response to client, end: finish send response
	ms = req.Stat.ResponseEnd.Sub(req.Stat.ResponseStart).Nanoseconds() / 1000000
	reqLog.WriteClientTime = proto.Uint32(uint32(ms))

	// time offset from start time of session (in ms)
	// start: start of session, end: finish send response
	ms = req.Stat.ResponseEnd.Sub(req.Session.StartTime).Nanoseconds() / 1000000
	reqLog.SessionOffsetTime = proto.Uint32(uint32(ms))

	// time duration: connect backend time(in ms)
	// start: connect to backend, end: connection established or got an idle connection
	// only record last connection time if request retry
	ct := uint32(0) // unknown sequence number
	if req.OutRequest != nil && req.OutRequest.State != nil {
		state := req.OutRequest.State
		if ms := state.ConnectBackendEnd.Sub(state.ConnectBackendStart).Nanoseconds() / 1000000; ms >= 0 {
			ct = uint32(ms)
		}
	}

	reqLog.ConnectBackendTime = proto.Uint32(ct)

	// proxy delay(in ms)
	ms = req.Stat.BackendFirst.Sub(req.Stat.ReadReqEnd).Nanoseconds() / 1000000
	reqLog.ProxyDelayTime = proto.Uint32(uint32(ms))
}

// isStreamResponse checks if the response is a streaming response
func isStreamResponse(req *bfe_basic.Request, res *bfe_http.Response) bool {
	return req.IsSse
}

// AI observability info
func reqAiInfoGen(reqLog *bfe_access_pb3.RequestLog, req *bfe_basic.Request, res *bfe_http.Response) {
	aiInfo := req.GetAiBasicInfo()
	if aiInfo == nil {
		return
	}

	// API Key
	if aiInfo.ClientApiKey != "" {
		reqLog.AiApikey = proto.String(aiInfo.ClientApiKey)
	}

	// API Key Tags
	if len(aiInfo.ApikeyTags) > 0 {
		for _, tag := range aiInfo.ApikeyTags {
			reqLog.AiApikeytags = append(reqLog.AiApikeytags, &bfe_access_pb3.ApikeyTag{
				Tagname:  proto.String(tag.TagName),
				Tagvalue: proto.String(tag.TagValue),
			})
		}
	}

	// Model
	if aiInfo.ClientModel != "" {
		reqLog.AiRequestedModel = proto.String(aiInfo.ClientModel)
	}
	if aiInfo.TargetModel != "" {
		reqLog.AiMappedModel = proto.String(aiInfo.TargetModel)
	}

	// Stream
	reqLog.AiStream = proto.Bool(isStreamResponse(req, res))

	// Token usage
	usage := aiInfo.GetTokenUsage()
	if usage != nil {
		reqLog.AiPromptTokens = proto.Int64(usage.PromptTokens)
		reqLog.AiOutputTokens = proto.Int64(usage.CompletionTokens)
		reqLog.AiTotalTokens = proto.Int64(usage.UsedQuota)
	}

	// TTFT / TPOT
	ti := aiInfo.TokenTimeInfo
	if ti.TTFT > 0 {
		reqLog.AiTtftUs = proto.Int64(ti.TTFT)
	}
	if ti.TPOT > 0 {
		reqLog.AiTpotUs = proto.Int64(ti.TPOT)
	}

	if len(aiInfo.AiAuthInfo.RejectReason) > 0 {
		reqLog.AiAuthRejectReason = proto.String(aiInfo.AiAuthInfo.RejectReason)
	}
	for _, item := range aiInfo.AiAuthInfo.RejectQuotaPlans {
		reqLog.AiAuthRejectQuotaPlans = append(reqLog.AiAuthRejectQuotaPlans, item)
	}

	// Rate limit hit info
	hitInfo := req.GetAiRateLimitHitInfo()
	if hitInfo != nil && len(hitInfo.HitPolicyDict) > 0 {
		for policyId, info := range hitInfo.HitPolicyDict {
			// TPM hit
			if len(info.TpmRules) > 0 {
				reqLog.AiRateLimitHits = append(reqLog.AiRateLimitHits, &bfe_access_pb3.RateLimitHit{
					RateLimitPolicyId: proto.String(policyId),
					RateLimitType:     proto.String("tpm"),
					RuleNames:         info.TpmRules,
				})
			}
			// RPM hit
			if len(info.RpmRules) > 0 {
				reqLog.AiRateLimitHits = append(reqLog.AiRateLimitHits, &bfe_access_pb3.RateLimitHit{
					RateLimitPolicyId: proto.String(policyId),
					RateLimitType:     proto.String("rpm"),
					RuleNames:         info.RpmRules,
				})
			}
			// Concurrency hit
			if info.IsConcurrency {
				reqLog.AiRateLimitHits = append(reqLog.AiRateLimitHits, &bfe_access_pb3.RateLimitHit{
					RateLimitPolicyId: proto.String(policyId),
					RateLimitType:     proto.String("concurrency"),
					RuleNames:         []string{"concurrency"},
				})
			}
			// Redis error
			if info.IsRedisError != "" {
				reqLog.AiRateLimitHits = append(reqLog.AiRateLimitHits, &bfe_access_pb3.RateLimitHit{
					RateLimitPolicyId: proto.String(policyId),
					RateLimitType:     proto.String("redis_error"),
					RuleNames:         []string{"acc_redis_error"},
				})
			}
		}
	}
}

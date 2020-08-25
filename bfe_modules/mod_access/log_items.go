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

const (
	FormatAllServeTime = iota
	FormatRequestTime
	FormatRequestLine
	FormatBackend
	FormatReqBodyLen
	FormatResBodyLen
	FormatClusterDuration
	FormatClusterName
	FormatConnectTime
	FormatReqHeaderLen
	FormatHost
	FormatIsTrustIP
	FormatLastBackendDuration
	FormatLogID
	FormatNthReqInSession
	FormatResHeaderLen
	FormatStatusCode
	FormatProduct
	FormatProxyDelayTime
	FormatReadReqDuration
	FormatReadWriteSrvTime
	FormatRedirect
	FormatRemoteAddr
	FormatReqCookie
	FormatReqErrorCode
	FormatReqHeader
	FormatReqURI
	FormatResCookie
	FormatResHeader
	FormatResDuration
	FormatResProto
	FormatResStatus
	FormatResLen
	FormatRetryNum
	FormatServerAddr
	FormatSinceSessionTime
	FormatSubclusterName
	FormatTime
	FormatURL
	FormatVIP
	FormatWriteServeTime
	FormatString

	FormatSesClientIP
	FormatSesEndTime
	FormatSesErrorCode
	FormatSesIsSecure
	FormatSesKeepaliveNum
	FormatSesOverHead
	FormatSesReadTotal
	FormatSesTLSClientRandom
	FormatSesTLSServerRandom
	FormatSesUse100
	FormatSesWriteTotal
	FormatSesStartTime
)

const (
	Request   = "Request"
	Session   = "Session"
	DomainAll = "DomainAll"
)

type LogFmtItem struct {
	Key  string
	Type int
}

var (
	fmtTable = map[string]int{
		"time": FormatTime,

		"all_time":              FormatAllServeTime,
		"request_time":          FormatRequestTime,
		"request_line":          FormatRequestLine,
		"backend":               FormatBackend,
		"req_body_len":          FormatReqBodyLen,
		"res_body_len":          FormatResBodyLen,
		"cluster_name":          FormatClusterName,
		"cluster_duration":      FormatClusterDuration,
		"connect_time":          FormatConnectTime,
		"error":                 FormatReqErrorCode,
		"host":                  FormatHost,
		"is_trust_clientip":     FormatIsTrustIP,
		"last_backend_duration": FormatLastBackendDuration,
		"log_id":                FormatLogID,
		"product":               FormatProduct,
		"proxy_delay":           FormatProxyDelayTime,
		"read_req_duration":     FormatReadReqDuration,
		"readwrite_serve_time":  FormatReadWriteSrvTime,
		"redirect":              FormatRedirect,
		"remote_addr":           FormatRemoteAddr,
		"req_cookie":            FormatReqCookie,
		"req_header":            FormatReqHeader,
		"req_nth":               FormatNthReqInSession,
		"req_uri":               FormatReqURI,
		"res_cookie":            FormatResCookie,
		"res_header":            FormatResHeader,
		"res_proto":             FormatResProto,
		"res_len":               FormatResLen,
		"response_duration":     FormatResDuration,
		"retry_num":             FormatRetryNum,
		"server_addr":           FormatServerAddr,
		"since_ses_start_time":  FormatSinceSessionTime,
		"status_code":           FormatStatusCode,
		"subcluster":            FormatSubclusterName,
		"url":                   FormatURL,
		"vip":                   FormatVIP,
		"write_serve_time":      FormatWriteServeTime,

		"ses_clientip":          FormatSesClientIP,
		"ses_end_time":          FormatSesEndTime,
		"ses_error":             FormatSesErrorCode,
		"ses_is_secure":         FormatSesIsSecure,
		"ses_overhead":          FormatSesOverHead,
		"ses_read_total":        FormatSesReadTotal,
		"ses_start_time":        FormatSesStartTime,
		"ses_tls_client_random": FormatSesTLSClientRandom,
		"ses_tls_server_random": FormatSesTLSServerRandom,
		"ses_use100":            FormatSesUse100,
		"ses_write_total":       FormatSesWriteTotal,
		"ses_keepalive_num":     FormatSesKeepaliveNum,
	}

	fmtItemDomainTable = map[int]string{
		FormatString: DomainAll,
		FormatTime:   DomainAll,

		FormatAllServeTime:        Request,
		FormatRequestTime:         Request,
		FormatRequestLine:         Request,
		FormatBackend:             Request,
		FormatReqBodyLen:          Request,
		FormatResBodyLen:          Request,
		FormatClusterDuration:     Request,
		FormatClusterName:         Request,
		FormatConnectTime:         Request,
		FormatHost:                Request,
		FormatIsTrustIP:           Request,
		FormatLastBackendDuration: Request,
		FormatLogID:               Request,
		FormatNthReqInSession:     Request,
		FormatStatusCode:          Request,
		FormatProduct:             Request,
		FormatProxyDelayTime:      Request,
		FormatReadReqDuration:     Request,
		FormatReadWriteSrvTime:    Request,
		FormatRedirect:            Request,
		FormatRemoteAddr:          Request,
		FormatReqCookie:           Request,
		FormatReqErrorCode:        Request,
		FormatReqHeaderLen:        Request,
		FormatReqHeader:           Request,
		FormatReqURI:              Request,
		FormatResCookie:           Request,
		FormatResDuration:         Request,
		FormatResHeader:           Request,
		FormatResHeaderLen:        Request,
		FormatResProto:            Request,
		FormatResStatus:           Request,
		FormatResLen:              Request,
		FormatRetryNum:            Request,
		FormatServerAddr:          Request,
		FormatSinceSessionTime:    Request,
		FormatSubclusterName:      Request,
		FormatURL:                 Request,
		FormatVIP:                 Request,
		FormatWriteServeTime:      Request,

		FormatSesClientIP:        Session,
		FormatSesEndTime:         Session,
		FormatSesErrorCode:       Session,
		FormatSesIsSecure:        Session,
		FormatSesOverHead:        Session,
		FormatSesReadTotal:       Session,
		FormatSesStartTime:       Session,
		FormatSesTLSClientRandom: Session,
		FormatSesTLSServerRandom: Session,
		FormatSesUse100:          Session,
		FormatSesWriteTotal:      Session,
		FormatSesKeepaliveNum:    Session,
	}

	fmtHandlerTable = map[int]interface{}{
		FormatAllServeTime:        onLogFmtAllServeTime,
		FormatRequestTime:         onLogFmtRequestTime,
		FormatRequestLine:         onLogFmtRequestLine,
		FormatBackend:             onLogFmtBackend,
		FormatReqBodyLen:          onLogFmtReqBodyLen,
		FormatResBodyLen:          onLogFmtResBodyLen,
		FormatClusterDuration:     onLogFmtClusterDuration,
		FormatClusterName:         onLogFmtClusterName,
		FormatConnectTime:         onLogFmtConnectBackendTime,
		FormatIsTrustIP:           onLogFmtIsTrustip,
		FormatLastBackendDuration: onLogFmtLastBackendDuration,
		FormatLogID:               onLogFmtLogId,
		FormatNthReqInSession:     onLogFmtNthReqInSession,
		FormatProduct:             onLogFmtProduct,
		FormatProxyDelayTime:      onLogFmtProxyDelayTime,
		FormatReadReqDuration:     onLogFmtReadReqDuration,
		FormatReadWriteSrvTime:    onLogFmtReadWriteSrvTime,
		FormatRedirect:            onLogFmtRedirect,
		FormatRemoteAddr:          onLogFmtClientIp,
		FormatReqCookie:           onLogFmtReqCookie,
		FormatReqErrorCode:        onLogFmtErrorCode,
		FormatReqHeader:           onLogFmtRequestHeader,
		FormatReqHeaderLen:        onLogFmtReqHeaderLen,
		FormatReqURI:              onLogFmtRequestUri,
		FormatResCookie:           onLogFmtResCookie,
		FormatResHeader:           onLogFmtResponseHeader,
		FormatResHeaderLen:        onLogFmtResHeaderLen,
		FormatResDuration:         onLogFmtResDuration,
		FormatResProto:            onLogFmtResProto,
		FormatResStatus:           onLogFmtResStatus,
		FormatResLen:              onLogFmtResLen,
		FormatRetryNum:            onLogFmtRetryNum,
		FormatServerAddr:          onLogFmtServerAddr,
		FormatSinceSessionTime:    onLogFmtSinceSessionTime,
		FormatStatusCode:          onLogFmtStatusCode,
		FormatSubclusterName:      onLogFmtSubclusterName,
		FormatURL:                 onLogFmtUrl,
		FormatVIP:                 onLogFmtVip,
		FormatWriteServeTime:      onLogFmtWriteSrvTime,
		FormatHost:                onLogFmtHost,

		FormatSesClientIP:        onLogFmtSesClientIp,
		FormatSesEndTime:         onLogFmtSesEndTime,
		FormatSesErrorCode:       onLogFmtSesErrorCode,
		FormatSesIsSecure:        onLogFmtSesIsSecure,
		FormatSesKeepaliveNum:    onLogFmtSesKeepAliveNum,
		FormatSesOverHead:        onLogFmtSesOverhead,
		FormatSesReadTotal:       onLogFmtSesReadTotal,
		FormatSesTLSClientRandom: onLogFmtSesTLSClientRandom,
		FormatSesTLSServerRandom: onLogFmtSesTLSServerRandom,
		FormatSesUse100:          onLogFmtSesUse100,
		FormatSesWriteTotal:      onLogFmtSesWriteTotal,
		FormatSesStartTime:       onLogFmtSesStartTime,
	}
)

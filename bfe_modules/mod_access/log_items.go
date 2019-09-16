// Copyright (c) 2019 Baidu, Inc.
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

// all format types
const (
	FormatAllServeTime = iota
	FormatBackend
	FormatBodyLenIn
	FormatBodyLenOut
	FormatClientReadTime
	FormatClusterDuration
	FormatClusterName
	FormatClusterServeTime
	FormatConnectTime
	FormatReqContentLen
	FormatReqHeaderLen
	FormatHost
	FormatHTTP
	FormatIsTrustIP
	FormatLastBackendDuration
	FormatLogID
	FormatNthReqInSession
	FormatResContentLen
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
	FormatRetryNum
	FormatServerAddr
	FormatSinceSessionTime
	FormatSubclusterName
	FormatTime
	FormatURI
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
	// each format item should be in one of following template place
	Request   = "Request"
	Session   = "Session"
	DomainAll = "DomainAll"
)

// each log format in log template
type LogFmtItem struct {
	Key  string
	Type int
}

var (
	// table of format string => format type mapping
	fmtTable map[string]int

	// table of formate type => item valid template
	fmtItemDomainTable map[int]string

	// table of format type => format print handler mapping
	fmtHandlerTable map[int]interface{}
)

// setup fmtTable, fmtItemDomainTable and fmtHandlerTable
func init() {
	fmtTable = map[string]int{
		"time": FormatTime,

		"all_time":              FormatAllServeTime,
		"backend":               FormatBackend,
		"body_len_in":           FormatBodyLenIn,
		"body_len_out":          FormatBodyLenOut,
		"client_read_time":      FormatClientReadTime,
		"cluster_name":          FormatClusterName,
		"cluster_duration":      FormatClusterDuration,
		"cluster_time":          FormatClusterServeTime,
		"connect_time":          FormatConnectTime,
		"error":                 FormatReqErrorCode,
		"host":                  FormatHost,
		"http":                  FormatHTTP,
		"is_trust_clientip":     FormatIsTrustIP,
		"last_backend_duration": FormatLastBackendDuration,
		"log_id":                FormatLogID,
		"product":               FormatProduct,
		"proxy_delay":           FormatProxyDelayTime,
		"read_req_duration":     FormatReadReqDuration,
		"readwrite_serve_time":  FormatReadWriteSrvTime,
		"redirect":              FormatRedirect,
		"remote_addr":           FormatRemoteAddr,
		"req_content_len":       FormatReqContentLen,
		"req_cookie":            FormatReqCookie,
		"req_header":            FormatReqHeader,
		"req_nth":               FormatNthReqInSession,
		"req_uri":               FormatReqURI,
		"res_content_len":       FormatResContentLen,
		"res_cookie":            FormatResCookie,
		"res_header":            FormatResHeader,
		"res_proto":             FormatResProto,
		"response_duration":     FormatResDuration,
		"retry_num":             FormatRetryNum,
		"server_addr":           FormatServerAddr,
		"since_ses_start_time":  FormatSinceSessionTime,
		"status_code":           FormatStatusCode,
		"subcluster":            FormatSubclusterName,
		"uri":                   FormatURI,
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
		FormatBackend:             Request,
		FormatBodyLenIn:           Request,
		FormatBodyLenOut:          Request,
		FormatClientReadTime:      Request,
		FormatClusterDuration:     Request,
		FormatClusterName:         Request,
		FormatClusterServeTime:    Request,
		FormatConnectTime:         Request,
		FormatReqContentLen:       Request,
		FormatHost:                Request,
		FormatHTTP:                Request,
		FormatIsTrustIP:           Request,
		FormatLastBackendDuration: Request,
		FormatLogID:               Request,
		FormatNthReqInSession:     Request,
		FormatResContentLen:       Request,
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
		FormatRetryNum:            Request,
		FormatServerAddr:          Request,
		FormatSinceSessionTime:    Request,
		FormatSubclusterName:      Request,
		FormatURI:                 Request,
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
		FormatBackend:             onLogFmtBackend,
		FormatBodyLenIn:           onLogFmtBodyLenIn,
		FormatBodyLenOut:          onLogFmtBodyLenOut,
		FormatClientReadTime:      onLogFmtClientReadTime,
		FormatClusterDuration:     onLogFmtClusterDuration,
		FormatClusterName:         onLogFmtClusterName,
		FormatClusterServeTime:    onLogFmtClusterServeTime,
		FormatConnectTime:         onLogFmtConnectBackendTime,
		FormatHTTP:                onLogFmtHttp,
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
		FormatReqContentLen:       onLogFmtReqContentLen,
		FormatReqErrorCode:        onLogFmtErrorCode,
		FormatReqHeader:           onLogFmtRequestHeader,
		FormatReqHeaderLen:        onLogFmtReqHeaderLen,
		FormatReqURI:              onLogFmtRequestUri,
		FormatResContentLen:       onLogFmtResContentLen,
		FormatResCookie:           onLogFmtResCookie,
		FormatResHeader:           onLogFmtResponseHeader,
		FormatResHeaderLen:        onLogFmtResHeaderLen,
		FormatResDuration:         onLogFmtResDuration,
		FormatResProto:            onLogFmtResProto,
		FormatResStatus:           onLogFmtResStatus,
		FormatRetryNum:            onLogFmtRetryNum,
		FormatServerAddr:          onLogFmtServerAddr,
		FormatSinceSessionTime:    onLogFmtSinceSessionTime,
		FormatStatusCode:          onLogFmtStatusCode,
		FormatSubclusterName:      onLogFmtSubclusterName,
		FormatURI:                 onLogFmtUri,
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
}

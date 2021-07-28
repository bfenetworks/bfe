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

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func onLogFmtAllServeTime(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.Stat == nil {
		return errors.New("req.Stat is nil")
	}

	now := time.Now()
	ms := now.Sub(req.Stat.ReadReqStart).Nanoseconds() / 1000000
	msg := fmt.Sprintf("%d", ms)
	buff.WriteString(msg)

	return nil
}

func onLogFmtRequestTime(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.Stat == nil {
		return errors.New("req.Stat is nil")
	}

	t := req.Stat.ReadReqStart.Format("[02/Jan/2006:15:04:05 -0700]")
	buff.WriteString(t)
	return nil
}

func onLogFmtRequestLine(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.HttpRequest == nil {
		return errors.New("req.HttpRequest is nil")
	}

	l := fmt.Sprintf("%s %s %s", req.HttpRequest.Method, req.HttpRequest.RequestURI, req.HttpRequest.Proto)
	buff.WriteString(l)
	return nil
}

func onLogFmtBackend(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := fmt.Sprintf("%s,%s,%s,%s", req.Backend.ClusterName, req.Backend.SubclusterName,
		req.Backend.BackendAddr, req.Backend.BackendName)
	buff.WriteString(msg)

	return nil
}

func onLogFmtReqBodyLen(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.Stat == nil {
		return errors.New("req.Stat is nil")
	}

	msg := fmt.Sprintf("%d", req.Stat.BodyLenIn)
	buff.WriteString(msg)

	return nil
}

func onLogFmtResBodyLen(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.Stat == nil {
		return errors.New("req.Stat is nil")
	}

	msg := fmt.Sprintf("%d", req.Stat.BodyLenOut)
	buff.WriteString(msg)

	return nil
}

func onLogFmtClusterDuration(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.Stat == nil {
		return errors.New("req.Stat is nil")
	}

	ms := req.Stat.ClusterEnd.Sub(req.Stat.ClusterStart).Nanoseconds() / 1000000
	msg := fmt.Sprintf("%d", ms)
	buff.WriteString(msg)

	return nil
}

func onLogFmtClusterName(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := "-"
	if req.Backend.ClusterName != "" {
		msg = req.Backend.ClusterName
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtConnectBackendTime(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil || req.OutRequest == nil {
		return errors.New("req is nil")
	}
	if req.OutRequest.State == nil {
		return errors.New("req.OutRequest.State is nil")
	}

	msg := "-"
	stat := req.OutRequest.State
	ms := stat.ConnectBackendEnd.Sub(stat.ConnectBackendStart).Nanoseconds() / 1000000
	if ms >= 0 {
		msg = fmt.Sprintf("%d", ms)
	}

	buff.WriteString(msg)

	return nil
}

func onLogFmtHost(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.HttpRequest == nil {
		return errors.New("req.HttpRequest is nil")
	}

	buff.WriteString(req.HttpRequest.Host)

	return nil
}

func onLogFmtIsTrustip(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := fmt.Sprintf("%v", req.Session.TrustSource())
	buff.WriteString(msg)

	return nil
}

func onLogFmtLastBackendDuration(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.Stat == nil {
		return errors.New("req.Stat is nil")
	}

	ms := req.Stat.BackendEnd.Sub(req.Stat.BackendStart).Nanoseconds() / 1000000
	msg := fmt.Sprintf("%d", ms)
	buff.WriteString(msg)

	return nil
}

func onLogFmtLogId(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := req.LogId
	buff.WriteString(msg)

	return nil
}

func onLogFmtNthReqInSession(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := "-"
	if req.HttpRequest != nil && req.HttpRequest.State != nil {
		msg = fmt.Sprintf("%d", req.HttpRequest.State.SerialNumber)
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtStatusCode(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := "-"
	if res != nil {
		msg = fmt.Sprintf("%d", res.StatusCode)
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtProduct(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := "-"
	if req.Route.Product != "" {
		msg = req.Route.Product
	}

	buff.WriteString(msg)

	return nil
}

func onLogFmtProxyDelayTime(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.Stat == nil {
		return errors.New("req.Stat is nil")
	}

	msg := "-"
	if !req.Stat.BackendFirst.IsZero() {
		ms := req.Stat.BackendFirst.Sub(req.Stat.ReadReqEnd).Nanoseconds() / 1000000
		msg = fmt.Sprintf("%d", ms)
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtReadReqDuration(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.Stat == nil {
		return errors.New("req.Stat is nil")
	}

	ms := req.Stat.ReadReqEnd.Sub(req.Stat.ReadReqStart).Nanoseconds() / 1000000
	msg := fmt.Sprintf("%d", ms)
	buff.WriteString(msg)

	return nil
}

func onLogFmtReadWriteSrvTime(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.Stat == nil {
		return errors.New("req.Stat is nil")
	}

	msg := "-"
	if !req.Stat.BackendStart.IsZero() {
		now := time.Now()
		ms := now.Sub(req.Stat.BackendStart).Nanoseconds() / 1000000
		msg = fmt.Sprintf("%d", ms)
	}

	buff.WriteString(msg)

	return nil
}

func onLogFmtRedirect(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := "-"
	if req.Redirect.Url != "" {
		msg = fmt.Sprintf("%s,%d", req.Redirect.Url, req.Redirect.Code)
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtClientIp(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := "-"
	if req.RemoteAddr != nil {
		msg = req.RemoteAddr.IP.String()
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtReqCookie(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := "-"
	if co, ok := req.Cookie(logItem.Key); ok {
		msg = co.Value
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtErrorCode(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := buildErrorMsg(req.ErrCode, req.ErrMsg)
	buff.WriteString(msg)

	return nil
}

func onLogFmtReqHeaderLen(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.Stat == nil {
		return errors.New("req.Stat is nil")
	}

	msg := fmt.Sprintf("%d", req.Stat.HeaderLenIn)
	buff.WriteString(msg)

	return nil
}

func onLogFmtRequestHeader(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.HttpRequest == nil {
		return errors.New("req.HttpRequest is nil")
	}

	msg := "-"
	if data := req.HttpRequest.Header.Get(logItem.Key); data != "" {
		msg = data
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtRequestUri(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.HttpRequest == nil {
		return errors.New("req.HttpRequest is nil")
	}

	buff.WriteString(req.HttpRequest.RequestURI)

	return nil
}

func onLogFmtResCookie(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	if res == nil {
		buff.WriteString("-")
		return nil
	}

	msg := "-"
	cookies := res.Cookies()
	for _, co := range cookies {
		if co.Name == logItem.Key {
			msg = co.Value
			break
		}
	}

	buff.WriteString(msg)

	return nil
}

func onLogFmtResDuration(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.Stat == nil {
		return errors.New("req.Stat is nil")
	}

	ms := req.Stat.ResponseEnd.Sub(req.Stat.ResponseStart).Nanoseconds() / 1000000
	msg := fmt.Sprintf("%d", ms)
	buff.WriteString(msg)

	return nil
}

func onLogFmtResProto(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := "-"
	if res != nil {
		msg = res.Proto
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtResponseHeader(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	if res == nil {
		buff.WriteString("-")
		return nil
	}

	msg := "-"
	data, found := res.Header[logItem.Key]
	if found {
		msg = strings.Join(data, ",")
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtResHeaderLen(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	if req.Stat == nil {
		return errors.New("req.Stat is nil")
	}

	msg := "-"
	if res != nil {
		msg = fmt.Sprintf("%d", req.Stat.HeaderLenOut)
	}

	buff.WriteString(msg)

	return nil
}

func onLogFmtResLen(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.Stat == nil {
		return errors.New("req.Stat is nil")
	}

	msg := "-"
	if res != nil {
		msg = fmt.Sprintf("%d", req.Stat.HeaderLenOut+req.Stat.BodyLenOut)
	}
	buff.WriteString(msg)
	return nil
}

func onLogFmtResStatus(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := "-"
	if res != nil {
		msg = res.Status
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtRetryNum(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := fmt.Sprintf("%d", req.RetryTime)
	buff.WriteString(msg)

	return nil
}

func onLogFmtServerAddr(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := "-"
	if req.Connection != nil {
		msg = req.Connection.LocalAddr().String()
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtSinceSessionTime(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	if req.Session == nil {
		return errors.New("req.Session is nil")
	}

	ms := time.Since(req.Session.StartTime).Nanoseconds() / 1000000
	msg := fmt.Sprintf("%d", ms)
	buff.WriteString(msg)

	return nil
}

func onLogFmtSubclusterName(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	buff.WriteString(req.Backend.SubclusterName)

	return nil
}

func onLogFmtTime(m *ModuleAccess, buff *bytes.Buffer) error {
	now := time.Now()
	timeNowStr := fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d",
		now.Year(), now.Month(), now.Day(),
		now.Hour(), now.Minute(), now.Second())
	buff.WriteString(timeNowStr)

	return nil
}

func onLogFmtVip(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}

	msg := "-"
	if req.Session.Vip != nil {
		if vip := req.Session.Vip.String(); len(vip) != 0 {
			msg = vip
		}
	}
	buff.WriteString(msg)

	return nil
}

func onLogFmtUrl(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.HttpRequest == nil {
		return errors.New("req.HttpRequest is nil")
	}

	buff.WriteString(req.HttpRequest.URL.String())

	return nil
}

func onLogFmtWriteSrvTime(m *ModuleAccess, logItem *LogFmtItem, buff *bytes.Buffer,
	req *bfe_basic.Request, res *bfe_http.Response) error {
	if req == nil {
		return errors.New("req is nil")
	}
	if req.Stat == nil {
		return errors.New("req.Stat is nil")
	}

	ms := req.Stat.BackendEnd.Sub(req.Stat.BackendStart).Nanoseconds() / 1000000
	msg := fmt.Sprintf("%d", ms)
	buff.WriteString(msg)

	return nil
}

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

package proxywasm010

import (
	"bytes"
	"io/ioutil"

	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/proxy-wasm-go-host/proxywasm/common"
	proxywasm "github.com/bfenetworks/proxy-wasm-go-host/proxywasm/v1"
)

type DefaultImportsHandler struct {
	proxywasm.DefaultImportsHandler
	Instance common.WasmInstance
	hc       *httpCallout
}

// override
func (d *DefaultImportsHandler) Log(level proxywasm.LogLevel, msg string) proxywasm.WasmResult {
	logFunc := log.Logger.Info

	switch level {
	case proxywasm.LogLevelTrace:
		logFunc = log.Logger.Trace
	case proxywasm.LogLevelDebug:
		logFunc = log.Logger.Debug
	case proxywasm.LogLevelInfo:
		logFunc = log.Logger.Info
	case proxywasm.LogLevelWarn:
		logFunc = func(arg0 interface{}, args ...interface{}) {
			log.Logger.Warn(arg0, args...)
		}
	case proxywasm.LogLevelError:
		logFunc = func(arg0 interface{}, args ...interface{}) {
			log.Logger.Error(arg0, args...)
		}
	case proxywasm.LogLevelCritical:
		logFunc = func(arg0 interface{}, args ...interface{}) {
			log.Logger.Error(arg0, args...)
		}
	}

	logFunc(msg)

	return proxywasm.WasmResultOk
}

var httpCalloutID int32

type httpCallout struct {
	id         int32
	d          *DefaultImportsHandler
	instance   common.WasmInstance
	abiContext *proxywasm.ABIContext

	urlString  string
	client     *http.Client
	req        *http.Request
	resp       *http.Response
	respHeader common.HeaderMap
	respBody   common.IoBuffer
	reqOnFly   bool
}

// override
func (d *DefaultImportsHandler) HttpCall(reqURL string, header common.HeaderMap, body common.IoBuffer,
	trailer common.HeaderMap, timeoutMilliseconds int32) (int32, proxywasm.WasmResult) {
	u, err := url.Parse(reqURL)
	if err != nil {
		log.Logger.Error("[proxywasm010][imports] HttpCall fail to parse url, err: %v, reqURL: %v", err, reqURL)
		return 0, proxywasm.WasmResultBadArgument
	}

	calloutID := atomic.AddInt32(&httpCalloutID, 1)

	d.hc = &httpCallout{
		id:         calloutID,
		d:          d,
		instance:   d.Instance,
		abiContext: d.Instance.GetData().(*proxywasm.ABIContext),
		urlString:  reqURL,
	}

	d.hc.client = &http.Client{Timeout: time.Millisecond * time.Duration(timeoutMilliseconds)}

	d.hc.req, err = http.NewRequest(http.MethodGet, u.String(), bytes.NewReader(body.Bytes()))
	if err != nil {
		log.Logger.Error("[proxywasm010][imports] HttpCall fail to create http req, err: %v, reqURL: %v", err, reqURL)
		return 0, proxywasm.WasmResultInternalFailure
	}

	header.Range(func(key, value string) bool {
		d.hc.req.Header.Add(key, value)
		return true
	})

	d.hc.reqOnFly = true

	return calloutID, proxywasm.WasmResultOk
}

// override
func (d *DefaultImportsHandler) Wait() proxywasm.Action {
	if d.hc == nil || !d.hc.reqOnFly {
		return proxywasm.ActionContinue
	}

	// release the instance lock and do sync http req
	d.Instance.Unlock()
	resp, err := d.hc.client.Do(d.hc.req)
	d.Instance.Lock(d.hc.abiContext)

	d.hc.reqOnFly = false

	if err != nil {
		log.Logger.Error("[proxywasm010][imports] HttpCall id: %v fail to do http req, err: %v, reqURL: %v",
			d.hc.id, err, d.hc.urlString)
		return proxywasm.ActionPause
	}
	d.hc.resp = resp

	// process http resp header
	// var respHeader common.HeaderMap = protocol.CommonHeader{}
	// for key, _ := range resp.Header {
	// 	respHeader.Set(key, resp.Header.Get(key))
	// }
	d.hc.respHeader = HeaderMapWrapper{Header: bfe_http.Header(resp.Header)}

	// process http resp body
	var respBody common.IoBuffer
	respBodyLen := 0

	respBodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Logger.Error("[proxywasm010][imports] HttpCall id: %v fail to read bytes from resp body, err: %v, reqURL: %v",
			d.hc.id, err, d.hc.urlString)
	}

	err = resp.Body.Close()
	if err != nil {
		log.Logger.Error("[proxywasm010][imports] HttpCall id: %v fail to close resp body, err: %v, reqURL: %v",
			d.hc.id, err, d.hc.urlString)
	}

	if respBodyBytes != nil {
		respBody = common.NewIoBufferBytes(respBodyBytes)
		respBodyLen = respBody.Len()
	}
	d.hc.respBody = respBody

	// call proxy_on_http_call_response
	rootContextID := d.hc.abiContext.Imports.GetRootContextID()
	exports := d.hc.abiContext.GetExports()

	err = exports.ProxyOnHttpCallResponse(rootContextID, d.hc.id, int32(len(resp.Header)), int32(respBodyLen), 0)
	if err != nil {
		log.Logger.Error("[proxywasm010][imports] httpCall id: %v fail to call ProxyOnHttpCallResponse, err: %v", d.hc.id, err)
	}
	return proxywasm.ActionContinue
}

// override
func (d *DefaultImportsHandler) GetHttpCallResponseHeaders() common.HeaderMap {
	if d.hc != nil && d.hc.respHeader != nil {
		return d.hc.respHeader
	}

	return nil
}

// override
func (d *DefaultImportsHandler) GetHttpCallResponseBody() common.IoBuffer {
	if d.hc != nil && d.hc.respBody != nil {
		return d.hc.respBody
	}

	return nil
}

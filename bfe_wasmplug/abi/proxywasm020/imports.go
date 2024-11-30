/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package proxywasm020

import (
	_ "io/ioutil"
	_ "net/http"
	_ "net/url"
	_ "sync/atomic"
	_ "time"

	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/proxy-wasm-go-host/proxywasm/common"
	proxywasm "github.com/bfenetworks/proxy-wasm-go-host/proxywasm/v2"
)

type DefaultImportsHandler struct {
	proxywasm.DefaultImportsHandler
	Instance common.WasmInstance
	//hc       *httpCallout
}

// override
func (d *DefaultImportsHandler) Log(level proxywasm.LogLevel, msg string) proxywasm.Result {
	logFunc := log.Logger.Info

	switch level {
	case proxywasm.LogLevelTrace:
		logFunc = log.Logger.Trace
	case proxywasm.LogLevelDebug:
		logFunc = log.Logger.Debug
	case proxywasm.LogLevelInfo:
		logFunc = log.Logger.Info
	case proxywasm.LogLevelWarning:
		logFunc = func(arg0 interface{}, args ...interface{}) {
			log.Logger.Warn(arg0, args...)
		}
	case proxywasm.LogLevelError:
		logFunc = func(arg0 interface{}, args ...interface{}) {
			log.Logger.Error(arg0, args...)
		}
	}

	logFunc(msg)

	return proxywasm.ResultOk
}

var httpCalloutID int32
/*
type httpCallout struct {
	id         int32
	d          *DefaultImportsHandler
	instance   common.WasmInstance
	abiContext *ABIContext

	urlString   string
	client      *http.Client
	req         *http.Request
	resp        *http.Response
	respHeader  api.HeaderMap
	respBody    buffer.IoBuffer
	respTrailer api.HeaderMap
	reqOnFly    bool
}

// override
func (d *DefaultImportsHandler) DispatchHttpCall(reqURL string, header common.HeaderMap, body common.IoBuffer,
	trailer common.HeaderMap, timeoutMilliseconds uint32) (uint32, proxywasm.Result) {
	u, err := url.Parse(reqURL)
	if err != nil {
		log.DefaultLogger.Errorf("[proxywasm020][imports] HttpCall fail to parse url, err: %v, reqURL: %v", err, reqURL)
		return 0, proxywasm.ResultBadArgument
	}

	calloutID := atomic.AddInt32(&httpCalloutID, 1)

	d.hc = &httpCallout{
		id:         calloutID,
		d:          d,
		instance:   d.Instance,
		abiContext: d.Instance.GetData().(*ABIContext),
		urlString:  reqURL,
	}

	d.hc.client = &http.Client{Timeout: time.Millisecond * time.Duration(timeoutMilliseconds)}

	d.hc.req, err = http.NewRequest(http.MethodGet, u.String(), buffer.NewIoBufferBytes(body.Bytes()))
	if err != nil {
		log.DefaultLogger.Errorf("[proxywasm020][imports] HttpCall fail to create http req, err: %v, reqURL: %v", err, reqURL)
		return 0, proxywasm.ResultInvalidOperation
	}

	if header != nil {
		header.Range(func(key, value string) bool {
			d.hc.req.Header.Add(key, value)
			return true
		})
	}

	if trailer != nil {
		trailer.Range(func(key, value string) bool {
			d.hc.req.Trailer.Add(key, value)
			return true
		})
	}

	d.hc.reqOnFly = true

	return uint32(calloutID), proxywasm.ResultOk
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
		log.DefaultLogger.Errorf("[proxywasm020][imports] HttpCall id: %v fail to do http req, err: %v, reqURL: %v",
			d.hc.id, err, d.hc.urlString)
		return proxywasm.ActionPause
	}
	d.hc.resp = resp

	// parse resp header
	var respHeader api.HeaderMap = protocol.CommonHeader{}
	for key := range resp.Header {
		respHeader.Set(key, resp.Header.Get(key))
	}
	d.hc.respHeader = respHeader

	// parse resp body
	var respBody buffer.IoBuffer
	respBodyLen := 0

	respBodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.DefaultLogger.Errorf("[proxywasm020][imports] HttpCall id: %v fail to read http resp body, err: %v, reqURL: %v",
			d.hc.id, err, d.hc.urlString)
		return proxywasm.ActionPause
	}

	err = resp.Body.Close()
	if err != nil {
		log.DefaultLogger.Errorf("[proxywasm020][imports] HttpCall id: %v fail to close http resp body, err: %v, reqURL: %v",
			d.hc.id, err, d.hc.urlString)
		return proxywasm.ActionPause
	}

	if respBodyBytes != nil {
		respBody = buffer.NewIoBufferBytes(respBodyBytes)
		respBodyLen = respBody.Len()
	}
	d.hc.respBody = respBody

	// parse resp trailer
	var respTrailer api.HeaderMap = protocol.CommonHeader{}
	for key := range resp.Trailer {
		respTrailer.Set(key, resp.Trailer.Get(key))
	}
	d.hc.respTrailer = respTrailer

	// call proxy_on_http_call_response
	exports := d.hc.abiContext.GetExports()

	err = exports.ProxyOnHttpCallResponse(1, d.hc.id, int32(len(resp.Header)), int32(respBodyLen), 0)
	if err != nil {
		log.DefaultLogger.Errorf("[proxywasm020][imports] HttpCall id: %v fail to call proxy_on_http_call_response, err: %v, reqURL: %v",
			d.hc.id, err, d.hc.urlString)
		return proxywasm.ActionPause
	}

	return proxywasm.ActionContinue
}

// override
func (d *DefaultImportsHandler) GetHttpCallResponseHeaders() common.HeaderMap {
	if d.hc != nil && d.hc.respHeader != nil {
		return HeaderMapWrapper{HeaderMap: d.hc.respHeader}
	}

	return nil
}

// override
func (d *DefaultImportsHandler) GetHttpCalloutResponseBody() common.IoBuffer {
	if d.hc != nil && d.hc.respBody != nil {
		return IoBufferWrapper{IoBuffer: d.hc.respBody}
	}

	return nil
}

// override
func (d *DefaultImportsHandler) GetHttpCallResponseTrailer() common.HeaderMap {
	if d.hc != nil && d.hc.respTrailer != nil {
		return HeaderMapWrapper{HeaderMap: d.hc.respTrailer}
	}

	return nil
}
*/
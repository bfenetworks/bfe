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

package bfe_wasmplug

import (
	"bytes"
	"io/ioutil"

	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_wasmplug/abi/proxywasm010"
	"github.com/bfenetworks/proxy-wasm-go-host/proxywasm/common"
	v1Host "github.com/bfenetworks/proxy-wasm-go-host/proxywasm/v1"
)

// v1 Imports
type v1Imports struct {
	proxywasm010.DefaultImportsHandler
	//factory *FilterConfigFactory
	plugin  WasmPlugin
	filter  *Filter
}

func (v1 *v1Imports) GetRootContextID() int32 {
	return v1.plugin.GetRootContextID()
}

func (v1 *v1Imports) GetVmConfig() common.IoBuffer {
	//return v1.factory.GetVmConfig()
	return common.NewIoBufferBytes([]byte{})
}

func (v1 *v1Imports) GetPluginConfig() common.IoBuffer {
	return common.NewIoBufferBytes(v1.plugin.GetPluginConfig())
	//return common.NewIoBufferBytes([]byte{})
}

func (v1 *v1Imports) GetHttpRequestHeader() common.HeaderMap {
	if v1.filter.request == nil {
		return nil
	}

	return &proxywasm010.HeaderMapWrapper{Header: v1.filter.request.HttpRequest.Header}
}

func (v1 *v1Imports) GetHttpRequestBody() common.IoBuffer {
	if v1.filter.request == nil {
		return nil
	}

	return nil
	// return &proxywasm010.IoBufferWrapper{IoBuffer: v1.filter.receiverFilterHandler.GetRequestData()}
}

func (v1 *v1Imports) GetHttpRequestTrailer() common.HeaderMap {
	if v1.filter.request == nil {
		return nil
	}

	return &proxywasm010.HeaderMapWrapper{Header: v1.filter.request.HttpRequest.Trailer}
}

func (v1 *v1Imports) GetHttpResponseHeader() common.HeaderMap {
	if v1.filter.request == nil || v1.filter.request.HttpResponse == nil {
		return nil
	}

	return &proxywasm010.HeaderMapWrapper{Header: v1.filter.request.HttpResponse.Header}
}

func (v1 *v1Imports) GetHttpResponseBody() common.IoBuffer {
	if v1.filter.request == nil || v1.filter.request.HttpResponse == nil {
		return nil
	}

	return nil
	//return &proxywasm010.IoBufferWrapper{IoBuffer: v1.filter.senderFilterHandler.GetResponseData()}
}

func (v1 *v1Imports) GetHttpResponseTrailer() common.HeaderMap {
	if v1.filter.request == nil || v1.filter.request.HttpResponse == nil {
		return nil
	}

	return &proxywasm010.HeaderMapWrapper{Header: v1.filter.request.HttpResponse.Trailer}
}

func (v1 *v1Imports) SendHttpResp(respCode int32, respCodeDetail common.IoBuffer, respBody common.IoBuffer, additionalHeaderMap common.HeaderMap, grpcCode int32) v1Host.WasmResult {
	resp := &bfe_http.Response{
		StatusCode: int(respCode),
		Status: string(respCodeDetail.Bytes()),
		Body: ioutil.NopCloser(bytes.NewReader(respBody.Bytes())),
		Header: make(bfe_http.Header),
	}

	additionalHeaderMap.Range(func(key, value string) bool {
		resp.Header.Add(key, value)
		return true
	})
	
	v1.filter.request.HttpResponse = resp
	return v1Host.WasmResultOk
}

// exports
type exportAdapter struct {
	v1 v1Host.Exports
}

func (e *exportAdapter) ProxyOnContextCreate(contextID int32, parentContextID int32) error {
		return e.v1.ProxyOnContextCreate(contextID, parentContextID)
}

func (e *exportAdapter) ProxyOnVmStart(rootContextID int32, vmConfigurationSize int32) (int32, error) {
		return e.v1.ProxyOnVmStart(rootContextID, vmConfigurationSize)
}

func (e *exportAdapter) ProxyOnConfigure(rootContextID int32, pluginConfigurationSize int32) (int32, error) {
		return e.v1.ProxyOnConfigure(rootContextID, pluginConfigurationSize)
}

func (e *exportAdapter) ProxyOnDone(contextID int32) (int32, error) {
		return e.v1.ProxyOnDone(contextID)
}

func (e *exportAdapter) ProxyOnDelete(contextID int32) error {
		return e.v1.ProxyOnDelete(contextID)
}

func (e *exportAdapter) ProxyOnRequestHeaders(contextID int32, headers int32, endOfStream int32) int {
		action, err := e.v1.ProxyOnRequestHeaders(contextID, headers, endOfStream)
		if err != nil || action != v1Host.ActionContinue {
			log.Logger.Error("[proxywasm][filter][v1] ProxyOnRequestHeaders action: %v, err: %v", action, err)
			return bfe_module.BfeHandlerResponse
		}

	return bfe_module.BfeHandlerGoOn
}

func (e *exportAdapter) ProxyOnRequestBody(contextID int32, bodyBufferLength int32, endOfStream int32) int {
		action, err := e.v1.ProxyOnRequestBody(contextID, bodyBufferLength, endOfStream)
		if err != nil || action != v1Host.ActionContinue {
			log.Logger.Error("[proxywasm][filter][v1] ProxyOnRequestBody action: %v, err: %v", action, err)
			return bfe_module.BfeHandlerResponse
		}

	return bfe_module.BfeHandlerGoOn
}

func (e *exportAdapter) ProxyOnRequestTrailers(contextID int32, trailers int32, endOfStream int32) int {
		action, err := e.v1.ProxyOnRequestTrailers(contextID, trailers)
		if err != nil || action != v1Host.ActionContinue {
			log.Logger.Error("[proxywasm][filter][v1] ProxyOnRequestTrailers action: %v, err: %v", action, err)
			return bfe_module.BfeHandlerResponse
		}

	return bfe_module.BfeHandlerGoOn
}

func (e *exportAdapter) ProxyOnResponseHeaders(contextID int32, headers int32, endOfStream int32) int {
		action, err := e.v1.ProxyOnResponseHeaders(contextID, headers, endOfStream)
		if err != nil || action != v1Host.ActionContinue {
			log.Logger.Error("[proxywasm][filter][v1] ProxyOnResponseHeaders action: %v, err: %v", action, err)
			return bfe_module.BfeHandlerResponse
		}

	return bfe_module.BfeHandlerGoOn
}

func (e *exportAdapter) ProxyOnResponseBody(contextID int32, bodyBufferLength int32, endOfStream int32) int {
		action, err := e.v1.ProxyOnResponseBody(contextID, bodyBufferLength, endOfStream)
		if err != nil || action != v1Host.ActionContinue {
			log.Logger.Error("[proxywasm][filter][v1] ProxyOnRequestBody action: %v, err: %v", action, err)
			return bfe_module.BfeHandlerResponse
		}

	return bfe_module.BfeHandlerGoOn
}

func (e *exportAdapter) ProxyOnResponseTrailers(contextID int32, trailers int32, endOfStream int32) int {
		action, err := e.v1.ProxyOnResponseTrailers(contextID, trailers)
		if err != nil || action != v1Host.ActionContinue {
			log.Logger.Error("[proxywasm][filter][v1] ProxyOnResponseTrailers action: %v, err: %v", action, err)
			return bfe_module.BfeHandlerResponse
		}

	return bfe_module.BfeHandlerGoOn
}

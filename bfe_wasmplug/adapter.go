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

package bfe_wasmplug

import (
	"bytes"
	"io/ioutil"

	"github.com/bfenetworks/bfe/bfe_http"
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

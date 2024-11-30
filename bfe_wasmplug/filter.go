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
	"sync"
	"sync/atomic"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"

	"github.com/baidu/go-lib/log"
	wasmABI "github.com/bfenetworks/bfe/bfe_wasmplug/abi"
	"github.com/bfenetworks/proxy-wasm-go-host/proxywasm/common"
	v1Host "github.com/bfenetworks/proxy-wasm-go-host/proxywasm/v1"
	v2Host "github.com/bfenetworks/proxy-wasm-go-host/proxywasm/v2"
)

type Filter struct {
	//ctx context.Context

	//factory *FilterConfigFactory

	//pluginName string
	plugin     WasmPlugin
	instance   common.WasmInstance
	abi        wasmABI.ABI
	exports    *exportAdapter

	rootContextID int32
	contextID     int32

	//receiverFilterHandler api.StreamReceiverFilterHandler
	//senderFilterHandler   api.StreamSenderFilterHandler
	request *bfe_basic.Request

	destroyOnce sync.Once
}

var contextIDGenerator int32

func newContextID(rootContextID int32) int32 {
	for {
		id := atomic.AddInt32(&contextIDGenerator, 1)
		if id != rootContextID {
			return id
		}
	}
}

func NewFilter(plugin *WasmPlugin, request *bfe_basic.Request) *Filter {
	instance := (*plugin).GetInstance()
	rootContextID := (*plugin).GetRootContextID()

	filter := &Filter{
		plugin:        *plugin,
		instance:      instance,
		rootContextID: rootContextID,
		contextID:     newContextID(rootContextID),
		request: request,
	}

	filter.abi = wasmABI.GetABIList(instance)[0]
	log.Logger.Info("[proxywasm][filter] abi version: %v", filter.abi.Name())
	if filter.abi.Name() == v1Host.ProxyWasmABI_0_1_0 {
		// v1
		imports := &v1Imports{plugin: *plugin, filter: filter}
		imports.DefaultImportsHandler.Instance = instance
		filter.abi.SetABIImports(imports)
		filter.exports = &exportAdapter{v1: filter.abi.GetABIExports().(v1Host.Exports)}
	} else if filter.abi.Name() == v2Host.ProxyWasmABI_0_2_0 {
		// v2
		imports := &v2Imports{plugin: *plugin, filter: filter}
		imports.DefaultImportsHandler.Instance = instance
		filter.abi.SetABIImports(imports)
		filter.exports = &exportAdapter{v2: filter.abi.GetABIExports().(v2Host.Exports)}
	} else {
		log.Logger.Error("[proxywasm][filter] unknown abi list: %v", filter.abi)
		return nil
	}

	filter.instance.Lock(filter.abi)
	defer filter.instance.Unlock()

	err := filter.exports.ProxyOnContextCreate(filter.contextID, filter.rootContextID)
	if err != nil {
		log.Logger.Error("[proxywasm][filter] NewFilter fail to create context id: %v, rootContextID: %v, err: %v",
			filter.contextID, filter.rootContextID, err)
		return nil
	}

	return filter
}
/*
func NewFilter(ctx context.Context, pluginName string, rootContextID int32, factory *FilterConfigFactory) *Filter {
	pluginWrapper := bfe_wasm.GetWasmManager().GetWasmPluginWrapperByName(pluginName)
	if pluginWrapper == nil {
		log.Logger.Error("[proxywasm][filter] NewFilter wasm plugin not exists, plugin name: %v", pluginName)
		return nil
	}

	plugin := pluginWrapper.GetPlugin()
	instance := plugin.GetInstance()

	filter := &Filter{
		ctx:           ctx,
		factory:       factory,
		pluginName:    pluginName,
		plugin:        plugin,
		instance:      instance,
		rootContextID: rootContextID,
		contextID:     newContextID(rootContextID),
	}

	filter.abi = wasmABI.GetABIList(instance)[0]
	log.Logger.Info("[proxywasm][filter] abi version: %v", filter.abi.Name())
	if filter.abi.Name() == v1Host.ProxyWasmABI_0_1_0 {
		// v1
		imports := &v1Imports{factory: filter.factory, filter: filter}
		imports.DefaultImportsHandler.Instance = instance
		filter.abi.SetABIImports(imports)
		filter.exports = &exportAdapter{v1: filter.abi.GetABIExports().(v1Host.Exports)}
	} else if filter.abi.Name() == v2Host.ProxyWasmABI_0_2_0 {
		// v2
		imports := &v2Imports{factory: filter.factory, filter: filter}
		imports.DefaultImportsHandler.Instance = instance
		filter.abi.SetABIImports(imports)
		filter.exports = &exportAdapter{v2: filter.abi.GetABIExports().(v2Host.Exports)}
	} else {
		log.Logger.Error("[proxywasm][filter] unknown abi list: %v", filter.abi)
		return nil
	}

	filter.instance.Lock(filter.abi)
	defer filter.instance.Unlock()

	err := filter.exports.ProxyOnContextCreate(filter.contextID, filter.rootContextID)
	if err != nil {
		log.Logger.Error("[proxywasm][filter] NewFilter fail to create context id: %v, rootContextID: %v, err: %v",
			filter.contextID, filter.rootContextID, err)
		return nil
	}

	return filter
}
*/
func (f *Filter) OnDestroy() {
	f.destroyOnce.Do(func() {
		f.instance.Lock(f.abi)

		_, err := f.exports.ProxyOnDone(f.contextID)
		if err != nil {
			log.Logger.Error("[proxywasm][filter] OnDestroy fail to call ProxyOnDone, err: %v", err)
		}

		err = f.exports.ProxyOnDelete(f.contextID)
		if err != nil {
			// warn instead of error as some proxy_abi_version_0_1_0 wasm don't
			// export proxy_on_delete
			log.Logger.Warn("[proxywasm][filter] OnDestroy fail to call ProxyOnDelete, err: %v", err)
		}

		f.instance.Unlock()
		f.plugin.ReleaseInstance(f.instance)
	})
}
/*
func (f *Filter) SetReceiveFilterHandler(handler api.StreamReceiverFilterHandler) {
	f.receiverFilterHandler = handler
}

func (f *Filter) SetSenderFilterHandler(handler api.StreamSenderFilterHandler) {
	f.senderFilterHandler = handler
}
*/
/*
func (f *Filter) OnReceive() int {
	f.instance.Lock(f.abi)
	defer f.instance.Unlock()

	status := f.exports.ProxyOnRequestHeaders(f.contextID, int32(len(f.request.HttpRequest.Header)), 0)
	if status != bfe_module.BfeHandlerGoOn {
		return status
	}

	endOfStream = 1
	if trailers != nil {
		endOfStream = 0
	}

	if buf != nil && buf.Len() > 0 {
		status = f.exports.ProxyOnRequestBody(f.contextID, int32(buf.Len()), int32(endOfStream))
		if status == api.StreamFilterStop {
			return api.StreamFilterStop
		}
	}

	if trailers != nil {
		status = f.exports.ProxyOnRequestTrailers(f.contextID, int32(headerMapSize(trailers)), int32(endOfStream))
		if status == api.StreamFilterStop {
			return api.StreamFilterStop
		}
	}

	return api.StreamFilterContinue
}

func (f *Filter) Append(ctx context.Context, headers api.HeaderMap, buf buffer.IoBuffer, trailers api.HeaderMap) api.StreamFilterStatus {
	f.instance.Lock(f.abi)
	defer f.instance.Unlock()

	endOfStream := 1
	if (buf != nil && buf.Len() > 0) || trailers != nil {
		endOfStream = 0
	}

	status := f.exports.ProxyOnResponseHeaders(f.contextID, int32(headerMapSize(headers)), int32(endOfStream))
	if status == api.StreamFilterStop {
		return api.StreamFilterStop
	}

	endOfStream = 1
	if trailers != nil {
		endOfStream = 0
	}

	if buf != nil && buf.Len() > 0 {
		status = f.exports.ProxyOnResponseBody(f.contextID, int32(buf.Len()), int32(endOfStream))
		if status == api.StreamFilterStop {
			return api.StreamFilterStop
		}
	}

	if trailers != nil {
		status = f.exports.ProxyOnResponseTrailers(f.contextID, int32(headerMapSize(trailers)), int32(endOfStream))
		if status == api.StreamFilterStop {
			return api.StreamFilterStop
		}
	}

	return api.StreamFilterContinue
}
*/
func (f *Filter) RequestHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
	f.instance.Lock(f.abi)
	defer f.instance.Unlock()

	status := f.exports.ProxyOnRequestHeaders(f.contextID, int32(len(request.HttpRequest.Header)), 0)
	if f.request.HttpResponse != nil {
		status = bfe_module.BfeHandlerResponse
	}
	return status, f.request.HttpResponse
}

func (f *Filter) ResponseHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
	f.instance.Lock(f.abi)
	defer f.instance.Unlock()

	status := f.exports.ProxyOnResponseHeaders(f.contextID, int32(len(request.HttpResponse.Header)), 0)
	return status, f.request.HttpResponse
}


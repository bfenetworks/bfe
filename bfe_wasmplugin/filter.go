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

package bfe_wasmplugin

import (
	"sync"
	"sync/atomic"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"

	"github.com/baidu/go-lib/log"
	wasmABI "github.com/bfenetworks/bfe/bfe_wasmplugin/abi"
	"github.com/bfenetworks/proxy-wasm-go-host/proxywasm/common"
	v1Host "github.com/bfenetworks/proxy-wasm-go-host/proxywasm/v1"
)

type Filter struct {
	plugin     WasmPlugin
	instance   common.WasmInstance
	abi        v1Host.ContextHandler
	exports    v1Host.Exports

	rootContextID int32
	contextID     int32

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

func NewFilter(plugin WasmPlugin, request *bfe_basic.Request) *Filter {
	instance := plugin.GetInstance()
	rootContextID := plugin.GetRootContextID()

	filter := &Filter{
		plugin:        plugin,
		instance:      instance,
		rootContextID: rootContextID,
		contextID:     newContextID(rootContextID),
		request: request,
	}

	filter.abi = wasmABI.GetABIList(instance)[0]
	log.Logger.Info("[proxywasm][filter] abi version: %v", filter.abi.Name())
	if filter.abi != nil {
		// v1
		imports := &v1Imports{plugin: plugin, filter: filter}
		imports.DefaultImportsHandler.Instance = instance
		filter.abi.SetImports(imports)
		filter.exports = filter.abi.GetExports()
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

func (f *Filter) RequestHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
	f.instance.Lock(f.abi)
	defer f.instance.Unlock()

	action, err := f.exports.ProxyOnRequestHeaders(f.contextID, int32(len(request.HttpRequest.Header)), 0)
	if err != nil {
		log.Logger.Error("[proxywasm][filter][v1] ProxyOnRequestHeaders action: %v, err: %v", action, err)
	}

	status := bfe_module.BfeHandlerGoOn
	if f.request.HttpResponse != nil {
		status = bfe_module.BfeHandlerResponse
	}
	return status, f.request.HttpResponse
}

func (f *Filter) ResponseHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
	f.instance.Lock(f.abi)
	defer f.instance.Unlock()

	action, err := f.exports.ProxyOnResponseHeaders(f.contextID, int32(len(request.HttpResponse.Header)), 0)
	if err != nil {
		log.Logger.Error("[proxywasm][filter][v1] ProxyOnResponseHeaders action: %v, err: %v", action, err)
	}

	status := bfe_module.BfeHandlerGoOn
	if f.request.HttpResponse != nil {
		status = bfe_module.BfeHandlerResponse
	}
	return status, f.request.HttpResponse
}

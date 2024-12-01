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

package abi

import (
	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_wasmplug/abi/proxywasm010"
	"github.com/bfenetworks/proxy-wasm-go-host/proxywasm/common"
	proxywasm "github.com/bfenetworks/proxy-wasm-go-host/proxywasm/v1"
)

func GetABIList(instance common.WasmInstance) []proxywasm.ContextHandler {
	if instance == nil {
		log.Logger.Error("[abi][registry] GetABIList nil instance: %v", instance)
		return nil
	}

	res := make([]proxywasm.ContextHandler, 0)

	abiNameList := instance.GetModule().GetABINameList()
	if len(abiNameList) > 0 {
		res = append(res, proxywasm010.ABIContextFactory(instance))
	}

	return res
}

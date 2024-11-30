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
	"sync"

	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/proxy-wasm-go-host/proxywasm/common"
)

// Factory is the ABI factory func.
type Factory func(instance common.WasmInstance) ABI

// string -> Factory.
var abiMap = sync.Map{}

// RegisterABI registers an abi factory.
func RegisterABI(name string, factory Factory) {
	abiMap.Store(name, factory)
}

func GetABI(instance common.WasmInstance, name string) ABI {
	if instance == nil || name == "" {
		log.Logger.Error("[abi][registry] GetABI invalid param, name: %v, instance: %v", name, instance)
		return nil
	}

	v, ok := abiMap.Load(name)
	if !ok {
		log.Logger.Error("[abi][registry] GetABI not found in registry, name: %v", name)
		return nil
	}

	abiNameList := instance.GetModule().GetABINameList()
	for _, abi := range abiNameList {
		if name == abi {
			factory := v.(Factory)
			return factory(instance)
		}
	}

	log.Logger.Error("[abi][register] GetABI not found in wasm instance, name: %v", name)

	return nil
}

func GetABIList(instance common.WasmInstance) []ABI {
	if instance == nil {
		log.Logger.Error("[abi][registry] GetABIList nil instance: %v", instance)
		return nil
	}

	res := make([]ABI, 0)

	abiNameList := instance.GetModule().GetABINameList()
	for _, abiName := range abiNameList {
		v, ok := abiMap.Load(abiName)
		if !ok {
			log.Logger.Warn("[abi][registry] GetABIList abi not registered, name: %v", abiName)
			continue
		}

		factory := v.(Factory)
		res = append(res, factory(instance))
	}

	return res
}

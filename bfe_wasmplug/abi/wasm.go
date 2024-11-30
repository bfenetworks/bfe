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
	"github.com/bfenetworks/proxy-wasm-go-host/proxywasm/common"
)

//
//	ABI
//

// ABI represents the abi between the host and wasm, which consists of three parts: exports, imports and life-cycle handler
// *exports* represents the exported elements of the wasm module, i.e., the abilities provided by wasm and exposed to host
// *imports* represents the imported elements of the wasm module, i.e., the dependencies that required by wasm
// *life-cycle handler* manages the life-cycle of an abi
type ABI interface {
	// Name returns the name of ABI
	Name() string

	// GetABIImports gets the imports part of the abi
	GetABIImports() interface{}

	// SetImports sets the import part of the abi
	SetABIImports(imports interface{})

	// GetExports returns the export part of the abi
	GetABIExports() interface{}

	ABIHandler
}

type ABIHandler interface {
	// life-cycle: OnInstanceCreate got called when instantiating the wasm instance
	OnInstanceCreate(instance common.WasmInstance)

	// life-cycle: OnInstanceStart got called when starting the wasm instance
	OnInstanceStart(instance common.WasmInstance)

	// life-cycle: OnInstanceDestroy got called when destroying the wasm instance
	OnInstanceDestroy(instance common.WasmInstance)
}

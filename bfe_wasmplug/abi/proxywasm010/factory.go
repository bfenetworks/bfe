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
	"github.com/bfenetworks/proxy-wasm-go-host/proxywasm/common"
	proxywasm "github.com/bfenetworks/proxy-wasm-go-host/proxywasm/v1"
)

func ABIContextFactory(instance common.WasmInstance) proxywasm.ContextHandler {
	return &proxywasm.ABIContext{
			Imports:  &DefaultImportsHandler{Instance: instance},
			Instance: instance,
		}
}

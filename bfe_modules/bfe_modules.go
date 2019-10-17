// Copyright (c) 2019 Baidu, Inc.
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

// set available modules in bfe

package bfe_modules

import (
	"github.com/baidu/bfe/bfe_module"
	"github.com/baidu/bfe/bfe_modules/mod_access"
	"github.com/baidu/bfe/bfe_modules/mod_block"
	"github.com/baidu/bfe/bfe_modules/mod_header"
	"github.com/baidu/bfe/bfe_modules/mod_http_code"
	"github.com/baidu/bfe/bfe_modules/mod_key_log"
	"github.com/baidu/bfe/bfe_modules/mod_logid"
	"github.com/baidu/bfe/bfe_modules/mod_redirect"
	"github.com/baidu/bfe/bfe_modules/mod_rewrite"
	"github.com/baidu/bfe/bfe_modules/mod_trust_clientip"
)

// list of all modules, the order is very important
var moduleList = []bfe_module.BfeModule{
	// mod_trust_clientip
	mod_trust_clientip.NewModuleTrustClientIP(),

	// mod_logid
	// Requirement: After mod_trust_clientip
	mod_logid.NewModuleLogId(),

	// mod_block
	// Requirement: After mod_dict_client, mod_logid
	mod_block.NewModuleBlock(),

	// mod_redirect
	// Requirement: After mod_dict_client, mod_logid
	mod_redirect.NewModuleRedirect(),

	// mod_rewrite
	// Requirement: After mod_dict_client
	mod_rewrite.NewModuleReWrite(),

	// mod_header
	mod_header.NewModuleHeader(),

	// mod_key_log
	mod_key_log.NewModuleKeyLog(),

	// mod_http_code
	mod_http_code.NewModuleHttpCode(),

	// mod_access
	mod_access.NewModuleAccess(),
}

// init modules list
func InitModuleList(modules []bfe_module.BfeModule) {
	moduleList = modules
}

// add all modules
func SetModules() {
	for _, module := range moduleList {
		bfe_module.AddModule(module)
	}
}

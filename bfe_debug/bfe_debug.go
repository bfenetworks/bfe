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

package bfe_debug

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_conf"
)

var (
	// DebugIsOpen is global debug switch, control by command line option (-d).
	DebugIsOpen = false

	// DebugServHTTP is debug switch for reverse proxy.
	DebugServHTTP = false

	// DebugBfeRoute is debug switch for bfe route.
	DebugBfeRoute = false

	// DebugBal is debug switch for bfe cluster.
	DebugBal = false

	// DebugHealthCheck is debug switch for health check.
	DebugHealthCheck = false
)

// SetDebugFlag initializes debug switches.
func SetDebugFlag(debugFlag bfe_conf.ConfigBasic) {
	DebugServHTTP = debugFlag.DebugServHttp
	DebugBfeRoute = debugFlag.DebugBfeRoute
	DebugBal = debugFlag.DebugBal
	DebugHealthCheck = debugFlag.DebugHealthCheck
}

// Copyright (c) 2025 The BFE Authors.
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

package waf_impl

import (
	"fmt"
	"net"

	mockWafSDK "github.com/bfenetworks/bfe-mock-waf/waf-bfe-sdk"
	bwi "github.com/bfenetworks/bwi/bwi"
)

type WafImplMethodBundle struct {
	NewWafServerWithPoolSize func(socketFactory func() (net.Conn, error), poolSize int) bwi.WafServer
	HealthCheck              func(conn net.Conn) error
}

var wafImplDict = map[string]*WafImplMethodBundle{
	//BFEMockWaf
	"BFEMockWaf": &WafImplMethodBundle{
		NewWafServerWithPoolSize: mockWafSDK.NewWafServerWithPoolSize,
		HealthCheck:              mockWafSDK.HealthCheck,
	},
	//AnHengWaf
	//ChaiTinWaf
}

func CheckWafSupport(wafName string) bool {
	_, ok := wafImplDict[wafName]
	return ok
}

func WafFactory(wafName string) (*WafImplMethodBundle, error) {
	bundle, ok := wafImplDict[wafName]
	if !ok {
		return nil, fmt.Errorf("don't support %s", wafName)
	}

	return bundle, nil
}

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

package route_rule_conf

import (
	"fmt"
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	pwd, _ := os.Getwd()
	fn := fmt.Sprintf("%s/testdata/route_rule.data", pwd)
	rt, err := RouteConfLoad(fn)
	if err != nil {
		t.Errorf("route conf load error %s", err)
	}

	if len(rt.RuleMap["product-b"]) != 2 {
		t.Errorf("product-2 condition len is not 2")
	}
}

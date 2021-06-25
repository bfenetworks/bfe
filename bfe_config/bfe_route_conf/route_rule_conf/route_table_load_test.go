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
		return
	}

	if len(rt.AdvancedRuleMap["product-b"]) != 2 {
		t.Errorf("product-b condition len is not 2")
	}
}
func TestLoad1(t *testing.T) {
	pwd, _ := os.Getwd()
	fn := fmt.Sprintf("%s/testdata/basic_route_rule1.data", pwd)
	rt, err := RouteConfLoad(fn)
	if err != nil {
		t.Errorf("route conf load error %s", err)
		return
	}

	if len(rt.BasicRuleMap["example_product"]) != 3 {
		t.Errorf("example_product rule number is not 3")
	}
}
func TestLoad2(t *testing.T) {
	pwd, _ := os.Getwd()
	fn := fmt.Sprintf("%s/testdata/basic_route_rule2.data", pwd)
	_, err := RouteConfLoad(fn)
	if err == nil {
		t.Errorf("route conf load should failed")
	}

}

func TestLoad3(t *testing.T) {
	pwd, _ := os.Getwd()
	fn := fmt.Sprintf("%s/testdata/basic_route_rule3.data", pwd)
	rt, err := RouteConfLoad(fn)
	if err != nil {
		t.Errorf("route conf load error %s", err)
		return
	}

	if len(rt.BasicRuleMap["example_product"]) != 3 {
		t.Errorf("example_product len is not 3")
	}

}

func TestLoad4(t *testing.T) {
	pwd, _ := os.Getwd()
	fn := fmt.Sprintf("%s/testdata/basic_route_rule4.data", pwd)
	_, err := RouteConfLoad(fn)
	if err == nil {
		t.Errorf("route conf load should fail ")
	}
}

func TestLoad5(t *testing.T) {
	pwd, _ := os.Getwd()
	fn := fmt.Sprintf("%s/testdata/basic_route_rule5.data", pwd)
	_, err := RouteConfLoad(fn)
	if err == nil {
		t.Errorf("route conf load should fail")
	}
}

func TestLoad6(t *testing.T) {
	pwd, _ := os.Getwd()
	fn := fmt.Sprintf("%s/testdata/basic_route_rule6.data", pwd)
	_, err := RouteConfLoad(fn)
	if err == nil {
		t.Errorf("route conf load should fail")
	}
}

func TestLoad7(t *testing.T) {
	pwd, _ := os.Getwd()
	fn := fmt.Sprintf("%s/testdata/basic_route_rule7.data", pwd)
	_, err := RouteConfLoad(fn)
	if err == nil {
		t.Errorf("route conf load should fail")
	}
}

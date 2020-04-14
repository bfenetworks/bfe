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

package config

import (
	"testing"
)

func TestAssertsFile(t *testing.T) {
	err := AssertsFile("./")
	if err == nil {
		t.Errorf("Bad assertion: path ./ was a directory")
	}

	err = AssertsFile("./utils.go")
	if err != nil {
		t.Errorf("Bad assertion: %+v", err)
	}
}

func TestMapKeys(t *testing.T) {
	m := map[string]interface{}{
		"a": 0,
		"b": 1,
		"c": 2,
	}

	keys := MapKeys(m)
	t.Log(keys)
	if len(keys) != 3 {
		t.Errorf("Bad result of MapKeys: %+v", keys)
	}
}

func TestMapConvert(t *testing.T) {
	type testStruct struct {
		A int
		B bool
		C string
	}
	m := map[string]interface{}{
		"A": 1,
		"B": true,
		"C": "string",
	}
	target := testStruct{}

	err := MapConvert(m, &target)
	if err != nil {
		t.Error(err)
	}
	t.Log(target)

	target = testStruct{}
	err = MapConvert(m, target)
	if err == nil {
		t.Errorf("something wrong with MapConvert(non-ptr)")
	}
}

func TestContains(t *testing.T) {
	target := []string{"a", "b", "c"}
	if !Contains(target, "a") || Contains(target, "d") {
		t.Errorf("something wrong with Contains()")
	}
}

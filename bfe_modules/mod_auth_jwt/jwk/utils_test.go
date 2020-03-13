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

package jwk

import (
	"reflect"
	"testing"
)

func TestBase64URLUintDecode(t *testing.T) {
	if _, bigInt, err := Base64URLUintDecode("AA"); err != nil ||
		!bigInt.IsInt64() || bigInt.Uint64() != 0 {
		t.Errorf("decoded value for base64url-encoded string 'AA' should be 0, not %+v", bigInt)
	}
}

func TestKeyCheck(t *testing.T) {
	target := map[string]interface{}{
		"a": 0,
		"b": "",
		"c": false,
	}
	err := KeyCheck(target, map[string]reflect.Kind{
		"a": reflect.Int,
		"b": reflect.String,
		"c": reflect.Bool,
	})
	if err != nil {
		t.Errorf("wrong key check: %s", err)
	}
	err = KeyCheck(target, map[string]reflect.Kind{
		"a": reflect.Bool,
	})
	t.Log(err)
	if err == nil {
		t.Error("wrong key check (type)")
	}
	err = KeyCheck(target, map[string]reflect.Kind{
		"d": reflect.Bool,
	})
	t.Log(err)
	if err == nil {
		t.Error("wrong key check (required)")
	}
}

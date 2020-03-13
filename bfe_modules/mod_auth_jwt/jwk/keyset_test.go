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
	"encoding/json"
	"io/ioutil"
	"testing"
)

var (
	secret []byte
	err    error
)

func init() {
	secret, err = ioutil.ReadFile("./../testdata/mod_auth_jwt/secret_test_jws_RS256.key")
	if err != nil {
		panic(err)
	}
}

func TestBuildRSAParams(t *testing.T) {
	keyMap := make(map[string]interface{})
	if err = json.Unmarshal(secret, &keyMap); err != nil {
		t.Fatal(err)
	}
	params, err := buildRSAParams(keyMap, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(params, params.Oth, params.Q.Decoded)
	if params.D.Raw != keyMap["d"] {
		t.Errorf("expected %s, got %s", keyMap["d"], params.D.Raw)
	}
}

func TestBuildRSAParamsWithOTH(t *testing.T) {
	keyMap := make(map[string]interface{})
	if err = json.Unmarshal(secret, &keyMap); err != nil {
		t.Fatal(err)
	}
	oth, err := json.Marshal([]map[string]interface{}{
		{
			"r": keyMap["p"],
			"d": keyMap["dp"],
			"t": keyMap["qi"],
		},
	})
	keyMap["oth"] = string(oth)
	if err != nil {
		t.Fatal(err)
	}
	params, err := buildRSAParams(keyMap, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(params.Oth, params.Oth[0].D.Decoded)
	r, p := params.Oth[0].R.Raw, keyMap["p"]
	if r != p {
		t.Errorf("expected %s, got %s", p, r)
	}
}

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
	"encoding/base64"
	"fmt"
	"math/big"
	"reflect"
)

// decoding base64url-encoded string (padding omitted)
var Base64URLDecode = base64.RawURLEncoding.DecodeString

// decoding base64urlUInt-encoded string
// see: https://tools.ietf.org/html/rfc7518#section-2
func Base64URLUintDecode(s string) (oct []byte, bigInt *big.Int, err error) {
	oct, err = Base64URLDecode(s)
	if err != nil {
		return nil, nil, err
	}
	return oct, new(big.Int).SetBytes(oct), nil
}

// check required key (type) for map
func KeyCheck(target map[string]interface{}, rule map[string]reflect.Kind) (err error) {
	for k, t := range rule {
		v, ok := target[k]
		if !ok {
			return fmt.Errorf("missing required key: %+v", k)
		}

		vType := reflect.TypeOf(v).Kind()
		if vType != t {
			return fmt.Errorf("key check failed (%+v): expected type %s, got %s", k, t, vType)
		}
	}

	return nil
}

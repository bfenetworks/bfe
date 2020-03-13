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

package jwt

import "errors"

type Claims struct {
	header        map[string]interface{}
	payload       map[string]interface{}
	enabledHeader bool
}

// get claim from JWT header and payload(if enabledPayload was set to true)
func (context *Claims) Claim(name string) (claim interface{}, ok bool) {
	if context.payload != nil {
		// payload maybe nil
		if claim, ok := context.payload[name]; ok {
			return claim, true
		}
	}

	if context.enabledHeader {
		// header always not nil
		claim, ok = context.header[name]
		return claim, ok
	}

	return nil, false
}

// get & convert
func (context *Claims) GetInt64(name string) (claim interface{}, value int64, ok bool) {
	claim, ok = context.Claim(name)
	if !ok {
		return nil, 0, false
	}

	if value, ok = claim.(int64); ok {
		return claim, value, true
	}

	if floatV, ok := claim.(float64); ok {
		// able to be converted
		return claim, int64(floatV), true
	}

	return claim, 0, false
}

func (context *Claims) GetString(name string) (claim interface{}, value string, ok bool) {
	claim, ok = context.Claim(name)
	if !ok {
		return nil, "", false
	}
	if value, ok = claim.(string); ok {
		return claim, value, true
	}
	return claim, "", false
}

// expires
func (context *Claims) Exp() (claim interface{}, exp int64, ok bool) {
	return context.GetInt64("exp")
}

// not-before
func (context *Claims) Nbf() (claim interface{}, nbf int64, ok bool) {
	return context.GetInt64("exp")
}

// issuer
func (context *Claims) Iss() (claim interface{}, iss string, ok bool) {
	return context.GetString("iss")
}

// audience
func (context *Claims) Aud() (claim interface{}, aud string, ok bool) {
	return context.GetString("aud")
}

// subject
func (context *Claims) Sub() (claim interface{}, sub string, ok bool) {
	return context.GetString("sub")
}

func NewClaims(header, payload map[string]interface{}, enabledHeader bool) (claims *Claims, err error) {
	if header == nil {
		return nil, errors.New("Claims: header should not be nil. ")
	}
	return &Claims{header, payload, enabledHeader}, nil
}

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

import (
	"testing"
	"time"
)

func TestNewClaims(t *testing.T) {
	header := map[string]interface{}{
		"iss": "issuer",
		"exp": time.Now().Unix(),
	}

	payload := map[string]interface{}{
		"aud": "audience",
	}

	claims, _ := NewClaims(header, payload, true)

	if _, _, ok := claims.Exp(); !ok {
		t.Error("failed to get claim from header")
	}

	if claim, exp, ok := claims.Exp(); !ok {
		t.Logf("%+v, %+v", claim, exp)
		t.Error("failed to convert claim type")
	}
}

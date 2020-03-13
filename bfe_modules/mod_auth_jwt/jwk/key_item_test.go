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
	"testing"
)

func TestNewBase64URL(t *testing.T) {
	value := base64.RawURLEncoding.EncodeToString([]byte("test"))
	b, err := NewBase64URL(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(b.Decoded) != "test" {
		t.Errorf("wrong decoded string: %s", b.Decoded)
	}
}

func TestNewBase64URLUint(t *testing.T) {
	b, err := NewBase64URLUint("AA")
	if err != nil {
		t.Fatal(err)
	}
	if !b.Decoded.IsInt64() || b.Decoded.Int64() != 0 {
		t.Errorf("wrong decoded value: %+v", b.Decoded)
	}
}

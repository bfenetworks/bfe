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

package jwa

import (
	"bytes"
	"crypto"
	"testing"
)

func TestConcatKDF_Derive(t *testing.T) {
	z := []byte{158, 86, 217, 29, 129, 113, 53, 211, 114, 131, 66, 131, 191, 132,
		38, 156, 251, 49, 110, 163, 218, 128, 106, 72, 246, 218, 167, 121,
		140, 254, 144, 196}
	otherInfo := []byte{0, 0, 0, 7, 65, 49, 50, 56, 71, 67, 77, 0, 0, 0, 5, 65, 108, 105,
		99, 101, 0, 0, 0, 3, 66, 111, 98, 0, 0, 0, 128}
	expected := []byte{86, 170, 141, 234, 248, 35, 109, 32, 92, 34, 40, 205, 113, 167, 16, 26}
	ret, err := NewConcatKDF(crypto.SHA256.New()).Derive(z, 128, otherInfo)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ret, expected) {
		t.Errorf("bad result from concatKDF: %+v", ret)
	}
}

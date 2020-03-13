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

import "testing"

func TestNewA256GCM(t *testing.T) {
	cek := []byte{177, 161, 244, 128, 84, 143, 225, 115, 63, 180, 3, 255, 107, 154,
		212, 246, 138, 7, 110, 91, 112, 46, 34, 105, 47, 130, 203, 46, 122,
		234, 64, 252}
	iv := []byte{227, 197, 117, 252, 2, 219, 233, 68, 180, 225, 77, 219}
	aad := []byte{101, 121, 74, 104, 98, 71, 99, 105, 79, 105, 74, 83, 85, 48, 69,
		116, 84, 48, 70, 70, 85, 67, 73, 115, 73, 109, 86, 117, 89, 121, 73,
		54, 73, 107, 69, 121, 78, 84, 90, 72, 81, 48, 48, 105, 102, 81}
	cipher := []byte{229, 236, 166, 241, 53, 191, 115, 196, 174, 43, 73, 109, 39, 122,
		233, 96, 140, 206, 120, 52, 51, 237, 48, 11, 190, 219, 186, 80, 111,
		104, 50, 142, 47, 167, 59, 61, 181, 127, 196, 21, 40, 82, 242, 32,
		123, 143, 168, 226, 73, 216, 176, 144, 138, 247, 106, 60, 16, 205,
		160, 109, 64, 63, 192}
	tag := []byte{92, 80, 104, 49, 133, 25, 161, 215, 173, 101, 219, 211, 136, 91,
		210, 145}
	handler, err := NewA256GCM(cek)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := handler.Decrypt(iv, aad, cipher, tag)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(msg))
}

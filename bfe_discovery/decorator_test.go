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

package bfe_discovery

import "testing"

func TestDecorate(t *testing.T) {
	var key string = "/TEST"
	key = Decorate(key, DefaultDecorators...)
	if key != "/bfe/test" {
		t.Fatal("unexpected")
	}
}

func TestDecorate2(t *testing.T) {
	var key string = "/TEST"
	key = Decorate(key, DecoPrefix, DecoToUpper, DecoSuffix)
	if key != "/BFE/TEST/" {
		t.Fatal("unexpected")
	}
}

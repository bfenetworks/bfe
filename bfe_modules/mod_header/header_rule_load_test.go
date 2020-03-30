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

package mod_header

import (
	"testing"
)

func TestHeaderConfLoad(t *testing.T) {
	_, err := HeaderConfLoad("./testdata/mod_header/header_rule.data")
	if err != nil {
		t.Errorf("HeaderConfLoad() failed for %v", err)
	}

	//Negative case: not exist conf file
	_, err = HeaderConfLoad("./testdata/not_exist.conf")
	if err == nil {
		t.Error("HeaderConfLoad() failed for not exist conf file")
	}
}

// Copyright (c) 2019 The BFE Authors.
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

package mod_compress

import (
	"testing"
)

func TestProductRuleConfLoadCorrect(t *testing.T) {
	config, err := ProductRuleConfLoad("./testdata/mod_compress/compress_rule.data")
	if err != nil {
		t.Errorf("ProductRuleConfLoad() error: %v", err)
		return
	}

	if len(*config.Config["unittest"]) != 1 {
		t.Errorf("length should be 1")
	}
}

func TestProductRuleConfLoadCmdError(t *testing.T) {
	_, err := ProductRuleConfLoad("./testdata/mod_compress/compress_rule.data.cmd_error")
	if err == nil ||
		err.Error() != "Config: ProductRules: unittest, compressRule: 0, invalid cmd: ERR_COMPRESS" {
		t.Errorf("error should be \"\", not \"%v\"", err)
	}
}

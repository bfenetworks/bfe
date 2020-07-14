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

package mod_header

import (
	"testing"
)

func TestConfModHeaderCase1(t *testing.T) {
	confPath := "./testdata/mod_header/mod_header_1.conf"
	dataPath := "/home/bfe/conf/mod_header.conf"

	config, err := ConfLoad(confPath, "")
	if err != nil {
		t.Errorf("ConfLoad():err=%s", err.Error())
		return
	}

	if config.Basic.DataPath != dataPath {
		t.Errorf("DataPath should be %s", dataPath)
	}
}

func TestConfModRewriteCase2(t *testing.T) {
	// illegal value
	confPath := "./testdata/mod_header/mod_header_2.conf"
	confRoot := "/home/bfe/conf"
	defaultDataPath := "/home/bfe/conf/mod_header/mod_header.data"

	config, _ := ConfLoad(confPath, confRoot)

	// use default value
	if config.Basic.DataPath != defaultDataPath {
		t.Errorf("DataPath should be %s", defaultDataPath)
	}
}

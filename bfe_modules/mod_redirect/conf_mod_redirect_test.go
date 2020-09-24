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

package mod_redirect

import (
	"testing"
)

func TestConfLoad(t *testing.T) {
	config, err := ConfLoad("./testdata/conf_mod_redirect/mod_redirect.conf", "")
	if err != nil {
		t.Errorf("confModRedirectLoad():err=%s", err.Error())
		return
	}

	if config.Basic.DataPath != "/home/bfe/conf/123.conf" {
		t.Error("DataPath should be /home/bfe/conf/123.conf")
	}
}

/* load config from config file    */
func TestConfLoadDefaultDataPath(t *testing.T) {
	// illegal value
	config, _ := ConfLoad("./testdata/conf_mod_redirect/mod_redirect.conf.default_data_path", "/home/bfe/conf")

	// use default value
	if config.Basic.DataPath != "/home/bfe/conf/mod_redirect/redirect.data" {
		t.Error("DataPath should be /home/bfe/conf/mod_redirect/redirect.data")
	}
}

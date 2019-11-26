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

package mod_geo

import (
	"fmt"
	"testing"
)

func Test_conf_mod_geo_case1(t *testing.T) {
	config, err := ConfLoad("./test_data/mod_geo/mod_geo.conf", "")
	if err != nil {
		msg := fmt.Sprintf("confModGeoLoad():err=%s", err.Error())
		t.Error(msg)
		return
	}

	if config.Basic.MaxMindDBPath != "mod_geo/GeoLite2-City.mmdb" {
		t.Error("MaxMindDBPath should be mod_geo/GeoLite2-City.mmdb")
	}
}

func Test_conf_mod_geo_case2(t *testing.T) {
	config, err := ConfLoad("./test_data/mod_geo/mod_geo1.conf", "")
	if err != nil {
		msg := fmt.Sprintf("confModGeoLoad():err=%s", err.Error())
		t.Error(msg)
		return
	}

	// MaxMindDBPath is empty, default use mod_geo/GeoLite2-City.mmdb
	if config.Basic.MaxMindDBPath != "mod_geo/GeoLite2-City.mmdb" {
		t.Error("MaxMindDBPath should be mod_geo/GeoLite2-City.mmdb")
	}
}

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

// test for gslb_conf_load.go

package gslb_conf

import (
	"testing"
)

func TestGslbConfLoad_1(t *testing.T) {
	config, err := GslbConfLoad("./testdata/gslb_1.data")
	if err != nil {
		t.Errorf("get err from GslbConfLoad():%s", err.Error())
		return
	}

	if len(*config.Clusters) != 11 {
		t.Error("len(config.Clusters) should be 11")
	}
}

func TestGslbConfLoad_2(t *testing.T) {
	if _, err := GslbConfLoad("./testdata/gslb_2.data"); err == nil {
		t.Error("it should be error in GslbConfLoad()")
	}
}

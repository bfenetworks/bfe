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

package cluster_table_conf

import (
	"testing"
)

func TestClusterTableLoad_1(t *testing.T) {
	config, err := ClusterTableLoad("./testdata/cluster_table_1.conf")
	if err != nil {
		t.Errorf("get err from ClusterTableLoad():%s", err.Error())
		return
	}

	if len(*config.Config) != 2 {
		t.Error("len(config) should be 2")
		return
	}

	if *(*config.Config)["p1"]["light.p1.dx"][0].Name != "p1.example10.a" {
		t.Error("invalid config")
		return
	}
}

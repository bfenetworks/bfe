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

package cluster_conf

import (
	"testing"
)

func TestClusterConfLoad_1(t *testing.T) {
	config, err := ClusterConfLoad("./testdata/cluster_conf_1.conf")
	if err != nil {
		t.Errorf("get err from ClusterConfLoad():%s", err.Error())
		return
	}

	if len(*config.Config) != 2 {
		t.Error("len(config.Config) should be 2")
		return
	}
}

func TestClusterConfLoad_2(t *testing.T) {
	if _, err := ClusterConfLoad("./testdata/cluster_conf_2.conf"); err == nil {
		t.Error("it should be error in ClusterConfLoad()")
		return
	}
}

func TestClusterConfLoad_3(t *testing.T) {
	config, err := ClusterConfLoad("./testdata/cluster_conf_3.conf")
	if err != nil {
		t.Errorf("ClusterConfLoad() error: %v", err)
		return
	}
	schem := *(*config.Config)["p2"].CheckConf.Schem
	if schem != "tcp" {
		t.Errorf("schem should be tcp, not %s", schem)
	}
}

func TestClusterConfLoad_4(t *testing.T) {
	_, err := ClusterConfLoad("./testdata/cluster_conf_4.conf")
	if err == nil {
		t.Error("it should be error in ClusterConfLoad()")
		return
	}
}

func TestClusterConfLoad_6(t *testing.T) {
	_, err := ClusterConfLoad("./testdata/cluster_conf_6.conf")
	if err == nil {
		t.Error("it should be error in ClusterConfLoad()")
		return
	}
}

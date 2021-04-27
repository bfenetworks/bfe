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

package bfe_route

import (
	"testing"
)

func TestClusterTable(t *testing.T) {
	tb := newClusterTable()
	clusterConfFile := "testdata/cluster_table/cluster_conf.data"

	if err := tb.Init(""); err == nil {
		t.Error("Init: case 0 should return err")
		return
	}

	if err := tb.Init(clusterConfFile); err != nil {
		t.Errorf("Init: case 1 should return nil. but err [%s]", err)
		return
	}

	if _, err := tb.Lookup(""); err == nil {
		t.Error("Lookup: case 0 should return err.")
		return
	}

	if _, err := tb.Lookup("cluster_d"); err == nil {
		t.Error("Lookup: case 1 should return err.")
		return
	}

	cluster, err := tb.Lookup("cluster_b")
	if err != nil {
		t.Errorf("Lookup: case 2 should return nil. but err [%s]", err)
		return
	}

	if cluster.Name != "cluster_b" {
		t.Errorf("Lookup: case 3 should return cluster_b")
		return
	}
}

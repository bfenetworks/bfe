// Copyright (c) 2025 The BFE Authors.
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

package mod_unified_waf

import (
	"fmt"
	"testing"
)

func TestAlbWafInstancesLoadAndCheck_1(t *testing.T) {
	albWafInstancesPath := "./testdata/alb_waf_instances.data"

	winsts, err := AlbWafInstancesLoadAndCheck(albWafInstancesPath)
	if err != nil {
		t.Errorf("AlbWafInstancesLoadAndCheck(): %v", err)
		return
	}

	if winsts.WafCluster[0].HealthCheckPort != winsts.WafCluster[0].Port {
		fmt.Println("=== TestAlbWafInstancesLoadAndCheck_1", winsts.WafCluster[0].HealthCheckPort, winsts.WafCluster[0].Port)
		t.Errorf("winsts.WafCluster[0].HealthCheckPort != winsts.WafCluster[0].Port")
		return
	}

	if winsts.WafCluster[1].HealthCheckPort != 5001 {
		t.Errorf("winsts.WafCluster[1].HealthCheckPort != 5001")
		return
	}

}

func TestAlbWafInstancesLoadAndCheck_2(t *testing.T) {
	albWafInstancesPath := "./testdata/alb_waf_instances_empty.data"

	_, err := AlbWafInstancesLoadAndCheck(albWafInstancesPath)
	if err != nil {
		t.Errorf("AlbWafInstancesLoadAndCheck(): %v", err)
		return
	}
}

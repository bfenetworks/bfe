// Copyright (c) 2026 The BFE Authors.
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

package bfe_server

import (
	"sync"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
)

// TestAISelectionConcurrent guards the AI target/key selection against the data
// race a shared math/rand.Rand would introduce: selection runs on the per
// request goroutines, so the random source must be safe for concurrent use.
// Run under -race (as `make test` does) to catch a regression.
func TestAISelectionConcurrent(t *testing.T) {
	targets := []bfe_basic.AiRouteTarget{
		{ClusterName: "cluster_a", Model: "", Weight: 60},
		{ClusterName: "cluster_b", Model: "", Weight: 30},
		{ClusterName: "cluster_c", Model: "", Weight: 10},
	}
	keys := []cluster_conf.AIKey{
		{Name: "key-1", Key: "sk-1", Weight: 50},
		{Name: "key-2", Key: "sk-2", Weight: 50},
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if got := SelectTarget(targets); got.ClusterName == "" {
					t.Error("SelectTarget returned an empty target")
					return
				}
				if _, idx := selectAIKey(keys); idx < 0 {
					t.Error("selectAIKey returned no key")
					return
				}
			}
		}()
	}
	wg.Wait()
}

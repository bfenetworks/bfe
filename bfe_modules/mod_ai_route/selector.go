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

package mod_ai_route

import (
	"math/rand"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
)

var targetRand = rand.New(rand.NewSource(time.Now().UnixNano()))

func SelectTarget(targets []bfe_basic.AiRouteTarget) bfe_basic.AiRouteTarget {
	if len(targets) == 1 {
		return targets[0]
	}

	r := targetRand.Intn(100)
	sum := 0
	for _, target := range targets {
		sum += target.Weight
		if r < sum {
			return target
		}
	}
	return targets[len(targets)-1]
}

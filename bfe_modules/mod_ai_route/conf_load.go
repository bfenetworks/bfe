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
	"fmt"

	"github.com/bfenetworks/bfe/bfe_util"

	gcfg "gopkg.in/gcfg.v1"
)

type ConfModAiRoute struct {
	Basic struct {
		RouteRulePath string // path for ai route rule
	}
	Log struct {
		OpenDebug bool
	}
}

func ConfLoad(filePath string, confRoot string) (*ConfModAiRoute, error) {
	var cfg ConfModAiRoute
	if err := gcfg.ReadFileInto(&cfg, filePath); err != nil {
		return &cfg, err
	}
	if err := cfg.Check(confRoot); err != nil {
		return &cfg, err
	}
	return &cfg, nil
}

func (cfg *ConfModAiRoute) Check(confRoot string) error {
	return ConfModAiRouteCheck(cfg, confRoot)
}

func ConfModAiRouteCheck(cfg *ConfModAiRoute, confRoot string) error {
	if cfg.Basic.RouteRulePath == "" {
		return fmt.Errorf("ConfModAiRouteCheck: RouteRulePath is empty")
	}
	cfg.Basic.RouteRulePath = bfe_util.ConfPathProc(cfg.Basic.RouteRulePath, confRoot)
	return nil
}

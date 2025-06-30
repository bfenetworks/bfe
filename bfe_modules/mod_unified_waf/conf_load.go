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
	"errors"
	"fmt"

	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_modules/mod_unified_waf/waf_impl"
	gcfg "gopkg.in/gcfg.v1"
)

type ConfBasic struct {
	WafProductName string
	// Concurrency  int
	ConnPoolSize int
}

type ConfModWaf struct {
	Basic ConfBasic

	ConfigPath struct {
		ModWafDataPath      string // configure path for mod_unified_waf.data
		ProductParamPath    string // configure path for product_param.data
		AlbWafInstancesPath string // configure path for alb_waf_instances.data
	}

	Log struct {
		OpenDebug bool
	}
}

func ConfLoad(path string, confRoot string) (*ConfModWaf, error) {
	var err error
	var cfg ConfModWaf

	// read config from file
	if err = gcfg.ReadFileInto(&cfg, path); err != nil {
		return &cfg, err
	}
	// check conf of mod_waf_client
	err = cfg.Check(confRoot)
	if err != nil {
		return &cfg, err
	}

	return &cfg, nil
}

// check also fix some configure value
func (cfg *ConfModWaf) Check(confRoot string) error {
	// if cfg.Basic.Concurrency <= 0 {
	// 	log.Logger.Warn("Basic.Concurrency is : %d, use DEFAULT_CONCURRENCY(%d)", cfg.Basic.Concurrency, DEFAULT_CONCURRENCY)
	// 	cfg.Basic.Concurrency = DEFAULT_CONCURRENCY
	// }
	if len(cfg.Basic.WafProductName) <= 0 {
		cfg.Basic.WafProductName = NoneWafName
	}

	if len(cfg.Basic.WafProductName) > 0 {
		twafName := cfg.Basic.WafProductName
		if (twafName != NoneWafName) && !waf_impl.CheckWafSupport(twafName) {
			err := fmt.Errorf("Basic.WafProductName:%s is illgal", cfg.Basic.WafProductName)
			return err
		}
	}

	if cfg.Basic.ConnPoolSize <= 0 {
		log.Logger.Warn("Basic.ConnPoolSize is : %d, use DEFAULT_POOL_SIZE(%d)", cfg.Basic.ConnPoolSize, DEFAULT_POOL_SIZE)
		cfg.Basic.ConnPoolSize = DEFAULT_POOL_SIZE
	}

	// check conf of ProductParamPath
	if cfg.ConfigPath.ProductParamPath == "" {
		log.Logger.Error("ConfigPath.ProductParamPath not set")
		return errors.New("ConfigPath.ProductParamPath not set")
	}

	// check conf of ModWafDataPath
	if cfg.ConfigPath.ModWafDataPath == "" {
		log.Logger.Error("ConfigPath.ModWafDataPath not set")
		return errors.New("ConfigPath.ModWafDataPath not set")
	}

	// check conf of AlbWafInstancesPath
	if cfg.ConfigPath.AlbWafInstancesPath == "" {
		log.Logger.Error("ConfigPath.AlbWafInstancesPath not set")
		return errors.New("ConfigPath.AlbWafInstancesPath not set")
	}

	return nil
}

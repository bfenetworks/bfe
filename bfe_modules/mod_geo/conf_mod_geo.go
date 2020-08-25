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

package mod_geo

import (
	"github.com/baidu/go-lib/log"
	"gopkg.in/gcfg.v1"
)

import (
	"github.com/bfenetworks/bfe/bfe_util"
)

type ConfModGeo struct {
	Basic struct {
		GeoDBPath string // path of geolocation database
	}

	Log struct {
		OpenDebug bool
	}
}

// ConfLoad loads config of geo module from file.
func ConfLoad(filePath string, confRoot string) (*ConfModGeo, error) {
	var err error
	var cfg ConfModGeo

	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return &cfg, err
	}

	err = cfg.Check(confRoot)
	if err != nil {
		return &cfg, err
	}

	return &cfg, nil
}

// Check validates module config
func (cfg *ConfModGeo) Check(confRoot string) error {
	return ConfModBlockCheck(cfg, confRoot)
}

func ConfModBlockCheck(cfg *ConfModGeo, confRoot string) error {
	if cfg.Basic.GeoDBPath == "" {
		// if GeoDBPath not set, default use mod_geo/geo.db
		// geo.db is GeoLite2 data created by MaxMind, available from https://dev.maxmind.com/geoip/geoip2/geolite2/
		log.Logger.Warn("ModGeo.GeoDBPath not set, use default value")
		cfg.Basic.GeoDBPath = GeoDBDefaultPath
	}
	cfg.Basic.GeoDBPath = bfe_util.ConfPathProc(cfg.Basic.GeoDBPath, confRoot)

	return nil
}

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
	"encoding/json"
	"fmt"
	"os"

	"github.com/bfenetworks/bfe/bfe_util"
)

type WafInstance struct {
	IpAddr          string
	Port            int
	HealthCheckPort int
}

// alb waf instance config for alb clusters
type ClusterConfigs struct {
	WafCluster []WafInstance `json:"WafCluster"`
}

// global param in config file
type AlbWafInstancesConfFile struct {
	Version *string
	Config  *ClusterConfigs
}

type AlbWafInstancesConf struct {
	Version    string        `json:"version"`
	WafCluster []WafInstance `json:"WafCluster"`
}

func (cfg *AlbWafInstancesConfFile) Check() error {
	if err := bfe_util.CheckNilField(*cfg, false); err != nil {
		return err
	}

	if cfg.Config.WafCluster != nil {
		for idx, instance := range cfg.Config.WafCluster {
			if instance.Port <= 0 {
				return fmt.Errorf("illegal waf instance Port, idx:%d", idx)
			}
			if cfg.Config.WafCluster[idx].HealthCheckPort <= 0 {
				cfg.Config.WafCluster[idx].HealthCheckPort = instance.Port
			}
		}
		if len(cfg.Config.WafCluster) <= 0 {
			return fmt.Errorf("WafCluster is empty")
		}
	}

	return nil
}

// reload_trigger adaptor interface
func AlbWafInstancesLoadAndCheck(filename string) (AlbWafInstancesConf, error) {
	var err error
	var data AlbWafInstancesConf

	// open the file
	file, err := os.Open(filename)
	defer file.Close()
	if err != nil {
		return data, err
	}

	// decode the file
	decoder := json.NewDecoder(file)
	var dataFile AlbWafInstancesConfFile
	err = decoder.Decode(&dataFile)
	if err != nil {
		return data, err
	}

	// check config
	if err := dataFile.Check(); err != nil {
		return data, err
	}

	// convert config
	data.Version = *dataFile.Version
	if dataFile.Config.WafCluster != nil {
		data.WafCluster = dataFile.Config.WafCluster
	}
	return data, nil
}

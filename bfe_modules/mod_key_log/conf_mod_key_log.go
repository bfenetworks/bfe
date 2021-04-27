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

package mod_key_log

import (
	gcfg "gopkg.in/gcfg.v1"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/access_log"
)

// ConfModKeyLog represents the basic config for mod_key_log.
type ConfModKeyLog struct {
	Basic struct {
		DataPath string // path of config data (key_log)
	}
	Log access_log.LogConfig
}

// Check validates module config
func (cfg *ConfModKeyLog) Check(confRoot string) error {
	return cfg.Log.Check(confRoot)
}

// ConfLoad loads config from file
func ConfLoad(filePath string, confRoot string) (*ConfModKeyLog, error) {
	var cfg ConfModKeyLog
	var err error

	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return nil, err
	}

	// check config
	err = cfg.Check(confRoot)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

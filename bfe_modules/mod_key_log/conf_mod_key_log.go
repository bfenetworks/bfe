// Copyright (c) 2019 Baidu, Inc.
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
	"fmt"
)

import (
	"github.com/baidu/go-lib/log/log4go"
	gcfg "gopkg.in/gcfg.v1"
)

import (
	"github.com/baidu/bfe/bfe_util"
)

// ConfModKeyLog represents the basic config for mod_key_log.
type ConfModKeyLog struct {
	Log struct {
		LogPrefix   string // log file prefix
		LogDir      string // log file dir
		RotateWhen  string // rotate time
		BackupCount int    // log file backup number
	}
}

// Check validates module config
func (cfg *ConfModKeyLog) Check(confRoot string) error {
	if cfg.Log.LogPrefix == "" {
		return fmt.Errorf("LogPrefix is empty")
	}

	if cfg.Log.LogDir == "" {
		return fmt.Errorf("LogDir is empty")
	}
	cfg.Log.LogDir = bfe_util.ConfPathProc(cfg.Log.LogDir, confRoot)

	if !log4go.WhenIsValid(cfg.Log.RotateWhen) {
		return fmt.Errorf("RotateWhen invalid: %s", cfg.Log.RotateWhen)
	}

	if cfg.Log.BackupCount <= 0 {
		return fmt.Errorf("BackupCount should > 0: %d", cfg.Log.BackupCount)
	}

	return nil
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

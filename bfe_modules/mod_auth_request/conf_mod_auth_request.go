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

package mod_auth_request

import (
	"fmt"
	"net/url"
)

import (
	"github.com/baidu/go-lib/log"
	"gopkg.in/gcfg.v1"
)

import (
	"github.com/bfenetworks/bfe/bfe_util"
)

const (
	defaultDataPath = "mod_auth_request/auth_request_rule.data"
)

type ConfModAuthRequest struct {
	Basic struct {
		DataPath    string // path of rule data
		AuthAddress string // address of auth server
		AuthTimeout int    // timeout for auth request (in ms)
	}

	Log struct {
		OpenDebug bool
	}
}

func ConfLoad(filePath string, confRoot string) (*ConfModAuthRequest, error) {
	var err error
	var cfg ConfModAuthRequest

	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return nil, err
	}

	err = cfg.Check(confRoot)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (cfg *ConfModAuthRequest) Check(confRoot string) error {
	if len(cfg.Basic.DataPath) == 0 {
		cfg.Basic.DataPath = defaultDataPath
		log.Logger.Warn("ModAuthRequest.DataPath not set, use default value: %s", defaultDataPath)
	}

	cfg.Basic.DataPath = bfe_util.ConfPathProc(cfg.Basic.DataPath, confRoot)

	if len(cfg.Basic.AuthAddress) == 0 {
		return fmt.Errorf("ModAuthRequest.AuthAddress not set")
	}
	_, err := url.Parse(cfg.Basic.AuthAddress)
	if err != nil {
		return fmt.Errorf("ModAuthRequest.AuthAddress is not a correct url")
	}

	if cfg.Basic.AuthTimeout <= 0 {
		return fmt.Errorf("ModAuthRequest.AuthTimeout must > 0")
	}

	return nil
}

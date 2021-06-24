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

package bfe_conf

import (
	gcfg "gopkg.in/gcfg.v1"
)

//SubConf is an interface implemented by BfeConfig.Server etc
type SubConf interface {
	SetDefaultConf()
	Check(string) error
}

type BfeConfig struct {
	// basic server config
	Server ConfigBasic

	// basic https config
	HttpsBasic ConfigHttpsBasic

	// session cache config
	SessionCache ConfigSessionCache

	// session cache config
	SessionTicket ConfigSessionTicket
}

func SetDefaultConf(conf *BfeConfig) {
	setDefaultConf(&conf.Server)
	setDefaultConf(&conf.HttpsBasic)
	setDefaultConf(&conf.SessionCache)
	setDefaultConf(&conf.SessionTicket)
}

// BfeConfigLoad loads config from config file.
// NOTICE: some value will be modified when not set or out of range!!
func BfeConfigLoad(filePath string, confRoot string) (BfeConfig, error) {
	var cfg BfeConfig
	var err error

	SetDefaultConf(&cfg)

	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return cfg, err
	}

	if err = check(&cfg.Server, confRoot); err != nil {
		return cfg, err
	}

	if err = check(&cfg.HttpsBasic, confRoot); err != nil {
		return cfg, err
	}

	if err = check(&cfg.SessionCache, confRoot); err != nil {
		return cfg, err
	}

	if err = check(&cfg.SessionTicket, confRoot); err != nil {
		return cfg, err
	}

	return cfg, nil
}

//check wraps as func so it can call  SubConf Check func
func check(sc SubConf, confRoot string) error {
	return sc.Check(confRoot)
}

//setDefaultConf wraps as func so it can call  SubConf setDefaultConf func
func setDefaultConf(sc SubConf) {
	sc.SetDefaultConf()
}

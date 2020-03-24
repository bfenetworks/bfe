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

package mod_doh

import (
	"fmt"
	"net"
)

import (
	gcfg "gopkg.in/gcfg.v1"
)

import (
	"github.com/baidu/bfe/bfe_basic/condition"
)

type ConfModDoh struct {
	Basic struct {
		Cond string
	}

	Address struct {
		Net  string
		Ip   string
		Port int
	}

	Log struct {
		OpenDebug bool
	}
}

func ConfLoad(filePath string, confRoot string) (*ConfModDoh, error) {
	var err error
	var cfg ConfModDoh

	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return nil, err
	}

	err = cfg.Check()
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (cfg *ConfModDoh) Check() error {
	_, err := condition.Build(cfg.Basic.Cond)
	if err != nil {
		return err
	}

	if cfg.Address.Net != "tcp" && cfg.Address.Net != "udp" {
		return fmt.Errorf("Address.Net should be \"tcp\" or \"udp\"")
	}

	ip := net.ParseIP(cfg.Address.Ip)
	if ip == nil {
		return fmt.Errorf("Address.IP is invalid IP: %s", cfg.Address.Ip)
	}

	if cfg.Address.Port < 1 || cfg.Address.Port > 65535 {
		return fmt.Errorf("Address.Port should be in [1, 65535]")
	}

	return nil
}

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

package mod_doh

import (
	"fmt"
	"net"
)

import (
	gcfg "gopkg.in/gcfg.v1"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

type DnsConf struct {
	Address  string
	RetryMax int
	Timeout  int // In Millisecond.
}

type ConfModDoh struct {
	Basic struct {
		Cond string
	}

	Dns DnsConf

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

	if cfg.Dns.RetryMax < 0 {
		return fmt.Errorf("RetryMax should >= 0.")
	}

	if cfg.Dns.Timeout < 1 {
		return fmt.Errorf("Timeout should > 0.")
	}

	_, err = net.ResolveUDPAddr("udp", cfg.Dns.Address)
	return err
}

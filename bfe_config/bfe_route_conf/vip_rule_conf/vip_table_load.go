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

// load host table from json file

package vip_rule_conf

import (
	"errors"
	"fmt"
	"net"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type VipList []string // list of vips

type Product2Vip map[string]VipList // product => vip list

type Vip2Product map[string]string // vip => product

type VipTableConf struct {
	Version string      // version of the config
	Vips    Product2Vip // product => vip list
}

type VipConf struct {
	Version string      // version of the config
	VipMap  Vip2Product // vip => product
}

func (conf *VipTableConf) LoadAndCheck(filename string) (string, error) {
	// open the file
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// decode the file
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(conf); err != nil {
		return "", err
	}

	// check config
	if err := VipTableConfCheck(conf); err != nil {
		return "", err
	}

	return conf.Version, nil
}

func VipTableConfCheck(conf *VipTableConf) error {
	if conf.Version == "" {
		return errors.New("no Version")
	}

	// check config for each product
	for product, vipList := range conf.Vips {
		var formattedVipList VipList
		for _, vip := range vipList {
			ip := net.ParseIP(vip)
			if ip == nil {
				return fmt.Errorf("invalid vip %s for %s", vip, product)
			}

			formattedVipList = append(formattedVipList, ip.String())
		}
		conf.Vips[product] = formattedVipList
	}
	return nil
}

// VipRuleConfLoad loads config of vip table from file.
func VipRuleConfLoad(filename string) (VipConf, error) {
	var vipConf VipConf

	// load vip config
	var config VipTableConf
	if _, err := config.LoadAndCheck(filename); err != nil {
		return vipConf, err
	}

	// convert from VipTableConf
	vipConf.Version = config.Version
	vipConf.VipMap = make(Vip2Product)
	for product, viplist := range config.Vips {
		for _, vip := range viplist {
			vipConf.VipMap[vip] = product
		}
	}

	return vipConf, nil
}

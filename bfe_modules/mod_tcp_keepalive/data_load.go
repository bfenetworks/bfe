// Copyright (c) 2021 The BFE Authors.
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

package mod_tcp_keepalive

import (
	"errors"
	"fmt"
	"net"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type ProductRuleConf struct {
	Version string
	Config  map[string]ProductRulesFile
}

type ProductRulesFile []ProductRuleFile
type ProductRuleFile struct {
	VipConf        []string
	KeepAliveParam KeepAliveParam
}

type ProductRuleData struct {
	Version string
	Config  ProductRules
}

type KeepAliveParam struct {
	Disable   bool // close the TCP-KeepAlive heartbeat message sending strategy
	KeepIdle  int  // period to send heartbeat message since there is no data transport in tcp connection
	KeepIntvl int  // period to send heartbeat message again when last message is not applied
	KeepCnt   int  // count to resend heartbeat message when last message is not applied
}
type KeepAliveRules map[string]KeepAliveParam
type ProductRules map[string]KeepAliveRules

func ConvertConf(c ProductRuleConf) (ProductRuleData, error) {
	data := ProductRuleData{}
	data.Version = c.Version
	data.Config = ProductRules{}

	for product, rules := range c.Config {
		data.Config[product] = KeepAliveRules{}
		for _, rule := range rules {
			for _, val := range rule.VipConf {
				ip, err := formatIP(val)
				if err != nil {
					return data, err
				}
				if _, ok := data.Config[product][ip]; ok {
					return data, fmt.Errorf("duplicated ip[%s] in product[%s]", val, product)
				}
				data.Config[product][ip] = rule.KeepAliveParam
			}
		}
	}

	return data, nil
}

func RulesCheck(conf KeepAliveRules) error {
	for ip, val := range conf {
		if net.ParseIP(ip) == nil {
			return fmt.Errorf("invalid ip: %s", ip)
		}

		if val.KeepIdle < 0 || val.KeepIntvl < 0 || val.KeepCnt < 0 {
			return fmt.Errorf("invalid keepalive param: %+v", val)
		}
	}

	return nil
}

func ProductRulesCheck(conf ProductRules) error {
	for product, rules := range conf {
		if product == "" {
			return fmt.Errorf("no product name")
		}
		if rules == nil {
			return fmt.Errorf("no rules for product: %s", product)
		}

		err := RulesCheck(rules)
		if err != nil {
			return fmt.Errorf("ProductRules:%s, %s", product, err.Error())
		}
	}

	return nil
}

func ProductRuleDataCheck(conf ProductRuleData) error {
	var err error

	// check Version
	if conf.Version == "" {
		return errors.New("no Version")
	}

	// check Config
	if conf.Config == nil {
		return errors.New("no Config")
	}

	err = ProductRulesCheck(conf.Config)
	if err != nil {
		return fmt.Errorf("Config: %s", err.Error())
	}

	return nil
}

func formatIP(s string) (string, error) {
	ip := net.ParseIP(s)
	if ip == nil {
		return "", fmt.Errorf("formatIP: net.ParseIP() error, ip: %s", s)
	}

	ret := ip.String()
	if ret == "<nil>" {
		return "", fmt.Errorf("formatIP: ip.String() error, ip: %s", s)
	}

	return ret, nil
}

func KeepAliveDataLoad(filename string) (ProductRuleData, error) {
	var err error
	var conf ProductRuleConf
	var data ProductRuleData

	// open the file
	file, err := os.Open(filename)
	if err != nil {
		return data, err
	}
	defer file.Close()

	// decode the file
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&conf)
	if err != nil {
		return data, err
	}

	// convert to ProductRuleData
	data, err = ConvertConf(conf)
	if err != nil {
		return data, err
	}

	// check data
	err = ProductRuleDataCheck(data)
	if err != nil {
		return data, err
	}

	return data, nil
}

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

package host_rule_conf

import (
	"errors"
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type HostnameList []string // list of hostname
type HostTagList []string  // list of host-tag

type HostTagToHost map[string]*HostnameList   // host-tag => hosts
type ProductToHostTag map[string]*HostTagList // product => host-tags

type Host2HostTag map[string]string    // hostname => host-tag
type HostTag2Product map[string]string // host-tag => product

type HostTableConf struct {
	Version        *string           // version of the config
	DefaultProduct *string           // default product
	Hosts          *HostTagToHost    // host-tag => hosts
	HostTags       *ProductToHostTag // product => host-tags
}

type HostConf struct {
	Version        string          // version of the config
	DefaultProduct string          // default product
	HostMap        Host2HostTag    // hostname => host-tag
	HostTagMap     HostTag2Product // host-tag => product
}

func (conf *HostTableConf) LoadAndCheck(filename string) (string, error) {
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
	if err := HostTableConfCheck(*conf); err != nil {
		return "", err
	}

	return *(conf.Version), nil
}

// HostTableConfCheck check HostTableConf config
func HostTableConfCheck(conf HostTableConf) error {
	if conf.Version == nil {
		return errors.New("no Version")
	}

	if conf.Hosts == nil {
		return errors.New("no Hosts")
	}

	if conf.HostTags == nil {
		return errors.New("no HostTags")
	}

	// check config for each product
	for product, hostTagList := range *conf.HostTags {
		if hostTagList == nil {
			return fmt.Errorf("no HostTagList for %s", product)
		}
	}

	// check config for each host-tag
	for hostTag, hostnameList := range *conf.Hosts {
		if hostnameList == nil {
			return fmt.Errorf("no HostnameList for %s", hostTag)
		}

		find := false
	HOST_TAG_CHECK:
		// check host-tag in Hosts should exist in HostTags
		for _, hostTagList := range *conf.HostTags {
			for _, ht := range *hostTagList {
				if ht == hostTag {
					find = true
					break HOST_TAG_CHECK
				}
			}
		}

		if !find {
			return fmt.Errorf("hostTag[%s] in Hosts should also exist in HostTags!", hostTag)
		}
	}

	// if default product is set, defaultProduct must exist in HostTags
	if conf.DefaultProduct != nil {
		hostTags := *conf.HostTags
		_, ok := hostTags[*conf.DefaultProduct]
		if !ok {
			return fmt.Errorf("defaultProruct[%s], must exist in HostTags", *conf.DefaultProduct)
		}
	}

	return nil
}

// HostRuleConfLoad loads config of host table from file.
func HostRuleConfLoad(filename string) (HostConf, error) {
	var conf HostConf
	var config HostTableConf

	if _, err := config.LoadAndCheck(filename); err != nil {
		return conf, err
	}

	// convert HostTagToHost to Host2HostTag
	host2HostTag := make(Host2HostTag)

	for hostTag, hostnameList := range *config.Hosts {
		for _, hostName := range *hostnameList {
			if host2HostTag[hostName] != "" {
				return conf, fmt.Errorf("host duplicate for %s", hostName)
			}
			host2HostTag[hostName] = hostTag
		}
	}

	// convert ProductToHostTag to HostTag2Product
	hostTag2Product := make(HostTag2Product)

	for product, hostTagList := range *config.HostTags {
		for _, hostTag := range *hostTagList {
			hostTag2Product[hostTag] = product
		}
	}

	// convert default product
	if config.DefaultProduct != nil {
		conf.DefaultProduct = *config.DefaultProduct
	}

	conf.Version = *config.Version
	conf.HostMap = host2HostTag
	conf.HostTagMap = hostTag2Product

	return conf, nil
}

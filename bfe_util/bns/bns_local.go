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

package bns

import (
	"fmt"
	"os"
	"sync"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type LocalNameConf map[string][]Instance

type NameConf struct {
	Version string
	Config  LocalNameConf
}

var localNameConf LocalNameConf
var localNameLock sync.RWMutex

// LoadLocalNameConf loads name conf file
func LoadLocalNameConf(filename string) error {
	// load local name conf
	nameConf, err := parseLocalNameConf(filename)
	if err != nil {
		return err
	}

	// update local name map
	localNameLock.Lock()
	localNameConf = nameConf
	localNameLock.Unlock()
	return nil
}

func getInstancesLocal(serviceName string) ([]Instance, bool) {
	localNameLock.RLock()
	instances, ok := localNameConf[serviceName]
	localNameLock.RUnlock()

	return instances, ok
}

func parseLocalNameConf(filename string) (LocalNameConf, error) {
	var conf NameConf
	var err error

	// open the file
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// decode the file
	decoder := json.NewDecoder(file)
	if err = decoder.Decode(&conf); err != nil {
		return nil, err
	}

	// check config
	err = checkLocalNameConf(conf)
	if err != nil {
		return nil, err
	}

	return conf.Config, nil
}

func checkLocalNameConf(conf NameConf) error {
	for name, instances := range conf.Config {
		for _, instance := range instances {
			if err := checkInstance(instance); err != nil {
				return fmt.Errorf("invalid instance for %s: %s", name, err)
			}
		}
	}
	return nil
}

func checkInstance(instance Instance) error {
	if len(instance.Host) == 0 {
		return fmt.Errorf("invalid host: %v", instance)
	}
	if instance.Port < 0 || instance.Port > 65535 {
		return fmt.Errorf("invalid port: %v", instance)
	}
	if instance.Weight < 0 {
		return fmt.Errorf("invalid weight: %v", instance)
	}
	return nil
}

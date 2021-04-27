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
)

type Client struct {
	//TODO: support thirdparty name service
}

func NewClient() *Client {
	return &Client{}
}

// Instance represents instance info
type Instance struct {
	Host   string // instance host
	Port   int    // instance port
	Weight int    // instance weight
}

// GetInstancesInfo returns instance addr and weight info of serviceName
func (c *Client) GetInstancesInfo(serviceName string) ([]Instance, error) {
	// check local conf
	if instances, ok := getInstancesLocal(serviceName); ok {
		return instances, nil
	}

	// check name service
	//TODO: support thirdparty name service
	return nil, fmt.Errorf("unknown name: %s", serviceName)
}

// GetInstancesAddr return instance addr info of serviceName
func (c *Client) GetInstancesAddr(serviceName string) ([]string, error) {
	instances, err := c.GetInstancesInfo(serviceName)
	if err != nil {
		return nil, err
	}

	addrList := make([]string, 0)
	for _, instance := range instances {
		address := fmt.Sprintf("%s:%d", instance.Host, instance.Port)
		addrList = append(addrList, address)
	}

	return addrList, nil
}

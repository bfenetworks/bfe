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

// load cluster conf from json file

package gslb_conf

import (
	"errors"
	"fmt"
	"os"
	"reflect"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/json"
)

var (
	ErrGslbNoHostname = errors.New("no HostName")
	ErrGslbNoTs       = errors.New("no Ts")
)

// GslbClusterConf is gslb conf for one cluster
type GslbClusterConf map[string]int // sub_cluster_name => weight

// GslbClustersConf is gslb conf for multiple clusters
type GslbClustersConf map[string]GslbClusterConf // cluster_name => conf

// GslbConf is conf of GSLB
type GslbConf struct {
	Clusters *GslbClustersConf // gslb conf for multiple clusters

	// hostname of gslb scheduler,
	// e.g., "gslb-sch.example.com"
	Hostname *string
	Ts       *string // timestamp, e.g., "20140516151616"
}

func (gslb GslbClustersConf) HasDiff(compared GslbClustersConf) bool {
	if len(gslb) != len(compared) {
		return true
	}

	for cluster, gslbClusterConf := range gslb {
		comparedGslbClusterConf, ok := (compared)[cluster]
		if !ok {
			return true
		}

		if gslbClusterConf.HasDiff(comparedGslbClusterConf) {
			return true
		}
	}

	return false
}

// IsSub compare two GslbConf and return true if compared contains all cluster in gslbConf
// and cluster has same GslbClusterConf.
func (gslbConf GslbConf) IsSub(compared GslbConf) bool {
	for cluster, gslbClusterConf := range *gslbConf.Clusters {
		comparedGslbClusterConf, ok := (*compared.Clusters)[cluster]
		if !ok {
			return false
		}

		if !gslbClusterConf.IsSame(comparedGslbClusterConf) {
			return false
		}
	}

	return true
}

// Check check GslbClusterConf conf.
func (conf GslbClusterConf) Check() error {
	total := 0
	for _, weight := range conf {
		if weight > 0 {
			total += weight
		}
	}

	// total <= 0, no available subcluster
	if total <= 0 {
		return errors.New("GslbClusterConf Check , total weight <= 0")
	}

	return nil
}

func (conf GslbClusterConf) IsSame(compared GslbClusterConf) bool {
	return !conf.HasDiff(compared)
}

func (conf GslbClusterConf) HasDiff(compared GslbClusterConf) bool {
	return !reflect.DeepEqual(conf, compared)
}

func (conf GslbClustersConf) Check() error {
	for cluster, clusterConf := range conf {
		if err := clusterConf.Check(); err != nil {
			return fmt.Errorf("[%s] check conf err [%s]", cluster, err)
		}
	}

	return nil
}

func (conf *GslbConf) Check() error {
	return GslbConfCheck(*conf)
}

func GslbConfNilCheck(conf GslbConf) error {
	if conf.Clusters == nil {
		return errors.New("no Clusters")
	}

	if conf.Hostname == nil {
		return ErrGslbNoHostname
	}

	if conf.Ts == nil {
		return ErrGslbNoTs
	}

	return nil
}

// GslbConfCheck check GslbConf.
func GslbConfCheck(conf GslbConf) error {
	if err := GslbConfNilCheck(conf); err != nil {
		return fmt.Errorf("Check Nil: %s", err)
	}

	if err := conf.Clusters.Check(); err != nil {
		return fmt.Errorf("Clusters check err %s", err)
	}

	return nil
}

// GslbConfLoad load gslb config from file.
func GslbConfLoad(filename string) (GslbConf, error) {
	var config GslbConf

	/* open the file    */
	file, err1 := os.Open(filename)
	if err1 != nil {
		return config, err1
	}

	/* decode the file  */
	decoder := json.NewDecoder(file)
	err2 := decoder.Decode(&config)
	file.Close()
	if err2 != nil {
		return config, err2
	}

	// check config
	err3 := GslbConfCheck(config)
	if err3 != nil {
		return config, err3
	}

	return config, nil
}

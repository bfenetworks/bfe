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

// load cluster table from json file

package cluster_table_conf

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"reflect"
	"sort"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/json"
)

// BackendConf is conf of backend
type BackendConf struct {
	Name   *string // e.g., "a-05.a"
	Addr   *string // e.g., "10.26.35.33"
	Port   *int    // e.g., 8000
	Weight *int    // weight in load balance, e.g., 10
}

func (b *BackendConf) AddrInfo() string {
	return fmt.Sprintf("%s:%d", *b.Addr, *b.Port)
}

type SubClusterBackend []*BackendConf
type ClusterBackend map[string]SubClusterBackend
type AllClusterBackend map[string]ClusterBackend

func (s SubClusterBackend) Len() int { return len(s) }
func (s SubClusterBackend) Less(i, j int) bool {
	if *s[i].Addr != *s[j].Addr {
		return *s[i].Addr < *s[j].Addr
	}

	return *s[i].Port < *s[j].Port
}
func (s SubClusterBackend) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// Sort sorted backends by addr and port
func (s SubClusterBackend) Sort() {
	sort.Sort(s)
}

// Shuffle random shuffle backends
func (s SubClusterBackend) Shuffle() {
	for i := len(s) - 1; i > 1; i-- {
		j := rand.Intn(i + 1)
		s[i], s[j] = s[j], s[i]
	}
}

// Sort sortes all the backends in allClusterBackend
func (allClusterBackend AllClusterBackend) Sort() {
	for _, clusterBackend := range allClusterBackend {
		for _, backends := range clusterBackend {
			backends.Sort()
		}
	}
}

// Shuffle shuffles all the backends in allClusterBackend
func (allClusterBackend AllClusterBackend) Shuffle() {
	for _, clusterBackend := range allClusterBackend {
		for _, backends := range clusterBackend {
			backends.Shuffle()
		}
	}
}

func (allClusterBackend AllClusterBackend) HasDiff(compared AllClusterBackend) bool {
	if len(allClusterBackend) != len(compared) {
		return true
	}

	for cluster, clusterBackend := range allClusterBackend {
		comparedClusterBackend, ok := compared[cluster]
		if !ok {
			return true
		}

		if clusterBackend.HasDiff(comparedClusterBackend) {
			return true
		}
	}

	return false
}

// IsSub Compare two AllClusterBackend, return true if compared contains  all cluster
// in allClusterBackend, and there cluster has same ClusterBackend value.
func (allClusterBackend AllClusterBackend) IsSub(compared AllClusterBackend) bool {
	for cluster, clusterBackend := range allClusterBackend {
		comparedClusterBackend, ok := compared[cluster]
		if !ok {
			return false
		}

		if !clusterBackend.IsSame(comparedClusterBackend) {
			return false
		}
	}

	return true
}

func (clusterBackend ClusterBackend) HasDiff(compared ClusterBackend) bool {
	return !reflect.DeepEqual(clusterBackend, compared)
}

func (clusterBackend ClusterBackend) IsSame(compared ClusterBackend) bool {
	return !clusterBackend.HasDiff(compared)
}

// ClusterTableConf is conf of cluster
type ClusterTableConf struct {
	Version *string // version of config
	Config  *AllClusterBackend
}

// BackendConfCheck check BackendConf config
func BackendConfCheck(conf *BackendConf) error {
	if conf.Name == nil {
		return errors.New("no Name")
	}

	if conf.Addr == nil {
		return errors.New("no Addr")
	}

	if conf.Port == nil {
		return errors.New("no Port")
	}

	if conf.Weight == nil {
		return errors.New("no Weight")
	}

	return nil
}

func (conf *AllClusterBackend) Check() error {
	return AllClusterBackendCheck(conf)
}

func (s *SubClusterBackend) Check() error {
	availBackend := false
	for index, backendConf := range *s {
		err := BackendConfCheck(backendConf)

		if err != nil {
			return fmt.Errorf("%d %s", index, err)
		}

		if *backendConf.Weight > 0 {
			availBackend = true
		}
	}

	if !availBackend {
		return fmt.Errorf("no avail backend")
	}

	return nil
}

// AllClusterBackendCheck check AllClusterBackend config
func AllClusterBackendCheck(conf *AllClusterBackend) error {
	for clusterName, clusterBackend := range *conf {
		for subClusterName, subClusterBackend := range clusterBackend {
			if err := subClusterBackend.Check(); err != nil {
				return fmt.Errorf("%s %s %s", clusterName, subClusterName, err)
			}
		}
	}
	return nil
}

// ClusterTableConfCheck check ClusterTableConf config
func ClusterTableConfCheck(conf ClusterTableConf) error {
	if conf.Version == nil {
		return errors.New("no Version")
	}

	if conf.Config == nil {
		return errors.New("no Config")
	}

	err := AllClusterBackendCheck(conf.Config)
	if err != nil {
		return fmt.Errorf("ClusterTableConf.Config:%s", err.Error())
	}

	return nil
}

// ClusterTableLoad loads config of cluster table from file
func ClusterTableLoad(filename string) (ClusterTableConf, error) {
	var config ClusterTableConf

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
	err3 := ClusterTableConfCheck(config)
	if err3 != nil {
		return config, err3
	}

	return config, nil
}

// ClusterTableDump dumps conf to file
func ClusterTableDump(conf ClusterTableConf, filename string) error {
	// marshal to json
	confJson, err := json.Marshal(conf)
	if err != nil {
		return err
	}

	// dump to file
	err = ioutil.WriteFile(filename, confJson, 0644)
	if err != nil {
		return err
	}

	return nil
}

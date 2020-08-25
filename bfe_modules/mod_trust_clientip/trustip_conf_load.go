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

package mod_trust_clientip

import (
	"errors"
	"fmt"
	"net"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type AddrScopeFile struct {
	Begin *string // start, e.g,. 119.75.215.0
	End   *string // end, e.g., 119.75.215.255
}

type AddrScope struct {
	Begin net.IP // start
	End   net.IP // end
}

type AddrScopeFileList []AddrScopeFile
type AddrScopeList []AddrScope

type SrcScopeMapFile map[string]*AddrScopeFileList // source => list of addr scope
type SrcScopeMap map[string]*AddrScopeList         // source => list of addr scope

type TrustIPConfFile struct {
	Version *string // version of the config
	Config  *SrcScopeMapFile
}

type TrustIPConf struct {
	Version string // version of the config
	Config  SrcScopeMap
}

func AddrScopeListCheck(conf *AddrScopeFileList) error {
	for index, scope := range *conf {
		// the check for ip address format will be done in convert function
		if scope.Begin == nil {
			return fmt.Errorf("%d:no start", index)
		}

		if scope.End == nil {
			return fmt.Errorf("%d:no end", index)
		}
	}
	return nil
}

func TrustIPConfCheck(conf *TrustIPConfFile) error {
	if conf.Version == nil {
		return errors.New("no Version")
	}

	if conf.Config == nil {
		return errors.New("no Config")
	}

	// check config for each source
	for src, scopeList := range *conf.Config {
		if scopeList == nil {
			return fmt.Errorf("no conf for src:%s", src)
		}

		if err := AddrScopeListCheck(scopeList); err != nil {
			return fmt.Errorf("src %s:%s", src, err.Error())
		}
	}

	return nil
}

// TrustIPConfLoad loads config of trust-ip from file
func TrustIPConfLoad(filename string) (TrustIPConf, error) {
	var conf TrustIPConf

	// open the file
	file, err1 := os.Open(filename)

	if err1 != nil {
		return conf, err1
	}

	// decode the file
	decoder := json.NewDecoder(file)

	config := TrustIPConfFile{}
	err2 := decoder.Decode(&config)
	file.Close()

	if err2 != nil {
		return conf, err2
	}

	// check config
	err3 := TrustIPConfCheck(&config)
	if err3 != nil {
		return conf, err3
	}

	/* convert config   */
	conf.Version = *config.Version
	conf.Config = make(SrcScopeMap)

	for src, scopeListFile := range *config.Config {
		scopeList := new(AddrScopeList)
		*scopeList = make([]AddrScope, 0)

		for index, scopeFile := range *scopeListFile {
			var startAddr, endAddr net.IP

			if startAddr = net.ParseIP(*scopeFile.Begin); startAddr == nil {
				return conf, fmt.Errorf("%d:illegal begin:%s", index, *scopeFile.Begin)
			}

			if endAddr = net.ParseIP(*scopeFile.End); endAddr == nil {
				return conf, fmt.Errorf("%d:illegal end:%s", index, *scopeFile.End)
			}

			scope := AddrScope{}
			scope.Begin = startAddr
			scope.End = endAddr

			*scopeList = append(*scopeList, scope)
		}

		conf.Config[src] = scopeList
	}

	return conf, nil
}

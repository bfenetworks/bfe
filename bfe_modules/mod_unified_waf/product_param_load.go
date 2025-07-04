// Copyright (c) 2025 The BFE Authors.
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

package mod_unified_waf

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/bfenetworks/bfe/bfe_util"
)

// product waf parameters
type WafParam struct {
	SendBody     bool // is need to send http body
	SendBodySize int  // send how many bytes of body
}

// each product's waf param
// key is product name
type ProductParams map[string]WafParam

// product parameters in config file
type ProductParamConfFile struct {
	Version *string        // version string
	Config  *ProductParams // product param
}

type ProductParamConf struct {
	Version string
	Config  ProductParams
}

func (cfg *ProductParamConfFile) Check() error {
	if err := bfe_util.CheckNilField(*cfg, false); err != nil {
		return err
	}

	if cfg.Config != nil {
		// check ProductWafFile
		for product, param := range *cfg.Config {
			if err := param.Check(); err != nil {
				return fmt.Errorf("%s: %s", product, err.Error())
			}
		}
	}

	return nil
}

func (p *WafParam) Check() error {
	if p.SendBodySize < 0 {
		return fmt.Errorf("SendBodySize should >= 0")
	}

	if p.SendBody && p.SendBodySize <= 0 {
		return fmt.Errorf("SendBody and SendBodySize should > 0")
	}

	return nil
}

// reload_trigger adaptor interface
func ProductParamLoadAndCheck(filename string) (ProductParamConf, error) {
	var err error
	var data ProductParamConf

	// open the file
	file, err := os.Open(filename)
	if err != nil {
		return data, err
	}
	defer file.Close()

	// decode the file
	decoder := json.NewDecoder(file)
	var dataFile ProductParamConfFile
	err = decoder.Decode(&dataFile)
	if err != nil {
		return data, err
	}

	// check config
	if err := dataFile.Check(); err != nil {
		return data, err
	}

	// convert config
	data.Version = *dataFile.Version
	if dataFile.Config != nil {
		data.Config = *dataFile.Config
	}

	return data, nil
}

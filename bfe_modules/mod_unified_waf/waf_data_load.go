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

	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_util"
)

const (
	DEFAULT_POOL_SIZE   = 8    // default waf client connection pool size
	DEFAULT_CONCURRENCY = 2000 // default waf client concurrency

	DEFAULT_MAX_WAIT_RATE        = 0.2 // default client max waiting rate
	DEFAULT_HALF_OPEN_THRES_RATE = 0.2 // default client half open thres rate
	DEFAULT_CLOSED_THRES_RATE    = 0.8 // default client closed thres rate
	DEFAULT_SAMPLE_PERCENT       = 5.0 // default client sample rate under half-open
	DEFAULT_STAT_TOTAL_OP_COUNT  = 100 // default client sample rate under half-open
)

type HealthCheckerConfFile struct {
	UnavailableFailedThres int64 //unavailable failed threshold
	HealthCheckInterval    int64 //health check interval(ms)
}

// global param for mod_unified_waf
type GlobalParamFile struct {
	WafClient struct {
		ConnectTimeout int // connect timeout for waf client
		Concurrency    int // how many concurrency call for one waf client
		//ConnPoolSize   int //connection pool size
		MaxWaitCount int //max wait rate for request waiting for token
	}

	WafDetect struct {
		ReqTimeout int // total timeout for a request detecting
		RetryMax   int // max retry number in each request detecting
	}

	HealthChecker HealthCheckerConfFile
}

// global param in config file
type GlobalParamConfFile struct {
	Version *string          // version string
	Config  *GlobalParamFile // global param for mod_unified_waf
}

// global param for mod_unified_waf
type GlobalParam struct {
	WafClient struct {
		ConnectTimeout int // connect timeout for waf client
		Concurrency    int // how many concurrency call for one waf client
		//ConnPoolSize   int //connection pool size
		MaxWaitCount int //max wait rate for request waiting for token
	}

	WafDetect struct {
		RetryMax   int // max retry number in each request detecting
		ReqTimeout int // total timeout for a request detecting
	}

	HealthChecker HealthCheckerConf
}

func (p *GlobalParam) GetReqTimeout(bodySize int) int {
	return p.WafDetect.ReqTimeout
}

type GlobalParamConf struct {
	Version string
	Config  GlobalParam
}

func (cfg *GlobalParamConfFile) Check() error {
	if err := bfe_util.CheckNilField(*cfg, false); err != nil {
		return err
	}

	if err := cfg.Config.Check(); err != nil {
		return err
	}

	return nil
}

func (p *GlobalParamFile) Check() error {
	if p.WafClient.ConnectTimeout <= 0 {
		return fmt.Errorf("WafClient.ConnectTimeout > 0")
	}

	if p.WafClient.Concurrency <= 0 {
		p.WafClient.Concurrency = DEFAULT_CONCURRENCY
		log.Logger.Warn("Concurrency is : %d, use DEFAULT_CONCURRENCY(%d)", p.WafClient.Concurrency, DEFAULT_CONCURRENCY)
	}

	if p.HealthChecker.UnavailableFailedThres <= 0 {
		return fmt.Errorf("WafClient.HealthChecker.UnavailableFailedThres <= 0")
	}

	if p.HealthChecker.HealthCheckInterval <= 0 {
		return fmt.Errorf("WafClient.HealthChecker.HealthCheckInterval <= 0")
	}

	if p.WafClient.MaxWaitCount <= 0 {
		return fmt.Errorf("WafClient.MaxWaitCount <= 0")
	}

	if p.WafDetect.RetryMax < 0 {
		return fmt.Errorf("WafDetect.RetryMax < 0")
	}

	if p.WafDetect.ReqTimeout <= 0 {
		return fmt.Errorf("WafDetect.ReqTimeout <= 0")
	}

	return nil
}

func (cfg *GlobalParamConfFile) cvtToConf() (*GlobalParamConf, error) {
	var data GlobalParamConf
	data.Version = *cfg.Version

	//data.Config = *dataFile.Config
	data.Config.WafClient.MaxWaitCount = cfg.Config.WafClient.MaxWaitCount
	data.Config.WafClient.ConnectTimeout = cfg.Config.WafClient.ConnectTimeout
	data.Config.WafClient.Concurrency = cfg.Config.WafClient.Concurrency

	data.Config.WafDetect.RetryMax = cfg.Config.WafDetect.RetryMax
	data.Config.WafDetect.ReqTimeout = cfg.Config.WafDetect.ReqTimeout

	data.Config.HealthChecker.HealthCheckInterval = cfg.Config.HealthChecker.HealthCheckInterval
	data.Config.HealthChecker.UnavailableFailedThres = cfg.Config.HealthChecker.UnavailableFailedThres

	return &data, nil
}

// reload_trigger adaptor interface
func WafDataParamLoadAndCheck(filename string) (*GlobalParamConf, error) {
	var err error
	// var data GlobalParamConf

	// open the file
	file, err := os.Open(filename)
	defer file.Close()
	if err != nil {
		return nil, err
	}

	// decode the file
	decoder := json.NewDecoder(file)
	var dataFile GlobalParamConfFile
	err = decoder.Decode(&dataFile)
	if err != nil {
		return nil, err
	}

	// check config
	if err := dataFile.Check(); err != nil {
		return nil, err
	}

	// convert config
	tdata, err := dataFile.cvtToConf()
	if err != nil {
		return nil, err
	}
	return tdata, nil
}

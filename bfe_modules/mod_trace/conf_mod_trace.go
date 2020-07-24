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

package mod_trace

import (
	"fmt"
)

import (
	"github.com/baidu/go-lib/log"
	"gopkg.in/gcfg.v1"
)

import (
	"github.com/bfenetworks/bfe/bfe_modules/mod_trace/trace"
	"github.com/bfenetworks/bfe/bfe_modules/mod_trace/trace/elastic"
	"github.com/bfenetworks/bfe/bfe_modules/mod_trace/trace/jaeger"
	"github.com/bfenetworks/bfe/bfe_modules/mod_trace/trace/zipkin"
	"github.com/bfenetworks/bfe/bfe_util"
)

const (
	defaultDataPath = "mod_trace/trace_rule.data"
)

var (
	supportedTraceAgent = map[string]bool{
		zipkin.Name:  true,
		jaeger.Name:  true,
		elastic.Name: true,
	}
)

type ConfModTrace struct {
	Basic struct {
		DataPath    string // The path of rule data
		ServiceName string // The name of this service
		TraceAgent  string // The type of trace agent: zipkin, jaeger or elastic
	}

	Log struct {
		OpenDebug bool
	}

	Zipkin  zipkin.Config  // Settings for zipkin, only useful when TraceAgent is zipkin
	Jaeger  jaeger.Config  // Settings for jaeger, only useful when TraceAgent is jaeger
	Elastic elastic.Config // Settings for elastic, only useful when TraceAgent is elastic
}

func ConfLoad(filePath string, confRoot string) (*ConfModTrace, error) {
	var err error
	var cfg ConfModTrace

	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return nil, err
	}

	err = cfg.Check(confRoot)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (cfg *ConfModTrace) Check(confRoot string) error {
	if len(cfg.Basic.DataPath) == 0 {
		cfg.Basic.DataPath = defaultDataPath
		log.Logger.Warn("ModTrace.DataPath not set, use default value: %s", defaultDataPath)
	}
	cfg.Basic.DataPath = bfe_util.ConfPathProc(cfg.Basic.DataPath, confRoot)

	if len(cfg.Basic.TraceAgent) == 0 {
		return fmt.Errorf("ModTrace.TraceAgent not set")
	}

	if _, ok := supportedTraceAgent[cfg.Basic.TraceAgent]; !ok {
		return fmt.Errorf("ModTrace.TraceAgent %s is not supported", cfg.Basic.TraceAgent)
	}

	return nil
}

func (cfg *ConfModTrace) GetTraceConfig() trace.TraceAgent {
	switch cfg.Basic.TraceAgent {
	case jaeger.Name:
		return &cfg.Jaeger
	case zipkin.Name:
		return &cfg.Zipkin
	case elastic.Name:
		return &cfg.Elastic
	default:
		return &cfg.Jaeger
	}
}

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

package mod_otel

import (
	"gopkg.in/gcfg.v1"
)

type ConfModOtel struct {
	Basic struct {
		ServiceName string  // The name of this service
		Endpoint    string  // OTLP endpoint (e.g., localhost:4317)
		Insecure    bool    // Use insecure connection
		SampleRate  float64 // The rate between 0.0 and 1.0 of requests to trace
		Enabled     bool    // Whether to enable OpenTelemetry
	}

	Log struct {
		OpenDebug bool // Enable debug logging
	}
}

func ConfLoad(filePath string, confRoot string) (*ConfModOtel, error) {
	var cfg ConfModOtel

	err := gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return nil, err
	}

	cfg.SetDefaults()

	return &cfg, nil
}

func (cfg *ConfModOtel) SetDefaults() {
	if cfg.Basic.ServiceName == "" {
		cfg.Basic.ServiceName = "bfe"
	}
	if cfg.Basic.Endpoint == "" {
		cfg.Basic.Endpoint = "localhost:4317"
	}
	if cfg.Basic.SampleRate <= 0 || cfg.Basic.SampleRate > 1 {
		cfg.Basic.SampleRate = 1.0
	}
}
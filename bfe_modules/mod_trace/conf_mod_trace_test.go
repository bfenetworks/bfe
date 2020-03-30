// Copyright (c) 2019 Baidu, Inc.
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
	"testing"
)

func TestConfLoadCase1(t *testing.T) {
	cfg, err := ConfLoad("testdata/mod_trace/mod_trace.conf", "testdata")
	if err != nil {
		t.Fatalf("shoule have no error, but error is %v", err)
	}

	expectDataPath := "testdata/mod_trace/trace_rule_test.data"
	if cfg.Basic.DataPath != expectDataPath {
		t.Fatalf("cfg.Basic.DataPath shoule %s, but it's %s", expectDataPath, cfg.Basic.DataPath)
	}

	if cfg.Log.OpenDebug != false {
		t.Fatalf("cfg.Log.OpenDebug should be false")
	}

	expectSampleRate := 1.0
	if cfg.Zipkin.SampleRate != expectSampleRate {
		t.Fatalf("cfg.Zipkin.SampleRate should be %f, but it's %f", expectSampleRate, cfg.Zipkin.SampleRate)
	}

	if cfg.Zipkin.SameSpan != false {
		t.Fatalf("cfg.Zipkin.SameSpan should be false")
	}

	if cfg.Zipkin.ID128Bit != true {
		t.Fatalf("cfg.Zipkin.ID128Bit should be true")
	}
}

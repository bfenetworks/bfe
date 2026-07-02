// Copyright (c) 2026 The BFE Authors.
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

package mod_ai_rate_limit

import (
	"testing"
)

func TestConfLoadNormalCase1(t *testing.T) {
	fileName := "testdata/mod_ai_rate_limit/mod_ai_rate_limit.conf"
	conf, err := ConfLoad(fileName, "")
	if err != nil {
		t.Error("load conf should success")
		return
	}

	if conf.Basic.ProductRulePath != "../data/mod_ai_rate_limit/ai_rate_limit.data" {
		t.Error("ProductRulePath != ../data/mod_ai_rate_limit/ai_rate_limit.data", conf.Basic.ProductRulePath)
		return
	}

	if conf.Redis.Bns != "BLB.ALB-redis" {
		t.Error("Redis.Bns != BLB.ALB-redis", conf.Redis.Bns)
		return
	}

	if conf.Redis.ConnectTimeout != 20 {
		t.Error("Redis.ConnectTimeout != 20", conf.Redis.ConnectTimeout)
		return
	}

	if conf.Redis.ReadTimeout != 20 {
		t.Error("Redis.ReadTimeout != 20", conf.Redis.ReadTimeout)
		return
	}

	if conf.Redis.WriteTimeout != 20 {
		t.Error("Redis.WriteTimeout != 20", conf.Redis.WriteTimeout)
		return
	}

	if conf.Redis.MaxIdle != 20 {
		t.Error("Redis.MaxIdle != 20", conf.Redis.MaxIdle)
		return
	}

	if conf.Log.OpenDebug != true {
		t.Error("Log.OpenDebug != true", conf.Log.OpenDebug)
		return
	}
}

func TestConfLoadErrCase1(t *testing.T) {
	fileName := "testdata/mod_ai_rate_limit/mod_ai_rate_limit.conf.1"
	if _, err := ConfLoad(fileName, ""); err == nil {
		t.Error("connectTimeout == 0, load conf should failed")
		return
	}
}

func TestConfLoadErrCase2(t *testing.T) {
	fileName := "testdata/mod_ai_rate_limit/mod_ai_rate_limit.conf.2"
	if _, err := ConfLoad(fileName, ""); err == nil {
		t.Error("readTimeout == 0, load conf should failed")
		return
	}
}

func TestConfLoadErrCase3(t *testing.T) {
	fileName := "testdata/mod_ai_rate_limit/mod_ai_rate_limit.conf.3"
	if _, err := ConfLoad(fileName, ""); err == nil {
		t.Error("writeTimeout == 0, load conf should failed")
		return
	}
}

func TestConfLoadErrCase4(t *testing.T) {
	fileName := "testdata/mod_ai_rate_limit/mod_ai_rate_limit.conf.4"
	if _, err := ConfLoad(fileName, ""); err == nil {
		t.Error("ProductRulePath is empty, load conf should failed")
		return
	}
}

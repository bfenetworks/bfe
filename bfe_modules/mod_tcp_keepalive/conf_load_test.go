// Copyright (c) 2021 The BFE Authors.
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

package mod_tcp_keepalive

import (
	"testing"
)

func TestConfModTcpKeepAlive_1(t *testing.T) {
	config, err := ConfLoad("./testdata/mod_tcp_keepalive.conf", "")
	if err != nil {
		t.Errorf("TcpKeepAlive ConfLoad() error: %s", err.Error())
		return
	}

	if config.Basic.DataPath != "../data/mod_tcp_keepalive/tcp_keepalive.data" {
		t.Error("DataPath should be ../data/mod_tcp_keepalive/tcp_keepalive.data")
		return
	}

	if config.Log.OpenDebug != true {
		t.Error("Log.OpenDebug should be true")
		return
	}
}

func TestConfModTcpKeepAlive_2(t *testing.T) {
	// invalid value
	config, err := ConfLoad("./testdata/mod_tcp_keepalive_2.conf", "")
	if err != nil {
		t.Errorf("CondLoad() error: %s", err.Error())
	}

	// use default value
	if config.Basic.DataPath != "mod_tcp_keepalive/tcp_keepalive.data" {
		t.Error("DataPath should be mod_tcp_keepalive/tcp_keepalive.data")
		return
	}
}

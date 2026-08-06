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

package mod_access_pb3

import (
	"fmt"
	"testing"
)

/* load config from config file    */
func Test_conf_mod_access_pb2_case1(t *testing.T) {
	config, err := ConfLoad("./test_data/conf_mod_access_pb2/bfe_1.conf")

	if err != nil {
		msg := fmt.Sprintf("BfeConfigLoad():err=%s", err.Error())
		t.Error(msg)
		return
	}

	if config.Log.LogPrefix != "pb_access3" {
		t.Error("LogPrefix should be pb_access3")
	}

	if config.Log.LogDir != "/home/work/bfe/log" {
		t.Error("LogDir should be /home/work/bfe/log")
	}

	if config.BasicConf.OpenDebug != true {
		t.Error("OpenDebug should be true")
	}
}

func TestConfLoadNotExist(t *testing.T) {
	_, err := ConfLoad("./test_data/conf_mod_access_pb2/not_exist.conf")
	if err == nil {
		t.Error("ConfLoad() should return error for non-existent file")
	}
}

func TestConfModAccessPbCheckEmptyLogPrefix(t *testing.T) {
	cfg := &ConfModAccessPb2{}
	cfg.Log.LogDir = "/home/work/bfe/log"
	cfg.Log.RotateWhen = "NEXTHOUR"
	cfg.Log.BackupCount = 2

	err := ConfModAccessPbCheck(cfg)
	if err == nil {
		t.Error("ConfModAccessPbCheck() should return error when LogPrefix is empty")
	}
}

func TestConfModAccessPbCheckEmptyLogDir(t *testing.T) {
	cfg := &ConfModAccessPb2{}
	cfg.Log.LogPrefix = "pb_access3"
	cfg.Log.RotateWhen = "NEXTHOUR"
	cfg.Log.BackupCount = 2

	err := ConfModAccessPbCheck(cfg)
	if err == nil {
		t.Error("ConfModAccessPbCheck() should return error when LogDir is empty")
	}
}

func TestConfModAccessPbCheckInvalidRotateWhen(t *testing.T) {
	cfg := &ConfModAccessPb2{}
	cfg.Log.LogPrefix = "pb_access3"
	cfg.Log.LogDir = "/home/work/bfe/log"
	cfg.Log.RotateWhen = "INVALID"
	cfg.Log.BackupCount = 2

	err := ConfModAccessPbCheck(cfg)
	if err == nil {
		t.Error("ConfModAccessPbCheck() should return error when RotateWhen is invalid")
	}
}

func TestConfModAccessPbCheckInvalidBackupCount(t *testing.T) {
	cfg := &ConfModAccessPb2{}
	cfg.Log.LogPrefix = "pb_access3"
	cfg.Log.LogDir = "/home/work/bfe/log"
	cfg.Log.RotateWhen = "NEXTHOUR"
	cfg.Log.BackupCount = 0

	err := ConfModAccessPbCheck(cfg)
	if err == nil {
		t.Error("ConfModAccessPbCheck() should return error when BackupCount <= 0")
	}
}

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

package main

import (
	"flag"
	"fmt"
	"path"
	"runtime"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/log/log4go"
)

import (
	"github.com/baidu/bfe/bfe_config/bfe_conf"
	"github.com/baidu/bfe/bfe_debug"
	"github.com/baidu/bfe/bfe_server"
	"github.com/baidu/bfe/bfe_util"
)

var (
	help     *bool   = flag.Bool("h", false, "to show help")
	confRoot *string = flag.String("c", "./conf", "root path of configuration")
	logPath  *string = flag.String("l", "./log", "dir path of log")
	stdOut   *bool   = flag.Bool("s", false, "to show log in stdout")
	showVer  *bool   = flag.Bool("v", false, "to show version of bfe")
	debugLog *bool   = flag.Bool("d", false, "to show debug log (otherwise >= info)")
)

var version string

func main() {
	var err error
	var config bfe_conf.BfeConfig
	var logSwitch string

	flag.Parse()
	if *help {
		flag.PrintDefaults()
		return
	}
	if *showVer {
		fmt.Printf("bfe: version %s\n", version)
		return
	}

	// debug switch
	if *debugLog {
		logSwitch = "DEBUG"
		bfe_debug.DebugIsOpen = true
	} else {
		logSwitch = "INFO"
		bfe_debug.DebugIsOpen = false
	}

	// initialize log
	log4go.SetLogBufferLength(10000)
	log4go.SetLogWithBlocking(false)
	log4go.SetLogFormat(log4go.FORMAT_DEFAULT_WITH_PID)
	log4go.SetSrcLineForBinLog(false)

	err = log.Init("bfe", logSwitch, *logPath, *stdOut, "midnight", 7)
	if err != nil {
		fmt.Printf("bfe: err in log.Init():%s\n", err.Error())
		bfe_util.AbnormalExit()
	}

	log.Logger.Info("bfe[version:%s] start", version)

	// load server config
	confPath := path.Join(*confRoot, "bfe.conf")
	config, err = bfe_conf.BfeConfigLoad(confPath, *confRoot)
	if err != nil {
		log.Logger.Error("main(): in BfeConfigLoad():%s", err.Error())
		bfe_util.AbnormalExit()
	}

	// set maximum number of cpus
	runtime.GOMAXPROCS(config.Server.MaxCpus)

	// set log level
	bfe_debug.SetDebugFlag(config.Server)

	// start and serve
	if err = bfe_server.StartUp(config, version, *confRoot); err != nil {
		log.Logger.Error("main(): bfe_server.StartUp(): %s", err.Error())
	}

	// waiting for logger finish jobs
	time.Sleep(1 * time.Second)
	log.Logger.Close()
}

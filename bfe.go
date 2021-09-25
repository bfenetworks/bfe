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
	_ "go.uber.org/automaxprocs"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_conf"
	"github.com/bfenetworks/bfe/bfe_debug"
	"github.com/bfenetworks/bfe/bfe_server"
	"github.com/bfenetworks/bfe/bfe_util"
)

var (
	help        = flag.Bool("h", false, "to show help")
	confRoot    = flag.String("c", "./conf", "root path of configuration")
	logPath     = flag.String("l", "./log", "dir path of log")
	stdOut      = flag.Bool("s", false, "to show log in stdout")
	showVersion = flag.Bool("v", false, "to show version of bfe")
	showVerbose = flag.Bool("V", false, "to show verbose information about bfe")
	debugLog    = flag.Bool("d", false, "to show debug log (otherwise >= info)")
	testConf    = flag.Bool("t", false, "test configuration and exit")
)

var version string
var commit string

func main() {
	var err error
	var config bfe_conf.BfeConfig
	var logSwitch string

	flag.Parse()
	if *help {
		flag.PrintDefaults()
		return
	}
	if *showVersion {
		fmt.Printf("bfe version: %s\n", version)
		return
	}
	if *showVerbose {
		fmt.Printf("bfe version: %s\n", version)
		fmt.Printf("go version: %s\n", runtime.Version())
		fmt.Printf("git commit: %s\n", commit)
		return
	}

	// debug switch
	if *debugLog {
		logSwitch = "DEBUG"
		bfe_debug.DebugIsOpen = true
	} else {
		// ignore under ERROR level
		if *testConf {
			logSwitch = "ERROR"
		} else {
			logSwitch = "INFO"
		}
		bfe_debug.DebugIsOpen = false
	}

	// initialize log
	log4go.SetLogBufferLength(10000)
	log4go.SetLogWithBlocking(false)
	log4go.SetLogFormat(log4go.FORMAT_DEFAULT_WITH_PID)
	log4go.SetSrcLineForBinLog(false)

	err = log.Init("bfe", logSwitch, *logPath, *stdOut || *testConf, "midnight", 7)
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
		if *testConf {
			fmt.Printf("bfe: configuration file %s test failed\n", confPath)
		}
		bfe_util.AbnormalExit()
	}

	// maximum number of CPUs (GOMAXPROCS) defaults to runtime.CPUNUM
	// if running on machine, or CPU quota if running on container
	// (with the help of "go.uber.org/automaxprocs").
	// here, we change maximum number of cpus if the MaxCpus is positive.
	if config.Server.MaxCpus > 0 {
		runtime.GOMAXPROCS(config.Server.MaxCpus)
	}

	// set log level
	bfe_debug.SetDebugFlag(config.Server)

	// start and serve
	if err = bfe_server.StartUp(config, version, *confRoot, *testConf); err != nil {
		log.Logger.Error("main(): bfe_server.StartUp(): %s", err.Error())
	}

	// waiting for logger finish jobs
	time.Sleep(1 * time.Second)
	log.Logger.Close()

	// output final configuration test result
	if *testConf {
		if err != nil {
			fmt.Printf("bfe: configuration file %s test failed\n", confPath)
			bfe_util.AbnormalExit()
		} else {
			fmt.Printf("bfe: configuration file %s test is successful\n", confPath)
			bfe_util.NormalExit()
		}
	}
}

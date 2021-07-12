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

// create bfe service and init

package bfe_server

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_conf"
	"github.com/bfenetworks/bfe/bfe_modules"
)

func StartUp(cfg bfe_conf.BfeConfig, version string, confRoot string) error {
	var err error

	// set all available modules
	bfe_modules.SetModules()

	// create bfe server
	bfeServer := NewBfeServer(cfg, confRoot, version)

	// initial http
	if err = bfeServer.InitHttp(); err != nil {
		log.Logger.Error("StartUp(): InitHttp():%s", err.Error())
		return err
	}

	// initial https
	if err = bfeServer.InitHttps(); err != nil {
		log.Logger.Error("StartUp(): InitHttps():%s", err.Error())
		return err
	}

	// load data
	if err = bfeServer.InitDataLoad(); err != nil {
		log.Logger.Error("StartUp(): bfeServer.InitDataLoad():%s",
			err.Error())
		return err
	}
	log.Logger.Info("StartUp(): bfeServer.InitDataLoad() OK")

	// setup signal table
	bfeServer.InitSignalTable()
	log.Logger.Info("StartUp():bfeServer.InitSignalTable() OK")

	// init web monitor
	monitorPort := cfg.Server.MonitorPort
	if err = bfeServer.InitWebMonitor(monitorPort); err != nil {
		log.Logger.Error("StartUp(): InitWebMonitor():%s", err.Error())
		return err
	}

	// register modules
	if err = bfeServer.RegisterModules(cfg.Server.Modules); err != nil {
		log.Logger.Error("StartUp(): RegisterModules():%s", err.Error())
		return err
	}

	// initialize modules
	if err = bfeServer.InitModules(); err != nil {
		log.Logger.Error("StartUp(): bfeServer.InitModules():%s",
			err.Error())
		return err
	}
	log.Logger.Info("StartUp():bfeServer.InitModules() OK")

	// load plugins
	if err = bfeServer.LoadPlugins(cfg.Server.Plugins); err != nil {
		log.Logger.Error("StartUp():bfeServer.LoadPlugins():%s", err.Error())
		return err
	}

	// initialize plugins
	if err = bfeServer.InitPlugins(); err != nil {
		log.Logger.Error("StartUp():bfeServer.InitPlugins():%s",
			err.Error())
		return err
	}
	log.Logger.Info("StartUp():bfeServer.InitPlugins() OK")

	// initialize listeners
	if err = bfeServer.InitListeners(cfg); err != nil {
		log.Logger.Error("StartUp(): InitListeners():%v", err)
		return err
	}

	// start embedded web server
	bfeServer.Monitor.Start()

	serveChan := make(chan error)

	// start goroutine to accept http connections
	for i := 0; i < cfg.Server.AcceptNum; i++ {
		go func() {
			httpErr := bfeServer.ServeHttp(bfeServer.HttpListener)
			serveChan <- httpErr
		}()
	}

	// start goroutine to accept https connections
	for i := 0; i < cfg.Server.AcceptNum; i++ {
		go func() {
			httpsErr := bfeServer.ServeHttps(bfeServer.HttpsListener)
			serveChan <- httpsErr
		}()
	}

	err = <-serveChan
	return err
}

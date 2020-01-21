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

// create bfe service and init

package bfe_server

import (
	"fmt"
	"net"
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/baidu/bfe/bfe_config/bfe_conf"
	"github.com/baidu/bfe/bfe_modules"
)

func StartUp(cfg bfe_conf.BfeConfig, version string, confRoot string) error {
	// create listeners
	lnMap, err := createListeners(cfg)
	if err != nil {
		log.Logger.Error("StartUp(): createListeners():%s", err.Error())
		return err
	}

	// set all available modules
	bfe_modules.SetModules()

	// create bfe server
	bfeServer := NewBfeServer(cfg, lnMap, version)

	// initial http
	err = bfeServer.InitHttp()
	if err != nil {
		log.Logger.Error("StartUp(): InitHttp():%s", err.Error())
		return err
	}

	// initial https
	err = bfeServer.InitHttps()
	if err != nil {
		log.Logger.Error("StartUp(): InitHttps():%s", err.Error())
		return err
	}

	// load data
	err = bfeServer.InitDataLoad()
	if err != nil {
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
	err = bfeServer.InitWebMonitor(monitorPort)
	if err != nil {
		log.Logger.Error("StartUp(): InitWebMonitor():%s", err.Error())
		return err
	}

	// register modules
	err = bfeServer.RegisterModules(cfg.Server.Modules)
	if err != nil {
		log.Logger.Error("StartUp(): RegisterModules():%s", err.Error())
		return err
	}

	// initialize modules
	err = bfeServer.InitModules(confRoot)
	if err != nil {
		log.Logger.Error("StartUp(): bfeServer.InitModules():%s",
			err.Error())
		return err
	}
	log.Logger.Info("StartUp():bfeServer.InitModules() OK")

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

func createListeners(config bfe_conf.BfeConfig) (map[string]net.Listener, error) {
	lnMap := make(map[string]net.Listener)
	lnConf := map[string]int{
		"HTTP":  config.Server.HttpPort,
		"HTTPS": config.Server.HttpsPort,
	}

	for proto, port := range lnConf {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return nil, err
		}

		// wrap underlying listener according to balancer type
		listener = NewBfeListener(listener, config)
		lnMap[proto] = listener
		log.Logger.Info("createListeners(): begin to listen port[:%d]", port)
	}

	return lnMap, nil
}

func (p *BfeServer) closeListeners() {
	for _, ln := range p.listenerMap {
		if err := ln.Close(); err != nil {
			log.Logger.Error("closeListeners(): %s, %s", err, ln.Addr())
		}
	}
}

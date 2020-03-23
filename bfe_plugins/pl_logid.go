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
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

import (
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/baidu/bfe/bfe_basic"
	"github.com/baidu/bfe/bfe_module"
)

var Name = "pl_logid"
var Version = "0.0.1"
var Description = "plugin for bfe unified log id generation"

func Init(cbs *bfe_module.BfeCallbacks,
	whs *web_monitor.WebHandlers,
	cr string) error {

	// register handler
	err := cbs.AddFilter(bfe_module.HandleAccept, sessionIdHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.sessionIdHandler): %s", Name, err.Error())
	}

	return nil
}

func sessionIdHandler(session *bfe_basic.Session) int {
	session.SessionId = genLogId()

	return bfe_module.BfeHandlerGoOn
}

func genLogId() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

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

package bfe_conf

import (
	"testing"
)

import (
	gcfg "gopkg.in/gcfg.v1"
)

func confSessionTicketLoad(filePath string, confRoot string) (BfeConfig, error) {
	var cfg BfeConfig
	var err error

	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return cfg, err
	}

	// check basic conf
	err = cfg.SessionTicket.Check(confRoot)
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}

func TestConfSessionTicketLoad(t *testing.T) {
	conf, err := confSessionTicketLoad("testdata/conf_session_cache/bfe_1.conf", "./")
	if err != nil {
		t.Errorf("load config err: %s", err)
		return
	}
	ticketConf := conf.SessionTicket

	if ticketConf.SessionTicketsDisabled {
		t.Errorf("wrong SessionTicketsDisabled, expect false")
	}

	keyFileExpect := "tls_conf/session_ticket_key.data"
	if ticketConf.SessionTicketKeyFile != keyFileExpect {
		t.Errorf("wrong servers, expect %s, actual %s",
			keyFileExpect, ticketConf.SessionTicketKeyFile)
	}
}

func TestConfSessionTicketLoad2(t *testing.T) {
	confFile := "testdata/conf_session_ticket/bfe_2.conf"
	_, err := confSessionTicketLoad(confFile, "./")
	if err == nil {
		t.Errorf("should found err while loading config %s", confFile)
	}
}

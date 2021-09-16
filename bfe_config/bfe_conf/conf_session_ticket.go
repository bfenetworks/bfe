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
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_util"
)

type ConfigSessionTicket struct {
	// disable session cache or not
	SessionTicketsDisabled bool

	// session ticket key (in hex format)
	SessionTicketKeyFile string
}

func (cfg *ConfigSessionTicket) SetDefaultConf() {
	cfg.SessionTicketsDisabled = true
	cfg.SessionTicketKeyFile = "tls_conf/session_ticket_key.data"
}

func (cfg *ConfigSessionTicket) Check(confRoot string) error {
	if cfg.SessionTicketsDisabled {
		return nil
	}

	return ConfSessionTicketCheck(cfg, confRoot)
}

func ConfSessionTicketCheck(cfg *ConfigSessionTicket, confRoot string) error {
	// check session ticket key
	if cfg.SessionTicketKeyFile == "" {
		log.Logger.Warn("SessionTicketKeyFile not set, use default value")
		cfg.SessionTicketKeyFile = "tls_conf/server_ticket_key.data"
	}
	cfg.SessionTicketKeyFile = bfe_util.ConfPathProc(cfg.SessionTicketKeyFile, confRoot)

	return nil
}

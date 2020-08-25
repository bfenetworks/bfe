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

package session_ticket_key_conf

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

const (
	RawSessionTicketKeySize = 48 // bytes
)

// SessionTicketKeyConf is session ticket key config.
type SessionTicketKeyConf struct {
	Version          string // version of config
	SessionTicketKey string // session ticket key (hex encode)
}

// SessionTicketKeyConfCheck check integrity of config.
func SessionTicketKeyConfCheck(conf SessionTicketKeyConf) error {
	if len(conf.Version) == 0 {
		return fmt.Errorf("no Version")
	}

	key, err := hex.DecodeString(conf.SessionTicketKey)
	if err != nil {
		return fmt.Errorf("session ticket key %s(%s)", err.Error(), conf.SessionTicketKey)
	}
	if len(key) != RawSessionTicketKeySize {
		return fmt.Errorf("session ticket key should be 96 bytes hex string (%s)", conf.SessionTicketKey)
	}

	return nil
}

// rawSessionTicketKeyLoad loads session ticket key from file in raw format (48 bytes binary file).
func rawSessionTicketKeyLoad(filename string) (SessionTicketKeyConf, error) {
	var config SessionTicketKeyConf

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return config, err
	}

	if len(data) != RawSessionTicketKeySize {
		return config, fmt.Errorf("invalid session ticket key(%d)", len(data))
	}

	config.Version = time.Now().String()
	config.SessionTicketKey = fmt.Sprintf("%x", data)
	return config, nil
}

// SessionTicketKeyConfLoad load session ticket key from file.
func SessionTicketKeyConfLoad(filename string) (SessionTicketKeyConf, error) {
	var config SessionTicketKeyConf

	// open the file
	file, err := os.Open(filename)
	if err != nil {
		return config, err
	}
	defer file.Close()

	// decode the file
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		log.Logger.Info("Load SessionTicketKey json file fail, fallback to raw format(%s)", err)
		if config, err = rawSessionTicketKeyLoad(filename); err != nil {
			return config, err
		}
	}

	// check conf
	err = SessionTicketKeyConfCheck(config)
	if err != nil {
		return config, err
	}

	return config, nil
}

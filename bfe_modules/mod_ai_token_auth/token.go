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

package mod_ai_token_auth

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"
)

const (
	TokenStatusEnabled   = 1 // don't use 0, 0 is the default value!
	TokenStatusDisabled  = 2 // also don't use 0
	TokenStatusExpired   = 3
	TokenStatusExhausted = 4
)

const (
    ActionCheckToken    = "CHECK_TOKEN"
)

// type Token struct {
// 	Id             int     `json:"id"`
// 	UserId         int     `json:"user_id"`
// 	Key            string  `json:"key"`
// 	Status         int     `json:"status"`
// 	Name           string  `json:"name"`
// 	CreatedTime    int64   `json:"created_time"`
// 	AccessedTime   int64   `json:"accessed_time"`
// 	ExpiredTime    int64   `json:"expired_time"` // -1 means never expired
// 	RemainQuota    int64   `json:"remain_quota"`
// 	UnlimitedQuota bool    `json:"unlimited_quota"`
// 	UsedQuota      int64   `json:"used_quota"` // used quota
// 	Models         *string `json:"models"`            // allowed models
// 	Subnet         *string `json:"subnet"`           // allowed subnet
// }

type Token struct {
	Key            string
	Status         int
	Name           string
	// CreatedTime    int64   `json:"created_time"`
	UpdateTime     int64
	ExpiredTime    int64
	RemainQuota    int64
	UsedQuota	   *atomic.Uint64
	UnlimitedQuota bool
	Models         []string
	Subnet         []*net.IPNet
}

type TokenFile struct {
	Key            string  `json:"key"`
	Status         int     `json:"status"`
	Name           string  `json:"name"`
	// CreatedTime    int64   `json:"created_time"`
	UpdateTime     int64   `json:"update_time"`
	ExpiredTime    int64   `json:"expired_time"` // -1 means never expired
	RemainQuota    int64   `json:"remain_quota"`
	UnlimitedQuota bool    `json:"unlimited_quota"`
	Models         *string `json:"models"`            // allowed models
	Subnet         *string `json:"subnet"`           // allowed subnet
	models         []string
	subnet         []*net.IPNet
}

func tokenCheck(conf *TokenFile) error {
	if conf.Key == "" {
		return errors.New("no Key")
	}
	if conf.Status < TokenStatusEnabled || conf.Status > TokenStatusExhausted {
		return fmt.Errorf("invalid Status: %d", conf.Status)
	}
	if conf.ExpiredTime < -1 {
		return fmt.Errorf("invalid ExpiredTime: %d", conf.ExpiredTime)
	}
	if conf.RemainQuota < 0 {
		return fmt.Errorf("invalid RemainQuota: %d", conf.RemainQuota)
	}
	if conf.UnlimitedQuota && conf.RemainQuota != 0 {
		return errors.New("if UnlimitedQuota is true, RemainQuota must be 0")
	}
	if conf.Models != nil {
		conf.models = strings.Split(*conf.Models, ",")
		for i := 0; i < len(conf.models); i++ {
			conf.models[i] = strings.TrimSpace(conf.models[i])
			if conf.models[i] == "" {
				return errors.New("Models cannot contain empty strings")
			}
		}
	}
	if conf.Subnet != nil {
		res := strings.Split(*conf.Subnet, ",")
		conf.subnet = make([]*net.IPNet, len(res))
		for i := 0; i < len(res); i++ {
			res[i] = strings.TrimSpace(res[i])
			_, subnet, err := net.ParseCIDR(res[i])
			if err != nil {
				return fmt.Errorf("invalid subnet %s: %v", res[i], err)
			}
			conf.subnet[i] = subnet
		}
	}
	return nil
}

func tokenConvert(tokenFile TokenFile) Token {
	return Token{
		Key:            tokenFile.Key,
		Status:         tokenFile.Status,
		Name:           tokenFile.Name,
		UpdateTime:     tokenFile.UpdateTime,
		ExpiredTime:    tokenFile.ExpiredTime,
		RemainQuota:    tokenFile.RemainQuota,
		UnlimitedQuota: tokenFile.UnlimitedQuota,
		Models:         tokenFile.models,
		Subnet:         tokenFile.subnet,
	}
}

type ActionFile struct {
        Cmd             string
}

type Action ActionFile

func ActionFileCheck(conf *ActionFile) error {
	if conf.Cmd != ActionCheckToken {
			return fmt.Errorf("invalid cmd: %s", conf.Cmd)
	}
	return nil
}

func actionConvert(actionFile ActionFile) Action {
	return Action(actionFile)
}

func GetUUID() string {
	code := uuid.New().String()
	code = strings.Replace(code, "-", "", -1)
	return code
}

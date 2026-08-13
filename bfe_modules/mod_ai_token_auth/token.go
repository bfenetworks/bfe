// Copyright (c) 2025 The BFE Authors.
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

	"github.com/bfenetworks/go-lib/quota"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
	"github.com/google/uuid"
)

const (
	TokenStatusEnabled   = 1
	TokenStatusDisabled  = 2
	TokenStatusExpired   = 3
	TokenStatusExhausted = 4
)

const (
	ActionCheckToken = "CHECK_TOKEN"
)

type Token struct {
	Key            string
	Status         int
	Name           string
	UpdateTime     int64
	ExpiredTime    int64
	UnlimitedQuota bool
	Models         []string
	BlockModels    []string
	Subnet         []*net.IPNet
	Tags           []bfe_basic.ApikeyTag
	QuotaPlans     []*QuotaPlan
}

type TokenFile struct {
	Key            string  `json:"key"`
	Enabled        int     `json:"enabled"`
	Status         int     `json:"status"`
	Name           string  `json:"name"`
	UpdateTime     int64   `json:"update_time"`
	ExpiredTime    int64   `json:"expired_time"` // -1 means never expired
	UnlimitedQuota bool    `json:"unlimited_quota"`
	Models         *string `json:"allow_models"` // allowed models
	BlockModels    *string `json:"block_models"` // blocked models
	Subnet         *string `json:"subnet"`       // allowed subnet
	Tags           []bfe_basic.ApikeyTag
	QuotaPlans     []string `json:"quota_plans"` // quotaPlan IDs
	models         []string
	blockModels    []string
	subnet         []*net.IPNet
}

type QuotaPlan struct {
	Id          string
	Unlimited   bool
	PassNoQuota bool
	RedisKey    string
	CreateTime  int64
	ExpiredTime int64 // -1 means never expired
	Quota       int64 // 配额总量，固定点整数：total_token 时为 Token 数；RMB 时为 1e-8 元
	ResetMode   int   // 0 – 非周期性；1 – 周期性的配额包
	Unit        string // "total_token" or "RMB"
	Currency    string // "RMB" when Unit is "RMB"
}

func (q *QuotaPlan) Deduct(client redis_client.Client, amount int64) (int64, error) {
	if amount <= 0 {
		return 0, nil
	}

	if q.Unlimited {
		return q.Quota, nil
	}

	if q.RedisKey == "" {
		return 0, errors.New("RedisKey is empty")
	}

	if quota.IsRMB(q.Unit) {
		return q.deductRMB(client, amount)
	}

	return q.deductToken(client, amount)
}

func (q *QuotaPlan) deductToken(client redis_client.Client, amount int64) (int64, error) {
	lua := `
		local current = tonumber(redis.call('GET', KEYS[1]) or '0')
		local amount = tonumber(ARGV[1])
		local deduct = math.min(current, amount)
		if deduct > 0 then
			redis.call('DECRBY', KEYS[1], deduct)
		end
		return math.max(0, current - deduct)
	`
	script := client.NewScript(lua)
	result, err := script.Run(q.RedisKey, amount)
	if err != nil {
		return 0, err
	}

	remaining, ok := result.(int64)
	if !ok {
		return 0, errors.New("invalid result type from redis")
	}

	return remaining, nil
}

func (q *QuotaPlan) deductRMB(client redis_client.Client, amount int64) (int64, error) {
	lua := `
		local raw = redis.call('GET', KEYS[1])
		local current
		if raw == false then
			current = tonumber(ARGV[2])
			redis.call('SET', KEYS[1], current)
		else
			current = tonumber(raw)
		end
		local cost = tonumber(ARGV[1])
		local deduct = math.min(current, cost)
		if deduct > 0 then
			redis.call('DECRBY', KEYS[1], deduct)
		end
		return math.max(0, current - deduct)
	`
	script := client.NewScript(lua)
	result, err := script.Run(q.RedisKey, amount, q.Quota)
	if err != nil {
		return 0, err
	}

	remaining, ok := result.(int64)
	if !ok {
		return 0, errors.New("invalid result type from redis")
	}

	return remaining, nil
}

func (q *QuotaPlan) HasBalance(client redis_client.Client) (bool, int64, error) {
	if q.Unlimited {
		return true, q.Quota, nil
	}

	if q.RedisKey == "" {
		return false, 0, errors.New("RedisKey is empty")
	}

	current, err := client.GetInt64(q.RedisKey)
	if err != nil {
		return false, 0, err
	}

	return current > 0, current, nil
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
	if !conf.UnlimitedQuota && len(conf.QuotaPlans) == 0 {
		return errors.New("if UnlimitedQuota is false, QuotaPlans must be non-empty")
	}
	if conf.Models != nil && *conf.Models != "" {
		conf.models = strings.Split(*conf.Models, ",")
		for i := 0; i < len(conf.models); i++ {
			conf.models[i] = strings.TrimSpace(conf.models[i])
			if conf.models[i] == "" {
				return errors.New("Models cannot contain empty strings")
			}
		}
	}
	if conf.BlockModels != nil && *conf.BlockModels != "" {
		conf.blockModels = strings.Split(*conf.BlockModels, ",")
		for i := 0; i < len(conf.blockModels); i++ {
			conf.blockModels[i] = strings.TrimSpace(conf.blockModels[i])
			if conf.blockModels[i] == "" {
				return errors.New("BlockModels cannot contain empty strings")
			}
		}
	}
	if conf.Subnet != nil && *conf.Subnet != "" {
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

func tokenConvert(tokenFile TokenFile, quotaPlansMap *QuotaPlanMap) (Token, error) {
	quotaPlans := make([]*QuotaPlan, 0)
	if len(tokenFile.QuotaPlans) > 0 && quotaPlansMap == nil {
		return Token{}, fmt.Errorf("quotaPlansMap is nil")
	}
	for _, quotaPlanId := range tokenFile.QuotaPlans {
		if quotaPlan, ok := (*quotaPlansMap)[quotaPlanId]; ok {
			quotaPlans = append(quotaPlans, quotaPlan)
		} else {
			return Token{}, fmt.Errorf("quotaPlan %s not found", quotaPlanId)
		}
	}

	return Token{
		Key:            tokenFile.Key,
		Status:         tokenFile.Status,
		Name:           tokenFile.Name,
		UpdateTime:     tokenFile.UpdateTime,
		ExpiredTime:    tokenFile.ExpiredTime,
		UnlimitedQuota: tokenFile.UnlimitedQuota,
		Models:         tokenFile.models,
		BlockModels:    tokenFile.blockModels,
		Subnet:         tokenFile.subnet,
		Tags:           tokenFile.Tags,
		QuotaPlans:     quotaPlans,
	}, nil
}

type ActionFile struct {
	Cmd string
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

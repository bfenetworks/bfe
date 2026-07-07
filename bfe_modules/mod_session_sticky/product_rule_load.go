// Copyright (c) 2026 The BFE Authors.
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

package mod_session_sticky

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

const (
	DefaultCookieKey       = "bfe_ssbl"
	DefaultMaxAge          = 3600
	DefaultMaskCode        = "defaultmask"
	DefaultStandbyMaskCode = "standbymask"
	DefaultHttpOnly        = false
	DefaultSecure          = false

	MinMaskCodeLen = 4
)

type RuleType int

const (
	RuleTypeCookie RuleType = iota
	RuleTypeSticky
	RuleTypeMax
)

type StickyRuleFile struct {
	Cond                *string // condition string for sticky
	Type                *string // "Cookie" | "Sticky"
	CookieKey           *string // cookie key
	Domain              *string // cookie domain
	Path                *string // cookie path
	MaxAge              *int    // max age
	MaskCode            *string // active mask code which used to mask original bytes
	StandbyMaskCode     *string // stand by mask code which used to mask original bytes
	Header              *string // sticky header name
	URIParam            *string // sticky uri param name
	Secure              *bool
	HttpOnly            *bool
	RenewWindow         *int    // renew window in seconds
	StickyRequestField  *string // JSON field name in request body for sticky ID (e.g., previous_response_id)
	StickyResponseField *string // JSON field name in response body for sticky ID (e.g., response_id)
}

type StickyRule struct {
	Cond                condition.Condition // condition for sticky
	Type                RuleType
	CookieKey           string // cookie key
	Domain              string // cookie domain
	Path                string // cookie path
	MaxAge              int    // max age
	MaskCode            string // active mask code which used to mask original bytes
	StandbyMaskCode     string // stand by mask code which used to mask original bytes
	Header              string // sticky header name
	URIParam            string // sticky uri param name
	Secure              bool
	HttpOnly            bool
	RenewWindow         int    // renew window in seconds
	StickyRequestField  string // JSON field name in request body for sticky ID (e.g., previous_response_id)
	StickyResponseField string // JSON field name in response body for sticky ID (e.g., response_id)
}

type StickyRuleFileList []StickyRuleFile
type StickyRuleList []StickyRule

type ProductRulesFile map[string]*StickyRuleFileList // product => list of sticky rules
type ProductRules map[string]*StickyRuleList

type ProductRuleConfFile struct {
	Version *string
	Config  *ProductRulesFile
}

type ProductRuleConf struct {
	Version string       // version of the config
	Config  ProductRules // product rules for sticky
}

func stickyRuleCheck(conf *StickyRuleFile) error {
	if conf.Cond == nil {
		return errors.New("stickyRuleCheck(): no Cond")
	}
	if conf.MaskCode != nil && len(*conf.MaskCode) < MinMaskCodeLen {
		return fmt.Errorf("stickyRuleCheck(): MaskCode should not less than %d", MinMaskCodeLen)
	}
	if conf.StandbyMaskCode != nil && len(*conf.StandbyMaskCode) < MinMaskCodeLen {
		return fmt.Errorf("stickyRuleCheck(): StandbyMaskCode should not less than %d", MinMaskCodeLen)
	}

	if conf.CookieKey != nil && strings.Trim(*conf.CookieKey, " ") == "" {
		return fmt.Errorf("stickyRuleCheck(): CookieKey should not empty")
	}
	if conf.MaxAge != nil && *conf.MaxAge < 0 {
		return fmt.Errorf("stickyRuleCheck(): Max Age should not less than 0")
	}
	if conf.Path != nil && !strings.HasPrefix(*conf.Path, "/") {
		return fmt.Errorf("stickyRuleCheck(): Path should start with /")
	}
	if conf.Type != nil && *conf.Type != "Cookie" && *conf.Type != "Sticky" {
		return fmt.Errorf("stickyRuleCheck(): Wrong Type: %s", *conf.Type)
	}
	if conf.Type != nil && *conf.Type == "Sticky" && conf.CookieKey == nil {
		return fmt.Errorf("stickyRuleCheck(): no CookieKey for Sticky rule")
	}

	return nil
}

// check StickyRuleFile
func stickyRuleListCheck(conf *StickyRuleFileList) error {
	for index, rule := range *conf {
		err := stickyRuleCheck(&rule)
		if err != nil {
			return fmt.Errorf("stickyRuleCheck():%d, %s", index, err.Error())
		}
	}

	return nil
}

// check ProductRules
func productRulesCheck(conf *ProductRulesFile) error {
	for product, ruleList := range *conf {
		if ruleList == nil {
			return fmt.Errorf("no StickyRuleFile for product:%s", product)
		}

		err := stickyRuleListCheck(ruleList)
		if err != nil {
			return fmt.Errorf("ProductRules:%s, %s", product, err.Error())
		}
	}

	return nil
}

// check productRuleConf
func ProductRuleConfCheck(conf ProductRuleConfFile) error {
	var err error

	// check Version
	if conf.Version == nil {
		return errors.New("no Version")
	}

	// check Config
	if conf.Config == nil {
		return errors.New("no Config")
	}

	err = productRulesCheck(conf.Config)
	if err != nil {
		return fmt.Errorf("Config:%s", err.Error())
	}

	return nil
}

func ruleConvert(ruleFile StickyRuleFile) (StickyRule, error) {
	rule := StickyRule{}

	cond, err := condition.Build(*ruleFile.Cond)
	if err != nil {
		return rule, err
	}
	rule.Cond = cond
	if ruleFile.CookieKey == nil {
		rule.CookieKey = DefaultCookieKey
	} else {
		rule.CookieKey = *(ruleFile.CookieKey)
	}

	if ruleFile.Domain != nil {
		rule.Domain = *(ruleFile.Domain)
	}

	if ruleFile.Path != nil {
		rule.Path = *(ruleFile.Path)
	}

	if ruleFile.MaxAge == nil {
		rule.MaxAge = DefaultMaxAge
	} else {
		rule.MaxAge = *(ruleFile.MaxAge)
	}

	if ruleFile.MaskCode == nil {
		rule.MaskCode = DefaultMaskCode
	} else {
		rule.MaskCode = *(ruleFile.MaskCode)
	}

	if ruleFile.StandbyMaskCode == nil {
		rule.StandbyMaskCode = DefaultStandbyMaskCode
	} else {
		rule.StandbyMaskCode = *(ruleFile.StandbyMaskCode)
	}

	if ruleFile.Type == nil {
		rule.Type = RuleTypeCookie
	} else {
		if *ruleFile.Type == "Cookie" {
			rule.Type = RuleTypeCookie
		} else if *ruleFile.Type == "Sticky" {
			rule.Type = RuleTypeSticky
		} else {
			return rule, fmt.Errorf("stickyRuleCheck(): Wrong Type: %s", *ruleFile.Type)
		}
	}

	if ruleFile.Header != nil {
		rule.Header = *(ruleFile.Header)
	}

	if ruleFile.URIParam != nil {
		rule.URIParam = *(ruleFile.URIParam)
	}

	if ruleFile.HttpOnly == nil {
		rule.HttpOnly = DefaultHttpOnly
	} else {
		rule.HttpOnly = *(ruleFile.HttpOnly)
	}

	if ruleFile.Secure == nil {
		rule.Secure = DefaultSecure
	} else {
		rule.Secure = *(ruleFile.Secure)
	}

	if ruleFile.RenewWindow == nil {
		rule.RenewWindow = 0
		if rule.MaxAge > 0 {
			rule.RenewWindow = rule.MaxAge / 2
		}
	} else {
		rule.RenewWindow = *(ruleFile.RenewWindow)
	}

	if ruleFile.StickyRequestField != nil {
		rule.StickyRequestField = *(ruleFile.StickyRequestField)
	}

	if ruleFile.StickyResponseField != nil {
		rule.StickyResponseField = *(ruleFile.StickyResponseField)
	}

	return rule, nil
}

func ruleListConvert(ruleFileList *StickyRuleFileList) (*StickyRuleList, error) {
	ruleList := new(StickyRuleList)
	*ruleList = make([]StickyRule, 0)

	for _, ruleFile := range *ruleFileList {
		rule, err := ruleConvert(ruleFile)
		if err != nil {
			return nil, err
		}
		*ruleList = append(*ruleList, rule)
	}

	return ruleList, nil
}

/*
load sticky rule config from file

	Returns:
	     (ProductRuleConf, error)
*/
func ProductRuleConfLoad(filename string) (ProductRuleConf, error) {
	var conf ProductRuleConf
	var err error

	// open the file
	file, err := os.Open(filename)
	defer file.Close()
	if err != nil {
		return conf, err
	}

	// decode the file
	decoder := json.NewDecoder(file)
	var config ProductRuleConfFile
	err = decoder.Decode(&config)
	if err != nil {
		return conf, err
	}

	// check config
	err = ProductRuleConfCheck(config)
	if err != nil {
		return conf, err
	}

	// convert config
	conf.Version = *config.Version
	conf.Config = make(ProductRules)
	for product, ruleFileList := range *config.Config {
		ruleList, err := ruleListConvert(ruleFileList)
		if err != nil {
			return conf, err
		}
		conf.Config[product] = ruleList
	}

	return conf, nil
}

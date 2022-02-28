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

package mod_cors

import (
	"strings"
	"testing"
)

// normal case
func TestCorsRuleFileLoad(t *testing.T) {
	corsRuleConf, err := CorsRuleFileLoad("testdata/mod_cors/cors_rule.data")
	if err != nil {
		t.Fatalf("should have no error, but error is %v", err)
	}

	expectVersion := "20200508210000"
	if corsRuleConf.Version != expectVersion {
		t.Fatalf("Version should be %s, but it's %s", expectVersion, corsRuleConf.Version)
	}

	if corsRuleConf.Config == nil {
		t.Fatalf("Config should not be nil")
	}

	ruleList, ok := corsRuleConf.Config[expectProduct]
	if !ok {
		t.Fatalf("config should have product: %s", expectProduct)
	}

	if len(ruleList) != 1 {
		t.Fatalf("len(ruleList) should be 1, but it's %d", len(ruleList))
	}
}

// rule file no version
func TestCorsRuleFileLoadCaseNoVersion(t *testing.T) {
	_, err := CorsRuleFileLoad("testdata/mod_cors/cors_rule_no_version.data")
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "no Version") {
		t.Fatalf("error message is not expected: %v", err)
	}
}

// rule file no config
func TestCorsRuleFileLoadCaseNoConfig(t *testing.T) {
	_, err := CorsRuleFileLoad("testdata/mod_cors/cors_rule_no_config.data")
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "no Config") {
		t.Fatalf("error message is not expected: %v", err)
	}
}

// wrong cond
func TestRuleConvertWrongCond(t *testing.T) {
	rawRule := CorsRuleRaw{
		Cond: "wrong cond",
	}

	_, err := ruleConvert(rawRule)
	if err == nil {
		t.Fatalf("should have error")
	}
}

// wrong origin variable for AccessControlAllowOrigins
func TestRuleConvertWrongOriginVariable(t *testing.T) {
	rawRule := CorsRuleRaw{
		Cond:                      "default_t()",
		AccessControlAllowOrigins: []string{"%wrongorgin"},
	}

	_, err := ruleConvert(rawRule)
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "AccessControlAllowOrigins %wrongorgin is not supported") {
		t.Fatalf("error is not expected, %v", err)
	}
}

// wrong Wildcard for AccessControlAllowOrigins
func TestRuleConvertOriginWrongWildcard(t *testing.T) {
	rawRule := CorsRuleRaw{
		Cond:                      "default_t()",
		AccessControlAllowOrigins: []string{"*wrongwildcard"},
	}

	_, err := ruleConvert(rawRule)
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "AccessControlAllowOrigins *wrongwildcard is not supported") {
		t.Fatalf("error is not expected, %v", err)
	}
}

// wrong credentials for AccessControlAllowOrigins
func TestRuleConvertWrongCredentials(t *testing.T) {
	rawRule := CorsRuleRaw{
		Cond:                          "default_t()",
		AccessControlAllowOrigins:     []string{"*"},
		AccessControlAllowCredentials: true,
	}

	_, err := ruleConvert(rawRule)
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "AccessControlAllowCredentials can not be true when AccessControlAllowOrigins is *") {
		t.Fatalf("error is not expected, %v", err)
	}
}

// wrong credentials for AccessControlAllowOrigins
func TestRuleConvertOriginWrongElementNum(t *testing.T) {
	rawRule := CorsRuleRaw{
		Cond:                      "default_t()",
		AccessControlAllowOrigins: []string{"null", "http://example.org"},
	}

	_, err := ruleConvert(rawRule)
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "AccessControlAllowOrigins can only contain one element when AccessControlAllowOrigins is null or *") {
		t.Fatalf("error is not expected, %v", err)
	}

	rawRule.AccessControlAllowOrigins = []string{"*", "http://example.org"}
	_, err = ruleConvert(rawRule)
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "AccessControlAllowOrigins can only contain one element when AccessControlAllowOrigins is null or *") {
		t.Fatalf("error is not expected, %v", err)
	}
}

func TestRuleConvertAllowHeaderWrongWildcard(t *testing.T) {
	rawRule := CorsRuleRaw{
		Cond:                      "default_t()",
		AccessControlAllowOrigins: []string{"*"},
		AccessControlAllowHeaders: []string{"*wrongheader"},
	}

	_, err := ruleConvert(rawRule)
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "AccessControlAllowHeaders *wrongheader is not supported") {
		t.Fatalf("error is not expected, %v", err)
	}
}

func TestRuleConvertAllowHeaderWrongElementNum(t *testing.T) {
	rawRule := CorsRuleRaw{
		Cond:                      "default_t()",
		AccessControlAllowOrigins: []string{"*"},
		AccessControlAllowHeaders: []string{"*", "X-Bfe-Test"},
	}

	_, err := ruleConvert(rawRule)
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "AccessControlAllowHeaders can only contain one element when AccessControlAllowHeaders is *") {
		t.Fatalf("error is not expected, %v", err)
	}
}

func TestRuleConvertExposeHeaderWrongWildcard(t *testing.T) {
	rawRule := CorsRuleRaw{
		Cond:                       "default_t()",
		AccessControlAllowOrigins:  []string{"*"},
		AccessControlExposeHeaders: []string{"*wrongheader"},
	}

	_, err := ruleConvert(rawRule)
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "AccessControlExposeHeaders *wrongheader is not supported") {
		t.Fatalf("error is not expected, %v", err)
	}
}

func TestRuleConvertExposeHeaderWrongElementNum(t *testing.T) {
	rawRule := CorsRuleRaw{
		Cond:                       "default_t()",
		AccessControlAllowOrigins:  []string{"*"},
		AccessControlExposeHeaders: []string{"*", "X-Bfe-Test"},
	}

	_, err := ruleConvert(rawRule)
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "AccessControlExposeHeaders can only contain one element when AccessControlExposeHeaders is *") {
		t.Fatalf("error is not expected, %v", err)
	}
}

func TestRuleConvertAllowMethodWrongWildcard(t *testing.T) {
	rawRule := CorsRuleRaw{
		Cond:                      "default_t()",
		AccessControlAllowOrigins: []string{"*"},
		AccessControlAllowMethods: []string{"*wrongmethod"},
	}

	_, err := ruleConvert(rawRule)
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "AccessControlAllowMethods *wrongmethod is not supported") {
		t.Fatalf("error is not expected, %v", err)
	}
}

func TestRuleConvertAllowMethodWrongElementNum(t *testing.T) {
	rawRule := CorsRuleRaw{
		Cond:                      "default_t()",
		AccessControlAllowOrigins: []string{"*"},
		AccessControlAllowMethods: []string{"*", "X-Bfe-Test"},
	}

	_, err := ruleConvert(rawRule)
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "AccessControlAllowMethods can only contain one element when AccessControlAllowMethods is *") {
		t.Fatalf("error is not expected, %v", err)
	}
}

func TestRuleConvertMaxAge(t *testing.T) {
	maxAge := -2
	rawRule := CorsRuleRaw{
		Cond:                      "default_t()",
		AccessControlAllowOrigins: []string{"*"},
		AccessControlMaxAge:       &maxAge,
	}

	_, err := ruleConvert(rawRule)
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "AccessControlMaxAge must be in [-1, 86400]") {
		t.Fatalf("error is not expected, %v", err)
	}
}

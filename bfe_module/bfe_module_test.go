// Copyright (c) 2021 The BFE Authors.
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

package bfe_module

import (
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/stretchr/testify/assert"
	"testing"
)

//mock module
type testModule struct {
}

func (tm testModule) Name() string {
	return "tm"
}

func (tm testModule) Init(cbs *BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	return nil
}

var tm testModule

func init() {
	AddModule(tm)
}

func TestNewBfeModules(t *testing.T) {
	bms := NewBfeModules()
	assert.NotNil(t, bms)
}

func TestBfeModulesRegisterModule(t *testing.T) {
	bms := NewBfeModules()
	assert.NotNil(t, bms)
	var err error
	err = bms.RegisterModule("")
	assert.Error(t, err)

	err = bms.RegisterModule("tm")
	assert.NoError(t, err)
}

func TestBfeModulesGetModule(t *testing.T) {
	bms := NewBfeModules()
	assert.NotNil(t, bms)
	err := bms.RegisterModule("tm")
	assert.NoError(t, err)

	bm := bms.GetModule("tm")
	assert.NotNil(t, bm)
}

func TestBfeModulesInit(t *testing.T) {
	bms := NewBfeModules()
	assert.NotNil(t, bms)
	var err error
	err = bms.RegisterModule("tm")
	assert.Nil(t, err)

	err = bms.Init(nil, nil, "")
	assert.Nil(t, err)
}

func TestModConfPath(t *testing.T) {
	s := ModConfPath("/home/bfe/conf", "mod_access")
	assert.EqualValues(t, "/home/bfe/conf/mod_access/mod_access.conf", s)
}

func TestModConfDir(t *testing.T) {
	s := ModConfDir("/home/bfe/conf", "mod_access")
	assert.EqualValues(t, "/home/bfe/conf/mod_access", s)
}

func TestModuleStatusGetJSON(t *testing.T) {
	var err error
	TestBfeModulesInit(t)
	_, err = ModuleStatusGetJSON()
	assert.NoError(t, err)
}

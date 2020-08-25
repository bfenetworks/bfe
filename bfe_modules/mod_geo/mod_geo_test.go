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

package mod_geo

import (
	"net"
	"net/url"
	"testing"
)

import (
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_module"
)

func initModGeo() (*ModuleGeo, error) {
	m := NewModuleGeo()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, "./test_data"); err != nil {
		return nil, err
	}
	return m, nil
}

func TestGetModuleName(t *testing.T) {
	m := NewModuleGeo()
	if m.Name() != ModGeo {
		t.Error("module name is wrong, expect \"mod_geo\"")
	}
}

func TestLoadModGeoConfigData(t *testing.T) {
	m := NewModuleGeo()

	// load conf data failed
	err := m.loadConfData(url.Values{})
	if err == nil {
		t.Error("the return value of load mod_geo data is err, expect err")
	}

	// build test query param
	testQuery := url.Values{}
	testQuery.Add("path", "./test_data/mod_geo/geo.db")

	// load conf data success
	err = m.loadConfData(testQuery)
	if err != nil {
		t.Errorf("load mod_geo conf data err: %s", err.Error())
	}
}

func TestGeoHandler(t *testing.T) {
	// init module geo
	m, err := initModGeo()
	if err != nil {
		t.Errorf("Test_mod_geo(): %s", err)
		return
	}

	// init request
	req := &bfe_basic.Request{}
	tcpAddr := net.TCPAddr{IP: net.ParseIP("123.114.119.152")}
	req.ClientAddr = &tcpAddr
	req.Context = make(map[interface{}]interface{})

	m.geoHandler(req)
	if req.GetContext(CtxCountryIsoCode).(string) != "CN" {
		t.Errorf("get country iso code(%s) err, expect CN", req.GetContext(CtxCountryIsoCode).(string))
	}
}

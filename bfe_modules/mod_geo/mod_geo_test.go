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

package mod_geo

import (
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

import (
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
	"github.com/oschwald/geoip2-golang"
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

func TestNewModuleGeo(t *testing.T) {
	m := NewModuleGeo()
	if m == nil {
		t.Fatal("NewModuleGeo() should not return nil")
	}
	if m.Name() != ModGeo {
		t.Errorf("module name is wrong, expect %q", ModGeo)
	}
}

func TestModuleGeoName(t *testing.T) {
	m := NewModuleGeo()
	if m.Name() != ModGeo {
		t.Errorf("module name is wrong, expect %q", ModGeo)
	}
}

func TestLoadModGeoConfigData(t *testing.T) {
	m := NewModuleGeo()

	// load conf data failed, no path set and no default data file
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

	// reload with default path (m.dataFilePath is still empty)
	err = m.loadConfData(url.Values{})
	if err == nil {
		t.Error("loadConfData with empty default path should return err")
	}
}

func TestGeoHandler(t *testing.T) {
	// init module geo
	m, err := initModGeo()
	if err != nil {
		t.Fatalf("initModGeo(): %s", err)
	}

	// init request
	req := &bfe_basic.Request{}
	tcpAddr := net.TCPAddr{IP: net.ParseIP("123.114.119.152")}
	req.ClientAddr = &tcpAddr
	req.Context = make(map[interface{}]interface{})

	ret, resp := m.geoHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("geoHandler ret should be %d, got %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("geoHandler resp should be nil")
	}
	if req.GetContext(CtxCountryIsoCode).(string) != "CN" {
		t.Errorf("get country iso code(%s) err, expect CN", req.GetContext(CtxCountryIsoCode).(string))
	}
}

func TestGeoHandlerNilClientAddr(t *testing.T) {
	m, err := initModGeo()
	if err != nil {
		t.Fatalf("initModGeo(): %s", err)
	}

	req := &bfe_basic.Request{}
	req.Context = make(map[interface{}]interface{})

	ret, resp := m.geoHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("geoHandler ret should be %d, got %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("geoHandler resp should be nil")
	}
}

func TestGeoHandlerLoopbackAddr(t *testing.T) {
	m, err := initModGeo()
	if err != nil {
		t.Fatalf("initModGeo(): %s", err)
	}

	req := &bfe_basic.Request{}
	tcpAddr := net.TCPAddr{IP: net.ParseIP("127.0.0.1")}
	req.ClientAddr = &tcpAddr
	req.Context = make(map[interface{}]interface{})

	ret, _ := m.geoHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("geoHandler ret should be %d, got %d", bfe_module.BfeHandlerGoOn, ret)
	}
}

func TestGeoHandlerInvalidIP(t *testing.T) {
	m, err := initModGeo()
	if err != nil {
		t.Fatalf("initModGeo(): %s", err)
	}

	req := &bfe_basic.Request{}
	tcpAddr := net.TCPAddr{IP: net.ParseIP("0.0.0.0")}
	req.ClientAddr = &tcpAddr
	req.Context = make(map[interface{}]interface{})

	ret, _ := m.geoHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("geoHandler ret should be %d, got %d", bfe_module.BfeHandlerGoOn, ret)
	}
}

func TestSetGeoInfoToReqContext(t *testing.T) {
	m := NewModuleGeo()
	req := &bfe_basic.Request{}
	req.Context = make(map[interface{}]interface{})

	cityInfo := &geoip2.City{
		Country: struct {
			GeoNameID         uint              `maxminddb:"geoname_id"`
			IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
			IsoCode           string            `maxminddb:"iso_code"`
			Names             map[string]string `maxminddb:"names"`
		}{
			IsoCode: "US",
		},
		Subdivisions: []struct {
			GeoNameID uint              `maxminddb:"geoname_id"`
			IsoCode   string            `maxminddb:"iso_code"`
			Names     map[string]string `maxminddb:"names"`
		}{
			{IsoCode: "CA"},
		},
		City: struct {
			GeoNameID uint              `maxminddb:"geoname_id"`
			Names     map[string]string `maxminddb:"names"`
		}{
			Names: map[string]string{"en": "Los Angeles"},
		},
		Location: struct {
			AccuracyRadius uint16  `maxminddb:"accuracy_radius"`
			Latitude       float64 `maxminddb:"latitude"`
			Longitude      float64 `maxminddb:"longitude"`
			MetroCode      uint    `maxminddb:"metro_code"`
			TimeZone       string  `maxminddb:"time_zone"`
		}{
			Latitude:  34.0522,
			Longitude: -118.2437,
		},
	}

	m.setGeoInfoToReqContext(req, cityInfo)

	if got := req.GetContext(CtxCountryIsoCode).(string); got != "US" {
		t.Errorf("country iso code = %q, want US", got)
	}
	if got := req.GetContext(CtxSubdivisionIsoCode).(string); got != "CA" {
		t.Errorf("subdivision iso code = %q, want CA", got)
	}
	if got := req.GetContext(CtxCityName).(string); got != "Los Angeles" {
		t.Errorf("city name = %q, want Los Angeles", got)
	}
	if got := req.GetContext(CtxLatitude).(string); got != "34.0522" {
		t.Errorf("latitude = %q, want 34.0522", got)
	}
	if got := req.GetContext(CtxLongitude).(string); got != "-118.2437" {
		t.Errorf("longitude = %q, want -118.2437", got)
	}
}

func TestSetGeoInfoToReqContextNoSubdivision(t *testing.T) {
	m := NewModuleGeo()
	req := &bfe_basic.Request{}
	req.Context = make(map[interface{}]interface{})

	cityInfo := &geoip2.City{
		Country: struct {
			GeoNameID         uint              `maxminddb:"geoname_id"`
			IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
			IsoCode           string            `maxminddb:"iso_code"`
			Names             map[string]string `maxminddb:"names"`
		}{
			IsoCode: "JP",
		},
		Subdivisions: nil,
		City: struct {
			GeoNameID uint              `maxminddb:"geoname_id"`
			Names     map[string]string `maxminddb:"names"`
		}{
			Names: map[string]string{"en": "Tokyo"},
		},
		Location: struct {
			AccuracyRadius uint16  `maxminddb:"accuracy_radius"`
			Latitude       float64 `maxminddb:"latitude"`
			Longitude      float64 `maxminddb:"longitude"`
			MetroCode      uint    `maxminddb:"metro_code"`
			TimeZone       string  `maxminddb:"time_zone"`
		}{
			Latitude:  35.6762,
			Longitude: 139.6503,
		},
	}

	m.setGeoInfoToReqContext(req, cityInfo)

	if got := req.GetContext(CtxCountryIsoCode).(string); got != "JP" {
		t.Errorf("country iso code = %q, want JP", got)
	}
	if got := req.GetContext(CtxSubdivisionIsoCode).(string); got != "" {
		t.Errorf("subdivision iso code = %q, want empty", got)
	}
	if got := req.GetContext(CtxCityName).(string); got != "Tokyo" {
		t.Errorf("city name = %q, want Tokyo", got)
	}
}

func TestGetState(t *testing.T) {
	m, err := initModGeo()
	if err != nil {
		t.Fatalf("initModGeo(): %s", err)
	}

	state, err := m.getState(map[string][]string{})
	if err != nil {
		t.Errorf("getState() error: %v", err)
	}
	if len(state) == 0 {
		t.Error("getState() should return non-empty state")
	}
}

func TestInit(t *testing.T) {
	m := NewModuleGeo()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, "./test_data"); err != nil {
		t.Errorf("Init() error: %v", err)
	}
}

func TestInitConfNotExist(t *testing.T) {
	m := NewModuleGeo()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	err := m.Init(cb, wh, "./test_data/not_exist")
	if err == nil {
		t.Error("Init() should return error when config file not exist")
	}
}

func TestInitInvalidGeoDBPath(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_geo_test")
	if err != nil {
		t.Fatalf("ioutil.TempDir error: %v", err)
	}
	defer os.RemoveAll(dir)

	confDir := filepath.Join(dir, "mod_geo")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}

	confContent := "[basic]\nGeoDBPath = mod_geo/missing.db\n"
	confPath := filepath.Join(confDir, "mod_geo.conf")
	if err := ioutil.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	m := NewModuleGeo()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	err = m.Init(cb, wh, dir)
	if err == nil {
		t.Error("Init() should return error when geolocation database not exist")
	}
}

func TestInitInvalidConfFormat(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_geo_test")
	if err != nil {
		t.Fatalf("ioutil.TempDir error: %v", err)
	}
	defer os.RemoveAll(dir)

	confDir := filepath.Join(dir, "mod_geo")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}

	confPath := filepath.Join(confDir, "mod_geo.conf")
	if err := ioutil.WriteFile(confPath, []byte("invalid config content"), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	m := NewModuleGeo()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	err = m.Init(cb, wh, dir)
	if err == nil {
		t.Error("Init() should return error when config format is invalid")
	}
}

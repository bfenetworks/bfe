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

// determine user geolocation by IP address using geolocation database

package mod_geo

import (
	"fmt"
	"net/url"
	"strconv"
	"sync"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/oschwald/geoip2-golang"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	ModGeo = "mod_geo"

	GeoDBDefaultPath = "mod_geo/geo.db"
)

var (
	CtxCountryIsoCode     = "mod_geo.country_iso_code"
	CtxSubdivisionIsoCode = "mod_geo.subdivision_iso_code"
	CtxCityName           = "mod_geo.city_name"
	CtxLatitude           = "mod_geo.latitude"
	CtxLongitude          = "mod_geo.longitude"
)

var (
	openDebug = false
)

type ModuleGeoIdState struct {
	ErrReloadGeoDatabase *metrics.Counter // counter for reload geolocation database
	ErrGetGeoInfo        *metrics.Counter // counter for get geolocation information
}

type ModuleGeo struct {
	name string // module name

	dataFilePath string         // path of geolocation database data
	geoDB        *geoip2.Reader // geolocation database
	lock         sync.RWMutex

	metrics metrics.Metrics  // monitor metrics
	state   ModuleGeoIdState // module state
}

func NewModuleGeo() *ModuleGeo {
	m := new(ModuleGeo)
	m.name = ModGeo
	m.metrics.Init(&m.state, ModGeo, 0)
	return m
}

func (m *ModuleGeo) Name() string {
	return m.name
}

func (m *ModuleGeo) loadConfData(query url.Values) error {
	// get file path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.dataFilePath
	}

	// load geolocation database
	geoDB, err := geoip2.Open(path)
	if err != nil {
		m.state.ErrReloadGeoDatabase.Inc(1)
		return fmt.Errorf("%s: geolocation database load err %s", m.name, err.Error())
	}
	m.lock.Lock()
	m.geoDB = geoDB
	m.lock.Unlock()

	return nil
}

// geoHandler is a handler for setting geolocation information.
func (m *ModuleGeo) geoHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	if req.ClientAddr == nil {
		if openDebug {
			log.Logger.Debug("%s: req.ClientAddr is nil", m.name)
		}

		return bfe_module.BfeHandlerGoOn, nil
	}

	m.lock.RLock()
	geoDB := m.geoDB
	m.lock.RUnlock()

	// get geolocation based on client IP address using geolocation database
	cityInfo, err := geoDB.City(req.ClientAddr.IP)
	if err != nil {
		m.state.ErrGetGeoInfo.Inc(1)

		if openDebug {
			log.Logger.Debug("%s: get city info err: %s", m.name, err)
		}

		return bfe_module.BfeHandlerGoOn, nil
	}

	m.setGeoInfoToReqContext(req, cityInfo)

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleGeo) setGeoInfoToReqContext(req *bfe_basic.Request, cityInfo *geoip2.City) {
	var conturyIsoCode string
	var subdivisionIsoCode string
	var cityName string
	var latitude string
	var longitude string

	// get country iso code
	conturyIsoCode = cityInfo.Country.IsoCode

	// get subdivision iso code
	if len(cityInfo.Subdivisions) > 0 {
		subdivisionIsoCode = cityInfo.Subdivisions[0].IsoCode
	}

	// get city name
	cityName = cityInfo.City.Names["en"]

	// get latitude and longitude
	latitude = strconv.FormatFloat(cityInfo.Location.Latitude, 'f', -1, 64)
	longitude = strconv.FormatFloat(cityInfo.Location.Longitude, 'f', -1, 64)

	// set request context
	req.SetContext(CtxCountryIsoCode, conturyIsoCode)
	req.SetContext(CtxSubdivisionIsoCode, subdivisionIsoCode)
	req.SetContext(CtxCityName, cityName)
	req.SetContext(CtxLatitude, latitude)
	req.SetContext(CtxLongitude, longitude)

	if openDebug {
		log.Logger.Debug("%s: the geolocation information: countryIsoCode(%s), subdivisionIsoCode(%s),"+
			"cityName(%s), longitude(%s) and latitude(%s)",
			m.name, conturyIsoCode, subdivisionIsoCode, cityName, latitude, longitude)
	}
}

func (m *ModuleGeo) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleGeo) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var conf *ConfModGeo
	var err error

	// load config
	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}
	m.dataFilePath = conf.Basic.GeoDBPath

	openDebug = conf.Log.OpenDebug

	// read geolocation database
	if err = m.loadConfData(nil); err != nil {
		return fmt.Errorf("%s: geolocation database load err %s", m.name, err.Error())
	}

	// register handler
	err = cbs.AddFilter(bfe_module.HandleBeforeLocation, m.geoHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.rewriteHandler): %s", m.name, err.Error())
	}

	// register web handler for reload
	err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.loadConfData): %s", m.name, err.Error())
	}
	// register web handler for monitor
	err = whs.RegisterHandler(web_monitor.WebHandleMonitor, m.name, m.getState)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.getState): %s", m.name, err.Error())
	}

	return nil
}

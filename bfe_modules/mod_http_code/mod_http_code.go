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

package mod_http_code

import (
	"fmt"
	"net/url"
)

import (
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	ModHttpCode = "mod_http_code"
)

// ModuleHttpCodeState holds the counters of HTTP status code.
type ModuleHttpCodeState struct {
	All2XX *metrics.Counter
	All3XX *metrics.Counter
	All4XX *metrics.Counter
	All5XX *metrics.Counter
}

// ModuleHttpCode counts HTTP status code.
type ModuleHttpCode struct {
	name    string              // name of module
	state   ModuleHttpCodeState // module state
	metrics metrics.Metrics     // module metrics
}

// NewModuleHttpCode returns a new ModuleHttpCode.
func NewModuleHttpCode() *ModuleHttpCode {
	m := new(ModuleHttpCode)
	m.name = ModHttpCode
	m.metrics.Init(&m.state, ModHttpCode, 0)
	return m
}

// Name returns the name of ModuleHttpCode.
func (m *ModuleHttpCode) Name() string {
	return m.name
}

func (m *ModuleHttpCode) getState(query url.Values) ([]byte, error) {
	d := m.metrics.GetAll()
	return d.Format(query)
}

func (m *ModuleHttpCode) getStateDiff(query url.Values) ([]byte, error) {
	d := m.metrics.GetDiff()
	return d.Format(query)
}

func (m *ModuleHttpCode) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}

	return handlers
}

// Init initializes ModuleHttpCode.
func (m *ModuleHttpCode) Init(cbs *bfe_module.BfeCallbacks,
	whs *web_monitor.WebHandlers, cr string) error {
	var err error

	// register handler
	err = cbs.AddFilter(bfe_module.HandleRequestFinish, m.requestFinish)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.requestFinish): %v", m.name, err)
	}

	// register web handlers for monitor
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.monitorHandlers): %v", m.name, err)
	}

	return nil
}

func (m *ModuleHttpCode) requestFinish(req *bfe_basic.Request, res *bfe_http.Response) int {
	if req.HttpResponse == nil {
		return bfe_module.BfeHandlerGoOn
	}

	statusCode := req.HttpResponse.StatusCode
	switch statusCode / 100 {
	case 2:
		m.state.All2XX.Inc(1)
	case 3:
		m.state.All3XX.Inc(1)
	case 4:
		m.state.All4XX.Inc(1)
	case 5:
		m.state.All5XX.Inc(1)
	}

	return bfe_module.BfeHandlerGoOn
}

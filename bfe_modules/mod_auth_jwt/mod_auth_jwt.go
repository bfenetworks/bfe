// Copyright (c) 2019 Baidu, Inc.
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

package mod_auth_jwt

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/url"
	"strings"
)

import (
	"github.com/baidu/bfe/bfe_basic"
	"github.com/baidu/bfe/bfe_http"
	"github.com/baidu/bfe/bfe_module"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/config"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwt"
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

type counters struct {
	AuthTotal   *metrics.Counter
	AuthSuccess *metrics.Counter
	AuthFailed  *metrics.Counter
}

type ModuleAuthJWT struct {
	counters *counters
	metrics  *metrics.Metrics
	config   *config.Config
	confPath string
}

var Debug bool

func NewModuleAuthJWT() (module *ModuleAuthJWT) {
	module = new(ModuleAuthJWT)
	// not initialized yet
	module.counters = new(counters)
	module.metrics = new(metrics.Metrics)

	return module
}

func (module *ModuleAuthJWT) Name() (name string) {
	return "mod_auth_jwt"
}

func (module *ModuleAuthJWT) Init(callbacks *bfe_module.BfeCallbacks,
	handlers *web_monitor.WebHandlers, confRoot string) (err error) {

	module.confPath = bfe_module.ModConfPath(confRoot, module.Name())

	// initialization for module config
	module.config, err = config.New(module.confPath)
	if err != nil {
		return err
	}

	debug, _ := module.config.Get("Log.OpenDebug")
	Debug = debug.Bool()

	// initialization for metrics
	err = module.metrics.Init(module.counters, module.Name(), 0)
	if err != nil {
		return err
	}

	// register filter for auth service
	err = callbacks.AddFilter(bfe_module.HandleFoundProduct, module.authService)
	if err != nil {
		return err
	}

	// register handler for monitor service
	err = web_monitor.RegisterHandlers(handlers, web_monitor.WebHandleMonitor, map[string]interface{}{
		module.Name():           module.getMetrics,
		module.Name() + ".diff": module.getMetricsDiff,
	})
	if err != nil {
		return err
	}

	// register handler for hot deployment service
	err = handlers.RegisterHandler(web_monitor.WebHandleReload, module.Name(), module.reloadService)
	if err != nil {
		return err
	}

	return nil
}

func (module *ModuleAuthJWT) authService(request *bfe_basic.Request) (flag int, response *bfe_http.Response) {
	product := request.Route.Product
	authConfig, ok := module.config.Search(product)
	if !ok || !authConfig.Cond.Match(request) {
		if Debug && ok {
			log.Logger.Debug("%s found product %s but mismatch with the condition: %s",
				module.Name(), product, authConfig.Cond)
		}

		return bfe_module.BfeHandlerGoOn, nil
	}

	if Debug {
		log.Logger.Debug("%s receive an auth request (product: %s)", module.Name(), product)
	}

	module.counters.AuthTotal.Inc(1)

	authorization := request.HttpRequest.Header.Get("Authorization")
	prefix := "Bearer "
	if !strings.HasPrefix(authorization, prefix) {
		if Debug {
			log.Logger.Debug("%s auth failed: bad token type given (product: %s)", module.Name(), product)
		}

		module.counters.AuthFailed.Inc(1)
		// send an unauthorized response
		return bfe_module.BfeHandlerResponse, createUnauthorizedResponse(
			request, "Bearer type token required.")
	}

	token := authorization[len(prefix):]

	// apply validation for token
	err := module.validateToken(token, &authConfig.AuthConfig)
	if err != nil {
		if Debug {
			log.Logger.Debug("%s auth failed: %s (product: %s)", module.Name(), err, product)
		} else {
			// hide error detail to user
			err = errors.New("your access token was rejected")
		}

		module.counters.AuthFailed.Inc(1)
		return bfe_module.BfeHandlerResponse, createUnauthorizedResponse(
			request, err.Error())
	}

	if Debug {
		log.Logger.Debug("%s auth success. (product: %s)", module.Name(), product)
	}

	module.counters.AuthSuccess.Inc(1)

	return bfe_module.BfeHandlerGoOn, nil
}

func (module *ModuleAuthJWT) validateToken(token string, config *config.AuthConfig) (err error) {
	mJWT, err := jwt.NewJWT(token, config)
	if err != nil {
		return err
	}

	return mJWT.Validate()
}

func createUnauthorizedResponse(request *bfe_basic.Request, body string) (response *bfe_http.Response) {
	response = bfe_basic.CreateInternalResp(request, bfe_http.StatusUnauthorized)
	response.Header.Set("WWW-Authenticate", "Bearer")
	response.Body = ioutil.NopCloser(bytes.NewBufferString(body))

	return response
}

func (module *ModuleAuthJWT) getMetrics(params map[string][]string) ([]byte, error) {
	return module.metrics.GetAll().Format(params)
}

func (module *ModuleAuthJWT) getMetricsDiff(params map[string][]string) ([]byte, error) {
	return module.metrics.GetDiff().Format(params)
}

func (module *ModuleAuthJWT) reloadService(query url.Values) (err error) {
	path := query.Get("path")
	if len(path) == 0 {
		return module.config.Reload()
	}

	return module.config.Update(path)
}

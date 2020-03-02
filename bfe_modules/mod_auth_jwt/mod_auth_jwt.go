package mod_auth_jwt

import (
	"bytes"
	"errors"
	"github.com/baidu/bfe/bfe_basic"
	"github.com/baidu/bfe/bfe_http"
	"github.com/baidu/bfe/bfe_module"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwt"
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"io/ioutil"
	"strings"
)

type counters struct {
	AuthTotal   *metrics.Counter
	AuthSuccess *metrics.Counter
	AuthFailed  *metrics.Counter
}

type moduleAuthJWT struct {
	counters *counters
	metrics  *metrics.Metrics
	config   *moduleConfigProxy
	confPath string
}

var Debug bool

func NewModuleAuthJWT() (module *moduleAuthJWT) {
	module = new(moduleAuthJWT)
	// not initialized yet
	module.counters = new(counters)
	module.metrics = new(metrics.Metrics)
	module.config = new(moduleConfigProxy)
	return module
}

func (module *moduleAuthJWT) Name() (name string) {
	return "mod_auth_jwt"
}

func (module *moduleAuthJWT) Init(callbacks *bfe_module.BfeCallbacks,
	handlers *web_monitor.WebHandlers, confRoot string) (err error) {
	module.confPath = bfe_module.ModConfPath(confRoot, module.Name())
	// initialization for module config
	if err := module.config.Update(module.confPath); err != nil {
		return NewTypedError(ModuleConfigLoadFailed, err)
	}
	debug, _ := module.config.GetWithLock("Log.OpenDebug")
	Debug = debug.Bool()
	// initialization for metrics
	err = module.metrics.Init(module.counters, module.Name(), 0)
	if err != nil {
		return NewTypedError(MetricsInitFailed, err)
	}
	// register filter for auth service
	err = callbacks.AddFilter(bfe_module.HandleFoundProduct, module.authService)
	if err != nil {
		return NewTypedError(AuthServiceRegisterFailed, err)
	}
	// register handler for monitor service
	err = web_monitor.RegisterHandlers(handlers, web_monitor.WebHandleMonitor, map[string]interface{}{
		module.Name():           module.getMetrics,
		module.Name() + ".diff": module.getMetricsDiff,
	})
	if err != nil {
		return NewTypedError(MonitorServiceRegisterFailed, err)
	}
	// register handler for hot deployment service
	err = handlers.RegisterHandler(web_monitor.WebHandleReload, module.Name(), module.reloadService)
	if err != nil {
		return NewTypedError(HotDeploymentServiceRegisterFailed, err)
	}
	return nil
}

func (module *moduleAuthJWT) authService(request *bfe_basic.Request) (flag int, response *bfe_http.Response) {
	config, ok := module.config.FindProductConfig(request.Route.Product)
	if !ok || !config.Cond.Match(request) {
		if Debug && ok {
			log.Logger.Debug("Product(%s) found but failed to match with the condition: %s",
				request.Route.Product, config.Cond)
		}
		return bfe_module.BfeHandlerGoOn, nil
	}
	if Debug {
		log.Logger.Debug("Auth for request %+v", request)
	}
	module.counters.AuthTotal.Inc(1)
	authorization := request.HttpRequest.Header.Get("Authorization")
	prefix := "Bearer "
	if !strings.HasPrefix(authorization, prefix) {
		module.counters.AuthFailed.Inc(1)
		// send an unauthorized response
		return bfe_module.BfeHandlerResponse, createUnauthorizedResponse(
			request, "Bearer type token required.")
	}
	token := authorization[len(prefix):]
	// apply validation for token
	err := module.validateToken(token, config)
	if err != nil {
		if Debug {
			log.Logger.Debug("Auth failed: %s", err)
		} else {
			// hide error detail for user
			err = errors.New("your token was rejected")
		}
		module.counters.AuthFailed.Inc(1)
		return bfe_module.BfeHandlerResponse, createUnauthorizedResponse(
			request, err.Error())
	}
	if Debug {
		log.Logger.Debug("Auth success.")
	}
	module.counters.AuthSuccess.Inc(1)
	return bfe_module.BfeHandlerGoOn, nil
}

func (module *moduleAuthJWT) validateToken(token string, config *ProductConfigItem) (err error) {
	mJWT, err := jwt.NewJWT(token, &config.Config)
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

func (module *moduleAuthJWT) getMetrics(params map[string][]string) ([]byte, error) {
	return module.metrics.GetAll().Format(params)
}

func (module *moduleAuthJWT) getMetricsDiff(params map[string][]string) ([]byte, error) {
	return module.metrics.GetDiff().Format(params)
}

func (module *moduleAuthJWT) reloadService() (err error) {
	// why not directly do as: return module.config.Update(module.confPath)
	// for error(*ptr) != nil always be true, thought ptr == nil
	// to view as a (type, value) tuple:
	// ((nil, nil) == nil) == true
	// ((*ptr, nil) == nil) == false
	if err := module.config.Update(module.confPath); err != nil {
		return err
	}
	return nil
}

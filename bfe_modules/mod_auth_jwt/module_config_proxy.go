package mod_auth_jwt

import (
	"reflect"
	"strings"
	"sync"
)

type moduleConfigProxy struct {
	lock          sync.RWMutex
	value         reflect.Value
	Config        *ModuleConfig
	ProductConfig *ProductConfig
}

var invalid = reflect.Value{}

func NewModuleConfigProxy(configPath string) (proxy *moduleConfigProxy, err *TypedError) {
	proxy = new(moduleConfigProxy)
	err = proxy.Update(configPath)
	if err != nil {
		return nil, err
	}
	return proxy, nil
}

// update for Config & ProductConfig
func (proxy *moduleConfigProxy) Update(path string) (err *TypedError) {
	proxy.lock.Lock()
	defer proxy.lock.Unlock()
	moduleConfig, err := LoadModuleConfig(path)
	if err != nil {
		return err
	}
	productConfig, err := LoadProductConfig(moduleConfig)
	if err != nil {
		return err
	}
	proxy.Config = moduleConfig
	proxy.ProductConfig = productConfig
	proxy.value = reflect.Indirect(reflect.ValueOf(moduleConfig))
	return nil
}

// get field from module Config by field name (with lock)
func (proxy *moduleConfigProxy) GetWithLock(name string) (v reflect.Value, ok bool) {
	proxy.lock.RLock()
	defer proxy.lock.RUnlock()
	v = proxy.value
	// support for getter like a.b.c..
	for _, field := range strings.Split(name, ".") {
		v = v.FieldByName(field)
		if !v.IsValid() {
			return invalid, false
		}
	}
	return v, true
}

// find product Config by product name (with lock)
func (proxy *moduleConfigProxy) FindProductConfig(name string) (config *ProductConfigItem, ok bool) {
	proxy.lock.RLock()
	conf, ok := proxy.ProductConfig.Config[name]
	proxy.lock.RUnlock()
	if !ok {
		return nil, false
	}
	return &conf, true
}

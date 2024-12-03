/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package bfe_wasmplug

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/baidu/go-lib/log"
	wasmABI "github.com/bfenetworks/bfe/bfe_wasmplug/abi"
	"github.com/bfenetworks/proxy-wasm-go-host/proxywasm/common"
	v1Host "github.com/bfenetworks/proxy-wasm-go-host/proxywasm/v1"
)

var (
	ErrEngineNotFound     = errors.New("fail to get wasm engine")
	ErrWasmBytesLoad      = errors.New("fail to load wasm bytes")
	ErrWasmBytesIncorrect = errors.New("incorrect hash of wasm bytes")
	ErrInstanceCreate     = errors.New("fail to create wasm instance")
	ErrModuleCreate       = errors.New("fail to create wasm module")
)

type WasmPluginConfig struct {
	PluginName  string        `json:"plugin_name,omitempty"`
	Path   string `json:"path,omitempty"`
	Md5    string `json:"md5,omitempty"`
	WasmVersion string
	ConfigVersion string
	InstanceNum int           `json:"instance_num,omitempty"`
}

// WasmPlugin manages the collection of wasm plugin instances
type WasmPlugin interface {
	// PluginName returns the name of wasm plugin
	PluginName() string

	// GetPluginConfig returns the config of wasm plugin
	GetPluginConfig() []byte

	// GetPluginConfig returns the config of wasm plugin
	GetConfig() WasmPluginConfig

	// EnsureInstanceNum tries to expand/shrink the num of instance to 'num'
	// and returns the actual instance num
	EnsureInstanceNum(num int) int

	// InstanceNum returns the current number of wasm instance
	InstanceNum() int

	// GetInstance returns one plugin instance of the plugin
	GetInstance() common.WasmInstance

	// ReleaseInstance releases the instance to the plugin
	ReleaseInstance(instance common.WasmInstance)

	// Exec execute the f for each instance
	Exec(f func(instance common.WasmInstance) bool)

	// Clear got called when the plugin is destroyed
	Clear()

	// OnPluginStart got called when starting the wasm plugin
	OnPluginStart()

	// OnPluginDestroy got called when destroying the wasm plugin
	OnPluginDestroy()

	GetRootContextID() int32
}

type wasmPluginImpl struct {
	config WasmPluginConfig

	lock sync.RWMutex

	instanceNum    int32
	instances      []common.WasmInstance
	instancesIndex int32

	occupy int32

	vm        common.WasmVM
	wasmBytes []byte
	module    common.WasmModule

	pluginConfig []byte
	rootContextID int32
}

func NewWasmPlugin(wasmConfig WasmPluginConfig) (WasmPlugin, error) {
	// check instance num
	instanceNum := wasmConfig.InstanceNum
	if instanceNum <= 0 {
		instanceNum = runtime.NumCPU()
	}

	wasmConfig.InstanceNum = instanceNum

	// get wasm engine
	vm := GetWasmEngine()
	if vm == nil {
		return nil, ErrEngineNotFound
	}

	// load wasm bytes
	var wasmBytes []byte
	var configBytes []byte
	var md5File []byte
	var err error

	wasmBytes, err = os.ReadFile(path.Join(wasmConfig.Path, wasmConfig.PluginName + ".wasm"))
	if err != nil {
		// wasm file error
		return nil, ErrWasmBytesLoad
	}
	configBytes, err = os.ReadFile(path.Join(wasmConfig.Path, wasmConfig.PluginName + ".conf"))
	if err != nil {
		// plugin config file error
		return nil, ErrWasmBytesLoad
	}
	md5File, err = os.ReadFile(path.Join(wasmConfig.Path, wasmConfig.PluginName + ".md5"))
	if err != nil {
		// md5 file error
		return nil, ErrWasmBytesLoad
	}
	md5str := strings.TrimSpace(string(md5File))

	if len(wasmBytes) == 0 {
		return nil, ErrWasmBytesLoad
	}

	md5Bytes := md5.Sum(wasmBytes)
	newMd5 := hex.EncodeToString(md5Bytes[:])
	if newMd5 != md5str {
		return nil, ErrWasmBytesIncorrect
	}

	// create wasm module
	module := vm.NewModule(wasmBytes)
	if module == nil {
		return nil, ErrModuleCreate
	}

	plugin := &wasmPluginImpl{
		config:    wasmConfig,
		vm:        vm,
		wasmBytes: wasmBytes,
		module:    module,
		pluginConfig: configBytes,
		rootContextID: newContextID(0),
	}

	// ensure instance num
	actual := plugin.EnsureInstanceNum(wasmConfig.InstanceNum)
	if actual == 0 {
		return nil, ErrInstanceCreate
	}

	return plugin, nil
}

// EnsureInstanceNum try to expand/shrink the num of instance to 'num'
// and return the actual instance num.
func (w *wasmPluginImpl) EnsureInstanceNum(num int) int {
	if num == w.InstanceNum() {
		return w.InstanceNum()
	}

	if num < w.InstanceNum() {
		w.lock.Lock()

		for i := num; i < len(w.instances); i++ {
			w.instances[i].Stop()
			w.instances[i] = nil
		}

		w.instances = w.instances[:num]
		atomic.StoreInt32(&w.instanceNum, int32(num))

		w.lock.Unlock()
	} else {
		newInstance := make([]common.WasmInstance, 0)
		numToCreate := num - w.InstanceNum()

		for i := 0; i < numToCreate; i++ {
			instance := w.module.NewInstance()
			if instance == nil {
				log.Logger.Error("[wasm][plugin] EnsureInstanceNum fail to create instance, i: %v", i)
				continue
			}

			// Instantiate any ABI needed by the guest.
			for _, abi := range wasmABI.GetABIList(instance) {
				//abi.OnInstanceCreate(instance)
				if err := instance.RegisterImports(abi.Name()); err != nil {
					panic(err)
				}
			}

			err := instance.Start()
			if err != nil {
				log.Logger.Error("[wasm][plugin] EnsureInstanceNum fail to start instance, i: %v, err: %v", i, err)
				continue
			}

			w.OnInstanceStart(instance)
			newInstance = append(newInstance, instance)
		}

		w.lock.Lock()

		w.instances = append(w.instances, newInstance...)
		atomic.AddInt32(&w.instanceNum, int32(len(newInstance)))

		w.lock.Unlock()
	}

	return w.InstanceNum()
}

func (w *wasmPluginImpl) InstanceNum() int {
	return int(atomic.LoadInt32(&w.instanceNum))
}

func (w *wasmPluginImpl) PluginName() string {
	return w.config.PluginName
}

func (w *wasmPluginImpl) Clear() {
	// do nothing
	log.Logger.Info("[wasm][plugin] Clear wasm plugin, config: %v, instanceNum: %v", w.config, w.instanceNum)
	w.EnsureInstanceNum(0)
	log.Logger.Info("[wasm][plugin] Clear wasm plugin done, config: %v, instanceNum: %v", w.config, w.instanceNum)
}

// Exec execute the f for each instance.
func (w *wasmPluginImpl) Exec(f func(instance common.WasmInstance) bool) {
	w.lock.RLock()
	defer w.lock.RUnlock()

	for _, iw := range w.instances {
		if !f(iw) {
			break
		}
	}
}

func (w *wasmPluginImpl) GetConfig() WasmPluginConfig {
	return w.config
}

func (w *wasmPluginImpl) GetPluginConfig() []byte {
	return w.pluginConfig
}

func (w *wasmPluginImpl) GetInstance() common.WasmInstance {
	w.lock.RLock()
	defer w.lock.RUnlock()

	for i := 0; i < len(w.instances); i++ {
		idx := int(atomic.LoadInt32(&w.instancesIndex)) % len(w.instances)
		atomic.AddInt32(&w.instancesIndex, 1)

		instance := w.instances[idx]
		if !instance.Acquire() {
			continue
		}

		atomic.AddInt32(&w.occupy, 1)
		return instance
	}

	return nil
}

func (w *wasmPluginImpl) ReleaseInstance(instance common.WasmInstance) {
	instance.Release()
	atomic.AddInt32(&w.occupy, -1)
}

func (w *wasmPluginImpl) OnInstanceStart(instance common.WasmInstance) bool {
		abi := wasmABI.GetABIList(instance)[0]
		var exports v1Host.Exports
		if abi != nil {
			// v1
			imports := &v1Imports{plugin: w}
			imports.DefaultImportsHandler.Instance = instance
			abi.SetImports(imports)
			exports = abi.GetExports()
		} else {
			log.Logger.Error("[proxywasm][factory] unknown abi list: %v", abi)
			return false
		}

		instance.Lock(abi)
		defer instance.Unlock()

		err := exports.ProxyOnContextCreate(w.rootContextID, 0)
		if err != nil {
			log.Logger.Error("[proxywasm][factory] OnPluginStart fail to create root context id, err: %v", err)
			return true
		}

		vmConfigSize := 0
		// no vm config

		_, err = exports.ProxyOnVmStart(w.rootContextID, int32(vmConfigSize))
		if err != nil {
			log.Logger.Error("[proxywasm][factory] OnPluginStart fail to create root context id, err: %v", err)
			return true
		}

		pluginConfigSize := 0
		if pluginConfigBytes := w.GetPluginConfig(); pluginConfigBytes != nil {
			pluginConfigSize = len(pluginConfigBytes)
		}

		_, err = exports.ProxyOnConfigure(w.rootContextID, int32(pluginConfigSize))
		if err != nil {
			log.Logger.Error("[proxywasm][factory] OnPluginStart fail to create root context id, err: %v", err)
			return true
		}

		return true
}

func (w *wasmPluginImpl) OnPluginStart() {
	// w.Exec(w.OnInstanceStart)
}

func (d *wasmPluginImpl) OnPluginDestroy() {}

func (w *wasmPluginImpl) GetRootContextID() int32 {
	return w.rootContextID
}

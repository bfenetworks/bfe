// Copyright (c) 2025 The BFE Authors.
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

package mod_unified_waf

import (
	"fmt"
	"sync"
	"time"

	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_modules/mod_unified_waf/waf_impl"
)

type WafClientPool struct {
	confBasic  ConfBasic
	wafEntries *waf_impl.WafImplMethodBundle

	wafClients   map[string]*WafClient // current working waf clients
	toDelClients []*WafClient          // to be deleted waf clients

	wafParam GlobalParam
	monitor  *MonitorStates // monitor states

	lock       sync.RWMutex // protect for wafClients and other members
	updateLock sync.Mutex   // protect for Update()
	curIdx     int
}

func NewWafClientPool(m *MonitorStates) *WafClientPool {
	p := WafClientPool{}
	p.wafClients = map[string]*WafClient{}
	p.toDelClients = []*WafClient{}

	p.monitor = m
	go p.deleteLoop()

	return &p
}

func (p *WafClientPool) SetConfBasic(confBasic ConfBasic) error {
	var err error
	p.confBasic = confBasic

	p.wafEntries, err = waf_impl.WafFactory(confBasic.WafProductName)
	if err != nil || p.wafEntries == nil {
		err := fmt.Errorf("illegal WafProductName:%s", confBasic.WafProductName)
		return err
	}
	return nil
}

func (p *WafClientPool) UpdateWafParam(data *GlobalParamConf) {
	param := &data.Config

	p.lock.Lock()
	p.wafParam = *param

	for _, c := range p.wafClients {
		c.UpdateWafGlobalParam(param)
	}
	p.lock.Unlock()
}

func (p *WafClientPool) deleteLoop() {
	t := time.NewTicker(time.Second * 1)
	defer t.Stop()

	for {
		// wait for ticker
		<-t.C

		p.lock.Lock()
		tryDeletes := p.toDelClients
		p.toDelClients = nil
		p.lock.Unlock()

		// try close waf clients
		toDelete := []*WafClient{}
		for _, client := range tryDeletes {
			if err := client.Close(); err != nil {
				// close failed, still should not delete
				toDelete = append(toDelete, client)
			} else {
				// client is closed
				log.Logger.Info("Waf client: %s is deleted.", client.serverAddress)
			}
		}

		// reset to delete clients
		p.lock.Lock()
		p.toDelClients = append(p.toDelClients, toDelete...)
		activeClientCount := int64(len(p.wafClients))
		toDeleteClientCount := int64(len(p.toDelClients))
		p.lock.Unlock()

		// for monitor
		p.monitor.state.SetNum(TO_DELETE_CLIENTS, toDeleteClientCount)
		p.monitor.state.SetNum(ACTIVE_CLIENTS, activeClientCount)
	}
}

func (p *WafClientPool) createClients(wafInstances map[string]WafInstance) map[string]*WafClient {
	clients := map[string]*WafClient{}

	for addr, wafInstance := range wafInstances {
		// new waf client has net.DialTimeout() call
		client, err := NewWafClient(p.wafEntries, addr, &wafInstance, &p.wafParam, p.confBasic.ConnPoolSize, p.monitor)
		if err != nil {
			log.Logger.Error("NewWafClient(): %s", err.Error())
		}
		clients[addr] = client

		log.Logger.Info("create waf client for %s", addr)

	}

	return clients
}

func (p *WafClientPool) addClients(clients map[string]*WafClient) {
	addedClients := []*WafClient{}

	p.lock.Lock()

	for addr, client := range clients {
		// check duplication; this should never happen.
		if _, found := p.wafClients[addr]; found {
			log.Logger.Warn("duplication waf client")

			// move to delete pool
			p.deleteClient(client)
			continue
		}

		p.wafClients[addr] = client
		addedClients = append(addedClients, client)
	}

	p.lock.Unlock()

	// for logging and monitor
	p.monitor.state.Inc(ADDED_CLIENTS, len(addedClients))
	for _, client := range addedClients {
		log.Logger.Info("Add waf client: %s", client.serverAddress)
	}
}

func (p *WafClientPool) deleteClient(client *WafClient) {
	client.SetDeleteTag()
	p.toDelClients = append(p.toDelClients, client)

	log.Logger.Info("Waf client: %s move to delete pool", client.serverAddress)
}

func (p *WafClientPool) deleteClients(toDel map[string]*WafClient) {
	p.lock.Lock()

	for addr, client := range toDel {
		// remove from p.wafClients
		delete(p.wafClients, addr)

		// move to delete pool
		p.deleteClient(client)
	}

	p.lock.Unlock()
}

// adjustInstances():
// 1, add new waf instances
// 2, remove to delete waf instances
// 3, change weight of waf instance
func (p *WafClientPool) adjustInstances(instanceMap map[string]WafInstance) (map[string]WafInstance, map[string]*WafClient) {
	toAdd := map[string]WafInstance{}
	toDel := map[string]*WafClient{}

	p.lock.RLock()

	// find new added instances
	for addr, instance := range instanceMap {
		if client, found := p.wafClients[addr]; !found {
			// new added waf instance
			toAdd[addr] = instance
		} else {
			// old waf instance, reset weight
			client.UpdateInstanceConf(&instance)
		}
	}

	// to delete instances
	for addr, client := range p.wafClients {
		if _, found := instanceMap[addr]; !found {
			// add to toDel list
			toDel[addr] = client
		}
	}

	p.lock.RUnlock()

	return toAdd, toDel
}

func (p *WafClientPool) Update(instances []WafInstance) {
	// protect from concurrent update
	p.updateLock.Lock()
	defer p.updateLock.Unlock()

	// check empty config
	if len(instances) == 0 {
		log.Logger.Warn("get empty waf instances, will remove all existed instances.")
	}

	// make instance map
	// Note: if there are some duplication instances, only one instance will be used.
	instanceMap := map[string]WafInstance{}
	for _, instance := range instances {
		addr := fmt.Sprintf("%s:%d", instance.IpAddr, instance.Port)
		instanceMap[addr] = instance
	}

	// adjust instances:
	// 1, find new added instances
	// 2, find to delete instances
	// 3, reset instance weight
	toAdd, toDel := p.adjustInstances(instanceMap)

	// add new waf clients
	clients := p.createClients(toAdd)
	p.addClients(clients)

	// delete waf clients
	p.deleteClients(toDel)
}

func (p *WafClientPool) Alloc() (*WafClient, error) {
	var client *WafClient
	var err error

	p.lock.Lock()
	client, err = p.rrBalance(p.wafClients)
	p.lock.Unlock()

	if err == nil {
		client.AddRefCount()
	}
	return client, err
}

func (p *WafClientPool) Release(client *WafClient) {
	client.DecRefCount()
}

func (p *WafClientPool) rrBalance(backs map[string]*WafClient) (*WafClient, error) {
	var best *WafClient
	var keys []string
	for key, client := range backs {
		// skip unavaliable backend
		if !client.IsAvailable() {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) <= 0 {
		return nil, fmt.Errorf("no available waf instance")
	}
	if p.curIdx >= len(keys) {
		p.curIdx = 0
	}
	best = backs[keys[p.curIdx]]
	p.curIdx = p.curIdx + 1

	return best, nil
}

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

// table for maintain backend cluster

package bfe_balance

import (
	"fmt"
	"strings"
	"sync"
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_balance/backend"
	"github.com/bfenetworks/bfe/bfe_balance/bal_gslb"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/gslb_conf"
	"github.com/bfenetworks/bfe/bfe_route"
)

// BalMap holds mappings from clusterName to BalanceGslb.
type BalMap map[string]*bal_gslb.BalanceGslb

type BalTable struct {
	lock     sync.RWMutex
	balTable BalMap // from cluster to balancer
	versions BalVersion
}

type BalVersion struct {
	ClusterTableConfVer string // cluster table conf version
	GslbConfTimeStamp   string // timestamp of gslb-conf
	GslbConfSrc         string // which gslb-scheduler come from?
}

type BalTableState struct {
	Balancers  map[string]*bal_gslb.GslbState // state of cluster
	BackendNum int                            // size of backendTable
}

func NewBalTable(checkConfFetcher backend.CheckConfFetcher) *BalTable {
	t := new(BalTable)
	t.balTable = make(BalMap)
	backend.SetCheckConfFetcher(checkConfFetcher)
	return t
}

func (t *BalTable) BalTableConfLoad(gslbConfFilename, clusterTableFilename string) (
	gslb_conf.GslbConf, cluster_table_conf.ClusterTableConf, error) {

	var gslbConf gslb_conf.GslbConf
	var backendConf cluster_table_conf.ClusterTableConf
	var err error

	gslbConf, err = gslb_conf.GslbConfLoad(gslbConfFilename)
	if err != nil {
		log.Logger.Error("gslb_conf.GslbConfLoad err [%s]", err)
		return gslbConf, backendConf, err
	}

	backendConf, err = cluster_table_conf.ClusterTableLoad(clusterTableFilename)
	if err != nil {
		log.Logger.Error("clusterBackendConfLoad err [%s]", err)
	}

	return gslbConf, backendConf, err
}

func (t *BalTable) Init(gslbConfFilename, clusterTableFilename string) error {
	gslbConf, backendConf, err := t.BalTableConfLoad(gslbConfFilename, clusterTableFilename)

	if err != nil {
		log.Logger.Error("BalTable conf load err %s", err)
		return err
	}

	// init gslb
	if err := t.gslbInit(gslbConf); err != nil {
		log.Logger.Error("clusterTable gslb init err [%s]", err)
		return err
	}

	// init backend
	if err := t.backendInit(backendConf); err != nil {
		log.Logger.Error("clusterTable backend init err [%s]", err)
		return err
	}

	log.Logger.Info("init bal table success")
	return nil
}

func (t *BalTable) gslbInit(gslbConfs gslb_conf.GslbConf) error {
	fails := make([]string, 0)

	for clusterName, gslbConf := range *gslbConfs.Clusters {
		bal := bal_gslb.NewBalanceGslb(clusterName)
		err := bal.Init(gslbConf)
		if err != nil {
			log.Logger.Error("BalTable.gslbInit():err[%s] in bal_gslb.GslbInit() for %s",
				err.Error(), clusterName)
			fails = append(fails, clusterName)
			continue
		}
		t.balTable[clusterName] = bal
	}

	// update versions
	t.versions.GslbConfTimeStamp = *gslbConfs.Ts
	t.versions.GslbConfSrc = *gslbConfs.Hostname

	if len(fails) != 0 {
		return fmt.Errorf("error in ClusterTable.gslbInit() for [%s]",
			strings.Join(fails, ","))
	}
	return nil
}

func (t *BalTable) backendInit(backendConfs cluster_table_conf.ClusterTableConf) error {
	fails := make([]string, 0)

	for clusterName, bal := range t.balTable {
		// get gslbConf
		backendConf, ok := (*backendConfs.Config)[clusterName]
		if !ok {
			// external checking guarantee. should not come here in theory
			log.Logger.Error("BalTable.backendInit():no backend conf for %s", clusterName)
			fails = append(fails, clusterName)
			continue
		}

		// initialize
		err := bal.BackendInit(backendConf)
		if err != nil {
			log.Logger.Error("ClusterTable.backendInit():err[%s] in cluster.BackendInit() for %s",
				err.Error(), clusterName)
			fails = append(fails, clusterName)
			continue
		}
	}

	// update versions
	t.versions.ClusterTableConfVer = *backendConfs.Version

	if len(fails) != 0 {
		return fmt.Errorf("error in ClusterTable.backendInit() for [%s]",
			strings.Join(fails, ","))
	}
	return nil
}

// SetGslbBasic sets gslb basic conf (from server data conf) for BalTable.
//
// Note:
//  - SetGslbBasic() is called after server reload gslb conf or server data conf
//  - SetGslbBasic() should be concurrency safe
func (t *BalTable) SetGslbBasic(clusterTable *bfe_route.ClusterTable) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if clusterTable == nil {
		return
	}

	for clusterName, bal := range t.balTable {
		cluster, err := clusterTable.Lookup(clusterName)
		if err != nil {
			continue
		}

		bal.SetGslbBasic(*cluster.GslbBasic)
	}
}

// SetSlowStart sets slow_start related conf (from server data conf) for BalTable.
//
// Note:
//  - SetSlowStart() is called after server reload server data conf
//  - SetSlowStart() should be concurrency safe
func (t *BalTable) SetSlowStart(clusterTable *bfe_route.ClusterTable) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	if clusterTable == nil {
		return
	}

	for clusterName, bal := range t.balTable {
		cluster, err := clusterTable.Lookup(clusterName)
		if err != nil {
			continue
		}

		bal.SetSlowStart(*cluster.BackendConf())
	}
}

func (t *BalTable) BalTableReload(gslbConfs gslb_conf.GslbConf,
	backendConfs cluster_table_conf.ClusterTableConf) error {
	t.lock.Lock()

	var fails []string
	bmNew := make(BalMap)
	for clusterName, gslbConf := range *gslbConfs.Clusters {
		bal, ok := t.balTable[clusterName]
		if !ok {
			// new one balance
			bal = bal_gslb.NewBalanceGslb(clusterName)
		} else {
			delete(t.balTable, clusterName)
		}

		// update balance
		if err := bal.Reload(gslbConf); err != nil {
			log.Logger.Error("BalTableReload():err[%s] in bal.Reload() for %s",
				err.Error(), clusterName)
			fails = append(fails, clusterName)
		}

		bmNew[clusterName] = bal
	}

	// remove bal not in configure file
	for _, remainder := range t.balTable {
		remainder.Release()
	}

	t.balTable = bmNew
	for clusterName, bal := range t.balTable {
		backendConf, ok1 := (*backendConfs.Config)[clusterName]
		if !ok1 {
			// never comes here
			log.Logger.Error("BalTableReload():no backend conf for %s", clusterName)
			fails = append(fails, clusterName)
			continue
		}

		if err := bal.BackendReload(backendConf); err != nil {
			log.Logger.Error("BalTableReload():err[%s] in bal.BackendReload() for %s",
				err.Error(), clusterName)
			fails = append(fails, clusterName)
		}
	}

	// update versions
	t.versions.ClusterTableConfVer = *backendConfs.Version
	t.versions.GslbConfTimeStamp = *gslbConfs.Ts
	t.versions.GslbConfSrc = *gslbConfs.Hostname

	t.lock.Unlock()

	if len(fails) != 0 {
		return fmt.Errorf("error in BalTableReload() for [%s]", strings.Join(fails, ","))
	}
	return nil
}

func (t *BalTable) lookup(clusterName string) (*bal_gslb.BalanceGslb, error) {
	bal, ok := t.balTable[clusterName]
	if !ok {
		return nil, fmt.Errorf("no bal found for %s", clusterName)
	}
	return bal, nil
}

// Lookup lookup BalanceGslb by cluster name.
func (t *BalTable) Lookup(clusterName string) (*bal_gslb.BalanceGslb, error) {
	t.lock.RLock()
	res, err := t.lookup(clusterName)
	t.lock.RUnlock()

	return res, err
}

func NewBalTableState() *BalTableState {
	state := new(BalTableState)
	state.Balancers = make(map[string]*bal_gslb.GslbState)

	return state
}

// GetState returns state of BalTable.
func (t *BalTable) GetState() *BalTableState {
	state := NewBalTableState()

	t.lock.RLock()

	// go through clusters
	for name, bal := range t.balTable {
		gs := bal_gslb.State(bal)
		state.Balancers[name] = gs
		state.BackendNum += gs.BackendNum
	}

	t.lock.RUnlock()

	return state
}

// GetVersions returns versions of BalTable.
func (t *BalTable) GetVersions() BalVersion {
	t.lock.RLock()
	versions := t.versions
	t.lock.RUnlock()

	return versions
}

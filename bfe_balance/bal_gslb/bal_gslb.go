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

// sub-cluster level load balance using gslb

package bal_gslb

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
)

import (
	bal_backend "github.com/bfenetworks/bfe/bfe_balance/backend"
	"github.com/bfenetworks/bfe/bfe_balance/bal_slb"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/gslb_conf"
)

const (
	DefaultRetryMax      = 3 // default max retries in assigned sub cluster
	DefaultCrossRetryMax = 1 // default max retries in other sub cluster, if retries in assigned sub cluster fail
)

type BalanceGslb struct {
	lock sync.Mutex

	name        string         // name of cluster, e.g., "news"
	subClusters SubClusterList // list of sub cluster

	totalWeight int  // sum weight of sub clusters that has a weight >0
	single      bool // only one sub cluster?
	avail       int  // if single is true, avail is index of avail sub cluster

	retryMax    int                   // max retries in assigned sub cluster
	crossRetry  int                   // max retries in other sub cluster, if all retry within assigned sub cluster fail
	hashConf    cluster_conf.HashConf // gslb hash conf
	BalanceMode string                // balanceMode, WRR or WLC, defined in cluster_conf
}

func NewBalanceGslb(name string) *BalanceGslb {
	bal := new(BalanceGslb)
	bal.name = name

	bal.retryMax = DefaultRetryMax
	bal.crossRetry = DefaultCrossRetryMax
	defaultStrategy := cluster_conf.ClientIpOnly
	defaultSessionSticky := false
	bal.hashConf = cluster_conf.HashConf{
		HashStrategy:  &defaultStrategy,
		SessionSticky: &defaultSessionSticky,
	}
	bal.BalanceMode = cluster_conf.BalanceModeWrr

	return bal
}

func (bal *BalanceGslb) SetGslbBasic(gslbBasic cluster_conf.GslbBasicConf) {
	bal.lock.Lock()

	bal.crossRetry = *gslbBasic.CrossRetry
	bal.retryMax = *gslbBasic.RetryMax
	bal.hashConf = *gslbBasic.HashConf
	bal.BalanceMode = *gslbBasic.BalanceMode

	bal.lock.Unlock()
}

func (bal *BalanceGslb) SetSlowStart(backendConf cluster_conf.BackendBasic) {
	bal.lock.Lock()

	for _, sub := range bal.subClusters {
		sub.setSlowStart(*backendConf.SlowStartTime)
	}

	bal.lock.Unlock()
}

// Init initializes gslb cluster with config
func (bal *BalanceGslb) Init(gslbConf gslb_conf.GslbClusterConf) error {
	totalWeight := 0

	for subClusterName, weight := range gslbConf {
		subCluster := newSubCluster(subClusterName)
		subCluster.weight = weight

		if weight > 0 {
			totalWeight += weight
		}

		// add sub-cluster to cluster
		bal.subClusters = append(bal.subClusters, subCluster)
	}

	if totalWeight == 0 {
		// should never be here, as ClusterCheck return true
		log.Logger.Critical("gslb total weight = 0 [%s]", bal.name)
		return fmt.Errorf("gslb total weight = 0 [%s]", bal.name)
	}

	bal.totalWeight = totalWeight

	// sort list to guarantee same order, since map iteration is not in order
	sort.Sort(SubClusterListSorter{bal.subClusters})
	availNum := 0
	for index, sub := range bal.subClusters {
		if sub.weight > 0 {
			bal.avail = index
			availNum += 1
		}
	}
	bal.single = (availNum == 1)

	return nil
}

func (bal *BalanceGslb) BackendInit(clusterBackend cluster_table_conf.ClusterBackend) error {
	bal.lock.Lock()

	for _, subCluster := range bal.subClusters {
		if backend, ok := clusterBackend[subCluster.Name]; ok {
			subCluster.init(backend)
		}
	}

	bal.lock.Unlock()
	return nil
}

// Reload reloads gslb config
func (bal *BalanceGslb) Reload(gslbConf gslb_conf.GslbClusterConf) error {
	bal.lock.Lock()
	defer bal.lock.Unlock()

	// create new SubClusterList
	var subListNew SubClusterList

	// create a map to record exist subCluster in gslbConf
	subExist := make(map[string]bool)

	// go through existing sub cluster, and doing update
	for i := 0; i < len(bal.subClusters); i++ {
		sub := bal.subClusters[i]

		// find new conf of sub in gslbConf
		weight, ok := gslbConf[sub.Name]

		if ok {
			// exist in new conf
			sub.weight = weight

			// add sub cluster to subListNew
			subListNew = append(subListNew, sub)
		} else {
			// release sub_cluster
			sub.release()
			log.Logger.Info("release subcluster %s", sub.Name)
		}

		// record in the map of subExist
		subExist[sub.Name] = true
	}

	// go through gslbConf, and doing init for those not in subExist
	for subName, weight := range gslbConf {
		_, ok := subExist[subName]

		if !ok {
			// create new sub cluster
			sub := newSubCluster(subName)
			sub.weight = weight

			// add sub cluster to subListNew
			subListNew = append(subListNew, sub)
		}
	}

	// sort list
	sort.Sort(SubClusterListSorter{subListNew})

	// calc total_weight
	totalWeight := 0
	availableNum := 0
	lastAvailIndex := 0

	for index, sub := range subListNew {
		if sub.weight > 0 {
			totalWeight += sub.weight
			availableNum += 1
			lastAvailIndex = index
		}
	}

	if totalWeight == 0 {
		// should never be here, as ClusterCheck return true
		log.Logger.Critical("gslb total weight = 0 [%s]", bal.name)
		return fmt.Errorf("gslb total weight = 0 [%s]", bal.name)
	}

	bal.totalWeight = totalWeight

	if availableNum == 1 {
		bal.single = true
		bal.avail = lastAvailIndex
	} else {
		bal.single = false
	}

	// update gslb.subClusters
	bal.subClusters = subListNew

	return nil
}

func (bal *BalanceGslb) BackendReload(clusterBackend cluster_table_conf.ClusterBackend) error {
	bal.lock.Lock()

	for _, subCluster := range bal.subClusters {
		if backend, ok := clusterBackend[subCluster.Name]; ok {
			subCluster.update(backend)
		}
	}

	bal.lock.Unlock()

	return nil
}

func (bal *BalanceGslb) Release() {
	bal.lock.Lock()

	// go through all sub clusters
	for i := 0; i < len(bal.subClusters); i++ {
		// release sub_cluster
		bal.subClusters[i].release()
	}

	bal.lock.Unlock()
}

// getHashKey returns hash key according hash strategy
func (bal *BalanceGslb) getHashKey(req *bfe_basic.Request) []byte {
	var clientIP net.IP
	var hashKey []byte

	if req.ClientAddr != nil {
		clientIP = req.ClientAddr.IP
	} else {
		clientIP = nil
	}

	switch *bal.hashConf.HashStrategy {
	case cluster_conf.ClientIdOnly:
		hashKey = getHashKeyByHeader(req, *bal.hashConf.HashHeader)

	case cluster_conf.ClientIpOnly:
		hashKey = clientIP

	case cluster_conf.ClientIdPreferred:
		hashKey = getHashKeyByHeader(req, *bal.hashConf.HashHeader)
		if hashKey == nil {
			hashKey = clientIP
		}

	case cluster_conf.RequestURI:
		hashKey = []byte(req.HttpRequest.RequestURI)
	}

	// if hashKey is empty, use random value
	if len(hashKey) == 0 {
		hashKey = make([]byte, 8)
		binary.BigEndian.PutUint64(hashKey, rand.Uint64())
	}

	return hashKey
}

func getHashKeyByHeader(req *bfe_basic.Request, header string) []byte {
	if val := req.HttpRequest.Header.Get(header); len(val) > 0 {
		return []byte(val)
	}

	if cookieKey, ok := cluster_conf.GetCookieKey(header); ok {
		if cookie, ok := req.Cookie(cookieKey); ok {
			return []byte(cookie.Value)
		}
	}

	return nil
}

// Balance selects a backend for given request.
func (bal *BalanceGslb) Balance(req *bfe_basic.Request) (*bal_backend.BfeBackend, error) {
	var backend *bal_backend.BfeBackend
	var current *SubCluster
	var err error
	var balAlgor int

	bal.lock.Lock()
	defer bal.lock.Unlock()

	if req.RetryTime > (bal.retryMax + bal.crossRetry) {
		// both in-cluster and cross-cluster retry failed.
		state.ErrBkRetryTooMany.Inc(1)
		// Note: req.ErrCode is not modified to ErrBkRetryTooMany, to record last error msg
		return nil, bfe_basic.ErrBkRetryTooMany
	}

	// select balance mode
	switch bal.BalanceMode {
	case cluster_conf.BalanceModeWlc:
		balAlgor = bal_slb.WlcSmooth
	default:
		balAlgor = bal_slb.WrrSmooth
	}

	// If use sticky session feature, bfe bind a user's session to a specific backend.
	// All requests from the user during the session are sent to the same backend.
	if *bal.hashConf.SessionSticky {
		balAlgor = bal_slb.WrrSticky
	}

	hashKey := bal.getHashKey(req)

	// subCluster-level balance
	current, err = bal.subClusterBalance(hashKey)
	if err != nil {
		// no sub cluster available
		state.ErrBkNoSubCluster.Inc(1)
		req.ErrCode = bfe_basic.ErrBkNoSubCluster
		return nil, bfe_basic.ErrBkNoSubCluster
	}
	req.Backend.SubclusterName = current.Name
	log.Logger.Debug("sub cluster=[%s],total_weight=[%d]",
		current.Name, bal.totalWeight)

	// after get the distribution subcluster

	// black hole
	if current.sType == TypeGslbBlackhole {
		state.ErrGslbBlackhole.Inc(1)
		req.ErrCode = bfe_basic.ErrGslbBlackhole
		return nil, bfe_basic.ErrGslbBlackhole
	}

	// still in-cluster selection
	if req.RetryTime <= bal.retryMax {
		backend, err = current.balance(balAlgor, hashKey)
		if err == nil {
			return backend, nil
		} else {
			// fail to get backend from current sub-cluster
			state.ErrBkNoBackend.Inc(1)
			log.Logger.Info("gslb.Balance():no backend(in cluster):cluster[%s], sub[%s], err[%s]",
				bal.name, current.Name, err.Error())
			req.ErrMsg = fmt.Sprintf("cluster[%s], sub[%s], err[%s]", bal.name, current.Name, err.Error())
			// Note: all backends down in current sub-cluster, may cross retry
			req.RetryTime = bal.retryMax
		}
	}

	// check if cross retry is disabled
	if bal.crossRetry <= 0 {
		req.ErrCode = bfe_basic.ErrBkNoBackend
		return nil, bfe_basic.ErrBkNoBackend
	}

	// in-cluster selection failed, select from cross-cluster
	log.Logger.Debug("start cross-cluster selection , retry = %d", req.RetryTime)
	if req.Stat != nil {
		req.Stat.IsCrossCluster = true
	}

	current, err = bal.randomSelectExclude(current)
	if err != nil {
		state.ErrBkNoSubClusterCross.Inc(1)
		req.ErrCode = bfe_basic.ErrBkNoSubClusterCross
		return nil, bfe_basic.ErrBkNoSubClusterCross
	}
	req.Backend.SubclusterName = current.Name

	backend, err = current.balance(balAlgor, hashKey)
	if err == nil {
		return backend, nil
	}

	// fail to get backend from current sub-cluster
	state.ErrBkNoBackend.Inc(1)
	req.ErrCode = bfe_basic.ErrBkNoBackend
	req.ErrMsg = fmt.Sprintf("cluster[%s], sub[%s], err[%s]", bal.name, current.Name, err.Error())
	log.Logger.Info("gslb.Balance():no backend(cross cluster):cluster[%s], sub[%s], err[%s]",
		bal.name, current.Name, err.Error())

	return backend, bfe_basic.ErrBkCrossRetryBalance
}

// subClusterBalance selects one sub cluster.
func (bal *BalanceGslb) subClusterBalance(value []byte) (*SubCluster, error) {
	var subCluster *SubCluster
	var w int

	if bal == nil {
		return subCluster, fmt.Errorf("gslb is nil")
	}

	if bal.totalWeight == 0 {
		return subCluster, fmt.Errorf("totalWeight is 0")
	}

	if bal.single {
		return bal.subClusters[bal.avail], nil
	}

	w = bal_slb.GetHash(value, uint(bal.totalWeight))

	for i := 0; i < len(bal.subClusters); i++ {
		subCluster = bal.subClusters[i]
		if subCluster.weight <= 0 {
			continue
		}
		w -= subCluster.weight
		// got it
		if w < 0 {
			break
		}
	}

	return subCluster, nil
}

// randomSelectExclude randomly selects a sub cluster, exclude exclude_sub_cluster, gslb blackhole.
func (bal *BalanceGslb) randomSelectExclude(excludeCluster *SubCluster) (*SubCluster, error) {
	var i int
	var subCluster *SubCluster

	available := 0

	for i = 0; i < len(bal.subClusters); i++ {
		subCluster = bal.subClusters[i]
		if subCluster != excludeCluster && subCluster.weight >= 0 &&
			subCluster.sType != TypeGslbBlackhole {
			available++
		}
	}

	if available == 0 {
		return subCluster, fmt.Errorf("no sub cluster available")
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	n := int(r.Int31()) % available

	for i = 0; i < len(bal.subClusters); i++ {
		subCluster = bal.subClusters[i]
		if subCluster != excludeCluster && subCluster.weight >= 0 &&
			subCluster.sType != TypeGslbBlackhole {
			if n == 0 {
				return subCluster, nil
			} else {
				n--
			}
		}
	}

	// never reach here
	return subCluster, fmt.Errorf("randomSelectExclude():should not reach here")
}

func (bal *BalanceGslb) SubClusterNum() int {
	return len(bal.subClusters)
}

type BalErrState struct {
	ErrBkNoSubCluster      *metrics.Counter
	ErrBkNoSubClusterCross *metrics.Counter
	ErrBkNoBackend         *metrics.Counter
	ErrBkRetryTooMany      *metrics.Counter
	ErrGslbBlackhole       *metrics.Counter
}

var state BalErrState

func GetBalErrState() *BalErrState {
	return &state
}

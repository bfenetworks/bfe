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

// cluster framework for bfe

package bfe_cluster

import (
	"sync"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
)

type BfeCluster struct {
	sync.RWMutex
	Name string // cluster's name

	backendConf *cluster_conf.BackendBasic  // backend's basic conf
	CheckConf   *cluster_conf.BackendCheck  // how to check backend
	GslbBasic   *cluster_conf.GslbBasicConf // gslb basic

	timeoutReadClient      time.Duration // timeout for read client body
	timeoutReadClientAgain time.Duration // timeout for read client again
	timeoutWriteClient     time.Duration // timeout for write response to client

	reqWriteBufferSize  int           // write buffer size for request
	reqFlushInterval    time.Duration // interval to flush request
	resFlushInterval    time.Duration // interval to flush response
	cancelOnClientClose bool          // cancel blocking operation in server if client conn gone
}

func NewBfeCluster(name string) *BfeCluster {
	cluster := new(BfeCluster)
	cluster.Name = name
	return cluster
}

func (cluster *BfeCluster) BasicInit(clusterConf cluster_conf.ClusterConf) {
	// set backendConf and checkConf
	cluster.backendConf = clusterConf.BackendConf
	cluster.CheckConf = clusterConf.CheckConf

	// set gslb retry conf
	cluster.GslbBasic = clusterConf.GslbBasic

	cluster.timeoutReadClient =
		time.Duration(*clusterConf.ClusterBasic.TimeoutReadClient) * time.Millisecond
	cluster.timeoutReadClientAgain =
		time.Duration(*clusterConf.ClusterBasic.TimeoutReadClientAgain) * time.Millisecond
	cluster.timeoutWriteClient =
		time.Duration(*clusterConf.ClusterBasic.TimeoutWriteClient) * time.Millisecond

	cluster.reqWriteBufferSize = *clusterConf.ClusterBasic.ReqWriteBufferSize
	cluster.reqFlushInterval =
		time.Duration(*clusterConf.ClusterBasic.ReqFlushInterval) * time.Millisecond
	cluster.resFlushInterval =
		time.Duration(*clusterConf.ClusterBasic.ResFlushInterval) * time.Millisecond
	cluster.cancelOnClientClose = *clusterConf.ClusterBasic.CancelOnClientClose

	log.Logger.Info("cluster %s init success", cluster.Name)
}

func (cluster *BfeCluster) BackendCheckConf() *cluster_conf.BackendCheck {
	cluster.RLock()
	res := cluster.CheckConf
	cluster.RUnlock()

	return res
}

func (cluster *BfeCluster) TimeoutConnSrv() int {
	cluster.RLock()
	t := *cluster.backendConf.TimeoutConnSrv
	cluster.RUnlock()

	return t
}

func (cluster *BfeCluster) BackendConf() *cluster_conf.BackendBasic {
	cluster.RLock()
	res := cluster.backendConf
	cluster.RUnlock()

	return res
}

func (cluster *BfeCluster) RetryLevel() int {
	cluster.RLock()
	retryLevel := cluster.backendConf.RetryLevel
	cluster.RUnlock()

	if retryLevel == nil {
		return cluster_conf.RetryConnect
	}
	return *retryLevel
}

func (cluster *BfeCluster) OutlierDetectionHttpCode() string {
	cluster.RLock()
	outlierDetectionHttpCode := cluster.backendConf.OutlierDetectionHttpCode
	cluster.RUnlock()
	return *outlierDetectionHttpCode
}

func (cluster *BfeCluster) TimeoutReadClient() time.Duration {
	cluster.RLock()
	res := cluster.timeoutReadClient
	cluster.RUnlock()

	return res
}

func (cluster *BfeCluster) TimeoutReadClientAgain() time.Duration {
	cluster.RLock()
	res := cluster.timeoutReadClientAgain
	cluster.RUnlock()

	return res
}

func (cluster *BfeCluster) TimeoutWriteClient() time.Duration {
	cluster.RLock()
	res := cluster.timeoutWriteClient
	cluster.RUnlock()

	return res
}

func (cluster *BfeCluster) ReqWriteBufferSize() int {
	cluster.RLock()
	res := cluster.reqWriteBufferSize
	cluster.RUnlock()

	return res
}

func (cluster *BfeCluster) ReqFlushInterval() time.Duration {
	cluster.RLock()
	res := cluster.reqFlushInterval
	cluster.RUnlock()

	return res
}

func (cluster *BfeCluster) ResFlushInterval() time.Duration {
	cluster.RLock()
	res := cluster.resFlushInterval
	cluster.RUnlock()

	return res
}

func (cluster *BfeCluster) DefaultSSEFlushInterval() time.Duration {
	return time.Second
}

func (cluster *BfeCluster) CancelOnClientClose() bool {
	cluster.RLock()
	res := cluster.cancelOnClientClose
	cluster.RUnlock()

	return res
}

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

package bfe_conf

import (
	"fmt"
	"strings"
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_util"
)

const (
	BalancerProxy = "PROXY" // layer4 balancer working in PROXY mode (eg. F5, Ctrix, ELB etc)
	BalancerNone  = "NONE"  // layer4 balancer not used
)

const (
	// LibrarySuffix defines BFE plugin's file suffix.
	LibrarySuffix = ".so"
)

type ConfigBasic struct {
	HttpPort       int  // listen port for http
	HttpsPort      int  // listen port for https
	MonitorPort    int  // web server port for monitor
	MaxCpus        int  // number of max cpus to use
	AcceptNum      int  // number of accept goroutine for each listener, default 1
	MonitorEnabled bool // web server for monitor enable or not

	// settings of layer-4 load balancer
	Layer4LoadBalancer string

	// settings of communicate with http client
	TlsHandshakeTimeout     int  // tls handshake timeout, in seconds
	ClientReadTimeout       int  // read timeout, in seconds
	ClientWriteTimeout      int  // read timeout, in seconds
	GracefulShutdownTimeout int  // graceful shutdown timeout, in seconds
	MaxHeaderBytes          int  // max header length in bytes in request
	MaxHeaderUriBytes       int  // max URI(in header) length in bytes in request
	MaxProxyHeaderBytes     int  // max header length in bytes in Proxy protocol
	KeepAliveEnabled        bool // if false, client connection is shutdown disregard of http headers

	Modules []string // modules to load
	Plugins []string // plugins to load

	// location of data files for bfe_route
	HostRuleConf  string // path of host_rule.data
	VipRuleConf   string // path of vip_rule.data
	RouteRuleConf string // path of route_rule.data

	// location of other data files
	ClusterTableConf string // path of cluster_table.data
	GslbConf         string // path of gslb.data
	ClusterConf      string // path of cluster_conf.data
	NameConf         string // path of name_conf.data

	// interval
	MonitorInterval int // interval for getting diff of proxy-state

	DebugServHttp    bool // whether open server http debug log
	DebugBfeRoute    bool // whether open bferoute debug log
	DebugBal         bool // whether open bal debug log
	DebugHealthCheck bool // whether open health check debug log
}

func (cfg *ConfigBasic) SetDefaultConf() {
	cfg.HttpPort = 8080
	cfg.HttpsPort = 8443
	cfg.MonitorPort = 8421
	cfg.MonitorEnabled = true
	cfg.MaxCpus = 0

	cfg.TlsHandshakeTimeout = 30
	cfg.ClientReadTimeout = 60
	cfg.ClientWriteTimeout = 60
	cfg.GracefulShutdownTimeout = 10
	cfg.MaxHeaderBytes = 1048576
	cfg.MaxHeaderUriBytes = 8192
	cfg.KeepAliveEnabled = true

	cfg.HostRuleConf = "server_data_conf/host_rule.data"
	cfg.VipRuleConf = "server_data_conf/vip_rule.data"
	cfg.RouteRuleConf = "server_data_conf/route_rule.data"

	cfg.ClusterTableConf = "cluster_conf/cluster_table.data"
	cfg.GslbConf = "cluster_conf/gslb.data"
	cfg.ClusterConf = "server_data_conf/cluster_conf.data"
	cfg.NameConf = "server_data_conf/name_conf.data"

	cfg.MonitorInterval = 20
}

func (cfg *ConfigBasic) Check(confRoot string) error {
	return ConfBasicCheck(cfg, confRoot)
}

func ConfBasicCheck(cfg *ConfigBasic, confRoot string) error {
	var err error

	// check basic conf
	err = basicConfCheck(cfg)
	if err != nil {
		return err
	}

	// check data file conf
	err = dataFileConfCheck(cfg, confRoot)
	if err != nil {
		return err
	}

	return nil
}

func basicConfCheck(cfg *ConfigBasic) error {
	// check HttpPort
	if cfg.HttpPort < 1 || cfg.HttpPort > 65535 {
		return fmt.Errorf("HttpPort[%d] should be in [1, 65535]",
			cfg.HttpPort)
	}

	// check HttpsPort
	if cfg.HttpsPort < 1 || cfg.HttpsPort > 65535 {
		return fmt.Errorf("HttpsPort[%d] should be in [1, 65535]",
			cfg.HttpsPort)
	}

	// check MonitorPort if MonitorEnabled enabled
	if cfg.MonitorEnabled && (cfg.MonitorPort < 1 || cfg.MonitorPort > 65535) {
		return fmt.Errorf("MonitorPort[%d] should be in [1, 65535]",
			cfg.MonitorPort)
	}

	// check MaxCpus
	if cfg.MaxCpus < 0 {
		return fmt.Errorf("MaxCpus[%d] is too small", cfg.MaxCpus)
	}

	// check Layer4LoadBalancer
	if err := checkLayer4LoadBalancer(cfg); err != nil {
		return err
	}

	// check AcceptNum
	if cfg.AcceptNum < 0 {
		return fmt.Errorf("AcceptNum[%d] is too small", cfg.AcceptNum)
	} else if cfg.AcceptNum == 0 {
		cfg.AcceptNum = 1
	}

	// check TlsHandshakeTimeout
	if cfg.TlsHandshakeTimeout <= 0 {
		return fmt.Errorf("TlsHandshakeTimeout[%d] should > 0", cfg.TlsHandshakeTimeout)
	}
	if cfg.TlsHandshakeTimeout > 1200 {
		return fmt.Errorf("TlsHandshakeTimeout[%d] should <= 1200", cfg.TlsHandshakeTimeout)
	}

	// check ClientReadTimeout
	if cfg.ClientReadTimeout <= 0 {
		return fmt.Errorf("ClientReadTimeout[%d] should > 0", cfg.ClientReadTimeout)
	}

	// check ClientWriteTimeout
	if cfg.ClientWriteTimeout <= 0 {
		return fmt.Errorf("ClientWriteTimeout[%d] should > 0", cfg.ClientWriteTimeout)
	}

	// check GracefulShutdownTimeout
	if cfg.GracefulShutdownTimeout <= 0 || cfg.GracefulShutdownTimeout > 300 {
		return fmt.Errorf("GracefulShutdownTimeout[%d] should be (0, 300]", cfg.GracefulShutdownTimeout)
	}

	// check MonitorInterval
	if cfg.MonitorInterval <= 0 {
		// not set, use default value
		log.Logger.Warn("MonitorInterval not set, use default value(20)")
		cfg.MonitorInterval = 20
	} else if cfg.MonitorInterval > 60 {
		log.Logger.Warn("MonitorInterval[%d] > 60, use 60", cfg.MonitorInterval)
		cfg.MonitorInterval = 60
	} else {
		if 60%cfg.MonitorInterval > 0 {
			return fmt.Errorf("MonitorInterval[%d] can not divide 60", cfg.MonitorInterval)
		}

		if cfg.MonitorInterval < 20 {
			return fmt.Errorf("MonitorInterval[%d] is too small(<20)", cfg.MonitorInterval)
		}
	}

	// check MaxHeaderUriBytes
	if cfg.MaxHeaderUriBytes <= 0 {
		return fmt.Errorf("MaxHeaderUriBytes[%d] should > 0", cfg.MaxHeaderUriBytes)
	}

	// check MaxHeaderBytes
	if cfg.MaxHeaderBytes <= 0 {
		return fmt.Errorf("MaxHeaderHeaderBytes[%d] should > 0", cfg.MaxHeaderBytes)
	}

	// check Plugins
	if err := checkPlugins(cfg); err != nil {
		return fmt.Errorf("plugins[%v] check failed. err: %s", cfg.Plugins, err.Error())
	}

	return nil
}

func checkLayer4LoadBalancer(cfg *ConfigBasic) error {
	if len(cfg.Layer4LoadBalancer) == 0 {
		cfg.Layer4LoadBalancer = BalancerNone // default NONE
	}

	switch cfg.Layer4LoadBalancer {
	case BalancerProxy:
		return nil
	case BalancerNone:
		return nil
	default:
		return fmt.Errorf("Layer4LoadBalancer[%s] should be PROXY/NONE", cfg.Layer4LoadBalancer)
	}
}

func checkPlugins(cfg *ConfigBasic) error {
	plugins := []string{}
	for _, pluginPath := range cfg.Plugins {
		pluginPath = strings.TrimSpace(pluginPath)
		if pluginPath == "" {
			continue
		}

		if !strings.HasSuffix(pluginPath, LibrarySuffix) {
			pluginPath += LibrarySuffix
		}
		plugins = append(plugins, pluginPath)
	}
	cfg.Plugins = plugins

	return nil
}

func dataFileConfCheck(cfg *ConfigBasic, confRoot string) error {
	// check HostRuleConf
	if cfg.HostRuleConf == "" {
		cfg.HostRuleConf = "server_data_conf/host_rule.data"
		log.Logger.Warn("HostRuleConf not set, use default value [%s]", cfg.HostRuleConf)
	}
	cfg.HostRuleConf = bfe_util.ConfPathProc(cfg.HostRuleConf, confRoot)

	// check VipRuleConf
	if cfg.VipRuleConf == "" {
		cfg.VipRuleConf = "server_data_conf/vip_rule.data"
		log.Logger.Warn("VipRuleConf not set, use default value [%s]", cfg.VipRuleConf)
	}
	cfg.VipRuleConf = bfe_util.ConfPathProc(cfg.VipRuleConf, confRoot)

	// check RouteRuleConf
	if cfg.RouteRuleConf == "" {
		cfg.RouteRuleConf = "server_data_conf/route_rule.data"
		log.Logger.Warn("RouteRuleConf not set, use default value [%s]", cfg.RouteRuleConf)
	}
	cfg.RouteRuleConf = bfe_util.ConfPathProc(cfg.RouteRuleConf, confRoot)

	// check ClusterTableConf
	if cfg.ClusterTableConf == "" {
		cfg.ClusterTableConf = "cluster_conf/cluster_table.data"
		log.Logger.Warn("ClusterTableConf not set, use default value [%s]", cfg.ClusterTableConf)
	}
	cfg.ClusterTableConf = bfe_util.ConfPathProc(cfg.ClusterTableConf, confRoot)

	// check GslbConf
	if cfg.GslbConf == "" {
		cfg.GslbConf = "cluster_conf/gslb.data"
		log.Logger.Warn("GslbConf not set, use default value [%s]", cfg.GslbConf)
	}
	cfg.GslbConf = bfe_util.ConfPathProc(cfg.GslbConf, confRoot)

	// check ClusterConf
	if cfg.ClusterConf == "" {
		cfg.ClusterConf = "server_data_conf/cluster_conf.data"
		log.Logger.Warn("ClusterConf not set, use default value [%s]", cfg.ClusterConf)
	}
	cfg.ClusterConf = bfe_util.ConfPathProc(cfg.ClusterConf, confRoot)

	// check NameConf (optional)
	if cfg.NameConf == "" {
		log.Logger.Warn("NameConf not set, ignore optional name conf")
	} else {
		cfg.NameConf = bfe_util.ConfPathProc(cfg.NameConf, confRoot)
	}

	return nil
}

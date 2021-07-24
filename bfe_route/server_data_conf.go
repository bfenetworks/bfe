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

// for route traffic to backend cluster

package bfe_route

import (
	"fmt"
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_route_conf/host_rule_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_route_conf/route_rule_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_route_conf/vip_rule_conf"
	"github.com/bfenetworks/bfe/bfe_route/bfe_cluster"
)

type ServerDataConf struct {
	HostTable    *HostTable
	ClusterTable *ClusterTable
}

func newServerDataConf() *ServerDataConf {
	c := new(ServerDataConf)

	// initialize HostTable & ClusterTable
	c.HostTable = newHostTable()
	c.ClusterTable = newClusterTable()

	return c
}

// LoadServerDataConf loads ServerDataConf config.
func LoadServerDataConf(hostFile, vipFile, routeFile, clusterConfFile string) (*ServerDataConf, error) {
	s := newServerDataConf()

	// load host table
	if err := s.hostTableLoad(hostFile, vipFile, routeFile); err != nil {
		return nil, fmt.Errorf("hostTableLoad Error %s", err)
	}

	// load cluster table
	if err := s.clusterTableLoad(clusterConfFile); err != nil {
		return nil, fmt.Errorf("clusterTableLoad Error %s", err)
	}

	// check host, route, cluster_conf dependent relationship
	if err := s.check(); err != nil {
		return nil, fmt.Errorf("ServerDataConf.check Error %s", err)
	}

	return s, nil
}

// hostTableLoad loads all data for host table from file.
func (s *ServerDataConf) hostTableLoad(hostFile, vipFile, routeFile string) error {
	// load host rule from file
	hostConf, err := host_rule_conf.HostRuleConfLoad(hostFile)
	if err != nil {
		log.Logger.Error("hostTableLoad():err in HostRuleConfLoad():%s", err.Error())
		return err
	}

	// load vip rule from file
	vipConf, err := vip_rule_conf.VipRuleConfLoad(vipFile)
	if err != nil {
		log.Logger.Error("vipTableLoad():err in VipRuleConfLoad():%s", err.Error())
		return err
	}

	// load route conf from file
	routeConf, err := route_rule_conf.RouteConfLoad(routeFile)
	if err != nil {
		log.Logger.Error("hostTableLoad():err in RouteConfLoad():%s", err.Error())
		return err
	}

	// update to host table
	s.HostTable.Update(hostConf, vipConf, routeConf)
	return nil
}

func (s *ServerDataConf) clusterTableLoad(clusterConf string) error {
	err := s.ClusterTable.Init(clusterConf)
	if err != nil {
		return err
	}

	log.Logger.Info("init cluster table success")
	return nil
}

func (s *ServerDataConf) check() error {
	// check product consistency in host and route
	for product1 := range s.HostTable.productAdvancedRouteTable {
		find := false
		for _, product2 := range s.HostTable.hostTagTable {
			if product1 == product2 {
				find = true
				break
			}
		}
		if !find {
			return fmt.Errorf("product[%s] in route should exist in host!", product1)
		}
	}

	for product1 := range s.HostTable.productBasicRouteTree {
		find := false
		for _, product2 := range s.HostTable.hostTagTable {
			if product1 == product2 {
				find = true
				break
			}
		}
		if !find {
			return fmt.Errorf("product[%s] in route should exist in host!", product1)
		}
	}

	// check cluster_name of advanced rule in route and cluster_conf
	for _, routeRules := range s.HostTable.productAdvancedRouteTable {
		for _, routeRule := range routeRules {
			if _, err := s.ClusterTable.Lookup(routeRule.ClusterName); err != nil {
				return fmt.Errorf("cluster[%s] in advanced route should exist in cluster_conf",
					routeRule.ClusterName)
			}
		}
	}

	// check cluster_name of basic rule table in route and cluster_conf
	for _, routeRules := range s.HostTable.productBasicRouteTable {
		for _, routeRule := range routeRules {
			if routeRule.ClusterName == route_rule_conf.AdvancedMode {
				continue
			}

			if _, err := s.ClusterTable.Lookup(routeRule.ClusterName); err != nil {
				return fmt.Errorf("cluster[%s] in basic route should exist in cluster_conf",
					routeRule.ClusterName)
			}
		}
	}

	return nil
}

// HostTableLookup find cluster name with given hostname.
// implement interface ServerDataConfInterface.
func (s *ServerDataConf) HostTableLookup(hostname string) (string, error) {
	return s.HostTable.LookupProduct(hostname)
}

// ClusterTableLookup find backend with given cluster-name and request.
// implement interface ServerDataConfInterface.
func (s *ServerDataConf) ClusterTableLookup(clusterName string) (*bfe_cluster.BfeCluster, error) {
	return s.ClusterTable.Lookup(clusterName)
}

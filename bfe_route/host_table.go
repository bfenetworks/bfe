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

// table for mapping hostname to cluster name

package bfe_route

import (
	"errors"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_route_conf/host_rule_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_route_conf/route_rule_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_route_conf/vip_rule_conf"
	"github.com/bfenetworks/bfe/bfe_route/trie"
	"github.com/bfenetworks/bfe/bfe_util/string_reverse"
)

var (
	ErrNoProduct     = errors.New("no product found")
	ErrNoProductRule = errors.New("no route rule found for product")
	ErrNoMatchRule   = errors.New("no rule match for this req")
)

// HostTable holds mappings from host to prduct and
// mappings from product to cluster rules.
type HostTable struct {
	versions Versions // record conf versions

	hostTable      host_rule_conf.Host2HostTag    // for get host-tag
	hostTagTable   host_rule_conf.HostTag2Product // for get product name by hostname
	vipTable       vip_rule_conf.Vip2Product      // for get product name by vip (backup)
	defaultProduct string                         // default product name

	hostTrie *trie.Trie

	productBasicRouteTable    route_rule_conf.ProductBasicRouteRule    // all product's basic route rules list
	productBasicRouteTree     route_rule_conf.ProductBasicRouteTree    // all product's basic route rules tree
	productAdvancedRouteTable route_rule_conf.ProductAdvancedRouteRule // all product's advanced route rules

}

type Versions struct {
	HostTag      string // version of host-tag
	Vip          string // version of vip rule
	ProductRoute string // version of product route rule
}

type Status struct {
	HostTableSize              int
	HostTagTableSize           int
	VipTableSize               int
	ProductRouteTableSize      int
	ProductBasicRouteTableSize int
}

type route struct {
	product string
	tag     string
}

func newHostTable() *HostTable {
	t := new(HostTable)
	return t
}

// updateHostTable updates host-tag related table
func (t *HostTable) updateHostTable(conf host_rule_conf.HostConf) {
	t.versions.HostTag = conf.Version
	t.hostTable = conf.HostMap
	t.hostTagTable = conf.HostTagMap
	t.defaultProduct = conf.DefaultProduct
	t.hostTrie = buildHostRoute(conf)
}

// updateVipTable updates vip table
func (t *HostTable) updateVipTable(conf vip_rule_conf.VipConf) {
	t.versions.Vip = conf.Version
	t.vipTable = conf.VipMap
}

// updateRouteTable updates product Route Rule
func (t *HostTable) updateRouteTable(conf *route_rule_conf.RouteTableConf) {
	t.versions.ProductRoute = conf.Version
	t.productBasicRouteTree = conf.BasicRuleTree
	t.productBasicRouteTable = conf.BasicRuleMap
	t.productAdvancedRouteTable = conf.AdvancedRuleMap
}

// Update updates host table
func (t *HostTable) Update(hostConf host_rule_conf.HostConf,
	vipConf vip_rule_conf.VipConf, routeConf *route_rule_conf.RouteTableConf) {

	t.updateHostTable(hostConf)
	t.updateVipTable(vipConf)
	t.updateRouteTable(routeConf)
}

// LookupHostTagAndProduct find hosttag and product with given hostname.
func (t *HostTable) LookupHostTagAndProduct(req *bfe_basic.Request) error {
	hostName := req.HttpRequest.Host

	// lookup product by hostname
	hostRoute, err := t.findHostRoute(hostName)

	// if failed, try to lookup product by visited vip
	if err != nil {
		if vip := req.Session.Vip; vip != nil {
			hostRoute, err = t.findVipRoute(vip.String())
		}
	}

	// if failed, use default proudct
	if err != nil && t.defaultProduct != "" {
		hostRoute, err = route{product: t.defaultProduct}, nil
	}

	// set hostTag and product
	req.Route.HostTag = hostRoute.tag
	req.Route.Product = hostRoute.product
	req.Route.Error = err

	return err
}

// LookupCluster find clusterName with given request.
func (t *HostTable) LookupCluster(req *bfe_basic.Request) error {
	var clusterName string

	// match basic route rules
	basicRules, ok := t.productBasicRouteTree[req.Route.Product]
	if ok {
		host := strings.SplitN(req.HttpRequest.Host, ":", 2)[0]

		path := ""
		if req.HttpRequest.URL != nil {
			path = req.HttpRequest.URL.Path
		}

		clusterName, found := basicRules.Get(host, path)
		if found && clusterName != route_rule_conf.AdvancedMode {
			// set clusterName
			req.Route.ClusterName = clusterName
			return nil
		}
	}

	// match advanced route rules
	rules, ok := t.productAdvancedRouteTable[req.Route.Product]
	if !ok {
		req.Route.ClusterName = ""
		req.Route.Error = ErrNoProductRule
		return req.Route.Error
	}

	// matching route rules
	for _, rule := range rules {
		if rule.Cond.Match(req) {
			clusterName = rule.ClusterName
			break
		}
	}

	if clusterName == "" {
		req.Route.ClusterName = ""
		req.Route.Error = ErrNoMatchRule
		return req.Route.Error
	}

	// set clusterName
	req.Route.ClusterName = clusterName

	return nil
}

// Lookup find cluster name with given hostname.
func (t *HostTable) Lookup(req *bfe_basic.Request) bfe_basic.RequestRoute {
	route := bfe_basic.RequestRoute{}

	// 1. look up hostTag and product
	if err := t.LookupHostTagAndProduct(req); err != nil {
		route.Error = err
		return route
	}

	// 2. set product and host tag
	route.Product = req.Route.Product
	route.HostTag = req.Route.HostTag

	// 3. lookup clusterName
	if err := t.LookupCluster(req); err != nil {
		route.Error = err
		return route
	}

	// 4. set cluter name
	route.ClusterName = req.Route.ClusterName

	return route
}

// LookupProductByVip find product name by vip.
func (t *HostTable) LookupProductByVip(vip string) (string, error) {
	hostRoute, err := t.findVipRoute(vip)
	if err != nil {
		return "", err
	}

	return hostRoute.product, nil
}

// LookupProduct find product name with given hostname.
func (t *HostTable) LookupProduct(hostname string) (string, error) {
	hostRoute, err := t.findHostRoute(hostname)
	if err != nil {
		return "", err
	}

	return hostRoute.product, nil
}

// GetVersions return versions of host table.
func (t *HostTable) GetVersions() Versions {
	return t.versions
}

// GetStatus return status of host table.
func (t *HostTable) GetStatus() Status {
	var s Status
	s.ProductBasicRouteTableSize = len(t.productBasicRouteTable)
	s.ProductRouteTableSize = len(t.productAdvancedRouteTable)
	s.HostTableSize = len(t.hostTable)
	s.HostTagTableSize = len(t.hostTagTable)
	s.VipTableSize = len(t.vipTable)
	return s
}

func (t *HostTable) findHostRoute(host string) (route, error) {
	if t.hostTrie == nil {
		return route{}, ErrNoProduct
	}

	host = strings.ToLower(host)
	// get host-tag by hostname
	match, ok := t.hostTrie.Get(strings.Split(string_reverse.ReverseFqdnHost(hostnameStrip(host)), "."))
	if ok {
		// get route success, return
		return match.(route), nil
	}

	return route{}, ErrNoProduct
}

func (t *HostTable) findVipRoute(vip string) (route, error) {
	if len(t.vipTable) == 0 {
		return route{}, ErrNoProduct
	}

	if product, ok := t.vipTable[vip]; ok {
		return route{product: product}, nil
	}

	return route{}, ErrNoProduct
}

// hostnameStrip remove ":port" in hostname.
func hostnameStrip(hostname string) string {
	return strings.Split(hostname, ":")[0]
}

func buildHostRoute(conf host_rule_conf.HostConf) *trie.Trie {
	hostTrie := trie.NewTrie()

	for host, tag := range conf.HostMap {
		host = strings.ToLower(host)
		product := conf.HostTagMap[tag]
		hostTrie.Set(strings.Split(string_reverse.ReverseFqdnHost(host), "."), route{product: product, tag: tag})
	}

	return hostTrie
}

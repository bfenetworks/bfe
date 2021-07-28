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

// trees for matching basic route rule : (host+path) -> clusterName

package route_rule_conf

import (
	"fmt"
	"strings"
)

import (
	"github.com/armon/go-radix"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/string_reverse"
)

const (
	treeMatchExact    = iota // index to exact match tree
	treeMatchWildcard        // index to wildcard match tree
	treeMatchTypeNum         // index to exact match tree
)

type hostTrees [treeMatchTypeNum]*radix.Tree // trees for host match
type pathTrees [treeMatchTypeNum]*radix.Tree // trees for path match

// BasicRouteRuleTree implements radix trees for host+path matching
// hostTree (key:hostname, value:pathTree)
// pathTree (key:path, value:clusterName)
type BasicRouteRuleTree struct {
	hosts hostTrees
}

func NewBasicRouteRuleTree() *BasicRouteRuleTree {
	return &BasicRouteRuleTree{
		hosts: [treeMatchTypeNum]*radix.Tree{radix.New(), radix.New()},
	}
}

// Insert adds a new basic route rule into BasicRouteRuleTree
// key: hostname, value: pathTrees
func (r *BasicRouteRuleTree) Insert(ruleConf *BasicRouteRuleFile) error {
	if len(ruleConf.Hostname) == 0 {
		ruleConf.Hostname = append(ruleConf.Hostname, "*")
	}

	if len(ruleConf.Path) == 0 {
		ruleConf.Path = append(ruleConf.Path, "*")
	}

	for _, host := range ruleConf.Hostname {
		if host == "" {
			// not allow
			return fmt.Errorf("hostname is empty string")
		}

		pathTree := r.hosts.insert(host)
		for _, path := range ruleConf.Path {
			if err := pathTree.insert(path, *ruleConf.ClusterName); err != nil {
				return err
			}
		}
	}

	return nil
}

// insert adds a new node in hostTrees, key: hostname, value: pathTrees
// return node's value: pathTrees
func (ht *hostTrees) insert(host string) pathTrees {
	var treeType int
	var key string

	// remove * from wildcard host
	// *.bar.foo.com -> .bar.foo.com
	if host[0] == '*' {
		key = host[1:]
		treeType = treeMatchWildcard
	} else {
		key = host
		treeType = treeMatchExact
	}

	// call ReverseFqdnHost reverse hostname
	// for case insensitive comparing, convert the reversed hostname to uppercase
	// .bar.foo.com -> MOC.OOF.RAB.
	key = strings.ToUpper(string_reverse.ReverseFqdnHost(key))

	// return pathTree if already existed
	if value, found := ht[treeType].Get(key); found {
		return value.(pathTrees)
	}

	// no host found, insert new node
	value := pathTrees{radix.New(), radix.New()}
	ht[treeType].Insert(key, value)

	return value
}

// get returns pathTree for hostname
func (ht *hostTrees) get(host string) (pathTrees, bool) {
	// convert key to uppercase for case insensitive comparing
	// baz.bar.foo.com -> MOC.OOF.RAB.ZAB
	key := strings.ToUpper(string_reverse.ReverseFqdnHost(host))

	//exact match firstly
	if value, found := ht[treeMatchExact].Get(key); found {
		return value.(pathTrees), true
	}

	// try wildcard match if exact match fail
	// note: * only match one label in hostname.
	// For example: *.aaa.com can match with bbb.aaa.com, but can't match with ccc.bbb.aaa.com
	if matchedPrefix, value, found := ht[treeMatchWildcard].LongestPrefix(key); found {

		// get remaining(unmatched) part of a hostname without prefix
		// For example:
		// 		wildcard host *.bar.foo.com, key in tree would be MOC.OOF.RAB.
		// 		to host baz.bar.foo.com, key for matching would be MOC.OOF.RAB.ZAB
		// 		so, in this case, remainingPart = ZAB
		remainingPart := strings.TrimPrefix(key, matchedPrefix)

		// the remaining part should not contain "."
		// if the remaining part contain ".", it means '*' matches multiple labels in hostname, which is not allowed
		if strings.Contains(remainingPart, ".") {
			// not matched, try again to match empty string "", which match any hostname
			if value, found := ht[treeMatchWildcard].Get(""); found {
				// matched with ""
				return value.(pathTrees), true
			}
		} else {
			// matched with wildcard host
			return value.(pathTrees), true
		}
	}

	return pathTrees{}, false
}

// insert adds a new node in pathTree
// key : path, value: clusterName
func (pt *pathTrees) insert(path, cluster string) error {
	var treeType int
	var key string

	if len(path) == 0 {
		return fmt.Errorf("empth path is not allowed")
	}

	if path[len(path)-1] == '*' {
		// wildcard path, remove trailing *
		key = path[:len(path)-1]

		// append slash if no trailing one
		// /foo, /foo/ or /foo/bar can match with /foo*, but /foobar can not
		if len(key) > 0 && key[len(key)-1] != '/' {
			key = key + "/"
		}
		treeType = treeMatchWildcard
	} else {
		key = path
		treeType = treeMatchExact
	}

	if old, updated := pt[treeType].Insert(key, cluster); updated {
		// if key exist, return error
		return fmt.Errorf("path[%s] is duplicated in same host, existing cluster: %s", path, old)
	}

	return nil
}

// get returns node's value (cluster name) referenced by a path
func (pt *pathTrees) get(path string) (string, bool) {
	// exact match firstly
	if value, found := pt[treeMatchExact].Get(path); found {
		return value.(string), true
	}

	// append trailing slash
	if len(path) > 0 && path[len(path)-1] != '/' {
		path = path + "/"
	}

	// wildcard match
	if _, value, found := pt[treeMatchWildcard].LongestPrefix(path); found {
		return value.(string), true
	}

	return "", false
}

// Get returns cluster name by host and path
func (r *BasicRouteRuleTree) Get(host, path string) (string, bool) {
	// match host to get pathTree
	pathTree, found := r.hosts.get(host)
	if !found {
		return "", false
	}

	// match path
	return pathTree.get(path)
}

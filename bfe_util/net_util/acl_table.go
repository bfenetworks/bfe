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

package net_util

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

import (
	"github.com/zmap/go-iptree/iptree"
)

const (
	ACL_NOT_FOUND = "acl_not_found" // it is used when clientip is not found in acl table
)

type AclTable struct {
	reloadTimestamp string         // timestamp of reloading acl table, e.g. "20171201145259"
	ipTree          *iptree.IPTree // tree for ip string to acl name, e.g. "yunnan.cmnet"
	mutex           sync.RWMutex   // mutex for access the ipTree and reloadTimestamp
}

func NewAclTable() *AclTable {
	aclTable := new(AclTable)
	aclTable.ipTree = iptree.New()
	return aclTable
}

func (t *AclTable) GetAclName(ip string) string {
	t.mutex.RLock()
	curTree := t.ipTree
	t.mutex.RUnlock()

	val, found, err := curTree.GetByString(ip)
	if err != nil || !found {
		return ACL_NOT_FOUND
	}

	return val.(string)
}

func (t *AclTable) LoadFromFile(file *os.File) error {
	scanner := bufio.NewScanner(file)
	parsing := "header" // "header" or "data"

	// make new ip tree
	newIpTree := iptree.New()

	// parse acl conf file, write new ip tree and acl map
	var err error
	var aclName string
	for scanner.Scan() {
		line := scanner.Text()
		if parsing == "header" {
			// get header
			if line[0:3] == "acl" {
				aclName, err = parseAclHeaderLine(line)
				if err != nil {
					return err
				}
				parsing = "data"
			}
		} else if parsing == "data" {
			if line == "};" {
				parsing = "header"
			} else {
				ip, err := parseAclDataLine(line)
				if err != nil {
					return err
				}

				newIpTree.AddByString(ip, aclName)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// get current timestamp
	now := time.Now()
	timestamp := fmt.Sprintf("%d%02d%02d%02d%02d%02d\n",
		now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second())

	// update acl table members synchronously
	t.mutex.Lock()
	t.ipTree = newIpTree
	t.reloadTimestamp = timestamp
	t.mutex.Unlock()

	return nil
}

// parse acl line, e.g.,: acl "henan.cnc" {
func parseAclHeaderLine(line string) (string, error) {
	var aclName string
	if line[0:3] == "acl" {
		tokens := strings.Split(line, " ")
		if len(tokens) != 3 {
			return aclName, errors.New("format error:" + line)
		}

		aclName = tokens[1]
		aclName = strings.Trim(aclName, "\"")
		return aclName, nil
	}

	return aclName, errors.New("format error:" + line)
}

// parse acl data line, e.g.: 36.193.110.0/23;
func parseAclDataLine(line string) (string, error) {
	var ip string
	n := len(line)
	if line[n-1] == ';' {
		ip = line[0 : n-1]
		return ip, nil
	}

	return ip, errors.New("format error:" + line)
}

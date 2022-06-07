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

package action

import (
	"fmt"
	"net/url"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
)

// queryParse parses request query.
func queryParse(req *bfe_basic.Request) url.Values {
	// re-use req.Query
	if req.Query == nil {
		req.Query = req.HttpRequest.URL.Query()
	}

	return req.Query
}

// queryDump dumps queries to string, e.g., key1=value1&key2=value2.
func queryDump(queries url.Values) string {
	strs := make([]string, 0)

	for key, values := range queries {
		for _, value := range values {
			str := fmt.Sprintf("%s=%s", key, value)
			strs = append(strs, str)
		}
	}

	return strings.Join(strs, "&")
}

// ReqQueryAdd adds some number of (key, value) to query.
func ReqQueryAdd(req *bfe_basic.Request, params []string) {
	var addQueryString string

	// parse the query
	queries := queryParse(req)

	// get number of pairs
	pairNum := len(params) / 2

	// add (key, value) to queries
	for i := 0; i < pairNum; i++ {
		key := params[2*i]
		value := params[2*i+1]

		// try to get value of given key
		oldValue := queries.Get(key)

		if oldValue == "" {
			// key not exist, use Set()
			queries.Set(key, value)
		} else {
			// key exist, use Add()
			queries.Add(key, value)
		}

		addQueryString = addQueryString + "&" + key + "=" + value
	}

	// add rawQuery directly
	if req.HttpRequest.URL.RawQuery == "" {
		// if RawQuery is empty, remove prefix "&"
		req.HttpRequest.URL.RawQuery = addQueryString[1:]
	} else {
		req.HttpRequest.URL.RawQuery += addQueryString
	}
}

// ReqQueryRename renames query key from old name to new name.
func ReqQueryRename(req *bfe_basic.Request, oldName string, newName string) {
	var values []string
	var ok bool

	// add prefix "&" to simplify process
	rawQuery := "&" + req.HttpRequest.URL.RawQuery

	// parse the query
	queries := queryParse(req)

	// renanme query key from old name to new name
	if values, ok = queries[oldName]; !ok {
		// not find
		return
	}

	queries.Del(oldName)
	queries[newName] = values

	// rename keys
	srcKey := "&" + oldName + "="
	dstKey := "&" + newName + "="
	rawQuery = strings.ReplaceAll(rawQuery, srcKey, dstKey)

	// remove prefix "&"
	req.HttpRequest.URL.RawQuery = rawQuery[1:]
}

// ReqQueryDel deletes some keys from query
func ReqQueryDel(req *bfe_basic.Request, keys []string) {
	// add "&" prefix and suffix to simplify process
	rawQuery := "&" + req.HttpRequest.URL.RawQuery + "&"

	// parse the query
	queries := queryParse(req)

	// delete some keys from queries
	for _, key := range keys {
		queries.Del(key)

		for {
			// find key start &key=
			start := strings.Index(rawQuery, "&"+key+"=")
			if start == -1 {
				break
			}

			// find value end
			end := strings.Index(rawQuery[start+1:], "&")
			if end == -1 {
				break
			}

			// remove start:start+end part
			rawQuery = rawQuery[:start] + rawQuery[start+end+1:]
		}
	}

	// set rawQuery, remove "&" prefix and suffix
	if len(rawQuery) == 1 {
		req.HttpRequest.URL.RawQuery = ""
	} else {
		req.HttpRequest.URL.RawQuery = rawQuery[1 : len(rawQuery)-1]
	}
}

// ReqQueryDelAllExcept deletes all keys from query, except some keys
func ReqQueryDelAllExcept(req *bfe_basic.Request, keys []string) {
	// add "&" prefix and suffix to simplify process
	rawQuery := "&" + req.HttpRequest.URL.RawQuery + "&"

	// parse the query
	queries := queryParse(req)

	// prepare map for keys
	keysMap := make(map[string]bool)
	for _, key := range keys {
		keysMap[key] = true
	}

	// delete some keys from queries, except keys in keysMap
	for key := range queries {
		if _, ok := keysMap[key]; ok {
			continue
		}

		queries.Del(key)
		for {
			// find key start
			start := strings.Index(rawQuery, "&"+key+"=")
			if start == -1 {
				break
			}

			// find value end
			end := strings.Index(rawQuery[start+1:], "&")
			if end == -1 {
				break
			}

			// remove start:start+end part
			rawQuery = rawQuery[:start] + rawQuery[start+end+1:]
		}
	}

	// set rawQuery, remove "&" prefix and suffix
	if len(rawQuery) == 1 {
		req.HttpRequest.URL.RawQuery = ""
	} else {
		req.HttpRequest.URL.RawQuery = rawQuery[1 : len(rawQuery)-1]
	}
}

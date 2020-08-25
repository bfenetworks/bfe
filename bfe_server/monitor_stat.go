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

// web monitor module stat

package bfe_server

import (
	"fmt"
	"net/url"
)

import (
	"github.com/baidu/go-lib/web-monitor/kv_encode"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/json"
)

// HostTableStatusGet returns status of HostTable in json.
func (srv *BfeServer) HostTableStatusGet(query url.Values) ([]byte, error) {
	srv.confLock.RLock()
	serverConf := srv.ServerConf
	srv.confLock.RUnlock()

	s := serverConf.HostTable.GetStatus()

	// get param for format
	format := query.Get("format")
	if len(format) == 0 {
		// default format is json
		format = "json"
	}

	var buff []byte
	var err error

	switch format {
	case "json":
		buff, err = json.Marshal(s)
	case "kv":
		buff, err = kv_encode.Encode(s)
	default:
		err = fmt.Errorf("invalid format:%s", format)
	}
	return buff, err
}

// HostTableVersionGet returns version of HostTable in json.
func (srv *BfeServer) HostTableVersionGet(query url.Values) ([]byte, error) {
	srv.confLock.RLock()
	serverConf := srv.ServerConf
	srv.confLock.RUnlock()

	versions := serverConf.HostTable.GetVersions()

	// get param for format
	format := query.Get("format")
	if len(format) == 0 {
		// default format is json
		format = "json"
	}

	var buff []byte
	var err error

	switch format {
	case "json":
		buff, err = json.Marshal(versions)
	case "kv":
		buff, err = kv_encode.Encode(versions)
	default:
		err = fmt.Errorf("invalid format:%s", format)
	}
	return buff, err
}

// ClusterTableVersionGet returns versions of clusterTable.
func (srv *BfeServer) ClusterTableVersionGet(query url.Values) ([]byte, error) {
	srv.confLock.RLock()
	serverConf := srv.ServerConf
	srv.confLock.RUnlock()

	// get versions
	output := serverConf.ClusterTable.GetVersions()

	// get param for format
	format := query.Get("format")
	if len(format) == 0 {
		// default format is json
		format = "json"
	}

	var buff []byte
	var err error

	switch format {
	case "json":
		buff, err = json.Marshal(output)
	case "kv":
		buff, err = kv_encode.Encode(output)
	default:
		err = fmt.Errorf("invalid format:%s", format)
	}
	return buff, err
}

// BalTableStatusGet returns state of balTable.
func (srv *BfeServer) BalTableStatusGet(query url.Values) ([]byte, error) {
	var buff []byte
	var err error
	// cluster_name is not giving
	clusterName := query.Get("cluster_name")

	if len(clusterName) == 0 {
		// get states
		output := srv.balTable.GetState()

		// convert to json
		buff, err = json.Marshal(output)
	} else {
		// search cluster whether is in balTable or not
		if bal, err1 := srv.balTable.Lookup(clusterName); err1 != nil {
			buff = []byte("{\"status\": \"Not Exist\"}")
		} else if bal.SubClusterNum() == 0 {
			buff = []byte("{\"status\": \"No SubCluster\"}")
		} else {
			buff = []byte("{\"status\": \"Exist\"}")
		}
	}

	return buff, err
}

// BalTableVersionGet returns versions of balTable.
func (srv *BfeServer) BalTableVersionGet(query url.Values) ([]byte, error) {
	// get versions
	output := srv.balTable.GetVersions()

	// get param for format
	format := query.Get("format")
	if len(format) == 0 {
		// default format is json
		format = "json"
	}

	var buff []byte
	var err error
	switch format {
	case "json":
		buff, err = json.Marshal(output)
	case "kv":
		buff, err = kv_encode.Encode(output)
	default:
		err = fmt.Errorf("invalid format:%s", format)
	}
	return buff, err
}

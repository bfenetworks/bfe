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
	"bytes"
	"fmt"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

const (
	// connection actions
	ActionClose  = "CLOSE"  // close, close the connection directly
	ActionPass   = "PASS"   // pass, do nothing
	ActionFinish = "FINISH" // finish, close connection after reply

	// header actions
	ActionReqHeaderAdd = "REQ_HEADER_ADD" // add request header
	ActionReqHeaderSet = "REQ_HEADER_SET" // set request header
	ActionReqHeaderDel = "REQ_HEADER_DEL" // del request header

	// host actions
	ActionHostSetFromPathPrefix = "HOST_SET_FROM_PATH_PREFIX" // set host from path prefix
	ActionHostSet               = "HOST_SET"                  // set host
	ActionHostSuffixReplace     = "HOST_SUFFIX_REPLACE"       // set host replaced suffx

	// path actions
	ActionPathSet        = "PATH_SET"         // set path
	ActionPathPrefixAdd  = "PATH_PREFIX_ADD"  // add path prefix
	ActionPathPrefixTrim = "PATH_PREFIX_TRIM" // trim path prefix

	// query actions
	ActionQueryAdd          = "QUERY_ADD"            // add query
	ActionQueryDel          = "QUERY_DEL"            // del query
	ActionQueryRename       = "QUERY_RENAME"         // rename query
	ActionQueryDelAllExcept = "QUERY_DEL_ALL_EXCEPT" // del query except given query key
)

type ActionFile struct {
	Cmd    *string // command of action
	Params []string
}

type Action struct {
	Cmd    string   // command of action
	Params []string // params of action
}

// UnmarshalJSON decodes given data in json format
func (ac *Action) UnmarshalJSON(data []byte) error {
	var actionFile ActionFile

	// decode
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&actionFile); err != nil {
		return fmt.Errorf("decode Action err: %s", err)
	}

	// check
	if err := ActionFileCheck(actionFile); err != nil {
		return fmt.Errorf("actionFileCheck err: %s", err)
	}

	// convert to action
	ac.Cmd = *actionFile.Cmd
	ac.Params = actionFile.Params

	return nil
}

func (ac *Action) Check(allowActions map[string]bool) error {
	cmd := ac.Cmd

	if _, ok := allowActions[cmd]; !ok {
		// cmd not allowed
		return fmt.Errorf("action cmd[%s], is not allowed", cmd)
	}

	return nil
}

// Do process action for request
func (ac *Action) Do(req *bfe_basic.Request) error {
	switch ac.Cmd {
	// for req header
	case ActionReqHeaderAdd:
		req.HttpRequest.Header.Add(ac.Params[0], ac.Params[1])
	case ActionReqHeaderSet:
		req.HttpRequest.Header.Set(ac.Params[0], ac.Params[1])
	case ActionReqHeaderDel:
		req.HttpRequest.Header.Del(ac.Params[0])

	// for host
	case ActionHostSet:
		ReqHostSet(req, ac.Params[0])
	case ActionHostSetFromPathPrefix:
		ReqHostSetFromFirstPathSegment(req)
	case ActionHostSuffixReplace:
		ReqHostSuffixReplace(req, ac.Params[0], ac.Params[1])

	// for path
	case ActionPathSet:
		ReqPathSet(req, ac.Params[0])
	case ActionPathPrefixAdd:
		ReqPathPrefixAdd(req, ac.Params[0])
	case ActionPathPrefixTrim:
		ReqPathPrefixTrim(req, ac.Params[0])

	// for query
	case ActionQueryAdd:
		ReqQueryAdd(req, ac.Params)
	case ActionQueryRename:
		ReqQueryRename(req, ac.Params[0], ac.Params[1])
	case ActionQueryDel:
		ReqQueryDel(req, ac.Params)
	case ActionQueryDelAllExcept:
		ReqQueryDelAllExcept(req, ac.Params)
	case ActionClose, ActionPass, ActionFinish:
		// pass
	default:
		return fmt.Errorf("unknown cmd[%s]", ac.Cmd)
	}

	return nil
}

const HeaderPrefix = "X-BFE-"

func ActionFileCheck(conf ActionFile) error {
	var paramsLenCheck int

	// check command
	if conf.Cmd == nil {
		return fmt.Errorf("no Cmd")
	}

	// validate command
	*conf.Cmd = strings.ToUpper(*conf.Cmd)
	switch *conf.Cmd {
	case ActionReqHeaderAdd, ActionReqHeaderSet:
		paramsLenCheck = 2
	case ActionReqHeaderDel:
		paramsLenCheck = 1
	case ActionClose, ActionPass, ActionFinish:
		paramsLenCheck = 0
	case ActionHostSetFromPathPrefix:
		paramsLenCheck = 0
	case ActionHostSet:
		paramsLenCheck = 1
	case ActionPathSet, ActionPathPrefixAdd, ActionPathPrefixTrim:
		paramsLenCheck = 1
	case ActionQueryAdd, ActionQueryRename:
		paramsLenCheck = 2
	case ActionQueryDel, ActionQueryDelAllExcept:
		paramsLenCheck = -1 // any is OK
	default:
		return fmt.Errorf("invalid cmd:%s", *conf.Cmd)
	}

	if paramsLenCheck != -1 && len(conf.Params) != paramsLenCheck {
		return fmt.Errorf("num of params:[ok:%d, now:%d]", paramsLenCheck, len(conf.Params))
	}

	for _, p := range conf.Params {
		if len(p) == 0 {
			return fmt.Errorf("empty Params")
		}
	}

	if *conf.Cmd == ActionReqHeaderSet || *conf.Cmd == ActionReqHeaderAdd {
		header := strings.ToUpper(conf.Params[0])

		if !strings.HasPrefix(header, HeaderPrefix) {
			return fmt.Errorf("add/set header key must start with X-Bfe-, got %s", header)
		}
	}

	return nil
}

// Copyright (c) 2019 Baidu, Inc.
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

package mod_rewrite

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/action"
)

var allowActions map[string]bool = map[string]bool{
	// host actions
	action.ActionHostSetFromPathPrefix: true, // set host from path prefix
	action.ActionHostSet:               true, //set host
	action.ActionHostSuffixReplace:     true, // replace host suffix

	// path actions
	action.ActionPathSet:        true, // set path
	action.ActionPathPrefixAdd:  true, // add path prefix
	action.ActionPathPrefixTrim: true, // trim path prefix

	// query actions
	action.ActionQueryAdd:          true, // add query
	action.ActionQueryDel:          true, // del query
	action.ActionQueryRename:       true, // rename query
	action.ActionQueryDelAllExcept: true, // del query except given query key
}

func reWriteActionsDo(req *bfe_basic.Request, actions []action.Action) {
	for _, action := range actions {
		action.Do(req)
	}
}

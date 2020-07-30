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

package mod_rewrite

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/action"
)

var allowActions = map[string]interface{}{
	// host actions
	action.ActionHostSetFromPathPrefix: nil, // set host from path prefix
	action.ActionHostSet:               nil, //set host
	action.ActionHostSuffixReplace:     nil, // replace host suffix

	// path actions
	action.ActionPathSet:        nil, // set path
	action.ActionPathPrefixAdd:  nil, // add path prefix
	action.ActionPathPrefixTrim: nil, // trim path prefix

	// query actions
	action.ActionQueryAdd:          nil, // add query
	action.ActionQueryDel:          nil, // del query
	action.ActionQueryRename:       nil, // rename query
	action.ActionQueryDelAllExcept: nil, // del query except given query key
}

func reWriteActionsDo(req *bfe_basic.Request, actions []action.Action) {
	for _, action := range actions {
		action.Do(req)
	}
}

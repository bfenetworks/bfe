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

// register modules for bfe

package bfe_server

import (
	"strings"
)

import (
	"github.com/baidu/go-lib/log"
)

// RegisterModules registers bfe work module.
func (srv *BfeServer) RegisterModules(modules []string) error {
	if modules == nil {
		return nil
	}

	for _, moduleName := range modules {
		moduleName = strings.TrimSpace(moduleName)
		if len(moduleName) == 0 {
			continue
		}

		if err := srv.Modules.RegisterModule(moduleName); err != nil {
			return err
		}

		log.Logger.Info("RegisterModule():moduleName=%s", moduleName)
	}

	return nil
}

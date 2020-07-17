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

package signal_table

import (
	"os"
)

import (
	"github.com/baidu/go-lib/log"
)

// TermHandler deal with the signal that should terminate the process
func TermHandler(s os.Signal) {
	log.Logger.Info("termHandler(): receive signal[%v], terminate.", s)
	log.Logger.Close()
	os.Exit(0)
}

// IgnoreHandler deal with the signal that should be ignored
func IgnoreHandler(s os.Signal) {
	log.Logger.Info("ignoreHandler(): receive signal[%v], ignore.", s)
}

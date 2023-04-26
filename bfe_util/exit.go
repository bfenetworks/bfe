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

package bfe_util

import (
	"os"
)

import (
	"github.com/baidu/go-lib/log"
)

func exit(code int) {
	// waiting for logger finish jobs
	log.Logger.Close()
	// exit
	os.Exit(code)
}

// AbnormalExit abnormal status exit with code 1.
func AbnormalExit() {
	exit(1)
}

// NormalExit normal status exit with code 0.
func NormalExit() {
	exit(0)
}

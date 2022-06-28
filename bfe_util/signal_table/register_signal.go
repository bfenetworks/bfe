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
	"syscall"
)

// RegisterSignalHandlers register signal handlers
func RegisterSignalHandlers(signalTable *SignalTable) {
	// term handlers
	signalTable.Register(syscall.SIGTERM, TermHandler)

	// ignore handlers
	signalTable.Register(syscall.SIGHUP, IgnoreHandler)
	signalTable.Register(syscall.SIGQUIT, IgnoreHandler)
	signalTable.Register(syscall.SIGILL, IgnoreHandler)
	signalTable.Register(syscall.SIGTRAP, IgnoreHandler)
	signalTable.Register(syscall.SIGABRT, IgnoreHandler)
}

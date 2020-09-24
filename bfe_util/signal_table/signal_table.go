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
	"os/signal"
)

import (
	"github.com/baidu/go-lib/web-monitor/module_state2"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type signalHandler func(s os.Signal)

type SignalTable struct {
	shs   map[os.Signal]signalHandler // signal handle table
	state module_state2.State         // signal handle state
}

// NewSignalTable creates and init signal table
func NewSignalTable() *SignalTable {
	table := new(SignalTable)
	table.shs = make(map[os.Signal]signalHandler)
	table.state.Init()
	return table
}

// Register registers signal handle to the table
func (t *SignalTable) Register(s os.Signal, handler signalHandler) {
	if _, ok := t.shs[s]; !ok {
		t.shs[s] = handler
	}
}

// handle handles the related signal
func (t *SignalTable) handle(sig os.Signal) {
	t.state.Inc(sig.String(), 1)

	if handler, ok := t.shs[sig]; ok {
		handler(sig)
	}
}

// signalHandle is the signal handle loop
func (t *SignalTable) signalHandle() {

	var sigs []os.Signal
	for sig := range t.shs {
		sigs = append(sigs, sig)
	}

	c := make(chan os.Signal, len(sigs))
	signal.Notify(c, sigs...)

	for {
		sig := <-c
		t.handle(sig)
	}
}

// StartSignalHandle start go-routine for signal handle
func (t *SignalTable) StartSignalHandle() {
	go t.signalHandle()
}

// SignalStateGet get state counter of signal handle
func (t *SignalTable) SignalStateGet() ([]byte, error) {

	buff, err := json.Marshal(t.state.GetAll())

	return buff, err
}

// SetKeyPrefix set key prefix
func (t *SignalTable) SetKeyPrefix(key string) {
	t.state.SetKeyPrefix(key)
}

// GetKeyPrefix get key prefix
func (t *SignalTable) GetKeyPrefix() string {
	return t.state.GetKeyPrefix()
}

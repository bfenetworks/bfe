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

package bfe_module

import (
	"testing"
)

import (
	"github.com/stretchr/testify/assert"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func TestCallbackPointName(t *testing.T) {
	point2Name := map[int]string{
		HandleAccept:         "HandleAccept",
		HandleHandshake:      "HandleHandshake",
		HandleBeforeLocation: "HandleBeforeLocation",
		HandleFoundProduct:   "HandleFoundProduct",
		HandleAfterLocation:  "HandleAfterLocation",
		HandleForward:        "HandleForward",
		HandleReadResponse:   "HandleReadResponse",
		HandleRequestFinish:  "HandleRequestFinish",
		HandleFinish:         "HandleFinish",
		-1:                   "HandleUnknown",
	}
	for key, value := range point2Name {
		if e := CallbackPointName(key); e != value {
			t.Errorf("TestCallbackPointName, expecting:\n%s\nGot:\n%s\n", value, e)
		}
	}
}

func TestNewBfeCallbacks(t *testing.T) {
	bcb := NewBfeCallbacks()
	assert.Len(t, bcb.callbacks, 9)
}

func TestBfeCallbacksAddFilter(t *testing.T) {
	bcb := NewBfeCallbacks()
	var err error
	assert.NotNil(t, bcb)
	err = bcb.AddFilter(HandleAccept, func(session *bfe_basic.Session) int {
		return 0
	})
	assert.NoError(t, err)

	err = bcb.AddFilter(HandleBeforeLocation, func(req *bfe_basic.Request) (int, *bfe_http.Response) {
		return 0, nil
	})
	assert.NoError(t, err)

	err = bcb.AddFilter(HandleForward, func(req *bfe_basic.Request) int {
		return 0
	})
	assert.NoError(t, err)

	err = bcb.AddFilter(HandleReadResponse, func(req *bfe_basic.Request, res *bfe_http.Response) int {
		return 0
	})
	assert.NoError(t, err)

	err = bcb.AddFilter(HandleFinish, func(session *bfe_basic.Session) int {
		return 0
	})
	assert.NoError(t, err)

	err = bcb.AddFilter(-1, func() {})
	assert.Error(t, err)

	bcb = &BfeCallbacks{}
	bcb.callbacks = make(map[int]*HandlerList)
	bcb.callbacks[HandleAccept] = NewHandlerList(-1)
	err = bcb.AddFilter(HandleAccept, func(session *bfe_basic.Session) int {
		return 0
	})
	assert.Error(t, err)

}

func TestBfeCallbacksGetHandlerList(t *testing.T) {
	bcb := NewBfeCallbacks()
	assert.NotNil(t, bcb)
	assert.Nil(t, bcb.GetHandlerList(-1))
	assert.NotNil(t, bcb.GetHandlerList(HandleAccept))

}

func TestBfeCallbacksModuleHandlersGetJSON(t *testing.T) {
	bcb := NewBfeCallbacks()
	assert.NotNil(t, bcb)
	_, err := bcb.ModuleHandlersGetJSON()
	assert.NoError(t, err)
}

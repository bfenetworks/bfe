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

func TestHandlerListFilterAccept(t *testing.T) {
	hl := NewHandlerList(HandlersAccept)
	assert.NotNil(t, hl)
	var err error
	var session *bfe_basic.Session
	err = hl.AddAcceptFilter(func(session *bfe_basic.Session) (int, error) {
		return 0, nil
	})
	assert.Error(t, err)

	err = hl.AddAcceptFilter(func(session *bfe_basic.Session) int {
		return -1
	})
	assert.NoError(t, err)

	retVal := hl.FilterAccept(session)
	assert.EqualValues(t, -1, retVal)
}

func TestHandlerListFilterRequest(t *testing.T) {
	hl := NewHandlerList(HandlersRequest)
	assert.NotNil(t, hl)
	var err error
	var req *bfe_basic.Request
	err = hl.AddRequestFilter(func(req *bfe_basic.Request) *bfe_http.Response {
		return nil
	})
	assert.Error(t, err)

	err = hl.AddRequestFilter(func(req *bfe_basic.Request) (int, *bfe_http.Response) {
		return -1, nil
	})
	assert.NoError(t, err)

	retVal, _ := hl.FilterRequest(req)
	assert.EqualValues(t, -1, retVal)
}

func TestHandlerListFilterForward(t *testing.T) {
	hl := NewHandlerList(HandlersForward)
	assert.NotNil(t, hl)
	var err error
	var req *bfe_basic.Request
	err = hl.AddForwardFilter(func(req *bfe_basic.Request) (int, error) {
		return 0, nil
	})
	assert.Error(t, err)

	err = hl.AddForwardFilter(func(req *bfe_basic.Request) int {
		return -1
	})
	assert.NoError(t, err)

	retVal := hl.FilterForward(req)
	assert.EqualValues(t, -1, retVal)
}

func TestHandlerListFilterResponse(t *testing.T) {
	hl := NewHandlerList(HandlersResponse)
	assert.NotNil(t, hl)
	var err error
	var req *bfe_basic.Request
	var res *bfe_http.Response
	err = hl.AddResponseFilter(func(req *bfe_basic.Request) int {
		return 0
	})
	assert.Error(t, err)

	err = hl.AddResponseFilter(func(req *bfe_basic.Request, res *bfe_http.Response) int {
		return -1
	})
	assert.NoError(t, err)

	retVal := hl.FilterResponse(req, res)
	assert.EqualValues(t, -1, retVal)
}

func TestHandlerListFilterFinish(t *testing.T) {
	hl := NewHandlerList(HandlersFinish)
	assert.NotNil(t, hl)
	var err error
	var session *bfe_basic.Session
	err = hl.AddFinishFilter(func(session *bfe_basic.Session) (int, error) {
		return 0, nil
	})
	assert.Error(t, err)

	err = hl.AddFinishFilter(func(session *bfe_basic.Session) int {
		return -1
	})
	assert.NoError(t, err)

	retVal := hl.FilterFinish(session)
	assert.EqualValues(t, -1, retVal)
}

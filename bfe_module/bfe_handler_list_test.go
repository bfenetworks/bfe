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
		return BfeHandlerGoOn, nil
	})
	assert.Error(t, err)

	err = hl.AddAcceptFilter(func(session *bfe_basic.Session) int {
		return BfeHandlerGoOn
	})
	assert.NoError(t, err)

	retVal := hl.FilterAccept(session)
	assert.EqualValues(t, BfeHandlerGoOn, retVal)
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
		return BfeHandlerGoOn, nil
	})
	assert.NoError(t, err)

	retVal, _ := hl.FilterRequest(req)
	assert.EqualValues(t, BfeHandlerGoOn, retVal)
}

func TestHandlerListFilterForward(t *testing.T) {
	hl := NewHandlerList(HandlersForward)
	assert.NotNil(t, hl)
	var err error
	var req *bfe_basic.Request
	err = hl.AddForwardFilter(func(req *bfe_basic.Request) (int, error) {
		return BfeHandlerRedirect, nil
	})
	assert.Error(t, err)

	err = hl.AddForwardFilter(func(req *bfe_basic.Request) int {
		return BfeHandlerRedirect
	})
	assert.NoError(t, err)

	retVal := hl.FilterForward(req)
	assert.EqualValues(t, BfeHandlerRedirect, retVal)
}

func TestHandlerListFilterResponse(t *testing.T) {
	hl := NewHandlerList(HandlersResponse)
	assert.NotNil(t, hl)
	var err error
	var req *bfe_basic.Request
	var res *bfe_http.Response
	err = hl.AddResponseFilter(func(req *bfe_basic.Request) int {
		return BfeHandlerResponse
	})
	assert.Error(t, err)

	err = hl.AddResponseFilter(func(req *bfe_basic.Request, res *bfe_http.Response) int {
		return BfeHandlerResponse
	})
	assert.NoError(t, err)

	retVal := hl.FilterResponse(req, res)
	assert.EqualValues(t, BfeHandlerResponse, retVal)
}

func TestHandlerListFilterFinish(t *testing.T) {
	hl := NewHandlerList(HandlersFinish)
	assert.NotNil(t, hl)
	var err error
	var session *bfe_basic.Session
	err = hl.AddFinishFilter(func(session *bfe_basic.Session) (int, error) {
		return BfeHandlerFinish, nil
	})
	assert.Error(t, err)

	err = hl.AddFinishFilter(func(session *bfe_basic.Session) int {
		return BfeHandlerFinish
	})
	assert.NoError(t, err)

	retVal := hl.FilterFinish(session)
	assert.EqualValues(t, BfeHandlerFinish, retVal)
}

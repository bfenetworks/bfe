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

// list of callback filters

package bfe_module

import (
	"container/list"
	"fmt"
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

// HandlerList type.
const (
	// HandlersAccept for AcceptFilter
	HandlersAccept = iota
	// HandlersRequest for RequestFilter
	HandlersRequest
	// HandlersForward for ForwardFilter
	HandlersForward
	// HandlersResponse for ResponseFilter
	HandlersResponse
	// HandlersFinish for FinishFilter
	HandlersFinish
)

// Return value of handler.
const (
	// BfeHandlerFinish to close the connection after response
	BfeHandlerFinish = iota
	// BfeHandlerGoOn to go on next handler
	BfeHandlerGoOn
	// BfeHandlerRedirect to redirect
	BfeHandlerRedirect
	// BfeHandlerResponse to send response
	BfeHandlerResponse
	// BfeHandlerClose to close the connection directly, with no data sent.
	BfeHandlerClose
)

type HandlerList struct {
	handlerType int        /* type of handlers */
	handlers    *list.List /* list of handlers */
}

// NewHandlerList creates a HandlerList.
func NewHandlerList(handlerType int) *HandlerList {
	handlers := new(HandlerList)

	handlers.handlerType = handlerType
	handlers.handlers = list.New()

	return handlers
}

// FilterAccept filters accept with HandlerList.
func (hl *HandlerList) FilterAccept(session *bfe_basic.Session) int {
	retVal := BfeHandlerGoOn

LOOP:
	for e := hl.handlers.Front(); e != nil; e = e.Next() {
		switch filter := e.Value.(type) {
		case AcceptFilter:
			retVal = filter.FilterAccept(session)
			if retVal != BfeHandlerGoOn {
				break LOOP
			}
		default:
			log.Logger.Error("%v (%T) is not a AcceptFilter\n",
				e.Value, e.Value)
			break LOOP
		}
	}
	return retVal
}

// FilterRequest filters request with HandlerList.
func (hl *HandlerList) FilterRequest(req *bfe_basic.Request) (int, *bfe_http.Response) {
	var res *bfe_http.Response
	retVal := BfeHandlerGoOn

LOOP:
	for e := hl.handlers.Front(); e != nil; e = e.Next() {
		switch filter := e.Value.(type) {
		case RequestFilter:
			retVal, res = filter.FilterRequest(req)
			if retVal != BfeHandlerGoOn {
				break LOOP
			}
		default:
			log.Logger.Error("%v (%T) is not a RequestFilter\n",
				e.Value, e.Value)
			break LOOP
		}
	}
	return retVal, res
}

// FilterForward filters forward with HandlerList.
func (hl *HandlerList) FilterForward(req *bfe_basic.Request) int {
	retVal := BfeHandlerGoOn

LOOP:
	for e := hl.handlers.Front(); e != nil; e = e.Next() {
		switch filter := e.Value.(type) {
		case ForwardFilter:
			retVal = filter.FilterForward(req)
			if retVal != BfeHandlerGoOn {
				break LOOP
			}
		default:
			log.Logger.Error("%v (%T) is not a ForwardFilter\n",
				e.Value, e.Value)
			break LOOP
		}
	}
	return retVal
}

// FilterResponse filters request with HandlerList.
func (hl *HandlerList) FilterResponse(req *bfe_basic.Request, res *bfe_http.Response) int {
	retVal := BfeHandlerGoOn

LOOP:
	for e := hl.handlers.Front(); e != nil; e = e.Next() {
		switch filter := e.Value.(type) {
		case ResponseFilter:
			retVal = filter.FilterResponse(req, res)
			if retVal != BfeHandlerGoOn {
				break LOOP
			}
		default:
			log.Logger.Error("%v (%T) is not a ResponseFilter\n",
				e.Value, e.Value)
			break LOOP
		}
	}
	return retVal
}

// FilterFinish filters finished session with HandlerList.
func (hl *HandlerList) FilterFinish(session *bfe_basic.Session) int {
	retVal := BfeHandlerGoOn

LOOP:
	for e := hl.handlers.Front(); e != nil; e = e.Next() {
		switch filter := e.Value.(type) {
		case FinishFilter:
			retVal = filter.FilterFinish(session)
			if retVal != BfeHandlerGoOn {
				break LOOP
			}
		default:
			log.Logger.Error("%v (%T) is not a FinishFilter\n",
				e.Value, e.Value)
			break LOOP
		}
	}
	return retVal
}

// AddAcceptFilter adds accept filter to handler list.
func (hl *HandlerList) AddAcceptFilter(f interface{}) error {
	callback, ok := f.(func(session *bfe_basic.Session) int)
	if !ok {
		return fmt.Errorf("AddAcceptFilter():invalid callback func")
	}

	hl.handlers.PushBack(NewAcceptFilter(callback))
	return nil
}

// AddRequestFilter adds request filter to handler list.
func (hl *HandlerList) AddRequestFilter(f interface{}) error {
	callback, ok := f.(func(req *bfe_basic.Request) (int, *bfe_http.Response))
	if !ok {
		return fmt.Errorf("AddRequestFilter():invalid callback func")
	}

	hl.handlers.PushBack(NewRequestFilter(callback))
	return nil
}

// AddForwardFilter adds forward filter to handler list.
func (hl *HandlerList) AddForwardFilter(f interface{}) error {
	callback, ok := f.(func(req *bfe_basic.Request) int)
	if !ok {
		return fmt.Errorf("AddForwardFilter():invalid callback func")
	}

	hl.handlers.PushBack(NewForwardFilter(callback))
	return nil
}

// AddResponseFilter adds response filter to handler list.
func (hl *HandlerList) AddResponseFilter(f interface{}) error {
	callback, ok := f.(func(req *bfe_basic.Request, res *bfe_http.Response) int)
	if !ok {
		return fmt.Errorf("AddResponseFilter():invalid callback func")
	}

	hl.handlers.PushBack(NewResponseFilter(callback))
	return nil
}

// AddFinishFilter adds finish filter to handler list.
func (hl *HandlerList) AddFinishFilter(f interface{}) error {
	callback, ok := f.(func(session *bfe_basic.Session) int)
	if !ok {
		return fmt.Errorf("AddFinishFilter():invalid callback func")
	}

	hl.handlers.PushBack(NewFinishFilter(callback))
	return nil
}

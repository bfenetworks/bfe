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

// callback framework for bfe

package bfe_module

import (
	"fmt"
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/json"
)

// Callback point.
const (
	HandleAccept = iota
	HandleHandshake
	HandleBeforeLocation
	HandleFoundProduct
	HandleAfterLocation
	HandleForward
	HandleReadResponse
	HandleRequestFinish
	HandleFinish
)

func CallbackPointName(point int) string {
	switch point {
	case HandleAccept:
		return "HandleAccept"
	case HandleHandshake:
		return "HandleHandshake"
	case HandleBeforeLocation:
		return "HandleBeforeLocation"
	case HandleFoundProduct:
		return "HandleFoundProduct"
	case HandleAfterLocation:
		return "HandleAfterLocation"
	case HandleForward:
		return "HandleForward"
	case HandleReadResponse:
		return "HandleReadResponse"
	case HandleRequestFinish:
		return "HandleRequestFinish"
	case HandleFinish:
		return "HandleFinish"
	default:
		return "HandleUnknown"
	}
}

type BfeCallbacks struct {
	callbacks map[int]*HandlerList
}

// NewBfeCallbacks creates a BfeCallbacks.
func NewBfeCallbacks() *BfeCallbacks {
	// create bfeCallbacks
	bfeCallbacks := new(BfeCallbacks)
	bfeCallbacks.callbacks = make(map[int]*HandlerList)

	// create handler list for each callback point
	// for HandlesAccept
	bfeCallbacks.callbacks[HandleAccept] = NewHandlerList(HandlersAccept)
	bfeCallbacks.callbacks[HandleHandshake] = NewHandlerList(HandlersAccept)

	// for HandlersRequest
	bfeCallbacks.callbacks[HandleBeforeLocation] = NewHandlerList(HandlersRequest)
	bfeCallbacks.callbacks[HandleFoundProduct] = NewHandlerList(HandlersRequest)
	bfeCallbacks.callbacks[HandleAfterLocation] = NewHandlerList(HandlersRequest)

	// for HandlersForward
	bfeCallbacks.callbacks[HandleForward] = NewHandlerList(HandlersForward)

	// for HandlersResponse
	bfeCallbacks.callbacks[HandleReadResponse] = NewHandlerList(HandlersResponse)
	bfeCallbacks.callbacks[HandleRequestFinish] = NewHandlerList(HandlersResponse)

	// for HandlersFinish
	bfeCallbacks.callbacks[HandleFinish] = NewHandlerList(HandlersFinish)

	return bfeCallbacks
}

// AddFilter adds filter to given callback point.
func (bcb *BfeCallbacks) AddFilter(point int, f interface{}) error {
	hl, ok := bcb.callbacks[point]

	if !ok {
		return fmt.Errorf("invalid callback point[%d]", point)
	}

	var err error
	switch hl.handlerType {
	case HandlersAccept:
		err = hl.AddAcceptFilter(f)
	case HandlersRequest:
		err = hl.AddRequestFilter(f)
	case HandlersForward:
		err = hl.AddForwardFilter(f)
	case HandlersResponse:
		err = hl.AddResponseFilter(f)
	case HandlersFinish:
		err = hl.AddFinishFilter(f)
	default:
		err = fmt.Errorf("invalid type of handler list[%d]", hl.handlerType)
	}
	return err
}

// GetHandlerList gets handler list for given callback point
func (bcb *BfeCallbacks) GetHandlerList(point int) *HandlerList {
	hl, ok := bcb.callbacks[point]

	if !ok {
		log.Logger.Warn("GetHandlerList():invalid callback point[%d]", point)
		return nil
	}

	return hl
}

// ModuleHandlersGetJSON get info of handlers
func (bcb *BfeCallbacks) ModuleHandlersGetJSON() ([]byte, error) {
	cbs := make(map[string][]string)

	for point, hl := range bcb.callbacks {
		pointName := fmt.Sprintf("%d#%s", point, CallbackPointName(point))
		handlerNames := make([]string, 0)
		for e := hl.handlers.Front(); e != nil; e = e.Next() {
			handlerNames = append(handlerNames, fmt.Sprintf("%s", e.Value))
		}
		cbs[pointName] = handlerNames
	}

	return json.Marshal(cbs)
}

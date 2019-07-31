// Copyright (c) 2019 Baidu, Inc.
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

// Callback point.
const (
	HANDLE_ACCEPT          = 0
	HANDLE_HANDSHAKE       = 1
	HANDLE_BEFORE_LOCATION = 2
	HANDLE_FOUND_PRODUCT   = 3
	HANDLE_AFTER_LOCATION  = 4
	HANDLE_FORWARD         = 5
	HANDLE_READ_BACKEND    = 6
	HANDLE_REQUEST_FINISH  = 7
	HANDLE_FINISH          = 8
)

type BfeCallbacks struct {
	callbacks map[int]*HandlerList
}

// NewBfeCallbacks creates a BfeCallbacks.
func NewBfeCallbacks() *BfeCallbacks {
	// create bfeCallbacks
	bfeCallbacks := new(BfeCallbacks)
	bfeCallbacks.callbacks = make(map[int]*HandlerList)

	// create handler list for each callback point
	// for HANDLERS_ACCEPT
	bfeCallbacks.callbacks[HANDLE_ACCEPT] = NewHandlerList(HANDLERS_ACCEPT)
	bfeCallbacks.callbacks[HANDLE_HANDSHAKE] = NewHandlerList(HANDLERS_ACCEPT)

	// for HANDLERS_REQUEST
	bfeCallbacks.callbacks[HANDLE_BEFORE_LOCATION] = NewHandlerList(HANDLERS_REQUEST)
	bfeCallbacks.callbacks[HANDLE_FOUND_PRODUCT] = NewHandlerList(HANDLERS_REQUEST)
	bfeCallbacks.callbacks[HANDLE_AFTER_LOCATION] = NewHandlerList(HANDLERS_REQUEST)

	// for HANDLERS_FORWARD
	bfeCallbacks.callbacks[HANDLE_FORWARD] = NewHandlerList(HANDLERS_FORWARD)

	// for HANDLERS_RESPONSE
	bfeCallbacks.callbacks[HANDLE_READ_BACKEND] = NewHandlerList(HANDLERS_RESPONSE)
	bfeCallbacks.callbacks[HANDLE_REQUEST_FINISH] = NewHandlerList(HANDLERS_RESPONSE)

	// for HANDLERS_FINISH
	bfeCallbacks.callbacks[HANDLE_FINISH] = NewHandlerList(HANDLERS_FINISH)

	return bfeCallbacks
}

// AddFilter adds filter to given callback point.
func (bcb *BfeCallbacks) AddFilter(point int, f interface{}) error {
	hl, ok := bcb.callbacks[point]

	if !ok {
		return fmt.Errorf("invalid callback point[%d]", point)
	}

	var err error
	switch hl.h_type {
	case HANDLERS_ACCEPT:
		err = hl.AddAcceptFilter(f)
	case HANDLERS_REQUEST:
		err = hl.AddRequestFilter(f)
	case HANDLERS_FORWARD:
		err = hl.AddForwardFilter(f)
	case HANDLERS_RESPONSE:
		err = hl.AddResponseFilter(f)
	case HANDLERS_FINISH:
		err = hl.AddFinishFilter(f)
	default:
		err = fmt.Errorf("invalid type of handler list[%d]", hl.h_type)
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

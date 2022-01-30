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

// Copyright 2010 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bfe_fcgi

import (
	"fmt"
	"net"
	"reflect"
)

type ConnectError struct {
	Addr string
	Err  error
}

func (e ConnectError) Error() string {
	return fmt.Sprintf("ConnectError: %s, %s", e.Err.Error(), e.Addr)
}

type WriteRequestError struct {
	Err error
}

func (e WriteRequestError) Error() string {
	return fmt.Sprintf("WriteRequestError: %s", e.Err.Error())
}

func (e WriteRequestError) CheckTargetError(addr net.Addr) bool {
	if err, ok := e.Err.(*net.OpError); ok {
		return reflect.DeepEqual(err.Addr, addr)
	}
	return false
}

type ReadRespHeaderError struct {
	Err error
}

func (e ReadRespHeaderError) Error() string {
	return fmt.Sprintf("ReadRespHeaderError: %s", e.Err.Error())
}

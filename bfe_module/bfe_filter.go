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

// Copyright (c) 2012 ngmoco:) inc.
//
// Permission is hereby granted, free of charge, to any person obtaining
// a copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
// OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

// bfe_filter.go define filter for various scenarios.
//  AcceptFilter: filter after accept connection from client
//  RequestFilter: filter after get http request from client
//  ForwardFilter: filter before forward http request to backend
//  ResponseFilter: filter after get http response from backend
//  FinishFilter: filter before close connection from client
//
// Part of the code is borrowed from falcore.

package bfe_module

import (
	"reflect"
	"runtime"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

// RequestFilter filters incoming requests and return a response or nil.
// Filters are chained together into a HandlerList.
type RequestFilter interface {
	FilterRequest(request *bfe_basic.Request) (int, *bfe_http.Response)
}

// NewRequestFilter create a Filter by passed func.
func NewRequestFilter(f func(request *bfe_basic.Request) (int, *bfe_http.Response)) RequestFilter {
	rf := new(genericRequestFilter)
	rf.f = f
	return rf
}

type genericRequestFilter struct {
	f func(request *bfe_basic.Request) (int, *bfe_http.Response)
}

func (f *genericRequestFilter) FilterRequest(request *bfe_basic.Request) (int, *bfe_http.Response) {
	return f.f(request)
}

func (f *genericRequestFilter) String() string {
	ptr := reflect.ValueOf(f.f).Pointer()
	return runtime.FuncForPC(ptr).Name()
}

// ResponseFilter filters outgoing responses. This can be used to modify the response
// before it is sent.
type ResponseFilter interface {
	FilterResponse(req *bfe_basic.Request, res *bfe_http.Response) int
}

// NewResponseFilter creates a Filter by passed func
func NewResponseFilter(f func(req *bfe_basic.Request, res *bfe_http.Response) int) ResponseFilter {
	rf := new(genericResponseFilter)
	rf.f = f
	return rf
}

type genericResponseFilter struct {
	f func(req *bfe_basic.Request, res *bfe_http.Response) int
}

func (f *genericResponseFilter) FilterResponse(req *bfe_basic.Request, res *bfe_http.Response) int {
	return f.f(req, res)
}

func (f *genericResponseFilter) String() string {
	ptr := reflect.ValueOf(f.f).Pointer()
	return runtime.FuncForPC(ptr).Name()
}

// AcceptFilter filters incoming connections.
type AcceptFilter interface {
	FilterAccept(*bfe_basic.Session) int
}

// NewAcceptFilter creates a Filter by passed func
func NewAcceptFilter(f func(session *bfe_basic.Session) int) AcceptFilter {
	rf := new(genericAcceptFilter)
	rf.f = f
	return rf
}

type genericAcceptFilter struct {
	f func(session *bfe_basic.Session) int
}

func (f *genericAcceptFilter) FilterAccept(session *bfe_basic.Session) int {
	return f.f(session)
}

func (f *genericAcceptFilter) String() string {
	ptr := reflect.ValueOf(f.f).Pointer()
	return runtime.FuncForPC(ptr).Name()
}

// ForwardFilter filters to forward request
type ForwardFilter interface {
	FilterForward(*bfe_basic.Request) int
}

// NewForwardFilter create a Filter by passed func
func NewForwardFilter(f func(req *bfe_basic.Request) int) ForwardFilter {
	rf := new(genericForwardFilter)
	rf.f = f
	return rf
}

type genericForwardFilter struct {
	f func(req *bfe_basic.Request) int
}

func (f *genericForwardFilter) FilterForward(req *bfe_basic.Request) int {
	return f.f(req)
}

func (f *genericForwardFilter) String() string {
	ptr := reflect.ValueOf(f.f).Pointer()
	return runtime.FuncForPC(ptr).Name()
}

// FinishFilter filters finished session(connection)
type FinishFilter interface {
	FilterFinish(*bfe_basic.Session) int
}

// NewFinishFilter create a Filter by passed func.
func NewFinishFilter(f func(session *bfe_basic.Session) int) FinishFilter {
	rf := new(genericFinishFilter)
	rf.f = f
	return rf
}

type genericFinishFilter struct {
	f func(session *bfe_basic.Session) int
}

func (f *genericFinishFilter) FilterFinish(session *bfe_basic.Session) int {
	return f.f(session)
}

func (f *genericFinishFilter) String() string {
	ptr := reflect.ValueOf(f.f).Pointer()
	return runtime.FuncForPC(ptr).Name()
}

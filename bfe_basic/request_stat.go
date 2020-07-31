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

// stat for request

package bfe_basic

import (
	"time"
)

type RequestStat struct {
	// time stat
	ReadReqStart time.Time // after read first line of http request
	ReadReqEnd   time.Time // after read http request

	FindProStart time.Time // just before find product
	FindProEnd   time.Time // after find product

	LocateStart time.Time // just before find location
	LocateEnd   time.Time // after find location

	ClusterStart time.Time // just before connect backend cluster
	ClusterEnd   time.Time // after get response from backend cluster

	// info for last successful connected backend
	BackendStart time.Time // just before connect backend
	BackendEnd   time.Time // after get response from backend

	ResponseStart time.Time // before write response to client
	ResponseEnd   time.Time // after write response to client

	BackendFirst time.Time // just before connect backend, for first invoke (retry may exist)

	// data length
	HeaderLenIn  int // length of request header
	BodyLenIn    int // length of request body
	HeaderLenOut int // length of response header
	BodyLenOut   int // length of response body

	// some status
	IsCrossCluster bool // with cross-cluster retry?
}

func NewRequestStat(start time.Time) *RequestStat {
	rs := new(RequestStat)
	rs.ReadReqStart = start
	return rs
}

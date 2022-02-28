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

package mod_prison

import (
	"bytes"
	"crypto/md5"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
)

type AccessSign [16]byte

type AccessSigner struct {
	AccessSignConf                // basic conf for signature
	UrlReg         *regexp.Regexp // url regular expression
}

func (s *AccessSigner) Sign(label string, req *bfe_basic.Request) (AccessSign, error) {
	// prepare raw data for signature
	data, err := s.prepareData(label, req)
	if err != nil {
		return AccessSign{}, err
	}

	// calculate signature
	return AccessSign(md5.Sum(data)), nil
}

func (s *AccessSigner) prepareData(label string, req *bfe_basic.Request) ([]byte, error) {
	var buf bytes.Buffer

	// label
	buildKeyValue(&buf, "label", label)
	// client ip
	if s.UseClientIP {
		if req.ClientAddr == nil {
			return nil, errors.New("request without client ip")

		}
		buildKeyValue(&buf, "clientIP", req.ClientAddr.IP.String())
	}
	// request url
	if s.UseUrl {
		buildKeyValue(&buf, "url", req.HttpRequest.RequestURI)
	}
	// request host
	if s.UseHost {
		buildKeyValue(&buf, "host", req.HttpRequest.Host)
	}
	// request path
	if s.UsePath {
		buildKeyValue(&buf, "path", req.HttpRequest.URL.Path)
	}
	// substring of url
	if len(s.UrlRegexp) > 0 {
		subStrs := s.UrlReg.FindStringSubmatch(req.HttpRequest.RequestURI)
		if len(subStrs) == 0 {
			return nil, errors.New("not matched url")
		}
		buildKeyValue(&buf, "urlpattern", strings.Join(subStrs[1:], ","))
	}
	// request header
	if len(s.Header) != 0 {
		reqHeader := req.HttpRequest.Header
		for _, k := range s.Header {
			v := reqHeader.Get(k)
			if len(v) == 0 {
				return nil, errors.New("request without Header")
			}
			buildKeyValue(&buf, k, v)
		}
	}
	// request cookie
	if len(s.Cookie) != 0 {
		cookie := req.CachedCookie()
		for _, k := range s.Cookie {
			v, ok := cookie.Get(k)
			if !ok {
				return nil, errors.New("request without Cookie")
			}
			buildKeyValue(&buf, k, v.Value)
		}
	}
	// request query
	if len(s.Query) != 0 {
		query := req.CachedQuery()
		for _, k := range s.Query {
			if ok := buildQueryValues(&buf, query, k); !ok {
				return nil, errors.New("request without Query")
			}
		}
	}
	// all request headers
	if s.UseHeaders {
		keys := req.HttpRequest.HeaderKeys
		for _, key := range keys { // headers by order
			val := req.HttpRequest.Header.Get(key)
			if key == "Host" {
				val = req.HttpRequest.Host
			}
			buildKeyValue(&buf, key, val)
		}
	}

	return buf.Bytes(), nil
}

func buildKeyValue(dst *bytes.Buffer, key string, val string) {
	dst.WriteString("&")
	dst.WriteString(key)
	dst.WriteString("=")
	dst.WriteString(val)
}

// buildQueryValues builds value from equivalent queries (separate by |, eg q1|q2)
func buildQueryValues(dst *bytes.Buffer, query url.Values, keys string) bool {
	// Note: output format &q1|q2=v1v2 (instead of &q1=v1&q2=v2)
	existQuery := false
	dst.WriteString("&")
	dst.WriteString(keys)
	dst.WriteString("=")
	keyList := strings.Split(keys, "|")
	for _, key := range keyList {
		if val := query.Get(key); len(val) > 0 {
			dst.WriteString(val)
			existQuery = true
		}
	}
	return existQuery
}

type AccessCounter struct {
	count     int32 // value of stat counter
	startTime int64 // timestamp when start count
}

func NewAccessCounter() *AccessCounter {
	c := new(AccessCounter)
	c.count = 0
	c.startTime = time.Now().UnixNano()
	return c
}

func (c *AccessCounter) IncAndCheck(checkPeriodNs int64, threshold int32) (bool, int64) {
	// check timestamp first
	now := time.Now().UnixNano()
	stime := atomic.LoadInt64(&c.startTime)
	if stime+checkPeriodNs < now { // reset count
		c.reset()
	}

	// increase counter
	count := atomic.AddInt32(&c.count, 1)

	// check threshold
	stime = atomic.LoadInt64(&c.startTime)
	return count > threshold, stime + checkPeriodNs - now
}

func (f *AccessCounter) reset() {
	atomic.StoreInt32(&f.count, 0)
	now := time.Now().UnixNano()
	atomic.StoreInt64(&f.startTime, now)
}

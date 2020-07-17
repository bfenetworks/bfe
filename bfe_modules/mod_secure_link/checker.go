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

package mod_secure_link

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
)

var (
	ErrReqWithoutExpiresKey   = fmt.Errorf("bad req: without expires key")
	ErrReqInvalidExpiresValue = fmt.Errorf("bad req: invalid expires val")
	ErrReqWithoutChecksumKey  = fmt.Errorf("bad req: without checksum key")
	ErrReqInvalidChecksum     = fmt.Errorf("bad req: invalid checksum key")
	ErrReqExpired             = fmt.Errorf("req overdue")
)

type CheckerConfig struct {
	ChecksumKey     string
	ExpiresKey      string
	ExpressionNodes []ExpressionNodeFile
}

type NodeConfig struct {
	ChecksumKey string
	ExpiresKey  string
	Expr        string
}

type Checker struct {
	Config     *CheckerConfig
	expression *Expression
}

// NewChecker gen
func NewChecker(cc *CheckerConfig) (*Checker, error) {
	exper, err := NewExpression(cc)
	if err != nil {
		return nil, err
	}

	return &Checker{
		Config:     cc,
		expression: exper,
	}, nil
}

// Check validate request
func (cs *Checker) Check(request *bfe_basic.Request) error {
	if ek := cs.Config.ExpiresKey; ek != "" {
		expired := request.CachedQuery().Get(ek)
		if expired == "" {
			return ErrReqWithoutExpiresKey
		}

		expiredUnix, err := strconv.Atoi(expired)
		if err != nil {
			return ErrReqInvalidExpiresValue
		}

		if time.Now().Unix() > int64(expiredUnix) {
			return ErrReqExpired
		}
	}

	origin := request.CachedQuery().Get(cs.Config.ChecksumKey)
	if origin == "" {
		return ErrReqWithoutChecksumKey
	}

	raw := cs.expression.Value(request)
	want := cs.encode(raw)
	if want == origin {
		return nil
	}

	return ErrReqInvalidChecksum
}

// encode do encode, the shell cmd has the same result:
// echo -n '2147483647/s/link127.0.0.1 secret' | openssl md5 -binary | openssl base64 | tr +/ -_ | tr -d =
func (cs *Checker) encode(origin string) string {
	tmpB := md5.Sum([]byte(origin))
	tmp := base64.StdEncoding.EncodeToString(tmpB[:])
	tmp = strings.ReplaceAll(tmp, "+", "-")
	tmp = strings.ReplaceAll(tmp, "/", "_")
	tmp = strings.ReplaceAll(tmp, "=", "")
	return tmp
}

type Expression struct {
	nodes []ExpressionNode
}

type ExpressionNode interface {
	Value(req *bfe_basic.Request) string
}

type queryNode struct {
	key string
}

func (n queryNode) Value(req *bfe_basic.Request) string {
	return req.CachedQuery().Get(n.key)
}

type headerNode struct {
	key string
}

func (n headerNode) Value(req *bfe_basic.Request) string {
	return req.HttpRequest.Header.Get(n.key)
}

type labelNode struct {
	val string
}

func (n labelNode) Value(req *bfe_basic.Request) string {
	return n.val
}

type hostNode struct{}

func (hn hostNode) Value(req *bfe_basic.Request) string {
	return req.HttpRequest.Host
}

type uriNode struct{}

func (n uriNode) Value(req *bfe_basic.Request) string {
	return req.HttpRequest.RequestURI
}

type remoteAddrNode struct{}

func (n remoteAddrNode) Value(req *bfe_basic.Request) string {
	return req.HttpRequest.RemoteAddr
}

func NewNode(enf ExpressionNodeFile) (ExpressionNode, error) {
	switch strings.ToLower(enf.Type) {
	case "label":
		return labelNode{
			val: enf.Param,
		}, nil
	case "query":
		return queryNode{
			key: enf.Param,
		}, nil
	case "header":
		return headerNode{
			key: enf.Param,
		}, nil
	case "host":
		return hostNode{}, nil
	case "uri":
		return uriNode{}, nil
	case "remote_addr":
		return remoteAddrNode{}, nil
	default:
		return nil, fmt.Errorf("bad node type: %v", enf.Type)
	}
}

func NewExpression(cc *CheckerConfig) (*Expression, error) {
	nodes := []ExpressionNode{}

	for _, enf := range cc.ExpressionNodes {
		en, err := NewNode(enf)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, en)
	}

	return &Expression{
		nodes: nodes,
	}, nil
}

func (exp *Expression) Value(req *bfe_basic.Request) string {
	buff := &bytes.Buffer{}

	for _, n := range exp.nodes {
		buff.WriteString(n.Value(req))
	}

	return buff.String()
}

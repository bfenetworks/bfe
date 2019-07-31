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

package bfe_util

import (
	"fmt"
	"net"
	"reflect"
)

import (
	"github.com/baidu/bfe/bfe_tls"
)

import (
	sys "golang.org/x/sys/unix"
)

// GetTCPConn returns underlying TCPConn of given conn.
func GetTCPConn(conn net.Conn) (*net.TCPConn, error) {
	switch conn.(type) {
	case *bfe_tls.Conn:
		c := conn.(*bfe_tls.Conn).GetNetConn()
		return c.(*net.TCPConn), nil
	case *net.TCPConn:
		return conn.(*net.TCPConn), nil
	default:
		return nil, fmt.Errorf("GetTCPConn(): conn type not support %s", reflect.TypeOf(conn))
	}
}

// GetsockoptMutiByte returns the value of the socket option opt for the
// socket associated with fd at the given socket level.
func GetsockoptMutiByte(fd, level, opt int) ([]byte, error) {
	val, err := sys.GetsockoptString(fd, level, opt)
	return []byte(val), err
}

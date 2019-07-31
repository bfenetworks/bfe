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
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

import (
	sys "golang.org/x/sys/unix"
)

const (
	OptionVIP  = 254 // ipv4 vip from tcp option
	OptionVIP6 = 240 // ipv6 vip from tcp option
)

type Layer4InfoFetcher interface {
	// GetVirtualAddr returns virtual ip and port of given conn
	GetVirtualAddr(conn net.Conn) (net.IP, int, error)
}

type BGWInfoFetcher struct{}

func (f *BGWInfoFetcher) GetVirtualAddr(conn net.Conn) (net.IP, int, error) {
	// get underlying tcp conn
	tcpConn, err := GetTCPConn(conn)
	if err != nil {
		return nil, 0, err
	}

	// get fd
	file, err := tcpConn.File() // copy of the underlying os.File
	defer file.Close()
	if err != nil {
		return nil, 0, err
	}
	fd := int(file.Fd())

	// get options
	var opt int
	remoteAddr := conn.RemoteAddr().(*net.TCPAddr)
	if len(remoteAddr.IP.To4()) == 4 {
		opt = OptionVIP
	} else {
		opt = OptionVIP6
	}

	// get vip and port raw info
	rawInfo, err := GetsockoptMutiByte(fd, sys.IPPROTO_TCP, opt)
	if err != nil {
		return nil, 0, err
	}

	// convert raw info to net.IP
	var ip net.IP
	if opt == OptionVIP {
		ip = net.IPv4(rawInfo[4], rawInfo[5], rawInfo[6], rawInfo[7]).To4()
	} else {
		ip = net.IP(rawInfo[8:24]).To16()
	}
	if ip == nil {
		return nil, 0, errors.New("ip format error")
	}

	// get port
	port := binary.BigEndian.Uint16(rawInfo[2:4])
	return ip, int(port), nil
}

var layer4InfoFetcher Layer4InfoFetcher

func InitLayer4InfoFetcher(fetcher Layer4InfoFetcher) {
	layer4InfoFetcher = fetcher
}

// GetVipAndPort returns vip and port of given conn
func GetVipAndPort(conn net.Conn) (net.IP, int, error) {
	if layer4InfoFetcher != nil {
		return layer4InfoFetcher.GetVirtualAddr(conn)
	}
	return nil, 0, fmt.Errorf("No layer4 load balancer configed")
}

// GetVip returns vip of given conn
func GetVip(conn net.Conn) net.IP {
	vip, _, err := GetVipAndPort(conn)
	if err != nil {
		return nil
	}
	return vip
}

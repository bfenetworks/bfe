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

package ipdict

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"sort"
)

const (
	IP_SIZE     = 16 // TODO: optimize memory usage for ipv4 address
	HEADER_LEN  = 32
	MAX_LINE    = 1000000
	MAX_LOC_LEN = 1024
)

// uppercasing the first letter for binary lib
type ipLocation struct {
	startIp  net.IP
	endIp    net.IP
	location []byte
}

// []byte to string ,remove last 0 in []bytes
func byteString(p []byte) string {
	for i := 0; i < len(p); i++ {
		if p[i] == 0 {
			return string(p[0:i])
		}
	}
	return string(p)
}

type IpLocationTable struct {
	Version   string
	maxSize   uint32
	LocLen    uint32
	offset    uint32
	locations []byte
}

func NewIpLocationTable(maxSize uint32, locLen uint32) (*IpLocationTable, error) {
	// maxSize max is MAX_LINE
	if maxSize == 0 || maxSize > MAX_LINE {
		return nil, fmt.Errorf("NewIpLocationTable caused by maxSize :%d", maxSize)
	}

	// LocLen max size is MAX_LOC_LEN
	if locLen == 0 || locLen > MAX_LOC_LEN {
		return nil, fmt.Errorf("NewIpLocationTable caused by LocLen :%d", locLen)
	}

	ipLocTable := new(IpLocationTable)
	ipLocTable.maxSize = maxSize
	ipLocTable.offset = 0
	ipLocTable.LocLen = locLen
	ipLocTable.locations = make([]byte, (HEADER_LEN+locLen)*maxSize)
	return ipLocTable, nil
}

// write ipLocation Struct to locations by [HeaderLen+t.LocLen]byte
func (t *IpLocationTable) writeStruct(idx uint32, ipLoc ipLocation) {
	sOffset := idx * (t.LocLen + HEADER_LEN)
	copy(t.locations[sOffset:sOffset+IP_SIZE], ipLoc.startIp)
	copy(t.locations[sOffset+IP_SIZE:sOffset+HEADER_LEN], ipLoc.endIp)
	copy(t.locations[sOffset+HEADER_LEN:sOffset+HEADER_LEN+t.LocLen], ipLoc.location)
}

// read ipLocation from locations by idx
func (t *IpLocationTable) readStruct(idx uint32) ipLocation {
	var ipLoc ipLocation
	sOffset := idx * (t.LocLen + HEADER_LEN)
	ipLoc.startIp = t.locations[sOffset : sOffset+IP_SIZE]
	ipLoc.endIp = t.locations[sOffset+IP_SIZE : sOffset+HEADER_LEN]
	ipLoc.location = t.locations[sOffset+HEADER_LEN : sOffset+HEADER_LEN+t.LocLen]
	return ipLoc
}

// Add ip location dict to locations buffer
// assume add startIP:EndIP have been sorted
// every add startIP:EndIP region does not overlap
func (t *IpLocationTable) Add(startIP, endIP net.IP, location string) error {
	if err := checkIPPair(startIP, endIP); err != nil {
		return fmt.Errorf("Add failed: %s", err.Error())
	}

	if t.offset >= t.maxSize {
		return errors.New("Add():caused by table is full")
	}

	startIP16 := startIP.To16()
	endIP16 := endIP.To16()

	// write unit(startip,endip,location) to locations buffer
	var loc ipLocation
	loc.startIp = startIP16
	loc.endIp = endIP16
	loc.location = make([]byte, t.LocLen)
	copy(loc.location[0:t.LocLen], location)
	t.writeStruct(t.offset, loc)

	t.offset++
	return nil
}

// Search find the ip's location.
// search sort of array(order from small to large)
func (t *IpLocationTable) Search(cip net.IP) (string, error) {
	ipAddr16 := cip.To16()
	if ipAddr16 == nil {
		return "", fmt.Errorf("invalid cip: %s", cip.String())
	}

	indexLen := t.offset
	if indexLen == 0 {
		return "", fmt.Errorf("Search() error caused by locations is null")
	}

	idx := sort.Search(int(indexLen),
		func(i int) bool {
			s := uint32(i) * (HEADER_LEN + t.LocLen)
			e := uint32(i)*(HEADER_LEN+t.LocLen) + IP_SIZE
			b := t.locations[s:e]
			return bytes.Compare(b, ipAddr16) >= 0
		})

	// get idx corresponding ip section's first ip
	var firstIp net.IP
	if uint32(idx) <= indexLen-1 {
		s := uint32(idx) * (HEADER_LEN + t.LocLen)
		e := uint32(idx)*(HEADER_LEN+t.LocLen) + IP_SIZE
		firstIp = t.locations[s:e]
	}

	var preIdx uint32

	if uint32(idx) == indexLen {
		// consider ipAdd last element(uint32(idx) == indexLen)
		preIdx = indexLen - 1
	} else if firstIp.Equal(ipAddr16) || idx == 0 {
		// consider ipAdd locate in first section (idx == 0)
		// consider ipAdd is first ip in ip's section(firstIp == ipAddr16)
		preIdx = uint32(idx)
	} else {
		// other think ipAdd location previous section
		preIdx = uint32(idx - 1)
	}

	// read unit(startip,endip,location) from locations buffer
	loc := t.readStruct(preIdx)
	if bytes.Compare(ipAddr16, loc.endIp) <= 0 && bytes.Compare(ipAddr16, loc.startIp) >= 0 {
		return byteString(loc.location[0:]), nil
	}
	return "", fmt.Errorf("Search() error caused by the ip's location does not exist")
}

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

// Copyright (c) pires.
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

package bfe_proxy

import (
	"testing"
)

func TestTCPoverIPv4(t *testing.T) {
	b := byte(TCPv4)
	if !AddressFamilyAndProtocol(b).IsIPv4() {
		t.Fail()
	}
	if !AddressFamilyAndProtocol(b).IsStream() {
		t.Fail()
	}
	if AddressFamilyAndProtocol(b).toByte() != b {
		t.Fail()
	}
	if AddressFamilyAndProtocol(b).String() != "tcp4" {
		t.Fail()
	}
}

func TestTCPoverIPv6(t *testing.T) {
	b := byte(TCPv6)
	if !AddressFamilyAndProtocol(b).IsIPv6() {
		t.Fail()
	}
	if !AddressFamilyAndProtocol(b).IsStream() {
		t.Fail()
	}
	if AddressFamilyAndProtocol(b).toByte() != b {
		t.Fail()
	}
	if AddressFamilyAndProtocol(b).String() != "tcp6" {
		t.Fail()
	}
}

func TestUDPoverIPv4(t *testing.T) {
	b := byte(UDPv4)
	if !AddressFamilyAndProtocol(b).IsIPv4() {
		t.Fail()
	}
	if !AddressFamilyAndProtocol(b).IsDatagram() {
		t.Fail()
	}
	if AddressFamilyAndProtocol(b).toByte() != b {
		t.Fail()
	}
	if AddressFamilyAndProtocol(b).String() != "udp4" {
		t.Fail()
	}
}

func TestUDPoverIPv6(t *testing.T) {
	b := byte(UDPv6)
	if !AddressFamilyAndProtocol(b).IsIPv6() {
		t.Fail()
	}
	if !AddressFamilyAndProtocol(b).IsDatagram() {
		t.Fail()
	}
	if AddressFamilyAndProtocol(b).toByte() != b {
		t.Fail()
	}
	if AddressFamilyAndProtocol(b).String() != "udp6" {
		t.Fail()
	}
}

func TestUnixStream(t *testing.T) {
	b := byte(UnixStream)
	if !AddressFamilyAndProtocol(b).IsUnix() {
		t.Fail()
	}
	if !AddressFamilyAndProtocol(b).IsStream() {
		t.Fail()
	}
	if AddressFamilyAndProtocol(b).toByte() != b {
		t.Fail()
	}
	if AddressFamilyAndProtocol(b).String() != "unix" {
		t.Fail()
	}
}

func TestUnixDatagram(t *testing.T) {
	b := byte(UnixDatagram)
	if !AddressFamilyAndProtocol(b).IsUnix() {
		t.Fail()
	}
	if !AddressFamilyAndProtocol(b).IsDatagram() {
		t.Fail()
	}
	if AddressFamilyAndProtocol(b).toByte() != b {
		t.Fail()
	}
	if AddressFamilyAndProtocol(b).String() != "unixgram" {
		t.Fail()
	}
}

func TestInvalidAddressFamilyAndProtocol(t *testing.T) {
	b := byte(UNSPEC)
	if !AddressFamilyAndProtocol(b).IsUnspec() {
		t.Fail()
	}
	if AddressFamilyAndProtocol(b).toByte() != b {
		t.Fail()
	}
	if AddressFamilyAndProtocol(b).String() != "unspec" {
		t.Fail()
	}
}

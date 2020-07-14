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
	"net"
	"testing"
	"time"
)

import (
	bufio "github.com/bfenetworks/bfe/bfe_bufio"
)

// Stuff to be used in both versions tests.

const (
	NO_PROTOCOL = "There is no spoon"
	IP4_ADDR    = "127.0.0.1"
	IP6_ADDR    = "::1"
	PORT        = 65533
)

var (
	v4addr = net.ParseIP(IP4_ADDR).To4()
	v6addr = net.ParseIP(IP6_ADDR).To16()
)

type timeoutReader []byte

func (t *timeoutReader) Read([]byte) (int, error) {
	time.Sleep(1000 * time.Millisecond)
	return 0, nil
}

func TestReadTimeoutV1Invalid(t *testing.T) {
	var b timeoutReader
	reader := bufio.NewReader(&b)
	_, err := ReadTimeout(reader, 50*time.Millisecond)
	if err == nil {
		t.Fatalf("TestReadTimeoutV1Invalid: expected error %s", ErrNoProxyProtocol)
	} else if err != ErrNoProxyProtocol {
		t.Fatalf("TestReadTimeoutV1Invalid: expected %s, actual %s", ErrNoProxyProtocol, err)
	}
}

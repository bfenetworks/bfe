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

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bfe_tls

import (
	"crypto/md5"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
)

var tests = []interface{}{
	&clientHelloMsg{},
	&serverHelloMsg{},
	&finishedMsg{},

	&certificateMsg{},
	&certificateRequestMsg{},
	&certificateVerifyMsg{},
	&certificateStatusMsg{},
	&clientKeyExchangeMsg{},
	&nextProtoMsg{},
	&newSessionTicketMsg{},
	&sessionState{},
}

type testMessage interface {
	marshal() []byte
	unmarshal([]byte) bool
	equal(interface{}) bool
}

func TestMarshalUnmarshal(t *testing.T) {
	rand := rand.New(rand.NewSource(0))

	for i, iface := range tests {
		ty := reflect.ValueOf(iface).Type()

		n := 100
		if testing.Short() {
			n = 5
		}
		for j := 0; j < n; j++ {
			v, ok := quick.Value(ty, rand)
			if !ok {
				t.Errorf("#%d: failed to create value", i)
				break
			}

			m1 := v.Interface().(testMessage)
			marshaled := m1.marshal()
			m2 := iface.(testMessage)
			if !m2.unmarshal(marshaled) {
				t.Errorf("#%d failed to unmarshal %#v %x", i, m1, marshaled)
				break
			}
			m2.marshal() // to fill any marshal cache in the message

			if !m1.equal(m2) {
				t.Errorf("#%d got:%#v want:%#v %x", i, m2, m1, marshaled)
				break
			}

			if i >= 3 {
				// The first three message types (ClientHello,
				// ServerHello and Finished) are allowed to
				// have parsable prefixes because the extension
				// data is optional and the length of the
				// Finished varies across versions.
				for j := 0; j < len(marshaled); j++ {
					if m2.unmarshal(marshaled[0:j]) {
						t.Errorf("#%d unmarshaled a prefix of length %d of %#v", i, j, m1)
						break
					}
				}
			}
		}
	}
}

func TestFuzz(t *testing.T) {
	rand := rand.New(rand.NewSource(0))
	for _, iface := range tests {
		m := iface.(testMessage)

		for j := 0; j < 1000; j++ {
			len := rand.Intn(100)
			bytes := randomBytes(len, rand)
			// This just looks for crashes due to bounds errors etc.
			m.unmarshal(bytes)
		}
	}
}

func randomBytes(n int, rand *rand.Rand) []byte {
	r := make([]byte, n)
	for i := 0; i < n; i++ {
		r[i] = byte(rand.Int31())
	}
	return r
}

func randomString(n int, rand *rand.Rand) string {
	b := randomBytes(n, rand)
	return string(b)
}

func (*clientHelloMsg) Generate(rand *rand.Rand, size int) reflect.Value {
	m := &clientHelloMsg{}
	m.vers = uint16(rand.Intn(65536))
	m.random = randomBytes(32, rand)
	m.sessionId = randomBytes(rand.Intn(32), rand)
	m.cipherSuites = make([]uint16, rand.Intn(63)+1)
	for i := 0; i < len(m.cipherSuites); i++ {
		m.cipherSuites[i] = uint16(rand.Int31())
	}
	m.compressionMethods = randomBytes(rand.Intn(63)+1, rand)
	if rand.Intn(10) > 5 {
		m.nextProtoNeg = true
	}
	if rand.Intn(10) > 5 {
		m.serverName = randomString(rand.Intn(255), rand)
	}
	m.ocspStapling = rand.Intn(10) > 5
	m.supportedPoints = randomBytes(rand.Intn(5)+1, rand)
	m.supportedCurves = make([]CurveID, rand.Intn(5)+1)
	for i := range m.supportedCurves {
		m.supportedCurves[i] = CurveID(rand.Intn(30000))
	}
	if rand.Intn(10) > 5 {
		m.ticketSupported = true
		if rand.Intn(10) > 5 {
			m.sessionTicket = randomBytes(rand.Intn(300), rand)
		}
	}
	if rand.Intn(10) > 5 {
		m.signatureAndHashes = supportedSKXSignatureAlgorithms
	}
	m.alpnProtocols = make([]string, rand.Intn(5))
	for i := range m.alpnProtocols {
		m.alpnProtocols[i] = randomString(rand.Intn(20)+1, rand)
	}

	return reflect.ValueOf(m)
}

func (*serverHelloMsg) Generate(rand *rand.Rand, size int) reflect.Value {
	m := &serverHelloMsg{}
	m.vers = uint16(rand.Intn(65536))
	m.random = randomBytes(32, rand)
	m.sessionId = randomBytes(rand.Intn(32), rand)
	m.cipherSuite = uint16(rand.Int31())
	m.compressionMethod = uint8(rand.Intn(256))

	if rand.Intn(10) > 5 {
		m.nextProtoNeg = true

		n := rand.Intn(10)
		m.nextProtos = make([]string, n)
		for i := 0; i < n; i++ {
			m.nextProtos[i] = randomString(20, rand)
		}
	}

	if rand.Intn(10) > 5 {
		m.ocspStapling = true
	}
	if rand.Intn(10) > 5 {
		m.ticketSupported = true
	}
	m.alpnProtocol = randomString(rand.Intn(32)+1, rand)

	return reflect.ValueOf(m)
}

func (*certificateMsg) Generate(rand *rand.Rand, size int) reflect.Value {
	m := &certificateMsg{}
	numCerts := rand.Intn(20)
	m.certificates = make([][]byte, numCerts)
	for i := 0; i < numCerts; i++ {
		m.certificates[i] = randomBytes(rand.Intn(10)+1, rand)
	}
	return reflect.ValueOf(m)
}

func (*certificateRequestMsg) Generate(rand *rand.Rand, size int) reflect.Value {
	m := &certificateRequestMsg{}
	m.certificateTypes = randomBytes(rand.Intn(5)+1, rand)
	numCAs := rand.Intn(100)
	m.certificateAuthorities = make([][]byte, numCAs)
	for i := 0; i < numCAs; i++ {
		m.certificateAuthorities[i] = randomBytes(rand.Intn(15)+1, rand)
	}
	return reflect.ValueOf(m)
}

func (*certificateVerifyMsg) Generate(rand *rand.Rand, size int) reflect.Value {
	m := &certificateVerifyMsg{}
	m.signature = randomBytes(rand.Intn(15)+1, rand)
	return reflect.ValueOf(m)
}

func (*certificateStatusMsg) Generate(rand *rand.Rand, size int) reflect.Value {
	m := &certificateStatusMsg{}
	if rand.Intn(10) > 5 {
		m.statusType = statusTypeOCSP
		m.response = randomBytes(rand.Intn(10)+1, rand)
	} else {
		m.statusType = 42
	}
	return reflect.ValueOf(m)
}

func (*clientKeyExchangeMsg) Generate(rand *rand.Rand, size int) reflect.Value {
	m := &clientKeyExchangeMsg{}
	m.ciphertext = randomBytes(rand.Intn(1000)+1, rand)
	return reflect.ValueOf(m)
}

func (*finishedMsg) Generate(rand *rand.Rand, size int) reflect.Value {
	m := &finishedMsg{}
	m.verifyData = randomBytes(12, rand)
	return reflect.ValueOf(m)
}

func (*nextProtoMsg) Generate(rand *rand.Rand, size int) reflect.Value {
	m := &nextProtoMsg{}
	m.proto = randomString(rand.Intn(255), rand)
	return reflect.ValueOf(m)
}

func (*newSessionTicketMsg) Generate(rand *rand.Rand, size int) reflect.Value {
	m := &newSessionTicketMsg{}
	m.ticket = randomBytes(rand.Intn(4), rand)
	return reflect.ValueOf(m)
}

func (*sessionState) Generate(rand *rand.Rand, size int) reflect.Value {
	s := &sessionState{}
	s.vers = uint16(rand.Intn(10000))
	s.cipherSuite = uint16(rand.Intn(10000))
	s.masterSecret = randomBytes(rand.Intn(100), rand)
	numCerts := rand.Intn(20)
	s.certificates = make([][]byte, numCerts)
	for i := 0; i < numCerts; i++ {
		s.certificates[i] = randomBytes(rand.Intn(10)+1, rand)
	}
	return reflect.ValueOf(s)
}

var ja3HashTests = []struct {
	vers            uint16
	cipherSuites    []uint16
	extensionIds    []uint16
	supportedCurves []CurveID
	supportedPoints []uint8
	ja3Hash         string
}{
	{769, []uint16{47, 53, 5, 10, 49161, 49162, 49171, 49172, 50, 56, 19, 4},
		[]uint16{0, 10, 11}, []CurveID{23, 24, 25}, []uint8{0},
		"ada70206e40642a3e4461f35503241d5"},
	{769, []uint16{4, 5, 10, 9, 100, 98, 3, 6, 19, 18, 99},
		[]uint16{}, []CurveID{}, []uint8{},
		"de350869b8c85de67a350c8d186f11e6"},
	{771, []uint16{56026, 4865, 4866, 4867, 49195, 49199, 49196, 49200, 52393, 52392, 49171, 49172, 156, 157, 47, 53},
		[]uint16{60138, 0, 23, 65281, 10, 11, 35, 16, 5, 13, 18, 51, 45, 43, 27, 17513, 56026, 21},
		[]CurveID{10794, 29, 23, 24},
		[]uint8{0},
		"cd08e31494f9531f560d64c695473da9"},
	{771, []uint16{4865, 4866, 4867, 49195, 49199, 49196, 49200, 52393, 52392, 49171, 49172, 156, 157, 47, 53},
		[]uint16{0, 23, 65281, 10, 11, 35, 16, 5, 13, 18, 51, 45, 43, 27, 17513, 56026, 21},
		[]CurveID{29, 23, 24},
		[]uint8{0},
		"cd08e31494f9531f560d64c695473da9"},
}

func TestJA3Hash(t *testing.T) {
	for i, d := range ja3HashTests {
		msg := clientHelloMsg{}
		msg.vers = d.vers
		msg.cipherSuites = d.cipherSuites
		msg.extensionIds = d.extensionIds
		msg.supportedCurves = d.supportedCurves
		msg.supportedPoints = d.supportedPoints
		ja3Value := md5.Sum([]byte(msg.JA3String()))
		if d.ja3Hash != fmt.Sprintf("%x", ja3Value) {
			t.Errorf("#%d: unexpected ja3 value", i)
		}
	}
}

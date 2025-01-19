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

// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bfe_tls

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Note: see comment in handshake_test.go for details of how the reference
// tests work.

// blockingSource is an io.Reader that blocks a Read call until it's closed.
type blockingSource chan bool

func (b blockingSource) Read([]byte) (n int, err error) {
	<-b
	return 0, io.EOF
}

// clientTest represents a test of the TLS client handshake against a reference
// implementation.
type clientTest struct {
	// name is a freeform string identifying the test and the file in which
	// the expected results will be stored.
	name string
	// command, if not empty, contains a series of arguments for the
	// command to run for the reference server.
	command []string
	// config, if not nil, contains a custom Config to use for this test.
	config *Config
	// cert, if not empty, contains a DER-encoded certificate for the
	// reference server.
	cert []byte
	// key, if not nil, contains either a *rsa.PrivateKey or
	// *ecdsa.PrivateKey which is the private key for the reference server.
	key interface{}
}

var defaultServerCommand = []string{"openssl", "s_server"}

// connFromCommand starts the reference server process, connects to it and
// returns a recordingConn for the connection. The stdin return value is a
// blockingSource for the stdin of the child process. It must be closed before
// Waiting for child.
func (test *clientTest) connFromCommand() (conn *recordingConn, child *exec.Cmd, stdin blockingSource, err error) {
	cert := testRSACertificate
	if len(test.cert) > 0 {
		cert = test.cert
	}
	certPath := tempFile(string(cert))
	defer os.Remove(certPath)

	var key interface{} = testRSAPrivateKey
	if test.key != nil {
		key = test.key
	}
	var pemType string
	var derBytes []byte
	switch key := key.(type) {
	case *rsa.PrivateKey:
		pemType = "RSA"
		derBytes = x509.MarshalPKCS1PrivateKey(key)
	case *ecdsa.PrivateKey:
		pemType = "EC"
		var err error
		derBytes, err = x509.MarshalECPrivateKey(key)
		if err != nil {
			panic(err)
		}
	default:
		panic("unknown key type")
	}

	var pemOut bytes.Buffer
	pem.Encode(&pemOut, &pem.Block{Type: pemType + " PRIVATE KEY", Bytes: derBytes})

	keyPath := tempFile(pemOut.String())
	defer os.Remove(keyPath)

	var command []string
	if len(test.command) > 0 {
		command = append(command, test.command...)
	} else {
		command = append(command, defaultServerCommand...)
	}
	command = append(command, "-cert", certPath, "-certform", "DER", "-key", keyPath)
	// serverPort contains the port that OpenSSL will listen on. OpenSSL
	// can't take "0" as an argument here so we have to pick a number and
	// hope that it's not in use on the machine. Since this only occurs
	// when -update is given and thus when there's a human watching the
	// test, this isn't too bad.
	const serverPort = 24323
	command = append(command, "-accept", strconv.Itoa(serverPort))

	cmd := exec.Command(command[0], command[1:]...)
	stdin = blockingSource(make(chan bool))
	cmd.Stdin = stdin
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, err
	}

	// OpenSSL does print an "ACCEPT" banner, but it does so *before*
	// opening the listening socket, so we can't use that to wait until it
	// has started listening. Thus we are forced to poll until we get a
	// connection.
	var tcpConn net.Conn
	for i := uint(0); i < 5; i++ {
		var err error
		tcpConn, err = net.DialTCP("tcp", nil, &net.TCPAddr{
			IP:   net.IPv4(127, 0, 0, 1),
			Port: serverPort,
		})
		if err == nil {
			break
		}
		time.Sleep((1 << i) * 5 * time.Millisecond)
	}
	if tcpConn == nil {
		close(stdin)
		out.WriteTo(os.Stdout)
		cmd.Process.Kill()
		return nil, nil, nil, cmd.Wait()
	}

	record := &recordingConn{
		Conn: tcpConn,
	}

	return record, cmd, stdin, nil
}

func (test *clientTest) dataPath() string {
	return filepath.Join("testdata", "Client-"+test.name)
}

func (test *clientTest) loadData() (flows [][]byte, err error) {
	in, err := os.Open(test.dataPath())
	if err != nil {
		return nil, err
	}
	defer in.Close()
	return parseTestData(in)
}

func (test *clientTest) run(t *testing.T, write bool) {
	var clientConn, serverConn net.Conn
	var recordingConn *recordingConn
	var childProcess *exec.Cmd
	var stdin blockingSource

	if write {
		var err error
		recordingConn, childProcess, stdin, err = test.connFromCommand()
		if err != nil {
			t.Fatalf("Failed to start subcommand: %s", err)
		}
		clientConn = recordingConn
	} else {
		clientConn, serverConn = net.Pipe()
	}

	config := test.config
	if config == nil {
		config = testConfig
	}
	client := Client(clientConn, config)

	doneChan := make(chan bool)
	go func() {
		if _, err := client.Write([]byte("hello\n")); err != nil {
			t.Logf("Client.Write failed: %s", err)
		}
		client.Close()
		clientConn.Close()
		doneChan <- true
	}()

	if !write {
		flows, err := test.loadData()
		if err != nil {
			t.Fatalf("%s: failed to load data from %s", test.name, test.dataPath())
		}
		for i, b := range flows {
			if i%2 == 1 {
				serverConn.Write(b)
				continue
			}
			bb := make([]byte, len(b))
			_, err := io.ReadFull(serverConn, bb)
			if err != nil {
				t.Fatalf("%s #%d: %s", test.name, i, err)
			}
			if !bytes.Equal(b, bb) {
				t.Fatalf("%s #%d: mismatch on read: got:%x want:%x", test.name, i, bb, b)
			}
		}
		serverConn.Close()
	}

	<-doneChan

	if write {
		path := test.dataPath()
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			t.Fatalf("Failed to create output file: %s", err)
		}
		defer out.Close()
		recordingConn.Close()
		close(stdin)
		childProcess.Process.Kill()
		childProcess.Wait()
		if len(recordingConn.flows) < 3 {
			childProcess.Stdout.(*bytes.Buffer).WriteTo(os.Stdout)
			t.Fatalf("Client connection didn't work")
		}
		recordingConn.WriteTo(out)
		fmt.Printf("Wrote %s\n", path)
	}
}

func runClientTestForVersion(t *testing.T, template *clientTest, prefix, option string) {
	test := *template
	test.name = prefix + test.name
	if len(test.command) == 0 {
		test.command = defaultClientCommand
	}
	test.command = append([]string(nil), test.command...)
	test.command = append(test.command, option)
	test.run(t, *update)
}

func runClientTestTLS10(t *testing.T, template *clientTest) {
	runClientTestForVersion(t, template, "TLSv10-", "-tls1")
}

func runClientTestTLS11(t *testing.T, template *clientTest) {
	runClientTestForVersion(t, template, "TLSv11-", "-tls1_1")
}

func runClientTestTLS12(t *testing.T, template *clientTest) {
	runClientTestForVersion(t, template, "TLSv12-", "-tls1_2")
}

func prepareClientConfig() *Config {
	config := testConfig.Clone()

	// Note: for avoid modify existing data file under testdata dir,
	// client should only use required ciphersuites.
	config.CipherSuites = []uint16{
		TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		TLS_ECDHE_RSA_WITH_RC4_128_SHA,
		TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
		TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		TLS_RSA_WITH_RC4_128_SHA,
		TLS_RSA_WITH_AES_128_CBC_SHA,
		TLS_RSA_WITH_AES_256_CBC_SHA,
		TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
		TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	}
	return config
}

func TestHandshakeClientRSARC4(t *testing.T) {
	config := prepareClientConfig()
	test := &clientTest{
		name:    "RSA-RC4",
		command: []string{"openssl", "s_server", "-cipher", "RC4-SHA"},
		config:  config,
	}
	runClientTestTLS10(t, test)
	runClientTestTLS11(t, test)
	runClientTestTLS12(t, test)
}

func TestHandshakeClientECDHERSAAES(t *testing.T) {
	config := prepareClientConfig()
	test := &clientTest{
		name:    "ECDHE-RSA-AES",
		command: []string{"openssl", "s_server", "-cipher", "ECDHE-RSA-AES128-SHA"},
		config:  config,
	}
	runClientTestTLS10(t, test)
	runClientTestTLS11(t, test)
	runClientTestTLS12(t, test)
}

func TestHandshakeClientECDHEECDSAAES(t *testing.T) {
	config := prepareClientConfig()
	test := &clientTest{
		name:    "ECDHE-ECDSA-AES",
		command: []string{"openssl", "s_server", "-cipher", "ECDHE-ECDSA-AES128-SHA"},
		cert:    testECDSACertificate,
		key:     testECDSAPrivateKey,
		config:  config,
	}
	runClientTestTLS10(t, test)
	runClientTestTLS11(t, test)
	runClientTestTLS12(t, test)
}

func TestHandshakeClientECDHEECDSAAESGCM(t *testing.T) {
	config := prepareClientConfig()
	test := &clientTest{
		name:    "ECDHE-ECDSA-AES-GCM",
		command: []string{"openssl", "s_server", "-cipher", "ECDHE-ECDSA-AES128-GCM-SHA256"},
		cert:    testECDSACertificate,
		key:     testECDSAPrivateKey,
		config:  config,
	}
	runClientTestTLS12(t, test)
}

func prepareClientConfigForChacha20() *Config {
	// Note: for avoid modify existing data file under testdata dir,
	// client should only use required ciphersuites.
	ciphers := []uint16{
		TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	}

	config := prepareClientConfig()
	config.CipherSuites = append(ciphers, config.CipherSuites...)
	return config
}

func TestHandshakeClientECDHERSACHACHA20POLY1305(t *testing.T) {
	config := prepareClientConfigForChacha20()
	test := &clientTest{
		name:    "ECDHE-RSA-CHACHA20-POLY1305",
		command: []string{"openssl", "s_server", "-cipher", "ECDHE-RSA-CHACHA20-POLY1305"},
		config:  config,
	}
	runClientTestTLS12(t, test)
}

func TestHandshakeClientECDHEECDSACHACHA20POLY1305(t *testing.T) {
	config := prepareClientConfigForChacha20()
	test := &clientTest{
		name:    "ECDHE-ECDSA-CHACHA20-POLY1305",
		command: []string{"openssl", "s_server", "-cipher", "ECDHE-ECDSA-CHACHA20-POLY1305"},
		cert:    testECDSACertificate,
		key:     testECDSAPrivateKey,
		config:  config,
	}
	runClientTestTLS12(t, test)
}

func TestHandshakeClientCertRSA(t *testing.T) {
	config := prepareClientConfig()
	cert, _ := X509KeyPair([]byte(clientCertificatePEM), []byte(clientKeyPEM))
	config.Certificates = []Certificate{cert}

	test := &clientTest{
		name:    "ClientCert-RSA-RSA",
		command: []string{"openssl", "s_server", "-cipher", "RC4-SHA", "-verify", "1"},
		config:  config,
	}

	runClientTestTLS10(t, test)
	runClientTestTLS12(t, test)

	test = &clientTest{
		name:    "ClientCert-RSA-ECDSA",
		command: []string{"openssl", "s_server", "-cipher", "ECDHE-ECDSA-AES128-SHA", "-verify", "1"},
		config:  config,
		cert:    testECDSACertificate,
		key:     testECDSAPrivateKey,
	}

	runClientTestTLS10(t, test)
	runClientTestTLS12(t, test)
}

func TestClientResumption(t *testing.T) {
	serverConfig := &Config{
		CipherSuites: []uint16{TLS_RSA_WITH_RC4_128_SHA, TLS_ECDHE_RSA_WITH_RC4_128_SHA},
		Certificates: testConfig.Certificates,
	}
	clientConfig := &Config{
		CipherSuites:       []uint16{TLS_RSA_WITH_RC4_128_SHA},
		InsecureSkipVerify: true,
		ClientSessionCache: NewLRUClientSessionCache(32),
	}

	testResumeState := func(test string, didResume bool) {
		hs, err := testHandshake(clientConfig, serverConfig)
		if err != nil {
			t.Fatalf("%s: handshake failed: %s", test, err)
		}
		if hs.DidResume != didResume {
			t.Fatalf("%s resumed: %v, expected: %v", test, hs.DidResume, didResume)
		}
	}

	testResumeState("Handshake", false)
	testResumeState("Resume", true)

	if _, err := io.ReadFull(serverConfig.rand(), serverConfig.SessionTicketKey[:]); err != nil {
		t.Fatalf("Failed to invalidate SessionTicketKey")
	}
	testResumeState("InvalidSessionTicketKey", false)
	testResumeState("ResumeAfterInvalidSessionTicketKey", true)

	clientConfig.CipherSuites = []uint16{TLS_ECDHE_RSA_WITH_RC4_128_SHA}
	testResumeState("DifferentCipherSuite", false)
	testResumeState("DifferentCipherSuiteRecovers", true)

	clientConfig.ClientSessionCache = nil
	testResumeState("WithoutSessionCache", false)
}

func TestLRUClientSessionCache(t *testing.T) {
	// Initialize cache of capacity 4.
	cache := NewLRUClientSessionCache(4)
	cs := make([]ClientSessionState, 6)
	keys := []string{"0", "1", "2", "3", "4", "5", "6"}

	// Add 4 entries to the cache and look them up.
	for i := 0; i < 4; i++ {
		cache.Put(keys[i], &cs[i])
	}
	for i := 0; i < 4; i++ {
		if s, ok := cache.Get(keys[i]); !ok || s != &cs[i] {
			t.Fatalf("session cache failed lookup for added key: %s", keys[i])
		}
	}

	// Add 2 more entries to the cache. First 2 should be evicted.
	for i := 4; i < 6; i++ {
		cache.Put(keys[i], &cs[i])
	}
	for i := 0; i < 2; i++ {
		if s, ok := cache.Get(keys[i]); ok || s != nil {
			t.Fatalf("session cache should have evicted key: %s", keys[i])
		}
	}

	// Touch entry 2. LRU should evict 3 next.
	cache.Get(keys[2])
	cache.Put(keys[0], &cs[0])
	if s, ok := cache.Get(keys[3]); ok || s != nil {
		t.Fatalf("session cache should have evicted key 3")
	}

	// Update entry 0 in place.
	cache.Put(keys[0], &cs[3])
	if s, ok := cache.Get(keys[0]); !ok || s != &cs[3] {
		t.Fatalf("session cache failed update for key 0")
	}

	// Adding a nil entry is valid.
	cache.Put(keys[0], nil)
	if s, ok := cache.Get(keys[0]); !ok || s != nil {
		t.Fatalf("failed to add nil entry to cache")
	}
}

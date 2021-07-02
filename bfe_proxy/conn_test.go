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

package bfe_proxy

import (
	"bytes"
	"crypto/rand"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

import (
	bufio "github.com/bfenetworks/bfe/bfe_bufio"
	"github.com/bfenetworks/bfe/bfe_util"
)

func TestProxyHeaderNormal(t *testing.T) {
	br := newBufioReader([]byte("PROXY TCP4 " + TCP4AddressesAndPorts + CRLF + "hello"))
	testProxyConnRead(t, br, 0, 0, "")
}

func TestProxyHeaderNoProxyProtocol(t *testing.T) {
	br := newBufioReader([]byte("hello"))
	testProxyConnRead(t, br, 0, 0, "")
}

func TestProxyHeaderExceedLimit(t *testing.T) {
	hdr := "PROXY TCP4 " + TCP4AddressesAndPorts + CRLF
	br := newBufioReader([]byte(hdr + "hello"))
	testProxyConnRead(t, br, 0, int64(len(hdr)-1), "EOF")
}

func TestProxyHeaderResetLimit(t *testing.T) {
	hdr := "PROXY TCP4 " + TCP4AddressesAndPorts + CRLF
	br := newBufioReader([]byte(hdr + "hello"))
	testProxyConnRead(t, br, 0, int64(len(hdr)), "")
}

func TestProxyHeaderTimeout(t *testing.T) {
	br := bufio.NewReader(new(timeoutReader))
	testProxyConnRead(t, br, 50*time.Millisecond, 0, "timeout")
}

func TestProxyConnAddrIPv4(t *testing.T) {
	br := newBufioReader([]byte("PROXY TCP4 1.1.1.1 2.2.2.2 12345 80" + CRLF))
	caddr := parseTCPAddr("tcp", "1.1.1.1:12345")
	vaddr := parseTCPAddr("tcp", "2.2.2.2:80")
	testProxyConnAddr(t, br, caddr, vaddr, true)
}

func TestProxyConnAddrIPv6(t *testing.T) {
	br := newBufioReader([]byte("PROXY TCP6 2001::68 2002::68 12345 80" + CRLF))
	caddr := parseTCPAddr("tcp6", "[2001::68]:12345")
	vaddr := parseTCPAddr("tcp6", "[2002::68]:80")
	testProxyConnAddr(t, br, caddr, vaddr, true)
}

func TestProxyConnAddrNoProtocol(t *testing.T) {
	br := newBufioReader([]byte("GET / HTTP1.1" + CRLF))
	testProxyConnAddr(t, br, nil, nil, false)
}

func TestProxyConnAddrInvalid(t *testing.T) {
	br := newBufioReader([]byte("PROXY TCP 2001::68 2002::68 12345 80" + CRLF))
	testProxyConnAddr(t, br, nil, nil, false)
}

func TestProxyConnReadNormal(t *testing.T) {
	hdr := []byte("PROXY TCP4 " + TCP4AddressesAndPorts + CRLF)
	msg := readRandBytes(16 * 1024)
	br := newBufioReader(append(hdr, msg...))

	testProxyConnOperation(t, br, func(c *Conn) {
		buf := make([]byte, 16*1024)
		_, err := io.ReadFull(c, buf)
		checkError(t, "", err)
		if !bytes.Equal(msg, buf) {
			t.Errorf("Read want %v: \ngot %v", msg, buf)
		}
	})
}

func TestProxyConnReadAfterClose(t *testing.T) {
	hdr := []byte("PROXY TCP4 " + TCP4AddressesAndPorts + CRLF)
	br := newBufioReader(append(hdr, readRandBytes(16*1024)...))

	testProxyConnOperation(t, br, func(c *Conn) {
		c.RemoteAddr()
		c.Close()
		_, err := io.ReadFull(c, make([]byte, 16*1024))
		checkError(t, "closed", err)
	})
}

func TestProxyConnSetWriteDeadline(t *testing.T) {
	br := newBufioReader([]byte("PROXY TCP4 " + TCP4AddressesAndPorts + CRLF + "hello"))
	testProxyConnOperation(t, br, func(c *Conn) {
		c.SetWriteDeadline(time.Now().Add(10 * time.Millisecond))
		time.Sleep(50 * time.Millisecond)
		_, err := c.Write([]byte("test"))
		checkError(t, "timeout", err)
	})
}

func TestProxyConnSetReadDeadline(t *testing.T) {
	hdr := []byte("PROXY TCP4 " + TCP4AddressesAndPorts + CRLF)
	br := newBufioReader(append(hdr, readRandBytes(16*1024)...))

	testProxyConnOperation(t, br, func(c *Conn) {
		c.RemoteAddr()
		c.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
		time.Sleep(50 * time.Millisecond)
		_, err := io.ReadFull(c, make([]byte, 16*1024))
		checkError(t, "timeout", err)
	})
}

type CheckHandler func()

func testMockServer(t *testing.T, hs bfe_util.MockHandler, hc bfe_util.MockHandler, check CheckHandler) {
	// init mock server
	ms := bfe_util.NewUnstartedServer(hs)
	ms.StartTCP()
	defer ms.Close()

	// init mock client
	cconn, err := net.Dial("tcp", ms.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer cconn.Close()

	// test and check
	go hc(cconn)
	check()
}

func testProxyConnRead(t *testing.T, br *bufio.Reader, timeout time.Duration, limit int64, e string) {
	done := make(chan error)

	testMockServer(t, func(c net.Conn) {
		// create proxy conn
		pconn := NewConn(c, timeout, limit)
		defer pconn.Close()

		// read from proxy conn
		_, err := pconn.Read(make([]byte, 1))

		// send error for check
		done <- err
	}, func(c net.Conn) {
		// send data to server
		io.Copy(c, br)
	}, func() {
		// check read result in server side
		err := <-done
		checkError(t, e, err)
	})
}

func testProxyConnAddr(t *testing.T, br *bufio.Reader, caddr, vaddr net.Addr, success bool) {
	cliDone := make(chan net.Conn)
	srvDone := make(chan net.Conn)

	testMockServer(t, func(c net.Conn) {
		pconn := NewConn(c, 0, 0)
		srvDone <- pconn
	}, func(c net.Conn) {
		io.Copy(c, br)
		cliDone <- c
	}, func() {
		srvConn := <-srvDone // conn for bfe
		cliConn := <-cliDone // conn for proxy
		sc := srvConn.(*Conn)

		// check address
		addrGot := []net.Addr{sc.RemoteAddr(), sc.VirtualAddr(), sc.BalancerAddr(), sc.LocalAddr()}
		addrWant := []net.Addr{caddr, vaddr, cliConn.LocalAddr(), cliConn.RemoteAddr()}
		if !success {
			addrWant = []net.Addr{cliConn.LocalAddr(), nil, nil, cliConn.RemoteAddr()}
		}

		for i, addr := range addrGot {
			if !checkAddrEqual(addr, addrWant[i]) {
				t.Fatalf("addr [%d] want %v, got %v", i, addrWant[i], addr)
			}
		}
	})
}

func testProxyConnOperation(t *testing.T, br *bufio.Reader, f func(*Conn)) {
	done := make(chan bool, 1)

	testMockServer(t, func(c net.Conn) {
		pconn := NewConn(c, 0, 0)
		defer pconn.Close()
		f(pconn)
		done <- true
	}, func(c net.Conn) {
		io.Copy(c, br)
	}, func() {
		<-done
	})
}

func parseTCPAddr(network string, address string) net.Addr {
	addr, _ := net.ResolveTCPAddr(network, address)
	return addr
}

func readRandBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func checkAddrEqual(a, b net.Addr) bool {
	if a == nil && b == nil {
		return true
	}
	if a != nil && b != nil && a.String() == b.String() {
		return true
	}
	return false
}

func checkError(t *testing.T, errWant string, errGot error) {
	if len(errWant) == 0 && errGot != nil {
		t.Errorf("Unexpected error: %v", errGot)
		return
	}
	if len(errWant) > 0 && errGot == nil {
		t.Errorf("Expect error: %s:", errWant)
		return
	}
	if len(errWant) > 0 && errGot != nil {
		if !strings.Contains(errGot.Error(), errWant) {
			t.Errorf("Expecting error got %v ; want %v", errGot, errWant)
			return
		}
	}
}

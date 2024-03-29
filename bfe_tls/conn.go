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

// TLS low level connection and record layer

package bfe_tls

import (
	"bytes"
	"crypto/cipher"
	"crypto/subtle"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type ConnParam interface {
	// Get virtual ip for current conn
	GetVip() net.IP
}

// A Conn represents a secured connection.
// It implements the net.Conn interface.
type Conn struct {
	// constant
	conn     net.Conn
	isClient bool

	// constant after initial
	param ConnParam

	// constant after handshake; protected by handshakeMutex
	handshakeMutex    sync.Mutex    // handshakeMutex < in.Mutex, out.Mutex, errMutex
	handshakeErr      error         // error resulting from handshake
	vers              uint16        // TLS version
	haveVers          bool          // version has been negotiated
	config            *Config       // configuration passed to constructor
	handshakeTime     time.Duration // TLS handshake time
	handshakeComplete bool
	didResume         bool // whether this connection was a session resumption
	cipherSuite       uint16
	ocspStaple        bool   // use ocsp stapling (in server side)
	ocspResponse      []byte // stapled OCSP response
	peerCertificates  []*x509.Certificate
	// verifiedChains contains the certificate chains that we built, as
	// opposed to the ones presented by the server.
	verifiedChains [][]*x509.Certificate
	// serverName contains the server name indicated by the client, if any.
	serverName          string
	grade               string         // tls security grade, usually is "A", "B", "C"
	clientAuth          ClientAuthType // tls client auth type
	clientCAs           *x509.CertPool
	clientCAName        string   // tls client CA name
	clientCRLPool       *CRLPool // tls client CRL pool
	enableDynamicRecord bool     // enable dynamic record size or not
	clientRandom        []byte   // random in client hello msg
	serverRandom        []byte   // random in server hello msg
	masterSecret        []byte   // master secret for conn
	clientCiphers       []uint16 // ciphers supported by client
	ja3Raw              string   // JA3 fingerprint string for TLS Client
	ja3Hash             string   // JA3 fingerprint hash for TLS Client

	clientProtocol         string
	clientProtocolFallback bool

	// input/output
	in, out  halfConn     // in.Mutex < out.Mutex
	rawInput *block       // raw input, right off the wire
	input    *block       // application data waiting to be read
	hand     bytes.Buffer // handshake data waiting to be read

	// total bytes of application data sent recently
	// byteOut will be reset to zero after inactivity
	byteOut int

	//readFromUntilLen len
	readFromUntilLen int

	// finish time of last writeRecord()
	lastOut time.Time

	tmp [16]byte

	// for sslv2 client hello
	sslv2Data []byte
}

// Access to net.Conn methods.
// Cannot just embed net.Conn because that would
// export the struct field too.

// LocalAddr returns the local network address.
func (c *Conn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr returns the remote network address.
func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// GetNetConn returns the underlying connection.
func (c *Conn) GetNetConn() net.Conn {
	return c.conn
}

// GetVip return the vip for underlying connection.
func (c *Conn) GetVip() net.IP {
	if c.param == nil {
		return nil
	}
	return c.param.GetVip()
}

// GetServerName returns server name indicated by the client, if any.
func (c *Conn) GetServerName() string {
	return c.serverName
}

func (c *Conn) SetConnParam(param ConnParam) {
	c.param = param
}

// SetDeadline sets the read and write deadlines associated with the connection.
// A zero value for t means Read and Write will not time out.
// After a Write has timed out, the TLS state is corrupt and all future writes will return the same error.
func (c *Conn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

// SetReadDeadline sets the read deadline on the underlying connection.
// A zero value for t means Read will not time out.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline on the underlying connection.
// A zero value for t means Write will not time out.
// After a Write has timed out, the TLS state is corrupt and all future writes will return the same error.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func (c *Conn) getClientCAs() *x509.CertPool {
	clientCAs := c.config.ClientCAs
	if c.clientCAs != nil {
		clientCAs = c.clientCAs
	}

	return clientCAs
}

// A halfConn represents one direction of the record layer
// connection, either sending or receiving.
type halfConn struct {
	sync.Mutex

	err     error       // first permanent error
	version uint16      // protocol version
	cipher  interface{} // cipher algorithm
	mac     macFunction
	seq     [8]byte // 64-bit sequence number
	bfree   *block  // list of free blocks

	nextCipher interface{} // next encryption state
	nextMac    macFunction // next MAC algorithm

	// used to save allocating a new buffer for each MAC.
	inDigestBuf, outDigestBuf []byte
}

func (hc *halfConn) setErrorLocked(err error) error {
	hc.err = err
	return err
}

func (hc *halfConn) error() error {
	hc.Lock()
	err := hc.err
	hc.Unlock()
	return err
}

// prepareCipherSpec sets the encryption and MAC states
// that a subsequent changeCipherSpec will use.
func (hc *halfConn) prepareCipherSpec(version uint16, cipher interface{}, mac macFunction) {
	hc.version = version
	hc.nextCipher = cipher
	hc.nextMac = mac
}

// changeCipherSpec changes the encryption and MAC states
// to the ones previously passed to prepareCipherSpec.
func (hc *halfConn) changeCipherSpec() error {
	if hc.nextCipher == nil {
		return alertInternalError
	}
	hc.cipher = hc.nextCipher
	hc.mac = hc.nextMac
	hc.nextCipher = nil
	hc.nextMac = nil
	for i := range hc.seq {
		hc.seq[i] = 0
	}
	return nil
}

// incSeq increments the sequence number.
func (hc *halfConn) incSeq() {
	for i := 7; i >= 0; i-- {
		hc.seq[i]++
		if hc.seq[i] != 0 {
			return
		}
	}

	// Not allowed to let sequence number wrap.
	// Instead, must renegotiate before it does.
	// Not likely enough to bother.
	panic("TLS: sequence number wraparound")
}

// resetSeq resets the sequence number to zero.
func (hc *halfConn) resetSeq() {
	for i := range hc.seq {
		hc.seq[i] = 0
	}
}

// removePadding returns an unpadded slice, in constant time, which is a prefix
// of the input. It also returns a byte which is equal to 255 if the padding
// was valid and 0 otherwise. See RFC 2246, section 6.2.3.2
func removePadding(payload []byte) ([]byte, byte) {
	if len(payload) < 1 {
		return payload, 0
	}

	paddingLen := payload[len(payload)-1]
	t := uint(len(payload)-1) - uint(paddingLen)
	// if len(payload) >= (paddingLen - 1) then the MSB of t is zero
	good := byte(int32(^t) >> 31)

	toCheck := 255 // the maximum possible padding length
	// The length of the padded data is public, so we can use an if here
	if toCheck+1 > len(payload) {
		toCheck = len(payload) - 1
	}

	for i := 0; i < toCheck; i++ {
		t := uint(paddingLen) - uint(i)
		// if i <= paddingLen then the MSB of t is zero
		mask := byte(int32(^t) >> 31)
		b := payload[len(payload)-1-i]
		good &^= mask&paddingLen ^ mask&b
	}

	// We AND together the bits of good and replicate the result across
	// all the bits.
	good &= good << 4
	good &= good << 2
	good &= good << 1
	good = uint8(int8(good) >> 7)

	toRemove := good&paddingLen + 1
	return payload[:len(payload)-int(toRemove)], good
}

// removePaddingSSL30 is a replacement for removePadding in the case that the
// protocol version is SSLv3. In this version, the contents of the padding
// are random and cannot be checked.
func removePaddingSSL30(payload []byte) ([]byte, byte) {
	if len(payload) < 1 {
		return payload, 0
	}

	paddingLen := int(payload[len(payload)-1]) + 1
	if paddingLen > len(payload) {
		return payload, 0
	}

	return payload[:len(payload)-paddingLen], 255
}

func roundUp(a, b int) int {
	return a + (b-a%b)%b
}

// cbcMode is an interface for block ciphers using cipher block chaining.
type cbcMode interface {
	cipher.BlockMode
	SetIV([]byte)
}

// decrypt checks and strips the mac and decrypts the data in b. Returns a
// success boolean, the number of bytes to skip from the start of the record in
// order to get the application payload, and an optional alert value.
func (hc *halfConn) decrypt(b *block) (ok bool, prefixLen int, alertValue alert) {
	// pull out payload
	payload := b.data[recordHeaderLen:]

	macSize := 0
	if hc.mac != nil {
		macSize = hc.mac.Size()
	}

	paddingGood := byte(255)
	explicitIVLen := 0

	// decrypt
	if hc.cipher != nil {
		switch c := hc.cipher.(type) {
		case cipher.Stream:
			c.XORKeyStream(payload, payload)
		case aead:
			explicitIVLen = c.explicitNonceLen()
			if len(payload) < explicitIVLen {
				return false, 0, alertBadRecordMAC
			}
			nonce := payload[:explicitIVLen]
			payload = payload[explicitIVLen:]
			if len(nonce) == 0 {
				nonce = hc.seq[:]
			}

			var additionalData [13]byte
			copy(additionalData[:], hc.seq[:])
			copy(additionalData[8:], b.data[:3])
			n := len(payload) - c.Overhead()
			additionalData[11] = byte(n >> 8)
			additionalData[12] = byte(n)
			var err error
			payload, err = c.Open(payload[:0], nonce, payload, additionalData[:])
			if err != nil {
				return false, 0, alertBadRecordMAC
			}
			b.resize(recordHeaderLen + explicitIVLen + len(payload))
		case cbcMode:
			blockSize := c.BlockSize()
			if hc.version >= VersionTLS11 {
				explicitIVLen = blockSize
			}

			if len(payload)%blockSize != 0 || len(payload) < roundUp(explicitIVLen+macSize+1, blockSize) {
				return false, 0, alertBadRecordMAC
			}

			if explicitIVLen > 0 {
				c.SetIV(payload[:explicitIVLen])
				payload = payload[explicitIVLen:]
			}
			c.CryptBlocks(payload, payload)
			if hc.version == VersionSSL30 {
				payload, paddingGood = removePaddingSSL30(payload)
			} else {
				payload, paddingGood = removePadding(payload)
			}
			b.resize(recordHeaderLen + explicitIVLen + len(payload))

			// note that we still have a timing side-channel in the
			// MAC check, below. An attacker can align the record
			// so that a correct padding will cause one less hash
			// block to be calculated. Then they can iteratively
			// decrypt a record by breaking each byte. See
			// "Password Interception in a SSL/TLS Channel", Brice
			// Canvel et al.
			//
			// However, our behavior matches OpenSSL, so we leak
			// only as much as they do.
		default:
			panic("unknown cipher type")
		}
	}

	// check, strip mac
	if hc.mac != nil {
		if len(payload) < macSize {
			return false, 0, alertBadRecordMAC
		}

		// strip mac off payload, b.data
		n := len(payload) - macSize
		b.data[3] = byte(n >> 8)
		b.data[4] = byte(n)
		b.resize(recordHeaderLen + explicitIVLen + n)
		remoteMAC := payload[n:]
		localMAC := hc.mac.MAC(hc.inDigestBuf, hc.seq[0:], b.data[:recordHeaderLen], payload[:n])

		if subtle.ConstantTimeCompare(localMAC, remoteMAC) != 1 || paddingGood != 255 {
			return false, 0, alertBadRecordMAC
		}
		hc.inDigestBuf = localMAC
	}
	hc.incSeq()

	return true, recordHeaderLen + explicitIVLen, 0
}

// padToBlockSize calculates the needed padding block, if any, for a payload.
// On exit, prefix aliases payload and extends to the end of the last full
// block of payload. finalBlock is a fresh slice which contains the contents of
// any suffix of payload as well as the needed padding to make finalBlock a
// full block.
func padToBlockSize(payload []byte, blockSize int) (prefix, finalBlock []byte) {
	overrun := len(payload) % blockSize
	paddingLen := blockSize - overrun
	prefix = payload[:len(payload)-overrun]
	finalBlock = make([]byte, blockSize)
	copy(finalBlock, payload[len(payload)-overrun:])
	for i := overrun; i < blockSize; i++ {
		finalBlock[i] = byte(paddingLen - 1)
	}
	return
}

// encrypt encrypts and macs the data in b.
func (hc *halfConn) encrypt(b *block, explicitIVLen int) (bool, alert) {
	// mac
	if hc.mac != nil {
		mac := hc.mac.MAC(hc.outDigestBuf, hc.seq[0:], b.data[:recordHeaderLen], b.data[recordHeaderLen+explicitIVLen:])

		n := len(b.data)
		b.resize(n + len(mac))
		copy(b.data[n:], mac)
		hc.outDigestBuf = mac
	}

	payload := b.data[recordHeaderLen:]

	// encrypt
	if hc.cipher != nil {
		switch c := hc.cipher.(type) {
		case cipher.Stream:
			c.XORKeyStream(payload, payload)
		case aead:
			payloadLen := len(b.data) - recordHeaderLen - explicitIVLen
			b.resize(len(b.data) + c.Overhead())
			nonce := b.data[recordHeaderLen : recordHeaderLen+explicitIVLen]
			if len(nonce) == 0 {
				nonce = hc.seq[:]
			}
			payload := b.data[recordHeaderLen+explicitIVLen:]
			payload = payload[:payloadLen]

			var additionalData [13]byte
			copy(additionalData[:], hc.seq[:])
			copy(additionalData[8:], b.data[:3])
			additionalData[11] = byte(payloadLen >> 8)
			additionalData[12] = byte(payloadLen)

			c.Seal(payload[:0], nonce, payload, additionalData[:])
		case cbcMode:
			blockSize := c.BlockSize()
			if explicitIVLen > 0 {
				c.SetIV(payload[:explicitIVLen])
				payload = payload[explicitIVLen:]
			}
			prefix, finalBlock := padToBlockSize(payload, blockSize)
			b.resize(recordHeaderLen + explicitIVLen + len(prefix) + len(finalBlock))
			c.CryptBlocks(b.data[recordHeaderLen+explicitIVLen:], prefix)
			c.CryptBlocks(b.data[recordHeaderLen+explicitIVLen+len(prefix):], finalBlock)
		default:
			panic("unknown cipher type")
		}
	}

	// update length to include MAC and any block padding needed.
	n := len(b.data) - recordHeaderLen
	b.data[3] = byte(n >> 8)
	b.data[4] = byte(n)
	hc.incSeq()

	return true, 0
}

// A block is a simple data buffer.
type block struct {
	data []byte
	off  int // index for Read
	link *block
}

// resize resizes block to be n bytes, growing if necessary.
func (b *block) resize(n int) {
	if n > cap(b.data) {
		b.reserve(n)
	}
	b.data = b.data[0:n]
}

// reserve makes sure that block contains a capacity of at least n bytes.
func (b *block) reserve(n int) {
	if cap(b.data) >= n {
		return
	}
	m := cap(b.data)
	if m == 0 {
		m = 1024
	}
	for m < n {
		m *= 2
	}
	data := make([]byte, len(b.data), m)
	copy(data, b.data)
	b.data = data
}

// readFromUntil reads from r into b until b contains at least n bytes
// or else returns an error.
func (b *block) readFromUntil(c *Conn, n int) error {
	r := c.conn
	// quick case
	if len(b.data) >= n {
		return nil
	}

	// read until have enough.
	b.reserve(n)
	for {
		m, err := r.Read(b.data[len(b.data):cap(b.data)])
		b.data = b.data[0 : len(b.data)+m]
		c.readFromUntilLen += m
		if len(b.data) >= n {
			// TODO(bradfitz,agl): slightly suspicious
			// that we're throwing away r.Read's err here.
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *block) Read(p []byte) (n int, err error) {
	n = copy(p, b.data[b.off:])
	b.off += n
	return
}

// newBlock allocates a new block, from hc's free list if possible.
func (hc *halfConn) newBlock() *block {
	b := hc.bfree
	if b == nil {
		return new(block)
	}
	hc.bfree = b.link
	b.link = nil
	b.resize(0)
	return b
}

// freeBlock returns a block to hc's free list.
// The protocol is such that each side only has a block or two on
// its free list at a time, so there's no need to worry about
// trimming the list, etc.
func (hc *halfConn) freeBlock(b *block) {
	b.link = hc.bfree
	hc.bfree = b
}

// splitBlock splits a block after the first n bytes,
// returning a block with those n bytes and a
// block with the remainder.  the latter may be nil.
func (hc *halfConn) splitBlock(b *block, n int) (*block, *block) {
	if len(b.data) <= n {
		return b, nil
	}
	bb := hc.newBlock()
	bb.resize(len(b.data) - n)
	copy(bb.data, b.data[n:])
	b.data = b.data[0:n]
	return b, bb
}

/* convertSSLv2ClientHello - convert SSLv2 ClientHello to TLS ClientHello
 *
 * Note:
 * 1. TLS clients that wish to support SSLv2 servers must send
 *    SSLv2 CLIENT-HELLO messages.
 * 2. Even TLS servers that do not support SSLv2 may accept SSLv2
 *    CLIENT-HELLO messages.
 * 3. For negotiation purposes, SSLv2 CLIENT-HELLO is interpreted
 *    the same way as a TLS ClientHello
 *
 * For more details, see:
 * - tls1.2 https://tools.ietf.org/html/rfc5246#appendix-E
 * - tls1.1 https://tools.ietf.org/html/rfc4346#appendix-E
 * - tls1.0 https://tools.ietf.org/html/rfc2246#appendix-E
 * - sslv3  https://tools.ietf.org/html/draft-ietf-tls-ssl-version3-00
 * - sslv2  http://www-archive.mozilla.org/projects/security/pki/nss/ssl/draft02.html
 *
 * SSLv2 compatible CLIENT-HEELO message
 *
 *   0                                 16 bit
 *   +----------------------------------+
 *   |              MsgLength           |   (Highest bit MUST be 1)
 *   +----------------+-----------------+
 *   |    MsgType     |                     (Must be 1, ClientHello)
 *   +----------------+-----------------+
 *   |  MajorVersion  |  MinorVersion   |
 *   +----------------+-----------------+
 *   |           CipherSpecLen          |   (Must be multiply of 3)
 *   +----------------+-----------------+
 *   |           SessionIdLen           |   (Must be zero or 16 bytes for tls1.0 client,
 *   +----------------+-----------------+    Must be zero for tls1.1/tls1.2 client)
 *   |           ChallengeLen           |
 *   +----------------+-----------------+
 *   |            CipherSpec            |
 *   .       (CipherSpecLen bytes)      .
 *   .                                  .
 *   +----------------+-----------------+
 *   |            Session Id            |
 *   .       (SessionIdLen bytes)       .
 *   .                                  .
 *   +----------------------------------+
 *   |            Challenge             |   (Client Hello random)
 *   .       (ChallengeLen bytes)       .
 *   .                                  .
 *   +----------------------------------+
 */
func convertSSLv2ClientHello(c *Conn, b *block) error {
	// high bit must be 1 for SSLv2 compatible client hello
	if (uint8(b.data[0]) & 128) != 128 {
		return c.sendAlert(alertUnexpectedMessage)
	}

	msgLength := (uint16(b.data[0]&0x7f) << 8) | uint16(b.data[1])
	if msgLength < 12 {
		return c.sendAlert(alertHandshakeFailure)
	}

	// check if this is an SSLv2 client-hello but TLS is supported
	msgType := uint8(b.data[2])
	majorVer := uint8(b.data[3])
	minorVer := uint8(b.data[4])
	version := uint16(majorVer)<<8 | uint16(minorVer)
	if !(msgType == typeClientHello && version >= VersionSSL30) {
		c.sendAlert(alertProtocolVersion)
		state.TlsHandshakeSslv2NotSupport.Inc(1)
		return c.in.setErrorLocked(errors.New("tls: unsupported SSLv2 handshake received"))
	}
	state.TlsHandshakeAcceptSslv2ClientHello.Inc(1)

	// read the rest of the bytes for client hello
	if err := b.readFromUntil(c, int(2+msgLength)); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		if e, ok := err.(net.Error); !ok || !e.Temporary() {
			c.in.setErrorLocked(err)
		}
		return err
	}

	// get the cipher spec length
	cipherSpecLength := uint16(b.data[5])<<8 | uint16(b.data[6])
	if cipherSpecLength <= 0 || (cipherSpecLength%3) != 0 {
		return c.sendAlert(alertHandshakeFailure)
	}

	// get the session id length
	sessionIdLength := uint16(b.data[7])<<8 | uint16(b.data[8])
	if sessionIdLength != 0 && sessionIdLength != 16 {
		return c.sendAlert(alertHandshakeFailure)
	}

	if len(b.data) < 11+int(cipherSpecLength+sessionIdLength) {
		return c.sendAlert(alertHandshakeFailure)
	}

	// read the cipher specs
	cipherSpecs := b.data[11 : 11+cipherSpecLength]

	// read the rest of the data
	challengeData := b.data[11+cipherSpecLength+sessionIdLength:]

	// mark this data as read?
	b, c.rawInput = c.in.splitBlock(b, int(2+msgLength))
	b.off = 2

	// create tls clientHello message
	helloMsg := clientHelloMsg{}
	helloMsg.vers = version
	helloMsg.sessionId = []byte{0}
	helloMsg.compressionMethods = []uint8{compressionNone}

	if len(challengeData) >= 32 {
		helloMsg.random = challengeData[:32]
	} else {
		helloMsg.random = make([]byte, 32-len(challengeData))
		helloMsg.random = append(helloMsg.random, challengeData...)
	}

	helloMsg.cipherSuites = make([]uint16, 0)
	for i := 0; i < len(cipherSpecs); i += 3 {
		// we can only support cipher specs starting with a high bit
		if cipherSpecs[i] == 0 {
			cipher := uint16(cipherSpecs[i+1])<<8 | uint16(cipherSpecs[i+2])
			helloMsg.cipherSuites = append(helloMsg.cipherSuites, cipher)
		}
	}

	// write clientHello message to the handshake buffer
	c.hand.Write(helloMsg.marshal())

	c.sslv2Data = b.data[2:]
	c.in.freeBlock(b)
	return nil
}

// readRecord reads the next TLS record from the connection
// and updates the record layer state.
// c.in.Mutex <= L; c.input == nil.
func (c *Conn) readRecord(want recordType) error {
	// Caller must be in sync with connection:
	// handshake data if handshake not yet completed,
	// else application data.  (We don't support renegotiation.)
	switch want {
	default:
		c.sendAlert(alertInternalError)
		return c.in.setErrorLocked(errors.New("tls: unknown record type requested"))
	case recordTypeHandshake, recordTypeChangeCipherSpec:
		if c.handshakeComplete {
			c.sendAlert(alertInternalError)
			return c.in.setErrorLocked(errors.New("tls: handshake or ChangeCipherSpec requested after handshake complete"))
		}
	case recordTypeApplicationData:
		if !c.handshakeComplete {
			c.sendAlert(alertInternalError)
			return c.in.setErrorLocked(errors.New("tls: application data record requested before handshake complete"))
		}
	}

Again:
	if c.rawInput == nil {
		c.rawInput = c.in.newBlock()
	}
	b := c.rawInput

	// Read header, payload.
	if err := b.readFromUntil(c, recordHeaderLen); err != nil {
		// RFC suggests that EOF without an alertCloseNotify is
		// an error, but popular web sites seem to do this,
		// so we can't make it an error.
		// if err == io.EOF {
		// 	err = io.ErrUnexpectedEOF
		// }
		if e, ok := err.(net.Error); !ok || !e.Temporary() {
			c.in.setErrorLocked(err)
		}
		return err
	}
	typ := recordType(b.data[0])

	// No valid TLS record has a type of 0x80, however SSLv2 handshakes
	// start with a uint16 length where the MSB is set and the first record
	// is always < 256 bytes long. Therefore typ == 0x80 strongly suggests
	// an SSLv2 client.
	if want == recordTypeHandshake && typ == 0x80 {
		// if this is an SSLv2 header, lets see if we can upgrade to TLS
		if c.config.EnableSslv2ClientHello {
			return convertSSLv2ClientHello(c, b)
		} else {
			c.sendAlert(alertProtocolVersion)
			state.TlsHandshakeSslv2NotSupport.Inc(1)
			return c.in.setErrorLocked(errors.New("tls: unsupported SSLv2 handshake received"))
		}
	}

	vers := uint16(b.data[1])<<8 | uint16(b.data[2])
	n := int(b.data[3])<<8 | int(b.data[4])
	if c.haveVers && vers != c.vers {
		c.sendAlert(alertProtocolVersion)
		return c.in.setErrorLocked(fmt.Errorf("tls: received record with version %x when expecting version %x", vers, c.vers))
	}
	if n > maxCiphertext {
		c.sendAlert(alertRecordOverflow)
		return c.in.setErrorLocked(fmt.Errorf("tls: oversized record received with length %d", n))
	}
	if !c.haveVers {
		// First message, be extra suspicious:
		// this might not be a TLS client.
		// Bail out before reading a full 'body', if possible.
		// The current max version is 3.1.
		// If the version is >= 16.0, it's probably not real.
		// Similarly, a clientHello message encodes in
		// well under a kilobyte.  If the length is >= 12 kB,
		// it's probably not real.
		if (typ != recordTypeAlert && typ != want) || vers >= 0x1000 || n >= 0x3000 {
			c.sendAlert(alertUnexpectedMessage)
			return c.in.setErrorLocked(fmt.Errorf("tls: first record does not look like a TLS handshake"))
		}
	}
	if err := b.readFromUntil(c, recordHeaderLen+n); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		if e, ok := err.(net.Error); !ok || !e.Temporary() {
			c.in.setErrorLocked(err)
		}
		return err
	}

	// Process message.
	b, c.rawInput = c.in.splitBlock(b, recordHeaderLen+n)
	ok, off, err := c.in.decrypt(b)
	if !ok {
		c.in.setErrorLocked(c.sendAlert(err))
	}
	b.off = off
	data := b.data[b.off:]
	if len(data) > maxPlaintext {
		err := c.sendAlert(alertRecordOverflow)
		c.in.freeBlock(b)
		return c.in.setErrorLocked(err)
	}

	switch typ {
	default:
		c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))

	case recordTypeAlert:
		if len(data) != 2 {
			c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
			break
		}
		if alert(data[1]) == alertCloseNotify {
			c.in.setErrorLocked(io.EOF)
			break
		}
		switch data[0] {
		case alertLevelWarning:
			// drop on the floor
			c.in.freeBlock(b)
			goto Again
		case alertLevelError:
			c.in.setErrorLocked(&net.OpError{Op: "remote error", Err: alert(data[1])})
		default:
			c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
		}

	case recordTypeChangeCipherSpec:
		if typ != want || len(data) != 1 || data[0] != 1 {
			c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
			break
		}
		err := c.in.changeCipherSpec()
		if err != nil {
			c.in.setErrorLocked(c.sendAlert(err.(alert)))
		}

	case recordTypeApplicationData:
		if typ != want {
			c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
			break
		}
		c.input = b
		b = nil

	case recordTypeHandshake:
		// TODO(rsc): Should at least pick off connection close.
		if typ != want {
			return c.in.setErrorLocked(c.sendAlert(alertNoRenegotiation))
		}
		c.hand.Write(data)
	}

	if b != nil {
		c.in.freeBlock(b)
	}
	return c.in.err
}

// sendAlert sends a TLS alert message.
// c.out.Mutex <= L.
func (c *Conn) sendAlertLocked(err alert) error {
	switch err {
	case alertNoRenegotiation, alertCloseNotify:
		c.tmp[0] = alertLevelWarning
	default:
		c.tmp[0] = alertLevelError
	}
	c.tmp[1] = byte(err)
	c.writeRecord(recordTypeAlert, c.tmp[0:2])
	// closeNotify is a special case in that it isn't an error:
	if err != alertCloseNotify {
		return c.out.setErrorLocked(&net.OpError{Op: "local error", Err: err})
	}
	return nil
}

// sendAlert sends a TLS alert message.
// L < c.out.Mutex.
func (c *Conn) sendAlert(err alert) error {
	c.out.Lock()
	defer c.out.Unlock()
	return c.sendAlertLocked(err)
}

/* choosePlaintextSize - choose size of record plaintext
 *
 * Note:
 * 1. We use small TLS records that fit into a single TCP segment for the
 *    first `bytesThreshold` Byte of data,
 * 2. increase record size to maxPlaintext after that to optimize throughput,
 * 3. and then reset record size back to a single segment
 *    after `inactiveSeconds` second of inactivity,
 * 4. lather, rinse, repeat
 */
func (c *Conn) choosePlaintextSize() int {
	if !c.enableDynamicRecord {
		return initPlaintext
	}

	if c.byteOut < bytesThreshold {
		return initPlaintext
	}

	if time.Since(c.lastOut) < inactiveSeconds {
		return maxPlaintext
	}

	// reset bytes sent recently
	c.byteOut = 0
	return initPlaintext
}

// writeRecord writes a TLS record with the given type and payload
// to the connection and updates the record layer state.
// c.out.Mutex <= L.
func (c *Conn) writeRecord(typ recordType, data []byte) (n int, err error) {
	// choose appropriate size for record
	// Note: Some IE browsers fail to parse fragmented TLS/SSL handshake message,
	// we just choose dynamic record size for application data message. For more information,
	// see https://support.microsoft.com/en-us/kb/2541763
	plaintextSize := maxPlaintext
	if typ == recordTypeApplicationData {
		plaintextSize = c.choosePlaintextSize()
	}

	b := c.out.newBlock()
	for len(data) > 0 {
		m := len(data)
		// split data into segment of size 'plaintextSize'
		if m > plaintextSize {
			m = plaintextSize
		}
		explicitIVLen := 0
		explicitIVIsSeq := false

		var cbc cbcMode
		if c.out.version >= VersionTLS11 {
			var ok bool
			if cbc, ok = c.out.cipher.(cbcMode); ok {
				explicitIVLen = cbc.BlockSize()
			}
		}
		if explicitIVLen == 0 {
			if c, ok := c.out.cipher.(aead); ok {
				explicitIVLen = c.explicitNonceLen()
				// The AES-GCM construction in TLS has an
				// explicit nonce so that the nonce can be
				// random. However, the nonce is only 8 bytes
				// which is too small for a secure, random
				// nonce. Therefore we use the sequence number
				// as the nonce.
				explicitIVIsSeq = explicitIVLen > 0
			}
		}
		b.resize(recordHeaderLen + explicitIVLen + m)
		b.data[0] = byte(typ)
		vers := c.vers
		if vers == 0 {
			// Some TLS servers fail if the record version is
			// greater than TLS 1.0 for the initial ClientHello.
			vers = VersionTLS10
		}
		b.data[1] = byte(vers >> 8)
		b.data[2] = byte(vers)
		b.data[3] = byte(m >> 8)
		b.data[4] = byte(m)
		if explicitIVLen > 0 {
			explicitIV := b.data[recordHeaderLen : recordHeaderLen+explicitIVLen]
			if explicitIVIsSeq {
				copy(explicitIV, c.out.seq[:])
			} else {
				if _, err = io.ReadFull(c.config.rand(), explicitIV); err != nil {
					break
				}
			}
		}
		copy(b.data[recordHeaderLen+explicitIVLen:], data)
		c.out.encrypt(b, explicitIVLen)
		_, err = c.conn.Write(b.data)
		if err != nil {
			break
		}
		n += m
		data = data[m:]
	}
	c.out.freeBlock(b)

	if typ == recordTypeChangeCipherSpec {
		err = c.out.changeCipherSpec()
		if err != nil {
			// Cannot call sendAlert directly,
			// because we already hold c.out.Mutex.
			c.tmp[0] = alertLevelError
			c.tmp[1] = byte(err.(alert))
			c.writeRecord(recordTypeAlert, c.tmp[0:2])
			return n, c.out.setErrorLocked(&net.OpError{Op: "local error", Err: err})
		}
	}

	// update total bytes of application data sent
	if typ == recordTypeApplicationData {
		c.byteOut += n
		c.lastOut = time.Now()
	}
	return
}

// readHandshake reads the next handshake message from
// the record layer.
// c.in.Mutex < L; c.out.Mutex < L.
func (c *Conn) readHandshake() (interface{}, error) {
	for c.hand.Len() < 4 {
		if err := c.in.err; err != nil {
			return nil, err
		}
		if err := c.readRecord(recordTypeHandshake); err != nil {
			return nil, err
		}
	}

	data := c.hand.Bytes()
	n := int(data[1])<<16 | int(data[2])<<8 | int(data[3])
	if n > maxHandshake {
		return nil, c.in.setErrorLocked(c.sendAlert(alertInternalError))
	}
	for c.hand.Len() < 4+n {
		if err := c.in.err; err != nil {
			return nil, err
		}
		if err := c.readRecord(recordTypeHandshake); err != nil {
			return nil, err
		}
	}
	data = c.hand.Next(4 + n)
	var m handshakeMessage
	switch data[0] {
	case typeClientHello:
		m = new(clientHelloMsg)
	case typeServerHello:
		m = new(serverHelloMsg)
	case typeNewSessionTicket:
		m = new(newSessionTicketMsg)
	case typeCertificate:
		m = new(certificateMsg)
	case typeCertificateRequest:
		m = &certificateRequestMsg{
			hasSignatureAndHash: c.vers >= VersionTLS12,
		}
	case typeCertificateStatus:
		m = new(certificateStatusMsg)
	case typeServerKeyExchange:
		m = new(serverKeyExchangeMsg)
	case typeServerHelloDone:
		m = new(serverHelloDoneMsg)
	case typeClientKeyExchange:
		m = new(clientKeyExchangeMsg)
	case typeCertificateVerify:
		m = &certificateVerifyMsg{
			hasSignatureAndHash: c.vers >= VersionTLS12,
		}
	case typeNextProtocol:
		m = new(nextProtoMsg)
	case typeFinished:
		m = new(finishedMsg)
	default:
		return nil, c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
	}

	// The handshake message unmarshallers
	// expect to be able to keep references to data,
	// so pass in a fresh copy that won't be overwritten.
	data = append([]byte(nil), data...)

	if !m.unmarshal(data) {
		return nil, c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
	}
	return m, nil
}

// Write writes data to the connection.
func (c *Conn) Write(b []byte) (int, error) {
	if err := c.Handshake(); err != nil {
		return 0, err
	}

	c.out.Lock()
	defer c.out.Unlock()

	if err := c.out.err; err != nil {
		return 0, err
	}

	if !c.handshakeComplete {
		return 0, alertInternalError
	}

	// SSL 3.0 and TLS 1.0 are susceptible to a chosen-plaintext
	// attack when using block mode ciphers due to predictable IVs.
	// This can be prevented by splitting each Application Data
	// record into two records, effectively randomizing the IV.
	//
	// http://www.openssl.org/~bodo/tls-cbc.txt
	// https://bugzilla.mozilla.org/show_bug.cgi?id=665814
	// http://www.imperialviolet.org/2012/01/15/beastfollowup.html

	var m int
	if len(b) > 1 && c.vers <= VersionTLS10 {
		if _, ok := c.out.cipher.(cipher.BlockMode); ok {
			n, err := c.writeRecord(recordTypeApplicationData, b[:1])
			if err != nil {
				return n, c.out.setErrorLocked(err)
			}
			m, b = 1, b[1:]
		}
	}

	n, err := c.writeRecord(recordTypeApplicationData, b)
	return n + m, c.out.setErrorLocked(err)
}

// Read can be made to time out and return a net.Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (c *Conn) Read(b []byte) (n int, err error) {
	if err = c.Handshake(); err != nil {
		return
	}
	if len(b) == 0 {
		// Put this after Handshake, in case people were calling
		// Read(nil) for the side effect of the Handshake.
		return
	}

	c.in.Lock()
	defer c.in.Unlock()

	// Some OpenSSL servers send empty records in order to randomize the
	// CBC IV. So this loop ignores a limited number of empty records.
	const maxConsecutiveEmptyRecords = 100
	for emptyRecordCount := 0; emptyRecordCount <= maxConsecutiveEmptyRecords; emptyRecordCount++ {
		for c.input == nil && c.in.err == nil {
			if err := c.readRecord(recordTypeApplicationData); err != nil {
				// Soft error, like EAGAIN
				return 0, err
			}
		}
		if err := c.in.err; err != nil {
			return 0, err
		}

		n, err = c.input.Read(b)
		if c.input.off >= len(c.input.data) {
			c.in.freeBlock(c.input)
			c.input = nil
		}

		// If a close-notify alert is waiting, read it so that
		// we can return (n, EOF) instead of (n, nil), to signal
		// to the HTTP response reading goroutine that the
		// connection is now closed. This eliminates a race
		// where the HTTP response reading goroutine would
		// otherwise not observe the EOF until its next read,
		// by which time a client goroutine might have already
		// tried to reuse the HTTP connection for a new
		// request.
		// See https://codereview.appspot.com/76400046
		// and http://golang.org/issue/3514
		if ri := c.rawInput; ri != nil &&
			n != 0 && err == nil &&
			c.input == nil && len(ri.data) > 0 && recordType(ri.data[0]) == recordTypeAlert {
			if recErr := c.readRecord(recordTypeApplicationData); recErr != nil {
				err = recErr // will be io.EOF on closeNotify
			}
		}

		if n != 0 || err != nil {
			return n, err
		}
	}

	return 0, io.ErrNoProgress
}

// Close closes the connection.
func (c *Conn) Close() error {
	var alertErr error

	c.handshakeMutex.Lock()
	defer c.handshakeMutex.Unlock()
	if c.handshakeComplete {
		alertErr = c.sendAlert(alertCloseNotify)
	}

	if err := c.conn.Close(); err != nil {
		return err
	}
	return alertErr
}

// Handshake runs the client or server handshake
// protocol if it has not yet been run.
// Most uses of this package need not call Handshake
// explicitly: the first Read or Write will call it automatically.
func (c *Conn) Handshake() error {
	c.handshakeMutex.Lock()
	defer c.handshakeMutex.Unlock()
	if err := c.handshakeErr; err != nil {
		return err
	}
	if c.handshakeComplete {
		return nil
	}

	if c.isClient {
		c.handshakeErr = c.clientHandshake()
	} else {
		c.handshakeErr = c.serverHandshake()
	}
	return c.handshakeErr
}

// ConnectionState returns basic TLS details about the connection.
func (c *Conn) ConnectionState() ConnectionState {
	c.handshakeMutex.Lock()
	defer c.handshakeMutex.Unlock()

	var state ConnectionState
	state.HandshakeComplete = c.handshakeComplete
	if c.handshakeComplete {
		state.Version = c.vers
		state.NegotiatedProtocol = c.clientProtocol
		state.DidResume = c.didResume
		state.NegotiatedProtocolIsMutual = !c.clientProtocolFallback
		state.CipherSuite = c.cipherSuite
		state.HandshakeTime = c.handshakeTime
		state.OcspStaple = c.ocspStaple
		state.PeerCertificates = c.peerCertificates
		state.VerifiedChains = c.verifiedChains
		state.ServerName = c.serverName
		state.ClientRandom = c.clientRandom
		state.ServerRandom = c.serverRandom
		state.MasterSecret = c.masterSecret
		state.ClientCiphers = c.clientCiphers
		if c.clientAuth == RequireAndVerifyClientCert {
			state.ClientAuth = true
		}
		state.ClientCAName = c.clientCAName
		state.JA3Raw = c.ja3Raw
		state.JA3Hash = c.ja3Hash
	}

	return state
}

// OCSPResponse returns the stapled OCSP response from the TLS server, if
// any. (Only valid for client connections.)
func (c *Conn) OCSPResponse() []byte {
	c.handshakeMutex.Lock()
	defer c.handshakeMutex.Unlock()

	return c.ocspResponse
}

// VerifyHostname checks that the peer certificate chain is valid for
// connecting to host.  If so, it returns nil; if not, it returns an error
// describing the problem.
func (c *Conn) VerifyHostname(host string) error {
	c.handshakeMutex.Lock()
	defer c.handshakeMutex.Unlock()
	if !c.isClient {
		return errors.New("tls: VerifyHostname called on TLS server connection")
	}
	if !c.handshakeComplete {
		return errors.New("tls: handshake has not yet been performed")
	}
	return c.peerCertificates[0].VerifyHostname(host)
}

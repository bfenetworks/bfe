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
	"crypto"
	"crypto/ecdsa"
	"crypto/md5"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// serverHandshakeState contains details of a server handshake in progress.
// It's discarded once the handshake has completed.
type serverHandshakeState struct {
	c               *Conn
	clientHello     *clientHelloMsg
	hello           *serverHelloMsg
	suite           *cipherSuite
	ellipticOk      bool
	ecdsaOk         bool
	chachaOk        bool
	sessionTicketOK bool
	useRC4          uint8
	sessionState    *sessionState
	finishedHash    finishedHash
	masterSecret    []byte
	certsFromClient [][]byte
	cert            *Certificate
}

// serverHandshake performs a TLS handshake as a server.
func (c *Conn) serverHandshake() error {
	handshakeStart := time.Now()
	config := c.config

	// If this is the first server handshake, we generate a random key to
	// encrypt the tickets with.
	config.serverInitOnce.Do(config.serverInit)

	hs := serverHandshakeState{
		c: c,
	}

	isResume, err := hs.readClientHello()
	if err == io.EOF && 0 == c.readFromUntilLen {
		state.TlsHandshakeZeroData.Inc(1)
	}
	if err != nil {
		state.TlsHandshakeReadClientHelloErr.Inc(1)
		return err
	}

	// Record JA3 fingerprint for TLS client
	c.ja3Raw = hs.clientHello.JA3String()
	sum := md5.Sum([]byte(c.ja3Raw))
	c.ja3Hash = hex.EncodeToString(sum[:])

	// For an overview of TLS handshaking, see https://tools.ietf.org/html/rfc5246#section-7.3
	if isResume {
		state.TlsHandshakeResumeAll.Inc(1)
		// The client has included a session ticket and so we do an abbreviated handshake.
		if err := hs.doResumeHandshake(); err != nil {
			return err
		}
		if err := hs.establishKeys(); err != nil {
			return err
		}
		if err := hs.sendFinished(); err != nil {
			return err
		}
		if err := hs.readFinished(); err != nil {
			return err
		}
		c.didResume = true
		state.TlsHandshakeResumeSucc.Inc(1)
	} else {
		state.TlsHandshakeFullAll.Inc(1)
		// The client didn't include a session ticket, or it wasn't
		// valid so we do a full handshake.
		if err := hs.doFullHandshake(); err != nil {
			return err
		}
		if err := hs.establishKeys(); err != nil {
			return err
		}
		if err := hs.readFinished(); err != nil {
			return err
		}
		if err := hs.sendSessionTicket(); err != nil {
			return err
		}
		if err := hs.sendFinished(); err != nil {
			return err
		}

		// update session cache
		if !c.config.SessionCacheDisabled && c.config.ServerSessionCache != nil {
			// If a server is planning on issuing a session ticket to a client that
			// does not present one, it SHOULD include an empty Session ID in the ServerHello
			// If the Session Id is not empty, the sever choose stateful resume(session id).
			if len(hs.hello.sessionId) > 0 {
				state := &sessionState{
					vers:         c.vers,
					cipherSuite:  hs.suite.id,
					sessionId:    hs.hello.sessionId,
					masterSecret: hs.masterSecret,
					certificates: hs.certsFromClient,
				}
				c.config.ServerSessionCache.Put(fmt.Sprintf("%x", hs.hello.sessionId),
					state.marshal())
			}
		}
		state.TlsHandshakeFullSucc.Inc(1)
	}
	c.handshakeComplete = true
	c.handshakeTime = time.Since(handshakeStart)

	// Record master secret for established tls conn. Master secret may
	// saved in NSS key log format so that external programs (eg. wireshark)
	// can decrypt TLS connections for trouble shooting.
	c.clientRandom = hs.clientHello.random
	c.serverRandom = hs.hello.random
	c.masterSecret = hs.masterSecret

	return nil
}

// readClientHello reads a ClientHello message from the client and decides
// whether we will perform session resumption.
func (hs *serverHandshakeState) readClientHello() (isResume bool, err error) {
	config := hs.c.config
	c := hs.c

	msg, err := c.readHandshake()
	if err != nil {
		return false, err
	}
	var ok bool
	hs.clientHello, ok = msg.(*clientHelloMsg)
	if !ok {
		c.sendAlert(alertUnexpectedMessage)
		return false, unexpectedMessageError(hs.clientHello, msg)
	}
	c.vers, ok = config.mutualVersion(hs.clientHello.vers)
	if !ok {
		c.sendAlert(alertProtocolVersion)
		return false, fmt.Errorf("tls: client offered an unsupported, maximum protocol version of %x", hs.clientHello.vers)
	}

	if len(hs.clientHello.serverName) > 0 {
		c.serverName = hs.clientHello.serverName
	}

	// get customized config for current connection
	var rule *Rule
	if config.ServerRule != nil {
		rule = config.ServerRule.Get(c)
	}

	c.grade = GradeC
	if rule != nil {
		c.grade = rule.Grade
		c.enableDynamicRecord = rule.DynamicRecord
	}

	c.vers, ok = config.checkVersionGrade(c.vers, c.grade)
	if !ok {
		c.sendAlert(alertProtocolVersion)
		return false, fmt.Errorf("tls: client offered an unsupported suite for this grade, grade is %s", c.grade)
	}

	c.haveVers = true

	hs.useRC4 = config.checkCipherGrade(c)

	hs.finishedHash = newFinishedHash(c.vers)
	if c.sslv2Data == nil {
		hs.finishedHash.Write(hs.clientHello.marshal())
	} else {
		hs.finishedHash.Write(c.sslv2Data)
		c.sslv2Data = nil
	}

	hs.hello = new(serverHelloMsg)

	supportedCurve := false
	preferredCurves := config.curvePreferences()
Curves:
	for _, curve := range hs.clientHello.supportedCurves {
		for _, supported := range preferredCurves {
			if supported == curve {
				supportedCurve = true
				break Curves
			}
		}
	}

	supportedPointFormat := false
	for _, pointFormat := range hs.clientHello.supportedPoints {
		if pointFormat == pointFormatUncompressed {
			supportedPointFormat = true
			break
		}
	}
	hs.ellipticOk = supportedCurve && supportedPointFormat

	foundCompression := false
	// We only support null compression, so check that the client offered it.
	for _, compression := range hs.clientHello.compressionMethods {
		if compression == compressionNone {
			foundCompression = true
			break
		}
	}

	if !foundCompression {
		c.sendAlert(alertHandshakeFailure)
		return false, errors.New("tls: client does not support uncompressed connections")
	}

	hs.hello.vers = c.vers
	hs.hello.random, err = generateHelloRandom(config.rand())
	if err != nil {
		c.sendAlert(alertInternalError)
		return false, err
	}
	hs.hello.secureRenegotiation = hs.clientHello.secureRenegotiation
	hs.hello.compressionMethod = compressionNone

	nextProtos := config.NextProtos
	if rule != nil {
		nextProtos = rule.NextProtos.Get(c)
	}

	if len(hs.clientHello.alpnProtocols) > 0 {
		if selectedProto, fallback := mutualProtocol(hs.clientHello.alpnProtocols, nextProtos); !fallback {
			hs.hello.alpnProtocol = selectedProto
			c.clientProtocol = selectedProto
		}
	} else {
		// Although sending an NPN extension without h2 is reasonable, some client
		// has a bug around this. Best to send NPN without h2.
		nextProtos = checkAndRemoveH2(nextProtos)

		// Although sending an empty NPN extension is reasonable, Firefox has
		// had a bug around this. Best to send nothing at all if
		// config.NextProtos is empty. See
		// https://code.google.com/p/go/issues/detail?id=5445.
		if hs.clientHello.nextProtoNeg && len(nextProtos) > 0 {
			hs.hello.nextProtoNeg = true
			hs.hello.nextProtos = nextProtos
		}
	}

	// Select certificate for current connection
	if len(config.Certificates) == 0 {
		c.sendAlert(alertInternalError)
		return false, errors.New("tls: no certificates configured")
	}
	hs.cert = &config.Certificates[0]
	if len(hs.clientHello.serverName) > 0 {
		hs.cert = config.getCertificateForName(hs.clientHello.serverName)
	}

	if tlsMultiCertificate != nil {
		// select certificate by third party policy
		if cert := tlsMultiCertificate.Get(c); cert != nil {
			hs.cert = cert
		}
	} else if config.MultiCert != nil {
		// select certificate by default policy
		if cert := config.MultiCert.Get(c); cert != nil {
			hs.cert = cert
		}
	}

	_, hs.ecdsaOk = hs.cert.PrivateKey.(*ecdsa.PrivateKey)

	// Select client auth policy for current connection
	c.clientAuth = config.ClientAuth
	if rule != nil && rule.ClientAuth {
		c.clientAuth = RequireAndVerifyClientCert
		c.clientCAs = rule.ClientCAs
		c.clientCAName = rule.ClientCAName
		c.clientCRLPool = rule.ClientCRLPool
	}

	// check whether chacha20-poly1305 is enabled for current connection
	if rule != nil {
		hs.chachaOk = rule.Chacha20
	}

	if hs.checkForResumption() {
		return true, nil
	}

	var preferenceList, supportedList []uint16
	if c.config.PreferServerCipherSuites {
		preferenceList = c.config.cipherSuites()
		supportedList = hs.clientHello.cipherSuites
	} else {
		preferenceList = hs.clientHello.cipherSuites
		supportedList = c.config.cipherSuites()
	}

	if c.config.PreferServerCipherSuites && len(c.config.CipherSuitesPriority) == len(preferenceList) {
		// Equivalent cipher suite negotiation
		hs.suite = hs.negotiateEquivalentCipherSuites(supportedList, preferenceList)
	} else {
		// Normal cipher suite negotiation
		for _, id := range preferenceList {
			if hs.suite, _ = c.tryCipherSuite(id, supportedList, c.vers,
				hs.ellipticOk, hs.ecdsaOk, hs.chachaOk, hs.useRC4); hs.suite != nil {
				break
			}
		}
	}

	// no cipher suite supported by both client and server.
	if hs.suite == nil {
		// If client proposes ECDHE without ECC extensions, just try ECDHE cipher suite
		// and choose CurveP256 and Uncompressed point format for client.
		//
		// Note: A client that proposes ECC cipher suites may choose not to include
		// elliptic curves extension or elliptic point format extension. In this case,
		// the server is free to choose any one of the elliptic curves or point formats.
		//
		// For more information, see RFC 4492 Section 4
		ellipticMayOk := hs.checkEllipticMayOk(supportedCurve, supportedPointFormat)
		if ellipticMayOk {
			for _, id := range preferenceList {
				if !CheckSuiteECDHE(id) {
					continue
				}
				if hs.suite, _ = c.tryCipherSuite(id, supportedList, c.vers, true, hs.ecdsaOk, hs.chachaOk, hs.useRC4); hs.suite != nil {
					break
				}
			}

			if hs.suite != nil {
				state.TlsHandshakeAcceptEcdheWithoutExt.Inc(1)
				hs.clientHello.supportedCurves = append(hs.clientHello.supportedCurves, CurveP256)
				hs.clientHello.supportedPoints = append(hs.clientHello.supportedPoints, pointFormatUncompressed)
			}
		}
	}

	if hs.suite == nil {
		c.sendAlert(alertHandshakeFailure)
		state.TlsHandshakeNoSharedCipherSuite.Inc(1)
		return false, fmt.Errorf("tls: no cipher suite supported by both client and server: %v",
			hs.clientHello.cipherSuites)
	}

	// See https://tools.ietf.org/html/draft-ietf-tls-downgrade-scsv-00.
	for _, id := range hs.clientHello.cipherSuites {
		if id == TLS_FALLBACK_SCSV {
			// The client is doing a fallback connection.
			if hs.clientHello.vers < c.config.MaxVersion {
				c.sendAlert(alertInappropriateFallback)
				return false, errors.New("tls: client using inppropriate protocol fallback")
			}
			break
		}
	}

	hs.validateHttp2Accepted()
	return false, nil
}

// Equivalent cipher suite negotiation
// Note: for equivalent cipher suites (cipher suites with same priority in server side),
// selects the client's most preferred ciphersuite.
func (hs *serverHandshakeState) negotiateEquivalentCipherSuites(clientSuites, serverSuites []uint16) *cipherSuite {
	var suiteSelected *cipherSuite
	var suiteServerOrder uint16
	var suiteClientOrder int

	c := hs.c
	for index, serverOrder := range c.config.CipherSuitesPriority {
		id := serverSuites[index]

		// check whether current suite is acceptable by client and client's preference for it
		if suite, clientOrder := c.tryCipherSuite(id, clientSuites, c.vers,
			hs.ellipticOk, hs.ecdsaOk, hs.chachaOk, hs.useRC4); suite != nil {
			// if found first acceptable suite or better suite
			if suiteSelected == nil || (serverOrder == suiteServerOrder && clientOrder < suiteClientOrder) {
				suiteSelected = suite
				suiteServerOrder = serverOrder
				suiteClientOrder = clientOrder
			}
		}

		// stop check suite with lower priority
		if suiteSelected != nil && suiteServerOrder < serverOrder {
			break
		}
	}
	return suiteSelected
}

func checkAndRemoveH2(protos []string) []string {
	nextProtos := make([]string, 0)
	for _, p := range protos {
		if p != "h2" {
			nextProtos = append(nextProtos, p)
		}
	}
	return nextProtos
}

func (hs *serverHandshakeState) validateHttp2Accepted() {
	// Note: implementations for http2 Must use TLS1.2 or higher and SHOULD not use
	// any of the cipher suites that are listed in the cipher suite black list.
	// See RFC 7540 Section 9.2
	c := hs.c
	if hs.hello.alpnProtocol == "h2" {
		if !checkCipherSuiteHttp2Accepted(hs.suite.id) || c.vers < VersionTLS12 {
			hs.hello.alpnProtocol = "http/1.1"
			c.clientProtocol = "http/1.1"
		}
	}
}

func (hs *serverHandshakeState) checkEllipticMayOk(supportedCurve, supportedPointFormat bool) bool {
	if hs.c.vers <= VersionSSL30 {
		return false
	}
	if supportedCurve && len(hs.clientHello.supportedPoints) == 0 {
		return true
	}
	if supportedPointFormat && len(hs.clientHello.supportedCurves) == 0 {
		return true
	}
	if len(hs.clientHello.supportedCurves) == 0 && len(hs.clientHello.supportedPoints) == 0 {
		return true
	}
	return false
}

// checkForResumption returns true if we should perform resumption on this connection.
func (hs *serverHandshakeState) checkForResumption() bool {
	c := hs.c

	var ok bool
	// check session ticket
	if !c.config.SessionTicketsDisabled && hs.clientHello.ticketSupported && len(hs.clientHello.sessionTicket) != 0 {
		state.TlsHandshakeCheckResumeSessionTicket.Inc(1)
		hs.sessionTicketOK = true
		if hs.sessionState, ok = c.decryptTicket(hs.clientHello.sessionTicket); !ok {
			return false
		}
	} else {
		// check session cache
		if len(hs.clientHello.sessionId) == 0 {
			return false
		}
		state.TlsHandshakeCheckResumeSessionCache.Inc(1)
		hs.sessionTicketOK = false
		hs.sessionState = nil
		if !c.config.SessionCacheDisabled && c.config.ServerSessionCache != nil {
			sessionCache := c.config.ServerSessionCache
			sessionParam, ok := sessionCache.Get(fmt.Sprintf("%x", hs.clientHello.sessionId))
			if !ok {
				return false
			}

			candidateSession := new(sessionState)
			if ok := candidateSession.unmarshal(sessionParam); !ok {
				return false
			}
			hs.sessionState = candidateSession
		}
	}

	if hs.sessionState == nil || hs.sessionState.vers > hs.clientHello.vers {
		return false
	}
	if vers, ok := c.config.mutualVersion(hs.sessionState.vers); !ok || vers != hs.sessionState.vers {
		return false
	}

	cipherSuiteOk := false
	// Check that the client is still offering the ciphersuite in the session.
	for _, id := range hs.clientHello.cipherSuites {
		if id == hs.sessionState.cipherSuite {
			cipherSuiteOk = true
			break
		}
	}
	if !cipherSuiteOk {
		return false
	}

	// Check that we also support the ciphersuite from the session.
	hs.suite, _ = c.tryCipherSuite(hs.sessionState.cipherSuite, c.config.cipherSuites(), hs.sessionState.vers,
		hs.ellipticOk, hs.ecdsaOk, hs.chachaOk, hs.useRC4)
	if hs.suite == nil {
		return false
	}

	sessionHasClientCerts := len(hs.sessionState.certificates) != 0
	needClientCerts := c.clientAuth == RequireAnyClientCert || c.clientAuth == RequireAndVerifyClientCert
	if needClientCerts && !sessionHasClientCerts {
		return false
	}
	if sessionHasClientCerts && c.clientAuth == NoClientCert {
		return false
	}

	if hs.sessionTicketOK {
		state.TlsHandshakeShouldResumeSessionTicket.Inc(1)
	} else {
		state.TlsHandshakeShouldResumeSessionCache.Inc(1)
	}

	hs.validateHttp2Accepted()

	return true
}

func (hs *serverHandshakeState) doResumeHandshake() error {
	c := hs.c

	hs.hello.cipherSuite = hs.suite.id
	// We echo the client's session ID in the ServerHello to let it know
	// that we're doing a resumption.
	hs.hello.sessionId = hs.clientHello.sessionId
	hs.finishedHash.Write(hs.hello.marshal())
	c.writeRecord(recordTypeHandshake, hs.hello.marshal())

	if len(hs.sessionState.certificates) > 0 {
		if _, err := hs.processCertsFromClient(hs.sessionState.certificates); err != nil {
			return err
		}
	}

	hs.masterSecret = hs.sessionState.masterSecret

	return nil
}

// judge whether ocspstapling update time suitable for server time
func (hs *serverHandshakeState) ocspTimeCheck() bool {
	if hs.cert.OCSPParse == nil {
		return false
	}

	if !OcspTimeRangeCheck(hs.cert.OCSPParse) {
		state.TlsHandshakeOcspTimeErr.Inc(1)
		return false
	}

	return true
}

func (hs *serverHandshakeState) doFullHandshake() error {
	config := hs.c.config
	c := hs.c

	if hs.clientHello.ocspStapling && len(hs.cert.OCSPStaple) > 0 {
		// check ocspstapling time
		if ocspOk := hs.ocspTimeCheck(); ocspOk {
			hs.hello.ocspStapling = true
			c.ocspStaple = true
		}
	}
	if hs.clientHello.ocspStapling {
		state.TlsStatusRequestExtCount.Inc(1)
	}

	hs.hello.ticketSupported = hs.clientHello.ticketSupported && !config.SessionTicketsDisabled
	if !hs.hello.ticketSupported && !c.config.SessionCacheDisabled {
		// create new session id
		hs.hello.sessionId = make([]byte, 32)
		if _, err := io.ReadFull(c.config.rand(), hs.hello.sessionId); err != nil {
			c.sendAlert(alertInternalError)
			return err
		}
	}

	hs.hello.cipherSuite = hs.suite.id
	hs.finishedHash.Write(hs.hello.marshal())
	c.writeRecord(recordTypeHandshake, hs.hello.marshal())

	var certMsg *certificateMsg
	var certMsgData []byte

	if hs.cert.message != nil {
		certMsgData = hs.cert.message
	} else {
		certMsg := new(certificateMsg)
		certMsg.certificates = hs.cert.Certificate
		certMsgData = certMsg.marshal()
	}

	hs.finishedHash.Write(certMsgData)
	c.writeRecord(recordTypeHandshake, certMsgData)

	if hs.hello.ocspStapling {
		certStatus := new(certificateStatusMsg)
		certStatus.statusType = statusTypeOCSP
		certStatus.response = hs.cert.OCSPStaple
		hs.finishedHash.Write(certStatus.marshal())
		c.writeRecord(recordTypeHandshake, certStatus.marshal())
	}

	keyAgreement := hs.suite.ka(c.vers)
	skx, err := keyAgreement.generateServerKeyExchange(config, hs.cert, hs.clientHello, hs.hello)
	if err != nil {
		c.sendAlert(alertHandshakeFailure)
		return err
	}
	if skx != nil {
		hs.finishedHash.Write(skx.marshal())
		c.writeRecord(recordTypeHandshake, skx.marshal())
	}

	if c.clientAuth >= RequestClientCert {
		// Request a client certificate
		certReq := new(certificateRequestMsg)
		certReq.certificateTypes = []byte{
			byte(certTypeRSASign),
			byte(certTypeECDSASign),
		}
		if c.vers >= VersionTLS12 {
			certReq.hasSignatureAndHash = true
			certReq.signatureAndHashes = supportedClientCertSignatureAlgorithms
		}

		// An empty list of certificateAuthorities signals to
		// the client that it may send any certificate in response
		// to our request. When we know the CAs we trust, then
		// we can send them down, so that the client can choose
		// an appropriate certificate to give to us.
		if clientCAs := c.getClientCAs(); clientCAs != nil {
			certReq.certificateAuthorities = clientCAs.Subjects()
		}
		hs.finishedHash.Write(certReq.marshal())
		c.writeRecord(recordTypeHandshake, certReq.marshal())
	}

	helloDone := new(serverHelloDoneMsg)
	hs.finishedHash.Write(helloDone.marshal())
	c.writeRecord(recordTypeHandshake, helloDone.marshal())

	var pub crypto.PublicKey // public key for client auth, if any

	msg, err := c.readHandshake()
	if err != nil {
		return err
	}

	var ok bool
	// If we requested a client certificate, then the client must send a
	// certificate message, even if it's empty.
	if c.clientAuth >= RequestClientCert {
		if certMsg, ok = msg.(*certificateMsg); !ok {
			c.sendAlert(alertUnexpectedMessage)
			return unexpectedMessageError(certMsg, msg)
		}
		hs.finishedHash.Write(certMsg.marshal())

		if len(certMsg.certificates) == 0 {
			// The client didn't actually send a certificate
			switch c.clientAuth {
			case RequireAnyClientCert, RequireAndVerifyClientCert:
				c.sendAlert(alertBadCertificate)
				return errors.New("tls: client didn't provide a certificate")
			}
		}

		pub, err = hs.processCertsFromClient(certMsg.certificates)
		if err != nil {
			return err
		}

		msg, err = c.readHandshake()
		if err != nil {
			return err
		}
	}

	// Get client key exchange
	ckx, ok := msg.(*clientKeyExchangeMsg)
	if !ok {
		c.sendAlert(alertUnexpectedMessage)
		return unexpectedMessageError(ckx, msg)
	}
	hs.finishedHash.Write(ckx.marshal())

	// If we received a client cert in response to our certificate request message,
	// the client will send us a certificateVerifyMsg immediately after the
	// clientKeyExchangeMsg.  This message is a digest of all preceding
	// handshake-layer messages that is signed using the private key corresponding
	// to the client's certificate. This allows us to verify that the client is in
	// possession of the private key of the certificate.
	if len(c.peerCertificates) > 0 {
		msg, err = c.readHandshake()
		if err != nil {
			return err
		}
		certVerify, ok := msg.(*certificateVerifyMsg)
		if !ok {
			c.sendAlert(alertUnexpectedMessage)
			return unexpectedMessageError(certVerify, msg)
		}

		switch key := pub.(type) {
		case *ecdsa.PublicKey:
			ecdsaSig := new(ecdsaSignature)
			if _, err = asn1.Unmarshal(certVerify.signature, ecdsaSig); err != nil {
				break
			}
			if ecdsaSig.R.Sign() <= 0 || ecdsaSig.S.Sign() <= 0 {
				err = errors.New("ECDSA signature contained zero or negative values")
				break
			}
			digest, _, _ := hs.finishedHash.hashForClientCertificate(signatureECDSA)
			if !ecdsa.Verify(key, digest, ecdsaSig.R, ecdsaSig.S) {
				err = errors.New("ECDSA verification failure")
				break
			}
		case *rsa.PublicKey:
			digest, hashFunc, _ := hs.finishedHash.hashForClientCertificate(signatureRSA)
			err = rsa.VerifyPKCS1v15(key, hashFunc, digest, certVerify.signature)
		}
		if err != nil {
			c.sendAlert(alertBadCertificate)
			return errors.New("could not validate signature of connection nonces: " + err.Error())
		}

		hs.finishedHash.Write(certVerify.marshal())
	}

	preMasterSecret, err := keyAgreement.processClientKeyExchange(config, hs.cert, ckx, c.vers)
	if err != nil {
		c.sendAlert(alertHandshakeFailure)
		return err
	}
	hs.masterSecret = masterFromPreMasterSecret(c.vers, preMasterSecret, hs.clientHello.random, hs.hello.random)

	return nil
}

func (hs *serverHandshakeState) establishKeys() error {
	c := hs.c

	clientMAC, serverMAC, clientKey, serverKey, clientIV, serverIV :=
		keysFromMasterSecret(c.vers, hs.masterSecret, hs.clientHello.random, hs.hello.random, hs.suite.macLen, hs.suite.keyLen, hs.suite.ivLen)

	var clientCipher, serverCipher interface{}
	var clientHash, serverHash macFunction

	if hs.suite.aead == nil {
		clientCipher = hs.suite.cipher(clientKey, clientIV, true /* for reading */)
		clientHash = hs.suite.mac(c.vers, clientMAC)
		serverCipher = hs.suite.cipher(serverKey, serverIV, false /* not for reading */)
		serverHash = hs.suite.mac(c.vers, serverMAC)
	} else {
		clientCipher = hs.suite.aead(clientKey, clientIV)
		serverCipher = hs.suite.aead(serverKey, serverIV)
	}

	c.in.prepareCipherSpec(c.vers, clientCipher, clientHash)
	c.out.prepareCipherSpec(c.vers, serverCipher, serverHash)

	return nil
}

func (hs *serverHandshakeState) readFinished() error {
	c := hs.c

	c.readRecord(recordTypeChangeCipherSpec)
	if err := c.in.error(); err != nil {
		return err
	}

	if hs.hello.nextProtoNeg {
		msg, err := c.readHandshake()
		if err != nil {
			return err
		}
		nextProto, ok := msg.(*nextProtoMsg)
		if !ok {
			c.sendAlert(alertUnexpectedMessage)
			return unexpectedMessageError(nextProto, msg)
		}
		hs.finishedHash.Write(nextProto.marshal())
		c.clientProtocol = nextProto.proto
	}

	msg, err := c.readHandshake()
	if err != nil {
		return err
	}
	clientFinished, ok := msg.(*finishedMsg)
	if !ok {
		c.sendAlert(alertUnexpectedMessage)
		return unexpectedMessageError(clientFinished, msg)
	}

	verify := hs.finishedHash.clientSum(hs.masterSecret)
	if len(verify) != len(clientFinished.verifyData) ||
		subtle.ConstantTimeCompare(verify, clientFinished.verifyData) != 1 {
		c.sendAlert(alertHandshakeFailure)
		return errors.New("tls: client's Finished message is incorrect")
	}

	hs.finishedHash.Write(clientFinished.marshal())
	return nil
}

func (hs *serverHandshakeState) sendSessionTicket() error {
	if !hs.hello.ticketSupported {
		return nil
	}

	c := hs.c
	m := new(newSessionTicketMsg)

	var err error
	state := sessionState{
		vers:         c.vers,
		cipherSuite:  hs.suite.id,
		masterSecret: hs.masterSecret,
		certificates: hs.certsFromClient,
	}
	m.ticket, err = c.encryptTicket(&state)
	if err != nil {
		return err
	}

	hs.finishedHash.Write(m.marshal())
	c.writeRecord(recordTypeHandshake, m.marshal())

	return nil
}

func (hs *serverHandshakeState) sendFinished() error {
	c := hs.c

	c.writeRecord(recordTypeChangeCipherSpec, []byte{1})

	finished := new(finishedMsg)
	finished.verifyData = hs.finishedHash.serverSum(hs.masterSecret)
	hs.finishedHash.Write(finished.marshal())

	c.writeRecord(recordTypeHandshake, finished.marshal())

	c.cipherSuite = hs.suite.id

	return nil
}

// processCertsFromClient takes a chain of client certificates either from a
// Certificates message or from a sessionState and verifies them. It returns
// the public key of the leaf certificate.
func (hs *serverHandshakeState) processCertsFromClient(certificates [][]byte) (crypto.PublicKey, error) {
	c := hs.c

	hs.certsFromClient = certificates
	certs := make([]*x509.Certificate, len(certificates))
	var err error
	for i, asn1Data := range certificates {
		if certs[i], err = x509.ParseCertificate(asn1Data); err != nil {
			c.sendAlert(alertBadCertificate)
			return nil, errors.New("tls: failed to parse client certificate: " + err.Error())
		}

		if c.clientCRLPool != nil && c.clientCRLPool.CheckCertRevoked(certs[i]) {
			c.sendAlert(alertCertificateRevoked)
			return nil, fmt.Errorf("tls: revoked client certificate: %s %s", strings.ToUpper(certs[i].SerialNumber.Text(16)), certs[i].Subject.CommonName)
		}
	}

	if c.clientAuth >= VerifyClientCertIfGiven && len(certs) > 0 {
		opts := x509.VerifyOptions{
			Roots:         c.getClientCAs(),
			CurrentTime:   c.config.time(),
			Intermediates: x509.NewCertPool(),
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}

		for _, cert := range certs[1:] {
			opts.Intermediates.AddCert(cert)
		}

		chains, err := certs[0].Verify(opts)
		if err != nil {
			c.sendAlert(alertBadCertificate)
			return nil, errors.New("tls: failed to verify client's certificate: " + err.Error())
		}

		ok := false
		for _, ku := range certs[0].ExtKeyUsage {
			if ku == x509.ExtKeyUsageClientAuth {
				ok = true
				break
			}
		}
		if !ok {
			c.sendAlert(alertHandshakeFailure)
			return nil, errors.New("tls: client's certificate's extended key usage doesn't permit it to be used for client authentication")
		}

		c.verifiedChains = chains
	}

	if len(certs) > 0 {
		var pub crypto.PublicKey
		switch key := certs[0].PublicKey.(type) {
		case *ecdsa.PublicKey, *rsa.PublicKey:
			pub = key
		default:
			c.sendAlert(alertUnsupportedCertificate)
			return nil, fmt.Errorf("tls: client's certificate contains an unsupported public key of type %T", certs[0].PublicKey)
		}
		c.peerCertificates = certs
		return pub, nil
	}

	return nil, nil
}

// tryCipherSuite returns a cipherSuite with the given id if that cipher suite
// is acceptable to use.
func (c *Conn) tryCipherSuite(id uint16, supportedCipherSuites []uint16, version uint16,
	ellipticOk, ecdsaOk, chachaOk bool, useRC4 uint8) (*cipherSuite, int) {
	for i, supported := range supportedCipherSuites {
		if id == supported {
			var candidate *cipherSuite

			for _, s := range cipherSuites {
				if s.id == id {
					candidate = s
					break
				}
			}
			if candidate == nil {
				continue
			}
			// Don't select a ciphersuite which we can't
			// support for this client.
			if (candidate.flags&suiteECDHE != 0) && !ellipticOk {
				continue
			}
			if (candidate.flags&suiteECDSA != 0) != ecdsaOk {
				continue
			}
			if version < VersionTLS12 && candidate.flags&suiteTLS12 != 0 {
				continue
			}
			if candidate.flags&suiteChacha20 != 0 && !chachaOk {
				continue
			}
			if candidate.flags&suiteRC4 != 0 && useRC4 == disableRC4 {
				continue
			}
			if candidate.flags&suiteRC4 == 0 && useRC4 == onlyRC4 {
				continue
			}

			return candidate, i
		}
	}

	return nil, 0
}

func IsEcdheCipherSuite(suite interface{}) bool {
	if s, ok := suite.(*cipherSuite); ok {
		return (s.flags & suiteECDHE) != 0
	}
	return false
}

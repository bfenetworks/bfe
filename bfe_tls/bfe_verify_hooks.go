// Copyright (c) 2025 The BFE Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package bfe_tls

import (
	"crypto/x509"
	"fmt"
	"net"
	"strings"
	"unicode/utf8"

	"github.com/baidu/go-lib/log"
)

type VerifyCertHook func(insecureSkipVerify bool, host string, certificates []*x509.Certificate, cas *x509.CertPool) error

type VerifyPeerCertHooks struct {
	InsecureSkipVerify bool
	Host               string
	certificates       []*x509.Certificate
	CAs                *x509.CertPool
	verifyFn           []VerifyCertHook
	disableDef         bool
}

func toLowerCaseASCII(in string) string {
	// If the string is already lower-case then there's nothing to do.
	isAlreadyLowerCase := true
	for _, c := range in {
		if c == utf8.RuneError {
			// If we get a UTF-8 error then there might be
			// upper-case ASCII bytes in the invalid sequence.
			isAlreadyLowerCase = false
			break
		}
		if 'A' <= c && c <= 'Z' {
			isAlreadyLowerCase = false
			break
		}
	}

	if isAlreadyLowerCase {
		return in
	}

	out := []byte(in)
	for i, c := range out {
		if 'A' <= c && c <= 'Z' {
			out[i] += 'a' - 'A'
		}
	}
	return string(out)
}
func matchExactly(hostA, hostB string) bool {
	if hostA == "" || hostA == "." || hostB == "" || hostB == "." {
		return false
	}
	return toLowerCaseASCII(hostA) == toLowerCaseASCII(hostB)
}

func matchHostnames(pattern, host string) bool {
	pattern = toLowerCaseASCII(pattern)
	host = toLowerCaseASCII(strings.TrimSuffix(host, "."))

	if len(pattern) == 0 || len(host) == 0 {
		return false
	}

	patternParts := strings.Split(pattern, ".")
	hostParts := strings.Split(host, ".")

	if len(patternParts) != len(hostParts) {
		return false
	}

	for i, patternPart := range patternParts {
		if i == 0 && patternPart == "*" {
			continue
		}
		if patternPart != hostParts[i] {
			return false
		}
	}
	return true
}

func validHostname(host string, isPattern bool) bool {
	if !isPattern {
		host = strings.TrimSuffix(host, ".")
	}
	if len(host) == 0 {
		return false
	}
	for i, part := range strings.Split(host, ".") {
		if part == "" {
			// Empty label.
			return false
		}
		if isPattern && i == 0 && part == "*" {
			// Only allow full left-most wildcards, as those are the only ones
			// we match, and matching literal '*' characters is probably never
			// the expected behavior.
			continue
		}
		for j, c := range part {
			if 'a' <= c && c <= 'z' {
				continue
			}
			if '0' <= c && c <= '9' {
				continue
			}
			if 'A' <= c && c <= 'Z' {
				continue
			}
			if c == '-' && j != 0 {
				continue
			}
			if c == '_' {
				// Not a valid character in hostnames, but commonly
				// found in deployments outside the WebPKI.
				continue
			}
			return false
		}
	}
	return true
}
func verifyIpFn(c *x509.Certificate, h string) error {
	candidateIP := h
	if len(h) >= 3 && h[0] == '[' && h[len(h)-1] == ']' {
		candidateIP = h[1 : len(h)-1]
	}
	if ip := net.ParseIP(candidateIP); ip != nil {
		// We only match IP addresses against IP SANs.
		// See RFC 6125, Appendix B.2.
		for _, candidate := range c.IPAddresses {
			if ip.Equal(candidate) {
				return nil
			}
		}
		return x509.HostnameError{Certificate: c, Host: candidateIP}
	}
	return nil
}

// Only allow full left-most wildcards
func verifyHost(c *x509.Certificate, h string) error {
	// Verify : SAN
	names := c.DNSNames
	if validHostname(c.Subject.CommonName, true) {
		// Verify : CN
		names = append(names, c.Subject.CommonName)
	}
	candidateName := toLowerCaseASCII(h) // Save allocations inside the loop.
	validCandidateName := validHostname(candidateName, false)
	for _, match := range names {
		if validCandidateName && validHostname(match, true) {
			if matchHostnames(match, candidateName) {
				return nil
			}
		} else {
			if matchExactly(match, candidateName) {
				return nil
			}
		}
	}
	return x509.HostnameError{Certificate: c, Host: h}
}

// NewVerifyPeerCertHooks : build hooks for bef_tls.Config.VerifyPeerCertificate and wait callback
func NewVerifyPeerCertHooks(insecureSkipVerify bool, host string, cas *x509.CertPool) *VerifyPeerCertHooks {
	return &VerifyPeerCertHooks{
		InsecureSkipVerify: insecureSkipVerify,
		Host:               host,
		CAs:                cas,
		verifyFn:           make([]VerifyCertHook, 0),
	}
}

func (p *VerifyPeerCertHooks) DisableDefaultHooks() *VerifyPeerCertHooks {
	p.disableDef = true
	return p
}

// Ready : ready to wait tls callback
func (p *VerifyPeerCertHooks) Ready() func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if !p.disableDef {
		p.verifyFn = append([]VerifyCertHook{defVerifyCA, defVerifyHost}, p.verifyFn...)
	}
	return p.verifyPeerCertificate
}

func (p *VerifyPeerCertHooks) verifyPeerCertificate(rawCerts [][]byte, _ [][]*x509.Certificate) error {

	var certificates []*x509.Certificate
	for _, rawCert := range rawCerts {
		cert, err := x509.ParseCertificate(rawCert)
		if err != nil {
			return err
		}
		certificates = append(certificates, cert)
	}
	p.certificates = certificates
	return p.apply()
}

func (p *VerifyPeerCertHooks) AppendHook(fn VerifyCertHook) *VerifyPeerCertHooks {
	p.verifyFn = append(p.verifyFn, fn)
	return p
}

func (p *VerifyPeerCertHooks) apply() error {
	if !p.InsecureSkipVerify {
		for _, fn := range p.verifyFn {
			if err := fn(p.InsecureSkipVerify, p.Host, p.certificates, p.CAs); err != nil {
				return err
			}
		}
	}
	return nil
}

// defVerifyCA : verify cert by cas
func defVerifyCA(insecureSkipVerify bool, _ string, certificates []*x509.Certificate, cas *x509.CertPool) (err error) {
	if !insecureSkipVerify {
		for _, cert := range certificates {
			if _, err = cert.Verify(x509.VerifyOptions{
				Roots: cas,
			}); err != nil {
				log.Logger.Debug("err=%s", err.Error())
				return err
			}
			log.Logger.Debug("HTTPS-Verify-CA-Success: %s", cert.Subject.String())
		}
	}
	return nil
}

// defVerifyHost : verify host over CN and SAN
func defVerifyHost(insecureSkipVerify bool, host string, certificates []*x509.Certificate, _ *x509.CertPool) (err error) {
	if !insecureSkipVerify {
		for _, cert := range certificates {
			var (
				err         error
				candidateIP = host
			)
			if len(host) >= 3 && host[0] == '[' && host[len(host)-1] == ']' {
				candidateIP = host[1 : len(host)-1]
			}
			if ip := net.ParseIP(candidateIP); ip != nil {
				err = verifyIpFn(cert, host)
			} else {
				err = verifyHost(cert, host)
			}
			if err == nil {
				return nil
			}
			log.Logger.Debug("debug_https not_match host=%s, sn=%s, cn=%s, san=%v, err=%v", host, cert.SerialNumber.String(), cert.Subject.CommonName, cert.DNSNames, err)
		}
		err = fmt.Errorf("host=%s not match CN/SAN", host)
		return err
	}
	return nil
}

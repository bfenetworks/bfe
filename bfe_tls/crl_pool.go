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

package bfe_tls

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
)

var (
	oidExtensionAuthorityKeyId = []int{2, 5, 29, 35}
)

// RFC 5280,  4.2.1.1
type authKeyId struct {
	Id []byte `asn1:"optional,tag:0"`
}

func unmarshalAuthKeyId(value []byte) (authKeyId, error) {
	var a authKeyId
	if rest, err := asn1.Unmarshal(value, &a); err != nil {
		return a, err
	} else if len(rest) != 0 {
		return a, errors.New("x509: trailing data after X.509 authority key-id")
	}
	return a, nil
}

type CRLPool struct {
	byIssuerAndSerial map[string]pkix.RevokedCertificate
}

func NewCRLPool() *CRLPool {
	crlPool := new(CRLPool)
	crlPool.byIssuerAndSerial = make(map[string]pkix.RevokedCertificate)
	return crlPool
}

func getAuthorityKeyId(crl *pkix.CertificateList) (authKeyId, error) {
	var err error
	var a authKeyId

	if crl == nil {
		return a, fmt.Errorf("crl is nil")
	}

	for _, exten := range crl.TBSCertList.Extensions {
		if exten.Id.Equal(oidExtensionAuthorityKeyId) {
			a, err = unmarshalAuthKeyId(exten.Value)
			if err != nil {
				return a, err
			}
		}
	}

	return a, nil
}

func (p *CRLPool) AddCRL(crl *pkix.CertificateList) error {
	if crl == nil {
		return errors.New("add nil CertificateList to CRLPool")
	}

	a, err := getAuthorityKeyId(crl)
	if err != nil {
		return err
	}

	if len(a.Id) == 0 {
		return fmt.Errorf("AuthorityKeyId not set in crl")
	}

	for _, revokedCert := range crl.TBSCertList.RevokedCertificates {
		p.byIssuerAndSerial[genCRLPoolKey(a.Id, revokedCert.SerialNumber)] = revokedCert
	}
	return nil
}

func genCRLPoolKey(authorityKeyId []byte, serialNum *big.Int) string {
	var buf bytes.Buffer
	buf.WriteString(hex.EncodeToString(authorityKeyId))
	buf.WriteString("_")
	buf.WriteString(serialNum.Text(16))
	return buf.String()
}

func (p *CRLPool) CheckCertRevoked(cert *x509.Certificate) bool {
	if _, ok := p.byIssuerAndSerial[genCRLPoolKey(cert.AuthorityKeyId, cert.SerialNumber)]; ok {
		return true
	}
	return false
}

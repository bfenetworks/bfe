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
	"encoding/pem"
	"fmt"
	"testing"
)

func TestKeysDecode(t *testing.T) {
	chain := append(BFE_R_CA_CRT.Bytes(), BFE_I_CA_CRT.Bytes()...)

	// 解码 PEM 数据
	var certs []*x509.Certificate
	block, rest := pem.Decode(chain)
	for block != nil {
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				fmt.Println("cannot parse certificate", err)
				return
			}
			certs = append(certs, cert)
			t.Log(cert.Subject)
		}
		if len(rest) > 0 {
			block, rest = pem.Decode(rest)
		} else {
			break
		}
	}

}

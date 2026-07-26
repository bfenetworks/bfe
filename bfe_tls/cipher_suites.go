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
	"fmt"
	crypto_tls "crypto/tls"
)

// Cipher suite constants — re-exported from crypto/tls for backward compatibility.
const (
	TLS_RSA_WITH_RC4_128_SHA                = crypto_tls.TLS_RSA_WITH_RC4_128_SHA
	TLS_RSA_WITH_3DES_EDE_CBC_SHA           = crypto_tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA
	TLS_RSA_WITH_AES_128_CBC_SHA            = crypto_tls.TLS_RSA_WITH_AES_128_CBC_SHA
	TLS_RSA_WITH_AES_256_CBC_SHA            = crypto_tls.TLS_RSA_WITH_AES_256_CBC_SHA
	TLS_RSA_WITH_AES_128_CBC_SHA256         = crypto_tls.TLS_RSA_WITH_AES_128_CBC_SHA256
	TLS_RSA_WITH_AES_128_GCM_SHA256         = crypto_tls.TLS_RSA_WITH_AES_128_GCM_SHA256
	TLS_RSA_WITH_AES_256_GCM_SHA384         = crypto_tls.TLS_RSA_WITH_AES_256_GCM_SHA384
	TLS_ECDHE_ECDSA_WITH_RC4_128_SHA        = crypto_tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA
	TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA    = crypto_tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA
	TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA    = crypto_tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA
	TLS_ECDHE_RSA_WITH_RC4_128_SHA          = crypto_tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA
	TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA     = crypto_tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA
	TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA      = crypto_tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA
	TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA      = crypto_tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA
	TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256 = crypto_tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256
	TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256   = crypto_tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256
	TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256   = crypto_tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
	TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256 = crypto_tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
	TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384   = crypto_tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
	TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384 = crypto_tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
	TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256   = crypto_tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
	TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256 = crypto_tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
	// TLS 1.3 cipher suites (handled automatically by crypto/tls, listed here for reference)
	TLS_AES_128_GCM_SHA256       = crypto_tls.TLS_AES_128_GCM_SHA256
	TLS_AES_256_GCM_SHA384       = crypto_tls.TLS_AES_256_GCM_SHA384
	TLS_CHACHA20_POLY1305_SHA256 = crypto_tls.TLS_CHACHA20_POLY1305_SHA256
)

// gradeMinVersion maps a Grade to the minimum TLS version for crypto/tls.Config.
func gradeMinVersion(grade string) uint16 {
	switch grade {
	case GradeAPlus, GradeA:
		return crypto_tls.VersionTLS12
	case GradeB:
		return crypto_tls.VersionTLS12
	default: // GradeC
		return crypto_tls.VersionTLS12
	}
}

// gradeCipherSuites returns the cipher suites (TLS ≤1.2) for a Grade.
// TLS 1.3 suites are always included automatically by crypto/tls.
func gradeCipherSuites(grade string, chacha20 bool) []uint16 {
	base := []uint16{
		TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	}
	if chacha20 {
		base = append(base,
			TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		)
	}
	switch grade {
	case GradeAPlus, GradeA:
		return base
	case GradeB:
		return append(base,
			TLS_RSA_WITH_AES_128_GCM_SHA256,
			TLS_RSA_WITH_AES_256_GCM_SHA384,
			TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		)
	default: // GradeC — permit broader set for legacy clients
		return append(base,
			TLS_RSA_WITH_AES_128_GCM_SHA256,
			TLS_RSA_WITH_AES_256_GCM_SHA384,
			TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			TLS_RSA_WITH_AES_128_CBC_SHA,
			TLS_RSA_WITH_AES_256_CBC_SHA,
			TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		)
	}
}


// CipherSuiteText returns a human-readable name for a TLS cipher suite ID.
func CipherSuiteText(suite uint16) string {
	for _, cs := range crypto_tls.CipherSuites() {
		if cs.ID == suite {
			return cs.Name
		}
	}
	for _, cs := range crypto_tls.InsecureCipherSuites() {
		if cs.ID == suite {
			return cs.Name
		}
	}
	return fmt.Sprintf("TLS_CIPHER_SUITE_%04x", suite)
}

// CipherSuiteTextForOpenSSL returns an OpenSSL-style name for a cipher suite.
// Falls back to the standard name if no OpenSSL alias is known.
var cipherSuiteOpenSSLNames = map[uint16]string{
	TLS_RSA_WITH_RC4_128_SHA:                "RC4-SHA",
	TLS_RSA_WITH_3DES_EDE_CBC_SHA:           "DES-CBC3-SHA",
	TLS_RSA_WITH_AES_128_CBC_SHA:            "AES128-SHA",
	TLS_RSA_WITH_AES_256_CBC_SHA:            "AES256-SHA",
	TLS_RSA_WITH_AES_128_GCM_SHA256:         "AES128-GCM-SHA256",
	TLS_RSA_WITH_AES_256_GCM_SHA384:         "AES256-GCM-SHA384",
	TLS_ECDHE_ECDSA_WITH_RC4_128_SHA:        "ECDHE-ECDSA-RC4-SHA",
	TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA:    "ECDHE-ECDSA-AES128-SHA",
	TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA:    "ECDHE-ECDSA-AES256-SHA",
	TLS_ECDHE_RSA_WITH_RC4_128_SHA:          "ECDHE-RSA-RC4-SHA",
	TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA:     "ECDHE-RSA-DES-CBC3-SHA",
	TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA:      "ECDHE-RSA-AES128-SHA",
	TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA:      "ECDHE-RSA-AES256-SHA",
	TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:   "ECDHE-RSA-AES128-GCM-SHA256",
	TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256: "ECDHE-ECDSA-AES128-GCM-SHA256",
	TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:   "ECDHE-RSA-AES256-GCM-SHA384",
	TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384: "ECDHE-ECDSA-AES256-GCM-SHA384",
	TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256:   "ECDHE-RSA-CHACHA20-POLY1305",
	TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256: "ECDHE-ECDSA-CHACHA20-POLY1305",
	TLS_AES_128_GCM_SHA256:       "TLS_AES_128_GCM_SHA256",
	TLS_AES_256_GCM_SHA384:       "TLS_AES_256_GCM_SHA384",
	TLS_CHACHA20_POLY1305_SHA256: "TLS_CHACHA20_POLY1305_SHA256",
}

func CipherSuiteTextForOpenSSL(suite uint16) string {
	if name, ok := cipherSuiteOpenSSLNames[suite]; ok {
		return name
	}
	return CipherSuiteText(suite)
}

// Copyright (c) 2026 The BFE Authors.
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

package bfe_util

import (
	"crypto/tls"
	"fmt"
)

// cipher suite names in OpenSSL format
// See: https://www.openssl.org/docs/man1.0.2/apps/ciphers.html
var cipherSuiteTextMapForOpenSSL = map[uint16]string{
	tls.TLS_RSA_WITH_RC4_128_SHA:                    "RC4-SHA",
	tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA:               "DES-CBC3-SHA",
	tls.TLS_RSA_WITH_AES_128_CBC_SHA:                "AES128-SHA",
	tls.TLS_RSA_WITH_AES_256_CBC_SHA:                "AES256-SHA",
	tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA:            "ECDHE-ECDSA-RC4-SHA",
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA:        "ECDHE-ECDSA-AES128-SHA",
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA:        "ECDHE-ECDSA-AES256-SHA",
	tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA:              "ECDHE-RSA-RC4-SHA",
	tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA:         "ECDHE-RSA-DES-CBC3-SHA",
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA:          "ECDHE-RSA-AES128-SHA",
	tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA:          "ECDHE-RSA-AES256-SHA",
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:       "ECDHE-RSA-AES128-GCM-SHA256",
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256:     "ECDHE-ECDSA-AES128-GCM-SHA256",
	tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256: "ECDHE-RSA-CHACHA20-POLY1305",
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256: "ECDHE-ECDSA-CHACHA20-POLY1305",
	tls.TLS_AES_128_GCM_SHA256:                      "TLS_AES_128_GCM_SHA256",
	tls.TLS_AES_256_GCM_SHA384:                      "TLS_AES_256_GCM_SHA384",
	tls.TLS_CHACHA20_POLY1305_SHA256:                "TLS_CHACHA20_POLY1305_SHA256",
}

// CipherSuiteTextForOpenSSL returns the OpenSSL-style name of given cipher suite
func CipherSuiteTextForOpenSSL(suite uint16) string {
	if text, ok := cipherSuiteTextMapForOpenSSL[suite]; ok {
		return text
	}
	return fmt.Sprintf("TLS_CIPHER_SUITE_%x", suite)
}

// version text in OpenSSL format
var versionTextMapForOpenSSL = map[uint16]string{
	tls.VersionSSL30: "SSLv3.0",
	tls.VersionTLS10: "TLSv1.0",
	tls.VersionTLS11: "TLSv1.1",
	tls.VersionTLS12: "TLSv1.2",
	tls.VersionTLS13: "TLSv1.3",
}

// VersionTextForOpenSSL returns the OpenSSL-style text of given tls version
func VersionTextForOpenSSL(ver uint16) string {
	if text, ok := versionTextMapForOpenSSL[ver]; ok {
		return text
	}
	return fmt.Sprintf("TLS_VERSION_%x", ver)
}

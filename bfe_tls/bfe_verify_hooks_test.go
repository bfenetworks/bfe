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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"
)

// 创建 CN 证书
func createCert(sn int64, cn string) *x509.Certificate {
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(sn),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
	}
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub := &priv.PublicKey
	certBytes, _ := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	cert, _ := x509.ParseCertificate(certBytes)
	return cert
}

// 创建带 SAN 的证书
func createCertWithSAN(sn int64, dnsNames []string, ips []net.IP) *x509.Certificate {
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(sn),
		Subject:               pkix.Name{},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ips,
	}
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub := &priv.PublicKey
	certBytes, _ := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	cert, _ := x509.ParseCertificate(certBytes)
	return cert
}

func TestDefVerifyHost(t *testing.T) {
	// _ = log.Init(fmt.Sprintf("test_%s", time.Now().String()), "DEBUG", "/tmp", true, "M", 5)
	type testcase struct {
		host     string
		cert     *x509.Certificate
		assertOk bool
	}

	var (
		cnCert0 = createCert(1, "example.com")
		cnCert1 = createCert(2, "*.example.com")
		sanCert = createCertWithSAN(3,
			[]string{"www.example.com", "*.example.org"},
			[]net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("192.168.0.1")},
		)
		_testcase = map[string]testcase{
			"cn0_assert_ok":             {"example.com", cnCert0, true},
			"cn0_assert_1_fail":         {"login.example.com", cnCert0, false},
			"cn0_assert_2_fail":         {"www.example.com", cnCert0, false},
			"cn1_assert_wildcard1_ok":   {"login.example.com", cnCert1, true},
			"cn1_assert_wildcard2_ok":   {"www.example.com", cnCert1, true},
			"cn1_assert_wildcard4_fail": {"example.com", cnCert1, false},
			"cn1_assert_wildcard5_fail": {"example.org", cnCert1, false},
			"cn1_assert_wildcard6_fail": {"www.login.example.org", cnCert1, false},

			"san_assert_wildcard1_fail": {"example.com", sanCert, false},
			"san_assert_wildcard2_fail": {"login.example.com", sanCert, false},
			"san_assert_wildcard3_ok":   {"www.example.com", sanCert, true},
			"san_assert_wildcard4_ok":   {"login.example.org", sanCert, true},
			"san_assert_wildcard5_ok":   {"www.example.org", sanCert, true},
			"san_assert_wildcard6_fail": {"example.org", sanCert, false},

			"san_assert_wildcard7_ok":   {"127.0.0.1", sanCert, true},
			"san_assert_wildcard8_ok":   {"192.168.0.1", sanCert, true},
			"san_assert_wildcard9_fail": {"192.168.0.2", sanCert, false},
		}
		fn = func(tc testcase) func(t *testing.T) {
			return func(t *testing.T) {
				err := defVerifyHost(false, tc.host, []*x509.Certificate{tc.cert}, nil)
				if tc.assertOk && err != nil {
					t.Error(err)
				} else if !tc.assertOk && err == nil {
					t.Error("assertOk=false, err==nil")
				}
			}
		}
	)
	// 测试验证
	for name, val := range _testcase {
		t.Run(name, fn(val))
	}
	time.Sleep(time.Second)
}

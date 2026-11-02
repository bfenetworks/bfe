// Copyright (c) 2025 The BFE Authors.
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

/*
action_variable_inner.go - built-in variable value fetchers (header/ssl/client info)
*/
package action

import (
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_tls"
	"github.com/bfenetworks/bfe/bfe_util"
)

type HeaderValueHandler func(req *bfe_basic.Request) string

const (
	UNKNOWN = "unknown"
)

var (
	VariableHandlers = map[string]HeaderValueHandler{
		// for client
		"bfe_clientip":          getClientIp,
		"bfe_clientport":        getClientPort,
		"bfe_clientip_location": getClientIpLocationName,

		// for connection info
		"bfe_cip":         getClientIp, // client ip (alias for bfe_clientip)
		"bfe_vip":         getBfeVip,   // virtual ip
		"bfe_bip":         getBfeBip,   // balancer ip
		"bfe_rip":         getBfeRip,   // bfe ip
		"bfe_domain_auto": getBfeDomainAuto,
		"session_id":      getSessionId,
		"bfe_logid":       getBfeLogId, // bfe logid

		// for bfe
		"bfe_server_name": getBfeServerName,

		// for backend
		"bfe_cluster":      getBfeCluster,
		"bfe_backend_info": getBfeBackendInfo,

		// for tls
		"bfe_ssl_info":                          getBfeSslInfo,
		"bfe_ssl_resume":                        getBfeSslResume,
		"bfe_ssl_cipher":                        getBfeSslCipher,
		"bfe_ssl_version":                       getBfeSslVersion,
		"bfe_ssl_ja3_raw":                       getBfeSslJa3Raw,
		"bfe_ssl_ja3_hash":                      getBfeSslJa3Hash,
		"bfe_protocol":                          getBfeProtocol,
		"client_cert_serial_number":             getClientCertSerialNumber,
		"client_cert_subject_title":             getClientCertSubjectTitle,
		"client_cert_subject_common_name":       getClientCertSubjectCommonName,
		"client_cert_subject_organization":      getClientCertSubjectOrganization,
		"client_cert_subject_organization_unit": getClientCertSubjectOrganizationalUnit,
		"client_cert_subject_province":          getClientCertSubjectProvince,
		"client_cert_subject_country":           getClientCertSubjectCountry,
		"client_cert_subject_locality":          getClientCertSubjectLocality,
		"client_cert_adu_car_role":              getClientCertAduCarRole,
		"client_cert_adu_reserve":               getClientCertAduReserve,
		"client_cert_pem":                       getClientCertPEM,
		"client_cert_subject_string":            getClientCertSubjectString,
		"client_cert_issuer_string":             getClientCertIssuerString,
		"client_cert_pub_key_algo":              getClientCertPublicKeyAlgorithm,
		"client_cert_sig_algo":                  getClientCertSignatureAlgorithm,

		// for url related
		"req_uri":  getReqUrI,
		"req_path": getReqPath,
	}
)

func uint16ToStr(u16 uint16) string {
	b := make([]byte, 2)
	b[0] = byte(u16 >> 8)
	b[1] = byte(u16)

	return hex.EncodeToString(b)
}

// get clientip
func getClientIp(req *bfe_basic.Request) string {
	if req.ClientAddr == nil {
		return ""
	}
	return req.ClientAddr.IP.String()
}

// get client port
func getClientPort(req *bfe_basic.Request) string {
	if req.ClientAddr == nil {
		return ""
	}
	return strconv.Itoa(req.ClientAddr.Port)
}

// get clientip location
func getClientIpLocationName(req *bfe_basic.Request) string {
	tags := req.GetTags(bfe_basic.TagsIPLocation)
	if len(tags) == 0 {
		return ""
	}
	return tags[0]
}

// get request host
func getBfeDomainAuto(req *bfe_basic.Request) string {
	return req.HttpRequest.Host
}

func getProto(proto string) string {
	switch proto {
	case "spdy/2":
		return "20"
	case "spdy/3":
		return "30"
	case "spdy/3.1":
		return "31"
	case "h2":
		return "h2"
	case "coap":
		return "cp"
	case "coapx":
		return "cx"
	case "stream":
		return "st"
	default:
		return "00"
	}
}

func getReqTime(req *bfe_basic.Request) int {
	// when send request to backend, Stat.BackendEnd is not set yet,
	// diff is negative
	diff := req.Stat.BackendEnd.Sub(req.Stat.ReadReqStart)
	if diff <= 0 {
		return 0
	}

	return int(diff / 1000000)
}

func getConnReused(req *bfe_basic.Request) string {
	state := req.HttpRequest.State
	if state == nil {
		return "U" // unknown
	}
	if state.SerialNumber == 1 {
		return "N"
	}
	return "R"
}

func getConnResume(state *bfe_tls.ConnectionState) string {
	if !state.DidResume {
		return "N"
	} else {
		return "R"
	}
	//} else if !state.DidZeroRTT {
	//	return "R"
	//} else {
	//	return "Z"
	//}
}

// For definition of BFE_SSL_INFO, see
// http://wiki.baidu.com/pages/viewpage.action?pageId=142386561 Section 2.4
func getBfeSslInfo(req *bfe_basic.Request) string {
	if req.Session.TlsState == nil {
		return ""
	}

	state := req.Session.TlsState
	qid := req.LogId                                             // same as request's logid
	proto := getProto(state.NegotiatedProtocol)                  // tls application protocol
	handshakeTime := int(state.HandshakeTime / time.Millisecond) // tls handshake time
	connResume := getConnResume(state)                           // use tls session resume
	connReused := getConnReused(req)                             // reuse conn
	reqTime := getReqTime(req)                                   // request time
	sslVer := uint16ToStr(state.Version)                         // tls version
	cipherSuite := uint16ToStr(state.CipherSuite)                // tls ciphersuite

	return fmt.Sprintf("%s_%s_%d_%s_%s_%d_%s_%s_Y", qid, proto,
		handshakeTime, connResume, connReused, reqTime, sslVer, cipherSuite)
}

func getBfeSslResume(req *bfe_basic.Request) string {
	if req.Session.TlsState == nil {
		return ""
	}

	state := req.Session.TlsState
	return getConnResume(state)
}

// get tls cipher suite
func getBfeSslCipher(req *bfe_basic.Request) string {
	if req.Session.TlsState == nil {
		return ""
	}

	state := req.Session.TlsState
	return bfe_tls.CipherSuiteTextForOpenSSL(state.CipherSuite)
}

// get tls version
func getBfeSslVersion(req *bfe_basic.Request) string {
	if req.Session.TlsState == nil {
		return ""
	}

	state := req.Session.TlsState
	return bfe_tls.VersionTextForOpenSSL(state.Version)
}

// get tls ja3 string
func getBfeSslJa3Raw(req *bfe_basic.Request) string {
	if req.Session.TlsState == nil {
		return ""
	}

	state := req.Session.TlsState
	return state.JA3Raw
}

// get tls ja3 hash
func getBfeSslJa3Hash(req *bfe_basic.Request) string {
	if req.Session.TlsState == nil {
		return ""
	}

	state := req.Session.TlsState
	return state.JA3Hash
}

// get protocol for application level
func getBfeProtocol(req *bfe_basic.Request) string {
	return req.Protocol()
}

// get client cert
func getClientCert(req *bfe_basic.Request) *x509.Certificate {
	tlsState := req.Session.TlsState
	if tlsState == nil {
		return nil
	}
	if len(tlsState.PeerCertificates) < 1 {
		return nil
	}
	return tlsState.PeerCertificates[0]
}

var (
	oidTitle      = asn1.ObjectIdentifier{2, 5, 4, 12}
	oidAduCarRole = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 16604}
	oidAduReserve = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 16605}
)

// get value of cert extension
func getCertExtVal(cert *x509.Certificate, oid asn1.ObjectIdentifier) []byte {
	for _, extn := range cert.Extensions {
		if extn.Id.Equal(oid) {
			return extn.Value
		}
	}
	return nil
}

func getCertPem(cert *x509.Certificate) []byte {
	certDerBytes := cert.Raw
	certBlock := pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDerBytes,
	}
	b := pem.EncodeToMemory(&certBlock)
	return b
}

// get PEM text of client cert
func getClientCertPEM(req *bfe_basic.Request) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}
	s := string(getCertPem(clientCert))
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.TrimSuffix(strings.TrimPrefix(s, "-----BEGIN CERTIFICATE-----"), "-----END CERTIFICATE-----")
	return s
}

// get subject string of client cert
func getClientCertSubjectString(req *bfe_basic.Request) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}

	return clientCert.Subject.String()
}

// get issuer string of client cert
func getClientCertIssuerString(req *bfe_basic.Request) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}

	return clientCert.Issuer.String()
}

// get public key algorithm of client cert
func getClientCertPublicKeyAlgorithm(req *bfe_basic.Request) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}

	return clientCert.PublicKeyAlgorithm.String()
}

// get signature algorithm of client cert
func getClientCertSignatureAlgorithm(req *bfe_basic.Request) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}

	return clientCert.SignatureAlgorithm.String()
}

// get serial number of client cert
func getClientCertSerialNumber(req *bfe_basic.Request) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}
	return clientCert.SerialNumber.String()
}

// get subject title of client cert
func getClientCertSubjectTitle(req *bfe_basic.Request) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}

	subject := clientCert.Subject
	for _, name := range subject.Names {
		if !name.Type.Equal(oidTitle) {
			continue
		}
		if val, ok := name.Value.(string); ok {
			return val
		}
	}
	return ""
}

// get subject common name of client cert
func getClientCertSubjectCommonName(req *bfe_basic.Request) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}

	return clientCert.Subject.CommonName
}

// get subject organization of client cert
func getClientCertSubjectOrganization(req *bfe_basic.Request) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}

	if len(clientCert.Subject.Organization) > 0 {
		return clientCert.Subject.Organization[0]
	}
	return ""
}

func getClientCertSubjectOrganizationalUnit(req *bfe_basic.Request) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}

	if len(clientCert.Subject.OrganizationalUnit) > 0 {
		return clientCert.Subject.OrganizationalUnit[0]
	}

	return ""
}

func getClientCertSubjectProvince(req *bfe_basic.Request) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}

	if len(clientCert.Subject.Province) > 0 {
		return clientCert.Subject.Province[0]
	}

	return ""
}

func getClientCertSubjectCountry(req *bfe_basic.Request) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}

	if len(clientCert.Subject.Country) > 0 {
		return clientCert.Subject.Country[0]
	}

	return ""
}

func getClientCertSubjectLocality(req *bfe_basic.Request) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}

	if len(clientCert.Subject.Locality) > 0 {
		return clientCert.Subject.Locality[0]
	}

	return ""
}

func getClientCertExtVal(req *bfe_basic.Request, oid asn1.ObjectIdentifier) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}

	extnVal := getCertExtVal(clientCert, oid)
	if extnVal == nil {
		return "nil"
	}

	return hex.EncodeToString(extnVal)
}

// get AduCarRole extn of client cert
func getClientCertAduCarRole(req *bfe_basic.Request) string {
	return getClientCertExtVal(req, oidAduCarRole)
}

// get AduReserve extn of client cert
func getClientCertAduReserve(req *bfe_basic.Request) string {
	return getClientCertExtVal(req, oidAduReserve)
}

func getBfeCluster(req *bfe_basic.Request) string {
	return req.Route.ClusterName
}

func getAddressFetcher(conn net.Conn) bfe_util.AddressFetcher {
	if c, ok := conn.(*bfe_tls.Conn); ok {
		conn = c.GetNetConn()
	}
	if f, ok := conn.(bfe_util.AddressFetcher); ok {
		return f
	}
	return nil
}

func getBfeVip(req *bfe_basic.Request) string {
	f := getAddressFetcher(req.Session.Connection)
	if f == nil {
		return UNKNOWN
	}

	vaddr := f.VirtualAddr()
	if vaddr == nil {
		return UNKNOWN
	}
	vip, _, err := net.SplitHostPort(vaddr.String())
	if err != nil { /* never come here */
		return UNKNOWN
	}

	return vip
}

func getBfeBip(req *bfe_basic.Request) string {
	f := getAddressFetcher(req.Session.Connection)
	if f == nil {
		return UNKNOWN
	}

	baddr := f.BalancerAddr()
	if baddr == nil {
		return UNKNOWN
	}
	bip, _, err := net.SplitHostPort(baddr.String())
	if err != nil { /* never come here */
		return UNKNOWN
	}

	return bip
}

func getBfeRip(req *bfe_basic.Request) string {
	conn := req.Session.Connection
	raddr := conn.LocalAddr()
	rip, _, err := net.SplitHostPort(raddr.String())
	if err != nil { /* never come here */
		return UNKNOWN
	}

	return rip
}

func getBfeBackendInfo(req *bfe_basic.Request) string {
	return fmt.Sprintf("ClusterName:%s,SubClusterName:%s,BackendName:%s(%s)",
		req.Backend.ClusterName, req.Backend.SubclusterName,
		req.Backend.BackendName, req.Backend.BackendAddr)
}

func getBfeServerName(req *bfe_basic.Request) string {
	hostname, err := os.Hostname()
	if err != nil {
		return UNKNOWN
	}

	return hostname
}

func getSessionId(req *bfe_basic.Request) string {
	return req.Session.SessionId
}

func getBfeLogId(req *bfe_basic.Request) string {
	return req.LogId
}

func getReqUrI(req *bfe_basic.Request) string {
	if req == nil || req.HttpRequest == nil || req.HttpRequest.URL == nil {
		return ""
	}

	return req.HttpRequest.URL.RequestURI()
}

func getReqPath(req *bfe_basic.Request) string {
	if req == nil || req.HttpRequest == nil || req.HttpRequest.URL == nil {
		return ""
	}

	return req.HttpRequest.URL.Path
}

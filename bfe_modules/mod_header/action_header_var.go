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

package mod_header

import (
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_modules/mod_geo"
	"github.com/bfenetworks/bfe/bfe_tls"
	"github.com/bfenetworks/bfe/bfe_util"
)

type HeaderValueHandler func(req *bfe_basic.Request) string

const (
	Unknown = "unknown"
)

var VariableHandlers = map[string]HeaderValueHandler{
	// for client
	"bfe_client_ip":    getClientIp,
	"bfe_client_port":  getClientPort,
	"bfe_request_host": getRequestHost,

	// for conn info
	"bfe_session_id": getSessionId,
	"bfe_log_id":     getLogId,
	"bfe_cip":        getClientIp, // client ip (alias for bfe_clientip)
	"bfe_vip":        getBfeVip,   // virtual ip
	"bfe_bip":        getBfeBip,   // balancer ip
	"bfe_rip":        getBfeRip,   // bfe ip

	// for bfe
	"bfe_server_name": getBfeServerName,

	// for backend
	"bfe_cluster":      getBfeCluster,
	"bfe_backend_info": getBfeBackendInfo,

	// for tls
	"bfe_ssl_resume":                          getBfeSslResume,
	"bfe_ssl_cipher":                          getBfeSslCipher,
	"bfe_ssl_version":                         getBfeSslVersion,
	"bfe_ssl_ja3_raw":                         getBfeSslJa3Raw,
	"bfe_ssl_ja3_hash":                        getBfeSslJa3Hash,
	"bfe_protocol":                            getBfeProtocol,
	"client_cert_serial_number":               getClientCertSerialNumber,
	"client_cert_subject_title":               getClientCertSubjectTitle,
	"client_cert_subject_common_name":         getClientCertSubjectCommonName,
	"client_cert_subject_organization":        getClientCertSubjectOrganization,
	"client_cert_subject_organizational_unit": getClientCertSubjectOrganizationalUnit,
	"client_cert_subject_province":            getClientCertSubjectProvince,
	"client_cert_subject_country":             getClientCertSubjectCountry,
	"client_cert_subject_locality":            getClientCertSubjectLocality,

	// for http2
	"bfe_http2_fingerprint": getBfeHTTP2Fingerprint,

	// for geo
	"bfe_client_geo_country_iso_code":     getClientGeoCountryIsoCode,
	"bfe_client_geo_subdivision_iso_code": getClientGeoSubdivisionIsoCode,
	"bfe_client_geo_city_name":            getClientGeoCityName,
	"bfe_client_geo_latitude":             getClientGeoLatitude,
	"bfe_client_geo_longitude":            getClientGeoLongitude,
}

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

// get request host
func getRequestHost(req *bfe_basic.Request) string {
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
	}
	return "R"
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
	oidTitle = asn1.ObjectIdentifier{2, 5, 4, 12}
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

func getClientCertSubjectCommonName(req *bfe_basic.Request) string {
	clientCert := getClientCert(req)
	if clientCert == nil {
		return ""
	}

	return clientCert.Subject.CommonName
}

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

func getBfeCluster(req *bfe_basic.Request) string {
	return req.Route.ClusterName
}

func getBfeVip(req *bfe_basic.Request) string {
	if req.Session.Vip != nil {
		return req.Session.Vip.String()
	}

	return Unknown
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

func getBfeBip(req *bfe_basic.Request) string {
	f := getAddressFetcher(req.Session.Connection)
	if f == nil {
		return Unknown
	}

	baddr := f.BalancerAddr()
	if baddr == nil {
		return Unknown
	}
	bip, _, err := net.SplitHostPort(baddr.String())
	if err != nil { /* never come here */
		return Unknown
	}

	return bip
}

func getBfeRip(req *bfe_basic.Request) string {
	conn := req.Session.Connection
	raddr := conn.LocalAddr()
	rip, _, err := net.SplitHostPort(raddr.String())
	if err != nil { /* never come here */
		return Unknown
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
		return Unknown
	}

	return hostname
}

func getSessionId(req *bfe_basic.Request) string {
	return req.Session.SessionId
}

func getLogId(req *bfe_basic.Request) string {
	return req.LogId
}

func getClientGeoCountryIsoCode(req *bfe_basic.Request) string {
	countryIsoCode := req.GetContext(mod_geo.CtxCountryIsoCode)
	if countryIsoCode == nil {
		return ""
	}

	return countryIsoCode.(string)
}

func getClientGeoSubdivisionIsoCode(req *bfe_basic.Request) string {
	subdivisionIsoCode := req.GetContext(mod_geo.CtxSubdivisionIsoCode)
	if subdivisionIsoCode == nil {
		return ""
	}

	return subdivisionIsoCode.(string)
}

func getClientGeoCityName(req *bfe_basic.Request) string {
	cityName := req.GetContext(mod_geo.CtxCityName)
	if cityName == nil {
		return ""
	}

	return cityName.(string)
}

func getClientGeoLatitude(req *bfe_basic.Request) string {
	latitude := req.GetContext(mod_geo.CtxLatitude)
	if latitude == nil {
		return ""
	}

	return latitude.(string)
}

func getClientGeoLongitude(req *bfe_basic.Request) string {
	longitude := req.GetContext(mod_geo.CtxLongitude)
	if longitude == nil {
		return ""
	}

	return longitude.(string)
}

func getBfeHTTP2Fingerprint(req *bfe_basic.Request) string {
	return req.HttpRequest.State.H2Fingerprint
}

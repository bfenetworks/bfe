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

package tls_rule_conf

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_tls_conf/server_cert_conf"
	"github.com/bfenetworks/bfe/bfe_tls"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

// Notes about `NextProtos`:
//  * NextProtos represents an ordered list of application level protocol items
//
//  * protocol item format: "protocol[;level=0;mcs=200;isw=65535;rate=100]"
//    - valid protocol should be h2, spdy/3.1, http/1.1, others are not supported
//
//    - level means protocol negotiation level and should be [0, 2]
//      + level is optional and is PROTO_OPTIONAL by default
//      + if level > PROTO_OPTIONAL, rate should be 100
//      + if level is PROTO_MANDATORY, NextProtos should not contain other items
//
//    - rate means presence rate for that protocol and should be [0,100]
//      + rate is optional and is 100 by default
//      + rate for http/1.1 should always be 100
//
//    - mcs means max concurrent streams per conn and should be > 0
//      + mcs is optional and is 200 by default
//
//    - isw means initial stream window size for server
// 	 + isw is optional and is 65535 by default
// 	 + valid isw should be [65535, 262144] for current implemention
//
// Notes about `SniConf`:
//  * SniConf represents an optional list of server names (hostname)
//  * When vip of incoming conn is missing or unknown:
//   - If SniConf is configured, server will select tls rule conf by name (from tls sni extension)
//   - Even through SniConf is not configured, server will try to select cert by name
//
// Notes about`ClientCAName`:
//  * The CA certificate file is <ClientCAName>.crt under ClientCABaseDir configured in bfe.conf

// application level protocols over tls
const (
	HTTP11 = "http/1.1" // https (http/1.1 over tls) protocol
	HTTP2  = "h2"       // http2 protocol
	SPDY31 = "spdy/3.1" // spdy/3.1 protocol
	STREAM = "stream"
)

var validNextProtos = []string{HTTP11, HTTP2, SPDY31, STREAM}

// negotiation level for protocols
const (
	PROTO_OPTIONAL    = 0 // proto is negotiatory, may be disabled if needed
	PROTO_NEGOTISTORY = 1 // proto is negotiatory, must not be disabled
	PROTO_MANDATORY   = 2 // proto is mandatory
)

type TlsRuleConf struct {
	VipConf       []string // list of vips for product
	SniConf       []string // list of hostnames for product (optional)
	CertName      string   // name of certificate
	NextProtos    []string // next protos over TLS
	Grade         string   // tls grade for product
	ClientAuth    bool     // require tls client auth
	ClientCAName  string   // client CA certificate name
	Chacha20      bool     // enable chacha20-poly1305 cipher suites
	DynamicRecord bool     // enable dynamic record size
}

type TlsRuleMap map[string]*TlsRuleConf // product -> pointer to tls rule conf

const (
	ProxyProtocolDisabled  = 0
	ProxyProtocolV1Enabled = 1
	ProxyProtocolV2Enabled = 2
)

type NextProtosParams struct {
	Level int // protocol negotiation level
	Mcs   int // max concurrent stream per conn
	Isw   int // initial stream window for server
	Rate  int // presence rate while level is PROTO_OPTIONAL
	PP    int // proxy protocol to backend, 0: disable, 1: enable v1 pp, 2: enable v2 pp
}

func GetDefaultNextProtosParams() NextProtosParams {
	return NextProtosParams{
		Level: PROTO_OPTIONAL,
		Mcs:   200,
		Isw:   65535,
		Rate:  100,
		PP:    0,
	}
}

type BfeTlsRuleConf struct {
	Version              string // version of config
	Config               TlsRuleMap
	DefaultNextProtos    []string
	DefaultChacha20      bool
	DefaultDynamicRecord bool
}

func TlsRuleConfCheck(conf *TlsRuleConf) error {
	if len(conf.CertName) == 0 {
		return fmt.Errorf("no CertName")
	}

	if err := checkNextProtos(conf.NextProtos); err != nil {
		return err
	}

	conf.Grade = strings.ToUpper(conf.Grade)
	if !checkGrade(conf) {
		return fmt.Errorf("invalid tls grade: %s, currently only A+,A,B,C supported", conf.Grade)
	}

	if conf.ClientAuth && len(conf.ClientCAName) == 0 {
		return fmt.Errorf("ClientAuth enabled, but ClientCAName is empty")
	}

	for i, vip := range conf.VipConf {
		vaddr := net.ParseIP(vip)
		if vaddr == nil {
			return fmt.Errorf("invalid vip (%d) %s", i, vip)
		}
		conf.VipConf[i] = vaddr.String()
	}

	return nil
}

func checkNextProtos(nextProtos []string) error {
	if len(nextProtos) == 0 {
		return nil
	}

	allProtos := make(map[string]bool)
	for _, proto := range nextProtos {
		if err := CheckValidProto(proto); err != nil {
			return fmt.Errorf("invalid proto (%s) in NextProtos: %s", proto, err)
		}
		if allProtos[proto] {
			return fmt.Errorf("found duplicated proto: %s", nextProtos)
		}
		allProtos[proto] = true
	}

	if checkMandatory(nextProtos) {
		if len(nextProtos) != 1 {
			return fmt.Errorf("should contain 1 protos if level=2 (eg. proto mandatory): %s", nextProtos)
		}
	} else {
		if !checkContainHTTP(nextProtos) {
			return fmt.Errorf("no \"http/1.1\" in nonempty NextProtos")
		}
	}

	return nil
}

func checkGrade(conf *TlsRuleConf) bool {
	if conf.Grade == "" {
		// set default grade to C
		conf.Grade = bfe_tls.GradeC
	}

	switch conf.Grade {
	case bfe_tls.GradeAPlus, bfe_tls.GradeA, bfe_tls.GradeB, bfe_tls.GradeC:
		return true
	default:
		return false
	}
}

func CheckValidProto(protoConf string) error {
	// parse proto and params
	proto, params, err := ParseNextProto(protoConf)
	if err != nil {
		return fmt.Errorf("proto format invalid: %s", err)
	}

	// check negotiation level
	if params.Level < PROTO_OPTIONAL || params.Level > PROTO_MANDATORY {
		return fmt.Errorf("proto level should be [0, 2]")
	}

	// check max concurrent requests
	if params.Mcs <= 0 {
		return fmt.Errorf("proto mcs should > 0")
	}

	// check initial stream window
	if params.Isw < 65535 || params.Isw > 262144 {
		return fmt.Errorf("proto isw should be [65535, 262144]")
	}

	// check presence rate
	if params.Rate < 0 || params.Rate > 100 {
		return fmt.Errorf("proto rate should be [0, 100]")
	}
	if params.Level > PROTO_OPTIONAL && params.Rate != 100 {
		return fmt.Errorf("proto rate should be 100 if level > 0")
	}
	if proto == HTTP11 && params.Rate != 100 {
		return fmt.Errorf("proto rate for http/1.1 should be 100")
	}

	// check proxy protocol
	if params.PP < ProxyProtocolDisabled || params.PP > ProxyProtocolV2Enabled {
		return fmt.Errorf("proto pp should be [%d, %d]", ProxyProtocolDisabled, ProxyProtocolV2Enabled)
	}
	if params.PP != ProxyProtocolDisabled && proto != STREAM {
		return fmt.Errorf("param pp is only available for %s proto", STREAM)
	}

	// check next proto
	for _, validProto := range validNextProtos {
		if proto == validProto {
			return nil
		}
	}
	return fmt.Errorf("proto not valid or not support")
}

func ParseNextProto(protoConf string) (proto string, params NextProtosParams, err error) {
	// Note: replace ';' separator by '&'
	// Go 1.17 refuse ';' in query string (see https://github.com/golang/go/issues/25192)
	// For forward compatibility, proto accept both "a=b;c=d" and "a=b&c=d" now.
	protoConf = strings.ReplaceAll(protoConf, ";", "&")

	items := strings.SplitN(protoConf, "&", 2)
	if len(items) == 1 { // eg: h2
		proto = protoConf
		params = GetDefaultNextProtosParams()
	} else if len(items) == 2 { // eg: h2;level=0;mcs=200;rate=100
		proto = items[0]
		params, err = parseProtoParams(items[1])
	} else {
		err = fmt.Errorf("empty next proto")
	}
	return
}

func parseProtoParams(protoConf string) (params NextProtosParams, err error) {
	params = GetDefaultNextProtosParams()

	conf, err := url.ParseQuery(protoConf)
	if err != nil {
		return params, fmt.Errorf("invalid proto params: %s", protoConf)
	}

	for key, vals := range conf {
		if len(vals) != 1 {
			return params, fmt.Errorf("invalid proto params: %s", protoConf)
		}

		switch key {
		case "level":
			if params.Level, err = strconv.Atoi(vals[0]); err != nil {
				return params, fmt.Errorf("invalid level: %s", vals[0])
			}
		case "mcs":
			if params.Mcs, err = strconv.Atoi(vals[0]); err != nil {
				return params, fmt.Errorf("invalid mcs: %s", vals[0])
			}
		case "isw":
			if params.Isw, err = strconv.Atoi(vals[0]); err != nil {
				return params, fmt.Errorf("invalid isw: %s", vals[0])
			}
		case "rate":
			if params.Rate, err = strconv.Atoi(vals[0]); err != nil {
				return params, fmt.Errorf("invalid rate: %s", vals[0])
			}
		case "pp":
			if params.PP, err = strconv.Atoi(vals[0]); err != nil {
				return params, fmt.Errorf("invalid pp: %s", vals[0])
			}
		default:
			return params, fmt.Errorf("unknown params: %s", key)
		}
	}
	return params, nil
}

func checkMandatory(nextProtos []string) bool {
	for _, protoConf := range nextProtos {
		_, params, _ := ParseNextProto(protoConf)
		if params.Level == PROTO_MANDATORY {
			return true
		}
	}
	return false
}

func checkContainHTTP(nextProtos []string) bool {
	for _, protoConf := range nextProtos {
		nextProto, _, _ := ParseNextProto(protoConf)
		if nextProto == HTTP11 {
			return true
		}
	}
	return false
}

func checkVip(ruleMap TlsRuleMap) error {
	allVip := make(map[string]bool)
	for product, rule := range ruleMap {
		for i, vip := range rule.VipConf {
			if allVip[vip] {
				return fmt.Errorf("found duplicated vip (%s:%d) %s", product, i, vip)
			}
			allVip[vip] = true
		}
	}
	return nil
}

func checkSniConf(ruleMap TlsRuleMap) error {
	allName := make(map[string]bool)
	for product, rule := range ruleMap {
		for i, name := range rule.SniConf {
			if allName[name] {
				return fmt.Errorf("found duplicated name (%s:%d) %s", product, i, name)
			}
			allName[name] = true
		}
	}
	return nil
}

func BfeTlsRuleConfCheck(conf *BfeTlsRuleConf) error {
	if len(conf.Version) == 0 {
		return fmt.Errorf("no Version")
	}

	if conf.Config == nil {
		return fmt.Errorf("no Config")
	}

	for product, rule := range conf.Config {
		if err := TlsRuleConfCheck(rule); err != nil {
			return fmt.Errorf("BfeTlsRuleConfCheck(): %s wrong rule %s", product, err)
		}
	}

	if err := checkVip(conf.Config); err != nil {
		return err
	}

	if err := checkSniConf(conf.Config); err != nil {
		return err
	}

	if err := checkNextProtos(conf.DefaultNextProtos); err != nil {
		return err
	}

	return nil
}

// CheckTlsConf check integrity of tls rule conf and cert conf.
func CheckTlsConf(certConf map[string]*bfe_tls.Certificate, ruleMap TlsRuleMap) error {
	for _, ruleConf := range ruleMap {
		// check whether cert specified in ruleConf exists
		cert, ok := certConf[ruleConf.CertName]
		if !ok {
			return fmt.Errorf("certificate %s not exist", ruleConf.CertName)
		}

		// check whether name specified in ruleConf matches cert
		certNames := server_cert_conf.GetNamesForCert(cert)
		for _, name := range ruleConf.SniConf {
			if !MatchCertNames(certNames, name) {
				return fmt.Errorf("%s not included in certificate %s", name, ruleConf.CertName)
			}
		}
	}

	return nil
}

// MatchCertNames check whether host matches names in cert.
func MatchCertNames(certNames []string, host string) bool {
	for _, cname := range certNames {
		if MatchHostnames(cname, host) {
			return true
		}
	}
	return false
}

// MatchHostnames check whether host matches pattern.
func MatchHostnames(pattern, host string) bool {
	if len(pattern) == 0 || len(host) == 0 {
		return false
	}

	patternParts := strings.Split(pattern, ".")
	hostParts := strings.Split(host, ".")

	if len(patternParts) != len(hostParts) {
		return false
	}

	for i, patternPart := range patternParts {
		if patternPart == "*" {
			continue
		}
		if patternPart != hostParts[i] {
			return false
		}
	}

	return true
}

func GetClientCACertificate(clientCADir string, clientCAName string) (*x509.CertPool, error) {
	clientCAFile := filepath.Join(clientCADir, clientCAName+".crt")

	// load and init client ca certificates
	return bfe_conf.LoadClientCAFile(clientCAFile)
}

// ClientCALoad load client CA certificates.
func ClientCALoad(tlsRuleMap TlsRuleMap, clientCADir string) (map[string]*x509.CertPool, error) {
	clientCAMap := make(map[string]*x509.CertPool)
	for productName, rule := range tlsRuleMap {
		if !rule.ClientAuth {
			continue
		}

		_, ok := clientCAMap[rule.ClientCAName]
		if !ok {
			roots, err := GetClientCACertificate(clientCADir, rule.ClientCAName)
			if err != nil {
				return nil, fmt.Errorf("product[%s] GetClientCACertificate() err: %v", productName, err)
			}

			clientCAMap[rule.ClientCAName] = roots
		}
	}

	return clientCAMap, nil
}

func getCientCRL(clientCRLDir, clientCAName string) ([]*pkix.CertificateList, error) {
	clientCRLSubDir := filepath.Join(clientCRLDir, clientCAName)
	crlFiles, err := ioutil.ReadDir(clientCRLSubDir)
	if err != nil {
		log.Logger.Debug("ioutil.ReadDir %s failed: %v", clientCRLSubDir, err)
		return nil, nil
	}

	var crls []*pkix.CertificateList
	for _, crlFile := range crlFiles {
		if crlFile.IsDir() {
			continue
		}

		if !strings.HasSuffix(crlFile.Name(), ".crl") {
			continue
		}

		crlFilePath := filepath.Join(clientCRLSubDir, crlFile.Name())
		fileContent, err := ioutil.ReadFile(crlFilePath)
		if err != nil {
			return nil, fmt.Errorf("read crl %s failed, %v", crlFilePath, err)
		}

		crl, err := x509.ParseCRL(fileContent)
		if err != nil {
			return nil, fmt.Errorf("parse crl %s failed, %v", crlFilePath, err)
		}

		log.Logger.Debug("read %s success", crlFile.Name())
		crls = append(crls, crl)
	}
	return crls, nil
}

func ClientCRLLoad(clientCAMap map[string]*x509.CertPool, clientCRLDir string) (map[string]*bfe_tls.CRLPool, error) {
	f, err := os.Stat(clientCRLDir)
	if err != nil || !f.IsDir() {
		return nil, fmt.Errorf("ClientCRLBaseDir %s not exists", clientCRLDir)
	}

	clientCRLPoolMap := make(map[string]*bfe_tls.CRLPool)
	for clientCAName := range clientCAMap {
		crls, err := getCientCRL(clientCRLDir, clientCAName)
		if err != nil {
			return nil, err
		}

		if crls == nil {
			continue
		}

		crlPool := bfe_tls.NewCRLPool()
		for _, crl := range crls {
			if err := crlPool.AddCRL(crl); err != nil {
				return nil, fmt.Errorf("client_ca %s read crl: %v", clientCAName, err)
			}
		}

		clientCRLPoolMap[clientCAName] = crlPool
	}

	return clientCRLPoolMap, nil
}

// TlsRuleConfLoad load config of rule from file.
func TlsRuleConfLoad(filename string) (BfeTlsRuleConf, error) {
	var config BfeTlsRuleConf

	// open the file
	file, err := os.Open(filename)
	if err != nil {
		return config, err
	}
	defer file.Close()

	// decode the file
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		return config, err
	}

	// check conf
	err = BfeTlsRuleConfCheck(&config)
	if err != nil {
		return config, err
	}

	return config, nil
}

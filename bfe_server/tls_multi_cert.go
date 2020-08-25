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

// an implementation of tls.MultiCertificate

package bfe_server

import (
	"fmt"
	"strings"
	"sync"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_tls_conf/server_cert_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_tls_conf/tls_rule_conf"
	"github.com/bfenetworks/bfe/bfe_tls"
)

type MultiCertMap struct {
	vipCertMap  map[string]*bfe_tls.Certificate // vip -> certificate
	nameCertMap *NameCertMap                    // name -> certificate
	defaultCert *bfe_tls.Certificate            // default cert
	lock        sync.RWMutex
	state       *ProxyState // state for MultiCertMap
}

func NewMultiCertMap(state *ProxyState) *MultiCertMap {
	m := new(MultiCertMap)
	m.vipCertMap = make(map[string]*bfe_tls.Certificate)
	m.nameCertMap = NewNameCertMap()
	m.state = state
	return m
}

// Get gets certificate for given connection.
func (m *MultiCertMap) Get(c *bfe_tls.Conn) *bfe_tls.Certificate {
	var cert *bfe_tls.Certificate
	m.state.TlsMultiCertGet.Inc(1)

	m.lock.RLock()
	defer m.lock.RUnlock()

	// choose certificate by vip
	vip := c.GetVip()
	if vip != nil {
		key := vip.String()
		cert = m.vipCertMap[key]
		if cert == nil {
			m.state.TlsMultiCertConnVipUnknown.Inc(1)
		}
	} else {
		m.state.TlsMultiCertConnWithoutVip.Inc(1)
	}

	// if vip for connection is not found unexpectedly, or vip for connection is unknown,
	// try to choose cert by SNI (Server Name Indication)
	if cert == nil {
		serverName := c.GetServerName()
		if len(serverName) > 0 {
			cert = m.nameCertMap.Get(serverName)
		} else {
			m.state.TlsMultiCertConnWithoutSni.Inc(1)
		}
	}

	// choose default cert
	if cert == nil {
		cert = m.defaultCert
		m.state.TlsMultiCertUseDefault.Inc(1)
	}

	return cert
}

func (m *MultiCertMap) GetDefault() *bfe_tls.Certificate {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if m.defaultCert == nil {
		return nil
	}

	return m.defaultCert
}

// Update updates all certificates.
func (m *MultiCertMap) Update(certConf map[string]*bfe_tls.Certificate,
	ruleMap tls_rule_conf.TlsRuleMap) error {
	m.state.TlsMultiCertUpdate.Inc(1)

	vipCertMap := make(map[string]*bfe_tls.Certificate)
	for _, ruleConf := range ruleMap {
		cert, ok := certConf[ruleConf.CertName]
		if !ok {
			m.state.TlsMultiCertUpdateErr.Inc(1)
			return fmt.Errorf("certificate %s not exist", ruleConf.CertName)
		}
		for _, vip := range ruleConf.VipConf {
			vipCertMap[vip] = cert
		}
	}

	nameCertMap := NewNameCertMap()
	nameCertMap.Update(certConf)

	defaultCert := certConf[server_cert_conf.DefaultCert]
	if defaultCert == nil {
		m.state.TlsMultiCertUpdateErr.Inc(1)
		return fmt.Errorf("default certificate not exist")
	}

	m.lock.Lock()
	m.vipCertMap = vipCertMap
	m.nameCertMap = nameCertMap
	m.defaultCert = defaultCert
	m.lock.Unlock()

	return nil
}

type NameCertMap struct {
	normalCertMap   map[string]*bfe_tls.Certificate // cert map for normal name
	wildcardCertMap map[string]*bfe_tls.Certificate // cert map for wildcard name
}

func NewNameCertMap() *NameCertMap {
	m := new(NameCertMap)
	m.normalCertMap = make(map[string]*bfe_tls.Certificate)
	m.wildcardCertMap = make(map[string]*bfe_tls.Certificate)
	return m
}

func (m *NameCertMap) Get(serverName string) *bfe_tls.Certificate {
	serverName = strings.ToLower(serverName)
	for len(serverName) > 0 && serverName[len(serverName)-1] == '.' {
		serverName = serverName[:len(serverName)-1]
	}

	if cert, ok := m.normalCertMap[serverName]; ok {
		return cert
	}

	// Note: Since number of wildcard names is too small,
	// just perform sequent matching here
	for name, cert := range m.wildcardCertMap {
		if tls_rule_conf.MatchHostnames(name, serverName) {
			return cert
		}
	}

	return nil
}

func (m *NameCertMap) Update(certConf map[string]*bfe_tls.Certificate) {
	normalCertMap := make(map[string]*bfe_tls.Certificate)
	wildcardCertMap := make(map[string]*bfe_tls.Certificate)

	for _, cert := range certConf {
		names := server_cert_conf.GetNamesForCert(cert)
		for _, name := range names {
			if strings.Contains(name, "*") {
				wildcardCertMap[name] = cert
			} else {
				normalCertMap[name] = cert
			}
		}
	}
	m.normalCertMap = normalCertMap
	m.wildcardCertMap = wildcardCertMap
}

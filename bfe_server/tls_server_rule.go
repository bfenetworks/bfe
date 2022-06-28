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

// server rule for tls

package bfe_server

import (
	"crypto/x509"
	"net"
	"strings"
	"sync"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/spaolacci/murmur3"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_tls_conf/tls_rule_conf"
	"github.com/bfenetworks/bfe/bfe_http2"
	"github.com/bfenetworks/bfe/bfe_stream"
	"github.com/bfenetworks/bfe/bfe_tls"
)

var (
	DefaultNextProtos = []string{tls_rule_conf.HTTP11}
)

type ServerRule struct {
	TlsRule    bfe_tls.Rule    // server rule for tls
	H2Rule     bfe_http2.Rule  // server rule for h2
	StreamRule bfe_stream.Rule // server rule for stream
}

type TLSServerRuleMap struct {
	lock       sync.RWMutex
	vipRuleMap map[string]*ServerRule // tls server rule for specified conn
	sniRuleMap map[string]*ServerRule // tls server rule for specified host (optional)

	nextProtosDef *NextProtosConf // default next protos conf
	enableHttp2   bool            // enable http2 globally or not
	enableSpdy    bool            // enable spdy globally or not
	chacha20Def   bool            // default chacha20 conf
	dynRecordDef  bool            // default dynamic record conf

	versions Version // version of tls_rule_conf

	state *ProxyState
}

type Version struct {
	TlsRuleConfVersion string
}

func NewTLSServerRuleMap(state *ProxyState) *TLSServerRuleMap {
	m := new(TLSServerRuleMap)
	m.vipRuleMap = make(map[string]*ServerRule)
	m.sniRuleMap = make(map[string]*ServerRule)
	m.enableHttp2 = true
	m.enableSpdy = true
	m.state = state
	return m
}

// Get returns tls rule for given connection.
func (m *TLSServerRuleMap) Get(c *bfe_tls.Conn) *bfe_tls.Rule {
	r := m.getRule(c)
	return &r.TlsRule
}

// GetHTTP2Rule returns h2 rule for given connection.
func (m *TLSServerRuleMap) GetHTTP2Rule(c *bfe_tls.Conn) *bfe_http2.Rule {
	r := m.getRule(c)
	return &r.H2Rule
}

// GetStreamRule returns stream rule for given connection.
func (m *TLSServerRuleMap) GetStreamRule(c *bfe_tls.Conn) *bfe_stream.Rule {
	r := m.getRule(c)
	return &r.StreamRule
}

func (m *TLSServerRuleMap) getRule(c *bfe_tls.Conn) *ServerRule {
	m.lock.RLock()
	defer m.lock.RUnlock()

	// get tls rule conf by vip
	if rule := m.getRuleByVip(c); rule != nil {
		return rule
	}

	// get tls rule conf by sni (supported by modern browser)
	if rule := m.getRuleBySni(c); rule != nil {
		return rule
	}

	// get default rule
	return m.getDefaultRule(c)
}

func (m *TLSServerRuleMap) getRuleByVip(c *bfe_tls.Conn) *ServerRule {
	vip := c.GetVip()
	if vip == nil {
		return nil
	}

	key := vip.String()
	return m.vipRuleMap[key]
}

func (m *TLSServerRuleMap) getRuleBySni(c *bfe_tls.Conn) *ServerRule {
	name := c.GetServerName()
	return m.sniRuleMap[name]
}

func (m *TLSServerRuleMap) getDefaultRule(c *bfe_tls.Conn) *ServerRule {
	rule := new(ServerRule)

	rule.TlsRule.NextProtos = m.nextProtosDef
	rule.TlsRule.Grade = bfe_tls.GradeC
	rule.TlsRule.ClientAuth = false
	rule.TlsRule.Chacha20 = m.chacha20Def
	rule.TlsRule.DynamicRecord = m.dynRecordDef

	rule.H2Rule.MaxConcurrentStreams = 0
	rule.H2Rule.MaxUploadBufferPerStream = 0
	rule.H2Rule.DisableDegrade = false

	rule.StreamRule.ProxyProtocol = 0

	return rule
}

func (m *TLSServerRuleMap) Update(conf tls_rule_conf.BfeTlsRuleConf,
	clientCAMap map[string]*x509.CertPool, clientCRLPoolMap map[string]*bfe_tls.CRLPool) {
	vipRuleMap := make(map[string]*ServerRule)
	sniRuleMap := make(map[string]*ServerRule)

	for _, ruleConf := range conf.Config {
		clientCAs := clientCAMap[ruleConf.ClientCAName]
		clientCRLPool := clientCRLPoolMap[ruleConf.ClientCAName]
		rule := m.createServerRule(ruleConf, clientCAs, clientCRLPool, conf.DefaultNextProtos)
		for _, vip := range ruleConf.VipConf {
			vipRuleMap[vip] = rule
		}
		for _, name := range ruleConf.SniConf {
			sniRuleMap[name] = rule
		}
	}

	defaultNextProtos := DefaultNextProtos
	if len(conf.DefaultNextProtos) != 0 {
		defaultNextProtos = conf.DefaultNextProtos
	}
	nextProtosDef := NewNextProtosConf(m, defaultNextProtos)

	versions := Version{
		TlsRuleConfVersion: conf.Version,
	}

	m.lock.Lock()
	m.vipRuleMap = vipRuleMap
	m.sniRuleMap = sniRuleMap
	m.nextProtosDef = nextProtosDef
	m.chacha20Def = conf.DefaultChacha20
	m.dynRecordDef = conf.DefaultDynamicRecord
	m.versions = versions
	m.lock.Unlock()
}

func (m *TLSServerRuleMap) createServerRule(conf *tls_rule_conf.TlsRuleConf,
	clientCAs *x509.CertPool, clientCRLPool *bfe_tls.CRLPool, defaultNextProtos []string) *ServerRule {
	r := new(ServerRule)

	// tls next protos
	if len(conf.NextProtos) != 0 {
		r.TlsRule.NextProtos = NewNextProtosConf(m, conf.NextProtos)
	} else {
		r.TlsRule.NextProtos = NewNextProtosConf(m, defaultNextProtos)
	}

	// tls security grade
	if conf.Grade != "" {
		r.TlsRule.Grade = conf.Grade
	} else {
		r.TlsRule.Grade = bfe_tls.GradeC
	}

	// tls client auth policy
	r.TlsRule.ClientAuth = conf.ClientAuth
	r.TlsRule.ClientCAs = clientCAs
	r.TlsRule.ClientCAName = conf.ClientCAName
	r.TlsRule.ClientCRLPool = clientCRLPool

	// enable chacha20-poly1305 cipher suites
	r.TlsRule.Chacha20 = conf.Chacha20

	// enable dynamic tls record
	r.TlsRule.DynamicRecord = conf.DynamicRecord

	// h2/stream related settings
	for _, protoConf := range conf.NextProtos {
		proto, params, _ := tls_rule_conf.ParseNextProto(protoConf)
		switch proto {
		case tls_rule_conf.HTTP2:
			r.H2Rule.MaxConcurrentStreams = uint32(params.Mcs)
			r.H2Rule.MaxUploadBufferPerStream = uint32(params.Isw)
			r.H2Rule.DisableDegrade = (params.Level > tls_rule_conf.PROTO_OPTIONAL)
		case tls_rule_conf.STREAM:
			r.StreamRule.ProxyProtocol = params.PP
		}
	}

	return r
}

func (m *TLSServerRuleMap) EnableNextProto(proto string, state bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if proto == "h2" {
		m.enableHttp2 = state
	} else if strings.HasPrefix(proto, "spdy") {
		m.enableSpdy = state
	}
	log.Logger.Info("TLSServerRuleMap: enable %s %v", proto, state)
}

func (m *TLSServerRuleMap) StatusNextProto() (enableHttp2 bool, enableSpdy bool) {
	m.lock.RLock()
	enableHttp2, enableSpdy = m.enableHttp2, m.enableSpdy
	m.lock.RUnlock()

	return enableHttp2, enableSpdy
}

type NextProtosConf struct {
	serverRule *TLSServerRuleMap // link back to TLSServerRuleMap
	protos     []string          // application level protocol over tls
	level      []int             // negatiation level for each protocol
	mcs        []int             // max concurrency per conn for each protocol
	rate       []int             // presence rate for each protocol
	pp         []int             // PROXY protocol option for connections to backend
}

func NewNextProtosConf(rule *TLSServerRuleMap, protoConf []string) *NextProtosConf {
	c := new(NextProtosConf)
	c.serverRule = rule
	c.protos = make([]string, len(protoConf))
	c.level = make([]int, len(protoConf))
	c.mcs = make([]int, len(protoConf))
	c.rate = make([]int, len(protoConf))
	c.pp = make([]int, len(protoConf))

	for i, protoString := range protoConf {
		proto, params, _ := tls_rule_conf.ParseNextProto(protoString)
		c.protos[i] = proto
		c.level[i] = params.Level
		c.mcs[i] = params.Mcs
		c.rate[i] = params.Rate
		c.pp[i] = params.PP
	}
	return c
}

func (c *NextProtosConf) Get(conn *bfe_tls.Conn) []string {
	r := c.serverRule

	// check if h2/spdy should be enabled
	enableHttp2, enableSpdy := r.StatusNextProto()

	// select next protos for current conn
	protos := make([]string, 0, len(c.protos))
	value := getHashValue(conn)

	for i, proto := range c.protos {
		// ignore optional protocol if needed
		if c.level[i] == tls_rule_conf.PROTO_OPTIONAL {
			if value >= c.rate[i] {
				continue
			}
			if !enableHttp2 && strings.HasPrefix(proto, "h2") {
				continue
			}
			if !enableSpdy && strings.HasPrefix(proto, "spdy") {
				continue
			}
		}
		protos = append(protos, proto)
	}

	if len(protos) == 0 {
		return DefaultNextProtos
	}

	return protos
}

func (c *NextProtosConf) Mandatory(conn *bfe_tls.Conn) (string, bool) {
	if len(c.protos) != 1 {
		return "", false
	}
	if c.level[0] != tls_rule_conf.PROTO_MANDATORY {
		return "", false
	}
	return c.protos[0], true
}

func getHashValue(conn *bfe_tls.Conn) int {
	remoteAddr := conn.RemoteAddr().(*net.TCPAddr)
	return int(murmur3.Sum32(remoteAddr.IP) % 100)
}

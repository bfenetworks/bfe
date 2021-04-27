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

// load config data for bfe

package bfe_server

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_tls_conf/server_cert_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_tls_conf/session_ticket_key_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_tls_conf/tls_rule_conf"
	"github.com/bfenetworks/bfe/bfe_route"
	"github.com/bfenetworks/bfe/bfe_util/bns"
)

// InitDataLoad load data when bfe start.
func (srv *BfeServer) InitDataLoad() error {
	// load ServerDataConf
	serverConf, err := bfe_route.LoadServerDataConf(srv.Config.Server.HostRuleConf,
		srv.Config.Server.VipRuleConf, srv.Config.Server.RouteRuleConf,
		srv.Config.Server.ClusterConf)
	if err != nil {
		return fmt.Errorf("InitDataLoad():bfe_route.LoadServerDataConf Error %s", err)
	}

	srv.ServerConf = serverConf
	srv.ReverseProxy.setTransports(srv.ServerConf.ClusterTable.ClusterMap())
	log.Logger.Info("init serverDataConf success")

	// load bal table
	if err := srv.balTable.Init(srv.Config.Server.GslbConf,
		srv.Config.Server.ClusterTableConf); err != nil {
		return fmt.Errorf("InitDataLoad():balTableInit Error %s", err)
	}

	// set gslb retry config, slow_start config
	if srv.ServerConf != nil {
		ct := srv.ServerConf.ClusterTable
		srv.balTable.SetGslbBasic(ct)
		srv.balTable.SetSlowStart(ct)
	}
	log.Logger.Info("init bal table success")

	// load name conf
	if len(srv.Config.Server.NameConf) > 0 {
		if err := srv.NameConfReload(nil); err != nil {
			return fmt.Errorf("InitDataLoad():NameConfLoad Error %s", err)
		}
		log.Logger.Info("init name conf success")
	}

	return nil
}

func joinPath(path, suffix string) string {
	words := strings.Split(suffix, "/")
	if len(words) == 0 {
		return ""
	}

	return filepath.Join(path, words[len(words)-1])
}

// ServerDataConfReload reloads host/route/cluster conf
func (srv *BfeServer) ServerDataConfReload(query url.Values) error {
	hostFile := srv.Config.Server.HostRuleConf
	vipFile := srv.Config.Server.VipRuleConf
	routeFile := srv.Config.Server.RouteRuleConf
	clusterConfFile := srv.Config.Server.ClusterConf

	path := query.Get("path")
	if path != "" {
		hostFile = joinPath(path, hostFile)
		vipFile = joinPath(path, vipFile)
		routeFile = joinPath(path, routeFile)
		clusterConfFile = joinPath(path, clusterConfFile)
	}

	return srv.serverDataConfReload(hostFile, vipFile, routeFile, clusterConfFile)
}

func (srv *BfeServer) serverDataConfReload(hostFile, vipFile, routeFile, clusterConfFile string) error {
	newServerConf, err := bfe_route.LoadServerDataConf(hostFile, vipFile, routeFile, clusterConfFile)
	if err != nil {
		log.Logger.Error("ServerDataConfReload():bfe_route.LoadServerDataConf: %s", err)
		return err
	}

	srv.confLock.Lock()
	srv.ServerConf = newServerConf
	srv.confLock.Unlock()

	srv.ReverseProxy.setTransports(srv.ServerConf.ClusterTable.ClusterMap())

	// set gslb basic
	srv.balTable.SetGslbBasic(newServerConf.ClusterTable)
	// set slow_start config
	srv.balTable.SetSlowStart(newServerConf.ClusterTable)

	return nil
}

// GslbDataConfReload reloads gslb and cluster conf.
func (srv *BfeServer) GslbDataConfReload(query url.Values) error {
	gslbFile := srv.Config.Server.GslbConf
	clusterTableFile := srv.Config.Server.ClusterTableConf

	path := query.Get("path")
	if path != "" {
		gslbFile = joinPath(path, gslbFile)
		clusterTableFile = joinPath(path, clusterTableFile)
	}

	return srv.gslbDataConfReload(gslbFile, clusterTableFile)
}

func (srv *BfeServer) gslbDataConfReload(gslbFile, clusterTableFile string) error {
	// load gslb and cluster_table file
	gslbConf, backendConf, err := srv.balTable.BalTableConfLoad(gslbFile, clusterTableFile)
	if err != nil {
		log.Logger.Error("GslbDataConfReload():BalTable conf load err [%s]", err)
		return err
	}

	if err := srv.balTable.BalTableReload(gslbConf, backendConf); err != nil {
		log.Logger.Error("GslbDataConfReload():BalTableReload err [%s]", err)
		return err
	}

	// set gslb basic conf
	srv.confLock.Lock()
	serverConf := srv.ServerConf
	srv.confLock.Unlock()
	srv.balTable.SetGslbBasic(serverConf.ClusterTable)
	// set slow_start config
	srv.balTable.SetSlowStart(serverConf.ClusterTable)

	return nil
}

// SessionTicketKeyReload reloads for session ticket key.
func (srv *BfeServer) SessionTicketKeyReload() error {
	log.Logger.Info("start session ticket key reload")
	sessionTicketConf := srv.Config.SessionTicket
	if sessionTicketConf.SessionTicketsDisabled {
		return nil
	}

	// load session ticket key
	keyFile := sessionTicketConf.SessionTicketKeyFile
	keyConf, err := session_ticket_key_conf.SessionTicketKeyConfLoad(keyFile)
	if err != nil {
		return err
	}
	key, err := hex.DecodeString(keyConf.SessionTicketKey)
	if err != nil { // never go here
		return fmt.Errorf("wrong session ticket key %s (%s)", err, key)
	}

	// update session ticket key
	srv.HttpsListener.UpdateSessionTicketKey(key)
	log.Logger.Debug("update session ticket key for %s", keyConf.SessionTicketKey)

	return nil
}

func (srv *BfeServer) TLSConfReload(query url.Values) error {
	log.Logger.Info("start tls rules reload (params: %s)", query)

	// enable or disable specified protocols
	protosParam := strings.Split(query.Get("enable"), ",")
	for _, proto := range protosParam {
		srv.enableTLSNextProto(proto)
	}

	// reload tls conf
	certConfFile := srv.Config.HttpsBasic.ServerCertConf
	tlsRuleFile := srv.Config.HttpsBasic.TlsRuleConf
	if path := query.Get("path"); path != "" {
		certConfFile = joinPath(path, certConfFile)
		tlsRuleFile = joinPath(path, tlsRuleFile)
	}

	return srv.tlsConfLoad(certConfFile, tlsRuleFile)
}

func (srv *BfeServer) tlsConfLoad(certConfFile string, tlsRuleFile string) error {
	// load certificate conf
	certConf, err := server_cert_conf.ServerCertConfLoad(certConfFile, srv.ConfRoot)
	if err != nil {
		return fmt.Errorf("in ServerCertConfLoad() :%s", err.Error())
	}

	// parse certificate
	certMap, err := server_cert_conf.ServerCertParse(certConf)
	if err != nil {
		return fmt.Errorf("in ServerCertParse() :%s", err.Error())
	}

	// load tls server rule
	tlsRule, err := tls_rule_conf.TlsRuleConfLoad(tlsRuleFile)
	if err != nil {
		return fmt.Errorf("in TlsRuleConfLoad() :%s", err.Error())
	}

	// load client CA certificates
	clientCABaseDir := srv.Config.HttpsBasic.ClientCABaseDir
	clientCAMap, err := tls_rule_conf.ClientCALoad(tlsRule.Config, clientCABaseDir)
	if err != nil {
		return fmt.Errorf("in ClientCALoad() :%s", err.Error())
	}

	// load client cert CRL
	clientCRLBaseDir := srv.Config.HttpsBasic.ClientCRLBaseDir
	clientCRLPoolMap, err := tls_rule_conf.ClientCRLLoad(clientCAMap, clientCRLBaseDir)
	if err != nil {
		return fmt.Errorf("in ClientCRLLoad(): %s", err.Error())
	}

	// validate tls conf
	if err := tls_rule_conf.CheckTlsConf(certMap, tlsRule.Config); err != nil {
		return fmt.Errorf("in CheckTlsConf() :%s", err.Error())
	}

	// update certificates and tls rule data
	srv.MultiCert.Update(certMap, tlsRule.Config)
	srv.TLSServerRule.Update(tlsRule, clientCAMap, clientCRLPoolMap)
	log.Logger.Debug("update tls server rule success")

	return nil
}

func (srv *BfeServer) enableTLSNextProto(proto string) {
	switch proto {
	case "+spdy", "spdy":
		srv.TLSServerRule.EnableNextProto("spdy", true)
		log.Logger.Info("spdy protocol is enabled")
	case "-spdy":
		srv.TLSServerRule.EnableNextProto("spdy", false)
		log.Logger.Info("spdy protocol is disabled")
	case "+h2", "h2":
		srv.TLSServerRule.EnableNextProto("h2", true)
		log.Logger.Info("http2 protocol is enabled")
	case "-h2":
		srv.TLSServerRule.EnableNextProto("h2", false)
		log.Logger.Info("http2 protocol is disabled")
	}
}

// NameConfReload reloads name conf data.
func (srv *BfeServer) NameConfReload(query url.Values) error {
	nameConfFile := query.Get("path")
	if nameConfFile == "" {
		nameConfFile = srv.Config.Server.NameConf
	}

	return bns.LoadLocalNameConf(nameConfFile)
}

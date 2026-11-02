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

/* inner_env.go - legacy %var built-in variables as expr env functions
/*
DESCRIPTION
	把旧 %var 内置变量（bfe_basic/action.VariableHandlers）以函数形式开放给
	表达式（动作参数及条件表达式均可调用），保证与 %{...} 替换语义逐项等价。
	命名：变量名 snake_case → PascalCase 方法名。
	req_uri / req_path 已由 BaseEnv.ReqUri()/ReqPath() 提供，不再重复。

	移植裁剪说明：BfeTcpOptNs（bfe_tcp_opt_ns）依赖目标仓库不存在的
	bfe_util.GetTcpOptNs（TTM 内核 sockopt），随 VariableHandlers 一并裁剪。
*/

package bfe_expr

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/action"
)

// InnerEnv 旧 %var 内置变量函数（委托 action.VariableHandlers，语义逐项等价）
type InnerEnv struct {
	req *bfe_basic.Request
}

// get 按变量名委托 VariableHandlers；未知变量返回 ""
func (e *InnerEnv) get(name string) string {
	if h, ok := action.VariableHandlers[name]; ok {
		return h(e.req)
	}
	return ""
}

// for client
func (e *InnerEnv) BfeClientIp() string         { return e.get("bfe_clientip") }
func (e *InnerEnv) BfeClientPort() string       { return e.get("bfe_clientport") }
func (e *InnerEnv) BfeClientIpLocation() string { return e.get("bfe_clientip_location") }

// for connection info
func (e *InnerEnv) BfeCip() string        { return e.get("bfe_cip") }
func (e *InnerEnv) BfeVip() string        { return e.get("bfe_vip") }
func (e *InnerEnv) BfeBip() string        { return e.get("bfe_bip") }
func (e *InnerEnv) BfeRip() string        { return e.get("bfe_rip") }
func (e *InnerEnv) BfeDomainAuto() string { return e.get("bfe_domain_auto") }
func (e *InnerEnv) SessionId() string     { return e.get("session_id") }
func (e *InnerEnv) BfeLogid() string      { return e.get("bfe_logid") }

// for bfe
func (e *InnerEnv) BfeServerName() string { return e.get("bfe_server_name") }

// for backend
// BfeRouteCluster 对应旧变量 %bfe_cluster（路由目标集群 req.Route.ClusterName）。
// 注意与 BaseEnv.RouteCluster（后端集群 req.Backend.ClusterName）区分。
func (e *InnerEnv) BfeRouteCluster() string { return e.get("bfe_cluster") }
func (e *InnerEnv) BfeBackendInfo() string  { return e.get("bfe_backend_info") }

// for tls
func (e *InnerEnv) BfeSslInfo() string   { return e.get("bfe_ssl_info") }
func (e *InnerEnv) BfeSslResume() string { return e.get("bfe_ssl_resume") }
func (e *InnerEnv) BfeSslCipher() string { return e.get("bfe_ssl_cipher") }
func (e *InnerEnv) BfeSslVersion() string {
	return e.get("bfe_ssl_version")
}
func (e *InnerEnv) BfeSslJa3Raw() string { return e.get("bfe_ssl_ja3_raw") }
func (e *InnerEnv) BfeSslJa3Hash() string {
	return e.get("bfe_ssl_ja3_hash")
}
func (e *InnerEnv) BfeProtocol() string { return e.get("bfe_protocol") }

// for client certificate
func (e *InnerEnv) ClientCertSerialNumber() string { return e.get("client_cert_serial_number") }
func (e *InnerEnv) ClientCertSubjectTitle() string {
	return e.get("client_cert_subject_title")
}
func (e *InnerEnv) ClientCertSubjectCommonName() string {
	return e.get("client_cert_subject_common_name")
}
func (e *InnerEnv) ClientCertSubjectOrganization() string {
	return e.get("client_cert_subject_organization")
}
func (e *InnerEnv) ClientCertSubjectOrganizationUnit() string {
	return e.get("client_cert_subject_organization_unit")
}
func (e *InnerEnv) ClientCertSubjectProvince() string {
	return e.get("client_cert_subject_province")
}
func (e *InnerEnv) ClientCertSubjectCountry() string {
	return e.get("client_cert_subject_country")
}
func (e *InnerEnv) ClientCertSubjectLocality() string {
	return e.get("client_cert_subject_locality")
}
func (e *InnerEnv) ClientCertAduCarRole() string { return e.get("client_cert_adu_car_role") }
func (e *InnerEnv) ClientCertAduReserve() string { return e.get("client_cert_adu_reserve") }
func (e *InnerEnv) ClientCertPem() string        { return e.get("client_cert_pem") }
func (e *InnerEnv) ClientCertSubjectString() string {
	return e.get("client_cert_subject_string")
}
func (e *InnerEnv) ClientCertIssuerString() string {
	return e.get("client_cert_issuer_string")
}
func (e *InnerEnv) ClientCertPubKeyAlgo() string {
	return e.get("client_cert_pub_key_algo")
}
func (e *InnerEnv) ClientCertSigAlgo() string { return e.get("client_cert_sig_algo") }

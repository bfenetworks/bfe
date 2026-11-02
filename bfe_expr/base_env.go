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

/* base_env.go - request/response/session/route access functions
/*
DESCRIPTION
	新风格条件表达式的取数函数（方法形式）。
	所有方法对 nil 字段安全：请求在不同处理阶段可用的数据不同，
	缺失数据返回空值（""、0、nil），不视为错误。

	移植裁剪说明（相对源仓库）：
	- ReqBaiduid：目标仓库 Request 无 Baiduid 字段，裁剪
	- BfeCluster：目标仓库 Session 无 BfeClusterName 字段，裁剪
	- SesTlsClientAuth：目标仓库 bfe_tls.ConnectionState 无 ClientAuthType/
	  VerifiedChainsGM 字段，无法等价实现，裁剪（legacyNameMap 同步移除
	  ses_tls_client_auth）
*/

package bfe_expr

import (
	"os"
	"strconv"
	"strings"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/go-lib/log"
	"github.com/tidwall/gjson"
)

// bodyJsonCachePrefix 请求体 JSON 取数结果在请求 Context 中的缓存前缀
const bodyJsonCachePrefix = "bfe_expr.jsoncache."

type BaseEnv struct {
	req *bfe_basic.Request
}

// ---------- 请求访问 ----------

func (e *BaseEnv) ReqMethod() string {
	if e.req == nil || e.req.HttpRequest == nil {
		return ""
	}
	return e.req.HttpRequest.Method
}

// ReqHost 返回 Host（去掉端口，保留原始大小写）
func (e *BaseEnv) ReqHost() string {
	if e.req == nil || e.req.HttpRequest == nil {
		return ""
	}
	return strings.SplitN(e.req.HttpRequest.Host, ":", 2)[0]
}

// ReqPort 返回 Host 中的端口，缺省 "80"（与旧 PortFetcher 语义一致）
func (e *BaseEnv) ReqPort() string {
	if e.req == nil || e.req.HttpRequest == nil {
		return ""
	}
	host := e.req.HttpRequest.Host
	if i := strings.Index(host, ":"); i > 0 {
		return host[i+1:]
	}
	return "80"
}

// ReqVport 返回会话的虚拟端口
func (e *BaseEnv) ReqVport() string {
	if e.req == nil || e.req.Session == nil {
		return ""
	}
	return strconv.Itoa(e.req.Session.Vport)
}

// ReqProto 返回协议（http/https/h2 等，等价旧 ProtoFetcher）
func (e *BaseEnv) ReqProto() string {
	if e.req == nil || e.req.HttpRequest == nil || e.req.Session == nil {
		return ""
	}
	return e.req.Protocol()
}

func (e *BaseEnv) ReqPath() string {
	if e.req == nil || e.req.HttpRequest == nil || e.req.HttpRequest.URL == nil {
		return ""
	}
	return e.req.HttpRequest.URL.Path
}

// ReqUri 返回原始 RequestURI（path + query）
func (e *BaseEnv) ReqUri() string {
	if e.req == nil || e.req.HttpRequest == nil {
		return ""
	}
	return e.req.HttpRequest.RequestURI
}

// ReqQuery 返回指定 query 参数的第一个值，不存在返回 ""
func (e *BaseEnv) ReqQuery(key string) string {
	if e.req == nil || e.req.HttpRequest == nil {
		return ""
	}
	return e.req.CachedQuery().Get(key)
}

// ReqQueryAll 返回指定 query 参数的全部值
func (e *BaseEnv) ReqQueryAll(key string) []string {
	if e.req == nil || e.req.HttpRequest == nil {
		return nil
	}
	return e.req.CachedQuery()[key]
}

// ReqQueryKeys 返回全部 query key
func (e *BaseEnv) ReqQueryKeys() []string {
	if e.req == nil || e.req.HttpRequest == nil {
		return nil
	}
	query := e.req.CachedQuery()
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	return keys
}

// ReqCookie 返回指定 cookie 的值，不存在返回 ""
func (e *BaseEnv) ReqCookie(key string) string {
	if e.req == nil || e.req.HttpRequest == nil {
		return ""
	}
	cookie, ok := e.req.Cookie(key)
	if !ok {
		return ""
	}
	return cookie.Value
}

// ReqHeader 返回指定请求头的值，不存在返回 ""
func (e *BaseEnv) ReqHeader(key string) string {
	if e.req == nil || e.req.HttpRequest == nil {
		return ""
	}
	return e.req.HttpRequest.Header.Get(key)
}

// ReqBody 返回请求体内容（有 IO 成本，谨慎在热路径使用）
func (e *BaseEnv) ReqBody() string {
	if e.req == nil || e.req.HttpRequest == nil {
		return ""
	}
	bodyAccessor, err := e.req.HttpRequest.GetBodyAccessor()
	if err != nil || bodyAccessor == nil {
		return ""
	}
	body, _ := bodyAccessor.GetBytes()
	return string(body)
}

// ReqBodyJson 用 gjson 路径取请求体 JSON 字段，不存在返回 ""（结果按请求缓存）
func (e *BaseEnv) ReqBodyJson(path string) string {
	if e.req == nil || e.req.HttpRequest == nil {
		return ""
	}
	cacheKey := bodyJsonCachePrefix + path
	if e.req.Context != nil {
		if cachedVal := e.req.GetContext(cacheKey); cachedVal != nil {
			if str, ok := cachedVal.(string); ok {
				return str
			}
		}
	}

	bodyAccessor, err := e.req.HttpRequest.GetBodyAccessor()
	if err != nil || bodyAccessor == nil {
		return ""
	}
	body, _ := bodyAccessor.GetBytes()
	val := gjson.GetBytes(body, path)
	if !val.Exists() {
		return ""
	}
	str := val.String()
	if e.req.Context != nil {
		e.req.SetContext(cacheKey, str)
	}
	return str
}

func (e *BaseEnv) ReqLogid() string {
	if e.req == nil {
		return ""
	}
	return e.req.LogId
}

// ReqErr 返回 BFE 处理错误码字符串，无错误返回 ""
func (e *BaseEnv) ReqErr() string {
	if e.req == nil || e.req.ErrCode == nil {
		return ""
	}
	return e.req.ErrCode.Error()
}

func (e *BaseEnv) ReqRetry() int {
	if e.req == nil {
		return 0
	}
	return e.req.RetryTime
}

// ReqCip 返回客户端 IP（真实源 IP，可能为空串）
func (e *BaseEnv) ReqCip() string {
	if e.req == nil || e.req.ClientAddr == nil {
		return ""
	}
	return e.req.ClientAddr.IP.String()
}

// ReqSip 返回对端 socket IP
func (e *BaseEnv) ReqSip() string {
	if e.req == nil || e.req.Session == nil || e.req.Session.RemoteAddr == nil {
		return ""
	}
	return e.req.Session.RemoteAddr.IP.String()
}

// ReqVip 返回访问的 VIP
func (e *BaseEnv) ReqVip() string {
	if e.req == nil || e.req.Session == nil || e.req.Session.Vip == nil {
		return ""
	}
	return e.req.Session.Vip.String()
}

// ---------- 响应访问（响应未生成时返回空值） ----------

func (e *BaseEnv) ResCode() int {
	if e.req == nil || e.req.HttpResponse == nil {
		return 0
	}
	return e.req.HttpResponse.StatusCode
}

func (e *BaseEnv) ResHeader(key string) string {
	if e.req == nil || e.req.HttpResponse == nil {
		return ""
	}
	return e.req.HttpResponse.Header.Get(key)
}

// ---------- 路由、集群与后端 ----------

func (e *BaseEnv) RouteProduct() string {
	if e.req == nil {
		return ""
	}
	return e.req.Route.Product
}

func (e *BaseEnv) RouteHostTag() string {
	if e.req == nil {
		return ""
	}
	return e.req.Route.HostTag
}

func (e *BaseEnv) RouteCluster() string {
	if e.req == nil {
		return ""
	}
	return e.req.Backend.ClusterName
}

func (e *BaseEnv) RouteSubCluster() string {
	if e.req == nil {
		return ""
	}
	return e.req.Backend.SubclusterName
}

func (e *BaseEnv) BackendAddr() string {
	if e.req == nil {
		return ""
	}
	return e.req.Backend.BackendAddr
}

func (e *BaseEnv) BackendPort() int {
	if e.req == nil {
		return 0
	}
	return int(e.req.Backend.BackendPort)
}

// ReqTags 返回指定类型的全部 tag
func (e *BaseEnv) ReqTags(name string) []string {
	if e.req == nil {
		return nil
	}
	return e.req.GetTags(name)
}

// ---------- 会话与 TLS ----------

func (e *BaseEnv) SesSecure() bool {
	if e.req == nil || e.req.Session == nil {
		return false
	}
	return e.req.Session.IsSecure
}

// SesTrusted 对端是否为可信来源（等价旧 req_cip_trusted）
func (e *BaseEnv) SesTrusted() bool {
	if e.req == nil || e.req.Session == nil {
		return false
	}
	return e.req.Session.TrustSource()
}

// SesSni 返回 TLS SNI，无则 ""
func (e *BaseEnv) SesSni() string {
	if e.req == nil || e.req.Session == nil {
		return ""
	}
	ses := e.req.Session
	if !ses.IsSecure || ses.TlsState == nil {
		return ""
	}
	return ses.TlsState.ServerName
}

// SesTlsClientCa 返回签发客户端证书的 CA 名，无则 ""
func (e *BaseEnv) SesTlsClientCa() string {
	if e.req == nil || e.req.Session == nil {
		return ""
	}
	ses := e.req.Session
	if !ses.IsSecure || ses.TlsState == nil || !ses.TlsState.ClientAuth {
		return ""
	}
	return ses.TlsState.ClientCAName
}

// ---------- 通用 ----------

// Env 读取进程环境变量，不存在返回 ""
func (e *BaseEnv) Env(name string) string {
	return os.Getenv(name)
}

// LogInfo 输出 info 日志（附带 logid），返回 true 便于嵌入条件表达式（cond && LogInfo(...)）
func (e *BaseEnv) LogInfo(msg string) bool {
	log.Logger.Info("bfe_expr: %s (logid: %s)", msg, e.logId())
	return true
}

// LogWarn 输出 warn 日志（附带 logid），返回 true 便于嵌入条件表达式
func (e *BaseEnv) LogWarn(msg string) bool {
	log.Logger.Warn("bfe_expr: %s (logid: %s)", msg, e.logId())
	return true
}

func (e *BaseEnv) logId() string {
	if e.req == nil {
		return ""
	}
	return e.req.LogId
}

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

/* http_env.go - http access for expressions (stub in phase 2)
/*
DESCRIPTION
	本期仅保留与 go-bfe-pro 一致的接口面，实现为 stub：
	调用记 warn（未实现）并返回零值，不改变规则执行结果。
	服务注册与配置热更 API（RegisterHttpService 等）本期不暴露；
	保留接口而非删除，是为让配置误用在运行期产生明确 warn，
	而不是以「未知函数」在加载期编译失败。
*/

package bfe_expr

import (
	"github.com/bfenetworks/go-lib/log"
)

// HttpEnv 提供表达式内的 http 访问函数（本期 stub）
type HttpEnv struct{}

// HttpClient http client 表达式句柄（本期 stub，所有方法返回零值）
type HttpClient struct{}

// HttpResp http 响应句柄（本期 stub，所有方法返回零值）
type HttpResp struct{}

// GetHttpClient 按超时返回 client（本期 stub）
func (e *HttpEnv) GetHttpClient(timeoutMs int) *HttpClient {
	warnExprStub("GetHttpClient")
	return &HttpClient{}
}

// HttpService 按名字获取 http 服务（本期 stub）
func (e *HttpEnv) HttpService(name string) *HttpClient {
	warnExprStub("HttpService")
	return &HttpClient{}
}

// HttpGet 发起 GET 请求（本期 stub）
func (e *HttpEnv) HttpGet(url string, headerKv ...string) *HttpResp {
	warnExprStub("HttpGet")
	return &HttpResp{}
}

// HttpPost 发起 POST 请求（本期 stub）
func (e *HttpEnv) HttpPost(url, contentType, body string, headerKv ...string) *HttpResp {
	warnExprStub("HttpPost")
	return &HttpResp{}
}

// Get 发起 GET 请求（本期 stub）
func (c *HttpClient) Get(rawURL string, headerKv ...string) *HttpResp {
	warnExprStub("HttpClient.Get")
	return &HttpResp{}
}

// Post 发起 POST 请求（本期 stub）
func (c *HttpClient) Post(rawURL, contentType, body string, headerKv ...string) *HttpResp {
	warnExprStub("HttpClient.Post")
	return &HttpResp{}
}

// Do 发起请求（本期 stub）
func (c *HttpClient) Do(method, rawURL, contentType, body string, headerKv ...string) *HttpResp {
	warnExprStub("HttpClient.Do")
	return &HttpResp{}
}

// WithHeader 返回带默认请求头的 client 副本（本期 stub）
func (c *HttpClient) WithHeader(name, value string) *HttpClient {
	warnExprStub("HttpClient.WithHeader")
	return &HttpClient{}
}

// Status 返回 HTTP 状态码（本期 stub，返回 0）
func (r *HttpResp) Status() int { return 0 }

// Body 返回响应体（本期 stub，返回 ""）
func (r *HttpResp) Body() string { return "" }

// Header 按名字取响应头（本期 stub，返回 ""）
func (r *HttpResp) Header(name string) string { return "" }

// BodyJson 按路径取响应体 JSON 字段（本期 stub，返回 ""）
func (r *HttpResp) BodyJson(path string) string { return "" }

// warnExprStub 记录数据源 stub 调用
func warnExprStub(name string) {
	log.Logger.Warn("bfe_expr: %s not implemented in phase 2, return zero value", name)
}

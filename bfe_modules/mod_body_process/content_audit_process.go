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

package mod_body_process

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"github.com/baidu/go-lib/log"
)

var (
	httpClient *http.Client
	mutex 	 sync.RWMutex
)

func GetHTTPClient() *http.Client {
	mutex.RLock()
	defer mutex.RUnlock()
	if httpClient == nil {
		return &http.Client{}
	}
	return httpClient
}

func SetHTTPClient(client *http.Client) {
	mutex.Lock()
	defer mutex.Unlock()
	if client == nil {
		httpClient = nil
	} else {
		httpClient = client
	}
}

func init() {
	// 初始化HTTP客户端
	SetHTTPClient(&http.Client{
		Timeout: 10 * time.Second, // 设置超时时间
	})
}

type ContentAudit struct {
	url     string
	replace bool // 是否替换内容
}

func NewContentAudit(urlStr string, replace bool) (*ContentAudit, error) {
	if replace {
		urlStr = strings.TrimSuffix(urlStr, "/") + "/text-replace"
	} else {
		urlStr = strings.TrimSuffix(urlStr, "/") + "/text-filter"
	}

	return &ContentAudit{url: urlStr, replace: replace}, nil
}

func GetAuditData(ev Event) ([]byte, error) {
	if ev == nil {
		return nil, fmt.Errorf("event is nil")
	}

	switch e := ev.(type) {
	case *RawEvent:
		return *e, nil
	case *SSEEvent:
		return e.Data, nil
	default:
		return nil, fmt.Errorf("unsupported event type: %T", ev)
	}
}

func SetAuditData(ev Event, data []byte) error {
	if ev == nil {
		return fmt.Errorf("event is nil")
	}

	switch e := ev.(type) {
	case *RawEvent:
		*e = data
	case *SSEEvent:
		e.Data = data
	default:
		return fmt.Errorf("unsupported event type: %T", ev)
	}
	return nil
}

func (caf *ContentAudit) Process(evs []Event) ([]Event, error) {
	// 这里可以实现内容审计逻辑
	// 返回处理后的事件列表和可能的错误
	client := GetHTTPClient()
	for _, ev := range evs {
		data, err := GetAuditData(ev)
		if err != nil {
			// return nil, fmt.Errorf("failed to get audit data: %w", err)
			log.Logger.Error("failed to get audit data: %v", err)
			continue // 如果获取数据失败，跳过当前事件
		}
		resp, err := client.PostForm(caf.url, url.Values{ "txt": {string(data)} })
		if err != nil {
			// return nil, fmt.Errorf("failed to audit content: %w", err)
			log.Logger.Error("failed to audit content: %v", err)
			continue // 如果请求失败，跳过当前事件
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			// return nil, fmt.Errorf("failed to read response body: %w", err)
			log.Logger.Error("failed to read response body: %v", err)
			continue // 如果读取响应失败，跳过当前事件
		}
		resp.Body.Close()
		var result TextFilterResult
		err = json.Unmarshal(body, &result)
		if err != nil {
			// return nil, fmt.Errorf("failed to unmarshal response: %w", err)
			log.Logger.Error("failed to unmarshal response: %v", err)
			continue // 如果解析响应失败，跳过当前事件
		}
		if caf.replace {
			if result.ResultText != "" {
				err = SetAuditData(ev, []byte(result.ResultText))
				if err != nil {
					// return nil, fmt.Errorf("failed to set audit data: %w", err)
					log.Logger.Error("failed to set audit data: %v", err)
					continue // 如果设置数据失败，跳过当前事件
				}
			}
		} else {
			if result.RiskLevel == "REJECT" || (result.RiskLevel == "REVIEW" && result.SentimentScore < -0.5) {
				return nil, fmt.Errorf("content audit failed: %v", result)
			}
		}
	}
	return evs, nil
}

type TextFilterResult struct {
	Code           int32
	Message        string
	RequestId      string
	RiskLevel      string
	RiskCode       string
	SentimentScore float32
	ResultText     string

	Details  []TextFilterDetailItem
	Contacts []TextFilterContactItem
}

type TextFilterDetailItem struct{
	RiskLevel string
	RiskCode string
	Position string
	Text string
}

type TextFilterContactItem struct {
	ContactType   string
	ContactString string
	Position      string
}

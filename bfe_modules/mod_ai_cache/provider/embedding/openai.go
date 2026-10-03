// Copyright (c) 2026 The BFE Authors.
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

package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// openAIEmbeddingPath is the OpenAI compatible embedding endpoint (also
// implemented by Ollama, vLLM, SiliconFlow, DashScope compatible mode, ...).
const openAIEmbeddingPath = "/v1/embeddings"

// maxEmbeddingRespBody bounds the embedding service response body (4MB).
const maxEmbeddingRespBody = 4 << 20

// Config carries the connection info of an OpenAI compatible embedding
// service.
type Config struct {
	ServiceHost string
	ServicePort int
	UseHttps    bool
	ApiKey      string // bearer credential, never logged
	Model       string
	TimeoutMs   int
}

type openAIProvider struct {
	endpoint string
	apiKey   string
	model    string
	timeout  time.Duration
	client   *http.Client
}

// NewOpenAIProvider creates an OpenAI compatible embedding provider.
func NewOpenAIProvider(cfg Config) (*openAIProvider, error) {
	if cfg.ServiceHost == "" {
		return nil, fmt.Errorf("embedding service host is empty")
	}
	if cfg.ServicePort < 1 || cfg.ServicePort > 65535 {
		return nil, fmt.Errorf("embedding service port %d invalid", cfg.ServicePort)
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("embedding model is empty")
	}
	timeout := cfg.TimeoutMs
	if timeout <= 0 {
		timeout = 500
	}

	scheme := "http"
	if cfg.UseHttps {
		scheme = "https"
	}

	return &openAIProvider{
		endpoint: fmt.Sprintf("%s://%s:%d%s", scheme, cfg.ServiceHost, cfg.ServicePort, openAIEmbeddingPath),
		apiKey:   cfg.ApiKey,
		model:    cfg.Model,
		timeout:  time.Duration(timeout) * time.Millisecond,
		client:   &http.Client{},
	}, nil
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed calls POST /v1/embeddings with {"model": ..., "input": text} and
// returns data[0].embedding. Any error (timeout, non-200, malformed
// response) is returned to the caller, which degrades to a cache miss.
func (p *openAIProvider) Embed(text string) ([]float32, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"model": p.model,
		"input": text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request err: %s", err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("new request err: %s", err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request err: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(http.MaxBytesReader(nil, resp.Body, maxEmbeddingRespBody))
	if err != nil {
		return nil, fmt.Errorf("read response err: %s", err.Error())
	}

	var embResp openAIEmbeddingResponse
	if err := json.Unmarshal(body, &embResp); err != nil {
		return nil, fmt.Errorf("parse response err: %s", err.Error())
	}
	if len(embResp.Data) == 0 || len(embResp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding in response")
	}

	return embResp.Data[0].Embedding, nil
}

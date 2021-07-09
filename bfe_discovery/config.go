// Copyright (c) 2019 Baidu, Inc.
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

package bfe_discovery

import "time"

const (
	Prefix = "/bfe"
	Suffix = "/"
)

type Config struct {
	Addrs       []string
	DialTimeout time.Duration

	// TODO bfe_ prefix
	PathPrefix string

	// TODO cancel context timeout
	OpTimeout time.Duration

	// TODO tls
	TLSConfig *TLSConfig

	Username string
	Password string
}

type TLSConfig struct {
	CertFile   string
	KeyFile    string
	CACertFile string
}

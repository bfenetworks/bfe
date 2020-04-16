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

package config

import (
	"io/ioutil"
)

import (
	"gopkg.in/square/go-jose.v2"
)

// auth config (common in module config & product config)
type AuthConfig struct {
	Secret              *jose.JSONWebKey
	SecretPath          string
	EnabledHeaderClaims bool
	ValidateNested      bool
	ValidateClaimExp    bool
	ValidateClaimNbf    bool
	ValidateClaimIss    string
	ValidateClaimSub    string
	ValidateClaimAud    string
}

func (config *AuthConfig) BuildSecret() (err error) {
	config.Secret = new(jose.JSONWebKey)

	secret, err := ioutil.ReadFile(config.SecretPath)
	if err != nil {
		return err
	}

	return config.Secret.UnmarshalJSON(secret)
}

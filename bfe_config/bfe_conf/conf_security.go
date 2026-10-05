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

package bfe_conf

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/bfenetworks/bfe/bfe_util"
)

// ConfigSecurity holds the file-encryption keyring configuration. The
// keyring decrypts enc$v1$ field-level ciphertext in control-plane
// distributed config files (token_rule.data Tokens keys, cluster_conf.data
// AIConf.Keys[].Key) so that no key material is persisted on disk in
// plaintext. An empty KeyFile disables decryption: marker-prefixed fields
// then fail the affected config load (previously effective config is kept
// on reload; startup fails fast). See
// docs/zh_cn/sys_design/config_file_field_encryption.md.
type ConfigSecurity struct {
	// path of the keyring file (multiple keyIDs coexist); empty = disabled
	KeyFile string
}

func (cfg *ConfigSecurity) SetDefaultConf() {
}

func (cfg *ConfigSecurity) Check(confRoot string) error {
	if cfg.KeyFile == "" {
		return nil
	}
	cfg.KeyFile = bfe_util.ConfPathProc(cfg.KeyFile, confRoot)
	return nil
}

// LoadSecurityConf reads only the [Security] section of <confRoot>/bfe.conf.
// Modules that need the file-encryption keyring path (but not the full
// server config) use this instead of a full BfeConfigLoad re-parse. It is a
// deliberate line scanner rather than a gcfg partial decode: gcfg reports
// every unrelated section as warnings, which would spam the logs on every
// module init.
func LoadSecurityConf(confRoot string) (ConfigSecurity, error) {
	var cfg ConfigSecurity

	confPath := path.Join(confRoot, "bfe.conf")
	file, err := os.Open(confPath)
	if err != nil {
		return cfg, fmt.Errorf("bfe_conf: open %s: %s", confPath, err)
	}
	defer file.Close()

	inSecurity := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			end := strings.IndexByte(line, ']')
			if end < 0 {
				continue
			}
			inSecurity = strings.EqualFold(strings.TrimSpace(line[1:end]), "Security")
			continue
		}
		if !inSecurity {
			continue
		}

		eq := strings.IndexByte(line, '=')
		if eq < 0 || !strings.EqualFold(strings.TrimSpace(line[:eq]), "KeyFile") {
			continue
		}
		cfg.KeyFile = parseBfeConfValue(strings.TrimSpace(line[eq+1:]))
	}
	if err := scanner.Err(); err != nil {
		return cfg, fmt.Errorf("bfe_conf: read %s: %s", confPath, err)
	}

	if err := cfg.Check(confRoot); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// parseBfeConfValue strips surrounding quotes from a bfe.conf value and
// cuts an unquoted trailing comment.
func parseBfeConfValue(value string) string {
	if len(value) >= 2 && value[0] == '"' {
		if end := strings.IndexByte(value[1:], '"'); end >= 0 {
			return value[1 : 1+end]
		}
		return strings.Trim(value, `"`)
	}
	if i := strings.Index(value, " #"); i >= 0 {
		value = value[:i]
	}
	return strings.TrimSpace(value)
}

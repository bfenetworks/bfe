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

package mod_access_pb3

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"

	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/go-lib/log"
)

// Length limits of the whitelisted header key/value, aligned with the
// downstream Doris column width (key VARCHAR(128), value VARCHAR(2048))
// so that over-long values never dirty the Routine Load import.
const (
	maxWhitelistedHeaderKeyLen   = 128
	maxWhitelistedHeaderValueLen = 2048
)

// knownDataSections lists the top-level sections of mod_access_pb.data
// understood by this version. Unknown sections are ignored with a Warn
// log (forward compatibility: a newer config producer must not break an
// older BFE).
var knownDataSections = map[string]bool{
	"Version":       true,
	"access_header": true,
}

// AccessHeaderConf is the in-memory form of the "access_header" section:
// whitelists of request/response headers collected into the proto fields
// req_headers (55) / res_headers (158). Names are stored in HTTP
// canonical form (see normalizeHeaderNames).
type AccessHeaderConf struct {
	ReqHeaders []string
	ResHeaders []string
}

// ModAccessPbConf is the in-memory form of mod_access_pb.data. New dynamic
// configuration sections are added here as new fields.
type ModAccessPbConf struct {
	Version      string
	AccessHeader AccessHeaderConf
}

func emptyModAccessPbConf() *ModAccessPbConf {
	return &ModAccessPbConf{}
}

// ModAccessPbConfHolder guards the runtime config with an RWMutex:
// validation happens outside the lock (in ModAccessPbDataLoad), Update
// only swaps the reference under the write lock, and readers take the
// snapshot under the read lock. A reload therefore never exposes a
// half-new/half-old config, and a failed reload keeps the old one.
type ModAccessPbConfHolder struct {
	lock sync.RWMutex
	conf *ModAccessPbConf
}

func NewModAccessPbConfHolder() *ModAccessPbConfHolder {
	return &ModAccessPbConfHolder{conf: emptyModAccessPbConf()}
}

func (h *ModAccessPbConfHolder) Update(c *ModAccessPbConf) {
	h.lock.Lock()
	h.conf = c
	h.lock.Unlock()
}

func (h *ModAccessPbConfHolder) Snapshot() *ModAccessPbConf {
	h.lock.RLock()
	c := h.conf
	h.lock.RUnlock()
	if c == nil {
		return emptyModAccessPbConf()
	}
	return c
}

// modAccessPbDataFile mirrors the JSON schema of mod_access_pb.data.
type modAccessPbDataFile struct {
	Version      string            `json:"Version"`
	AccessHeader *accessHeaderFile `json:"access_header"`
}

type accessHeaderFile struct {
	ReqHeaders []string `json:"req_headers"`
	ResHeaders []string `json:"res_headers"`
}

// ModAccessPbDataLoad loads and validates mod_access_pb.data. It returns
// an error for any malformed content; the caller decides whether a
// missing file is tolerable (it is at module Init).
func ModAccessPbDataLoad(path string) (*ModAccessPbConf, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// detect unknown top-level sections (forward compatibility)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf, &raw); err != nil {
		return nil, fmt.Errorf("mod_access_pb.data: invalid json: %s", err.Error())
	}
	for key := range raw {
		if !knownDataSections[key] {
			log.Logger.Warn("mod_access_pb.data: unknown section %q ignored", key)
		}
	}

	var file modAccessPbDataFile
	if err := json.Unmarshal(buf, &file); err != nil {
		return nil, fmt.Errorf("mod_access_pb.data: unmarshal err: %s", err.Error())
	}

	conf := &ModAccessPbConf{Version: file.Version}

	if file.AccessHeader != nil {
		conf.AccessHeader.ReqHeaders, err = normalizeHeaderNames(file.AccessHeader.ReqHeaders)
		if err != nil {
			return nil, fmt.Errorf("mod_access_pb.data: access_header.req_headers: %s", err.Error())
		}
		conf.AccessHeader.ResHeaders, err = normalizeHeaderNames(file.AccessHeader.ResHeaders)
		if err != nil {
			return nil, fmt.Errorf("mod_access_pb.data: access_header.res_headers: %s", err.Error())
		}
	}

	return conf, nil
}

// normalizeHeaderNames validates the configured header names and
// canonicalizes them (http.CanonicalHeaderKey) so that runtime matching
// is a single map lookup and is case-insensitive by construction. The
// canonical form is also what gets written to the proto key, keeping the
// aggregation key stable in downstream Doris. Duplicate names after
// canonicalization are an error (fail-fast, consistent with BFE config
// checking style). No sensitive-header restriction is applied here by
// design: whitelist content is not limited on the data plane.
func normalizeHeaderNames(names []string) ([]string, error) {
	if names == nil {
		return nil, nil
	}
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return nil, fmt.Errorf("empty header name")
		}
		if len(trimmed) > maxWhitelistedHeaderKeyLen {
			return nil, fmt.Errorf("header name too long (%d > %d): %q",
				len(trimmed), maxWhitelistedHeaderKeyLen, trimmed)
		}
		if !isValidHeaderName(trimmed) {
			return nil, fmt.Errorf("invalid header name %q (allowed: RFC 7230 token chars)", trimmed)
		}
		canonical := bfe_http.CanonicalHeaderKey(trimmed)
		if seen[canonical] {
			return nil, fmt.Errorf("duplicate header name %q after canonicalization", canonical)
		}
		seen[canonical] = true
		out = append(out, canonical)
	}
	return out, nil
}

// isValidHeaderName reports whether s consists of RFC 7230 token
// characters (the legal set for HTTP header field names).
func isValidHeaderName(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isTokenChar(s[i]) {
			return false
		}
	}
	return len(s) > 0
}

func isTokenChar(c byte) bool {
	if c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	return false
}

// truncateHeaderValue trims v to the downstream column width. The cut may
// split a multi-byte rune; that is accepted for a best-effort diagnostic
// field and still beats rejecting the whole log batch on import.
func truncateHeaderValue(v string) string {
	if len(v) > maxWhitelistedHeaderValueLen {
		return v[:maxWhitelistedHeaderValueLen]
	}
	return v
}

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

package bfe_model_protocol

import (
	"sort"

	"github.com/bfenetworks/bfe/bfe_http"
)

// CredentialHeaderNames returns the credential headers declared by the
// registered adapters, in canonical form and sorted. These are the headers a
// client may use to authenticate to BFE (detect.go recognizes the same set),
// and therefore the set that must not survive into an outbound request whose
// credential is injected by the gateway (issue #1447: an upstream picks its
// credential by its own provider-private priority, so a residual client
// header would override the injected one). The list is derived from the
// registry, so a newly registered protocol is covered without further work.
func CredentialHeaderNames() []string {
	seen := make(map[string]struct{}, len(adapters))
	for _, a := range adapters {
		name := bfe_http.CanonicalHeaderKey(a.CredentialHeader())
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ForeignCredentialHeaders returns the recognized credential headers present
// in h other than keep, in canonical form. A header present with an empty
// value counts as present: an empty residual header is harmless upstream
// today, but "present means remove" keeps the rule unambiguous. keep may be
// given in any case; an empty keep selects every recognized credential
// header.
func ForeignCredentialHeaders(h bfe_http.Header, keep string) []string {
	keep = bfe_http.CanonicalHeaderKey(keep)
	var found []string
	for _, name := range CredentialHeaderNames() {
		if name == keep {
			continue
		}
		if len(h.Values(name)) > 0 {
			found = append(found, name)
		}
	}
	return found
}

// RemoveForeignCredentialHeaders deletes every recognized credential header
// other than keep from h and returns the removed header names. Unrecognized
// credential-like headers (e.g. an Azure-style api-key) are left untouched.
func RemoveForeignCredentialHeaders(h bfe_http.Header, keep string) []string {
	removed := ForeignCredentialHeaders(h, keep)
	for _, name := range removed {
		h.Del(name)
	}
	return removed
}

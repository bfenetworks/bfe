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

package bfe_server

import (
	"net/url"
	"testing"

	"github.com/bfenetworks/bfe/bfe_config/bfe_conf"
	"github.com/bfenetworks/bfe/bfe_http"
)

// issue #1468: the Accept-Encoding of an AI upstream request is narrowed to the
// coding set the gateway can decode, so a compressed streaming response still
// yields a parseable usage. The inbound request must never be touched.

func newAcceptEncodingRequest(inbound string) (*bfe_http.Request, *bfe_http.Request) {
	inreq := &bfe_http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/v1/chat/completions"},
		Header: bfe_http.Header{},
	}
	if inbound != "" {
		inreq.Header.Set("Accept-Encoding", inbound)
	}

	// Mirror doSingleAIForward: a shallow copy sharing the header map.
	outreq := new(bfe_http.Request)
	*outreq = *inreq
	return inreq, outreq
}

func TestApplyAIUpstreamAcceptEncodingAuto(t *testing.T) {
	cases := []struct {
		name     string
		inbound  string
		mode     string
		want     string
		wantSame bool // the header must be left byte-identical
	}{
		{
			name:    "narrows the client list",
			inbound: "gzip, deflate, br, zstd",
			mode:    bfe_conf.AIUpstreamAcceptEncodingAuto,
			want:    "gzip, deflate",
		},
		{
			name:    "keeps q values",
			inbound: "gzip;q=0.8, zstd",
			mode:    bfe_conf.AIUpstreamAcceptEncodingAuto,
			want:    "gzip;q=0.8",
		},
		{
			name:    "expands the wildcard to the decodable set",
			inbound: "*",
			mode:    bfe_conf.AIUpstreamAcceptEncodingAuto,
			want:    "gzip, deflate",
		},
		{
			name:    "drops a refused coding",
			inbound: "gzip, br;q=0",
			mode:    bfe_conf.AIUpstreamAcceptEncodingAuto,
			want:    "gzip",
		},
		{
			name:     "already inside the decodable set",
			inbound:  "gzip, deflate",
			mode:     bfe_conf.AIUpstreamAcceptEncodingAuto,
			wantSame: true,
		},
		{
			name:     "empty intersection keeps the client value",
			inbound:  "zstd",
			mode:     bfe_conf.AIUpstreamAcceptEncodingAuto,
			wantSame: true,
		},
		{
			name:     "absent header stays absent",
			inbound:  "",
			mode:     bfe_conf.AIUpstreamAcceptEncodingAuto,
			wantSame: true,
		},
		{
			name:     "off forwards the client value",
			inbound:  "gzip, deflate, br, zstd",
			mode:     bfe_conf.AIUpstreamAcceptEncodingOff,
			wantSame: true,
		},
		{
			name:    "unknown mode behaves as auto",
			inbound: "gzip, zstd",
			mode:    "bogus",
			want:    "gzip",
		},
		{
			name:    "identity strips compression",
			inbound: "gzip, deflate, br, zstd",
			mode:    bfe_conf.AIUpstreamAcceptEncodingIdentity,
			want:    "identity",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inreq, outreq := newAcceptEncodingRequest(tc.inbound)

			applyAIUpstreamAcceptEncoding(outreq, tc.mode)

			if tc.wantSame {
				if got := outreq.Header.Get("Accept-Encoding"); got != tc.inbound {
					t.Errorf("outbound Accept-Encoding = %q, want the client value %q", got, tc.inbound)
				}
			} else if got := outreq.Header.Get("Accept-Encoding"); got != tc.want {
				t.Errorf("outbound Accept-Encoding = %q, want %q", got, tc.want)
			}

			// The header map is shared with the inbound request, so any mutation
			// must happen on a clone.
			if got := inreq.Header.Get("Accept-Encoding"); got != tc.inbound {
				t.Errorf("inbound Accept-Encoding = %q, want %q (must not be modified)", got, tc.inbound)
			}
		})
	}
}

func TestApplyAIUpstreamAcceptEncodingNilRequest(t *testing.T) {
	// Must not panic: the helper is called on the forward path.
	applyAIUpstreamAcceptEncoding(nil, bfe_conf.AIUpstreamAcceptEncodingAuto)
}

func TestNarrowAcceptEncodingUnchangedValue(t *testing.T) {
	// Nothing dropped => no rewrite at all (the header bytes stay as sent).
	for _, value := range []string{"gzip", "deflate", "gzip,deflate"} {
		got, changed := narrowAcceptEncoding(value)
		if changed {
			t.Errorf("narrowAcceptEncoding(%q) reported a change (%q)", value, got)
		}
	}
}

func TestNormalizeAIUpstreamAcceptEncoding(t *testing.T) {
	cases := map[string]string{
		"":          bfe_conf.AIUpstreamAcceptEncodingAuto,
		"auto":      bfe_conf.AIUpstreamAcceptEncodingAuto,
		"AUTO":      bfe_conf.AIUpstreamAcceptEncodingAuto,
		" identity": bfe_conf.AIUpstreamAcceptEncodingIdentity,
		"off":       bfe_conf.AIUpstreamAcceptEncodingOff,
		"nonsense":  bfe_conf.AIUpstreamAcceptEncodingAuto,
	}

	for in, want := range cases {
		if got := bfe_conf.NormalizeAIUpstreamAcceptEncoding(in); got != want {
			t.Errorf("NormalizeAIUpstreamAcceptEncoding(%q) = %q, want %q", in, got, want)
		}
	}
}

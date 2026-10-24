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
	"strconv"
	"testing"
)

// Tests for the access-log logid conversion (bfenetworks/bfe#1417).
//
// Background: the report table `bfe_ai_request_log` dedups on
// (hostid, log_time, ai_apikey_id, ai_requested_model, logid) with a
// second-granularity log_time and a full-row ON DUPLICATE KEY UPDATE. A logid
// stuck at 0 therefore turned every same-second concurrent request of one API
// key and model into an overwrite, losing n-1 of n rows. The fix makes logid a
// real per-request unique non-zero value:
//
//	B1: genLogId emits a decimal string <= 2^63-1, so the conversion into the
//	    uint64 field is lossless (logid_str == FormatUint(logid, 10));
//	fallback: when req.LogId cannot be converted, a local random id is used and
//	    counted, so the row stays dedup-safe and the condition is observable.

func TestLogIdToUint64(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want uint64
		ok   bool
	}{
		{"typical 19 digits", "1234567890123456789", 1234567890123456789, true},
		{"max int64", "9223372036854775807", 9223372036854775807, true},
		{"zero", "0", 0, true},
		{"one above max int64", "9223372036854775808", 0, false},
		{"max uint64", "18446744073709551615", 0, false},
		{"empty (mod_logid not loaded)", "", 0, false},
		{"legacy 32-char hex", "0f1e2d3c4b5a69788796a5b4c3d2e1f0", 0, false},
		{"hex letters inside", "12a45", 0, false},
		{"negative", "-1", 0, false},
		{"leading space", " 123", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := logIdToUint64(tt.in)
			if ok != tt.ok {
				t.Fatalf("logIdToUint64(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("logIdToUint64(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestRandUint64NonZero(t *testing.T) {
	seen := make(map[uint64]bool)
	for i := 0; i < 64; i++ {
		v := randUint64NonZero()
		if v == 0 || v > maxLogid {
			t.Fatalf("randUint64NonZero() = %d, want in [1, %d]", v, maxLogid)
		}
		if seen[v] {
			t.Fatalf("randUint64NonZero() returned duplicate value %d", v)
		}
		seen[v] = true
	}
}

// TestRequestLogGenLogidParsed covers the normal path: req.LogId is a decimal
// string, so the logged logid is that very value — the consumer can reproduce
// the string with FormatUint(logid, 10) or paste it into SQL as is.
func TestRequestLogGenLogidParsed(t *testing.T) {
	m, req, res := makeRequestLogTest(t)
	bfeLog := m.requestLogGen(req, res)

	if bfeLog.Logid == nil || *bfeLog.Logid == 0 {
		t.Fatalf("Logid = %v, want non-zero", bfeLog.Logid)
	}
	if got := strconv.FormatUint(*bfeLog.Logid, 10); got != req.LogId {
		t.Errorf("FormatUint(Logid) = %q, want req.LogId %q (lossless conversion)", got, req.LogId)
	}
	if n := m.state.LogidRegenerated.Get(); n != 0 {
		t.Errorf("LogidRegenerated = %d, want 0", n)
	}
}

// TestRequestLogGenLogidFallback covers every path that cannot yield a usable
// id: empty (mod_logid not loaded), malformed, out-of-range, and a legacy
// 32-char hex value seen while an uplevel BFE is still on the old version.
// Each must still produce a unique non-zero logid — the row stays dedup-safe —
// and must be counted so the condition is observable (assertion A-03).
func TestRequestLogGenLogidFallback(t *testing.T) {
	cases := []string{
		"",
		"abc",
		"12a45",
		"18446744073709551615",
		"0f1e2d3c4b5a69788796a5b4c3d2e1f0",
	}

	for _, logId := range cases {
		m, req, res := makeRequestLogTest(t)
		req.LogId = logId
		bfeLog := m.requestLogGen(req, res)

		if bfeLog.Logid == nil || *bfeLog.Logid == 0 {
			t.Errorf("req.LogId = %q: Logid = %v, want non-zero", logId, bfeLog.Logid)
			continue
		}
		if *bfeLog.Logid > maxLogid {
			t.Errorf("req.LogId = %q: Logid = %d, above max %d", logId, *bfeLog.Logid, maxLogid)
		}
		if n := m.state.LogidRegenerated.Get(); n != 1 {
			t.Errorf("req.LogId = %q: LogidRegenerated = %d, want 1", logId, n)
		}
	}

	// two requests falling back at the same second must not share an id,
	// otherwise they would overwrite each other under the report table's
	// unique key again
	m1, req1, res1 := makeRequestLogTest(t)
	req1.LogId = ""
	id1 := *m1.requestLogGen(req1, res1).Logid

	m2, req2, res2 := makeRequestLogTest(t)
	req2.LogId = ""
	id2 := *m2.requestLogGen(req2, res2).Logid

	if id1 == id2 {
		t.Errorf("regenerated ids collided: %d == %d", id1, id2)
	}
}

// TestSessionLogGenLogidFallback covers the shared fallback of the session id
// column (RequestLog.session_id and the session log's logid): it uses its own
// counter, and never writes 0.
func TestSessionLogGenLogidFallback(t *testing.T) {
	m := NewModuleAccessPb3()
	session, _ := makeSessionLogTest(t)
	session.SessionId = "0f1e2d3c4b5a69788796a5b4c3d2e1f0" // legacy hex

	bfeLog := m.sessionLogGen(session)

	if bfeLog.Logid == nil || *bfeLog.Logid == 0 {
		t.Fatalf("Logid = %v, want non-zero", bfeLog.Logid)
	}
	if *bfeLog.Logid > maxLogid {
		t.Errorf("Logid = %d, above max %d", *bfeLog.Logid, maxLogid)
	}
	if n := m.state.SessionIdRegenerated.Get(); n != 1 {
		t.Errorf("SessionIdRegenerated = %d, want 1", n)
	}
	if n := m.state.LogidRegenerated.Get(); n != 0 {
		t.Errorf("LogidRegenerated = %d, want 0 (separate counter)", n)
	}
}

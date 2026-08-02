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

package mod_doh

import (
	"path/filepath"
	"testing"
)

func TestConfLoadFailures(t *testing.T) {
	// missing file
	_, err := ConfLoad("./testdata/mod_doh/not_exist.conf", "")
	if err == nil {
		t.Errorf("ConfLoad() should fail with missing file")
	}

	// invalid condition
	cr := prepareConfRoot(t, `[Basic]
Cond = "bad_cond()"
[Dns]
Address = "127.0.0.1:53"
Timeout = 1000
`)
	_, err = ConfLoad(filepath.Join(cr, "mod_doh", "mod_doh.conf"), "")
	if err == nil {
		t.Errorf("ConfLoad() should fail with invalid condition")
	}

	// invalid RetryMax
	cr = prepareConfRoot(t, `[Basic]
Cond = "default_t()"
[Dns]
Address = "127.0.0.1:53"
RetryMax = -1
Timeout = 1000
`)
	_, err = ConfLoad(filepath.Join(cr, "mod_doh", "mod_doh.conf"), "")
	if err == nil {
		t.Errorf("ConfLoad() should fail with invalid RetryMax")
	}

	// invalid Timeout
	cr = prepareConfRoot(t, `[Basic]
Cond = "default_t()"
[Dns]
Address = "127.0.0.1:53"
Timeout = 0
`)
	_, err = ConfLoad(filepath.Join(cr, "mod_doh", "mod_doh.conf"), "")
	if err == nil {
		t.Errorf("ConfLoad() should fail with invalid Timeout")
	}

	// invalid DNS address
	cr = prepareConfRoot(t, `[Basic]
Cond = "default_t()"
[Dns]
Address = "not-an-address"
Timeout = 1000
`)
	_, err = ConfLoad(filepath.Join(cr, "mod_doh", "mod_doh.conf"), "")
	if err == nil {
		t.Errorf("ConfLoad() should fail with invalid DNS address")
	}
}

func TestConfLoadValues(t *testing.T) {
	config, err := ConfLoad("./testdata/mod_doh/mod_doh.conf", "")
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}

	if config.Basic.Cond != "default_t()" {
		t.Errorf("Basic.Cond = %s, want default_t()", config.Basic.Cond)
	}
	if config.Dns.Address != "127.0.0.1:53" {
		t.Errorf("Dns.Address = %s, want 127.0.0.1:53", config.Dns.Address)
	}
	if config.Dns.Timeout != 1000 {
		t.Errorf("Dns.Timeout = %d, want 1000", config.Dns.Timeout)
	}
	if config.Dns.RetryMax != 0 {
		t.Errorf("Dns.RetryMax = %d, want 0", config.Dns.RetryMax)
	}
	if !config.Log.OpenDebug {
		t.Errorf("Log.OpenDebug = false, want true")
	}
}

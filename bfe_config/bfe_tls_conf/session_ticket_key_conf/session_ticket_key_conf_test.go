// Copyright (c) 2019 The BFE Authors.
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

package session_ticket_key_conf

import (
	"testing"
)

func TestSessionTicketKeyConfLoad(t *testing.T) {
	conf, err := SessionTicketKeyConfLoad("./testdata/session_ticket_key.data")
	if err != nil {
		t.Errorf("found err while loading conf: %s", err.Error())
	}

	versionExpect := "20141112095308"
	if conf.Version != versionExpect {
		t.Errorf("wrong version (expect: %s, actual: %s)", versionExpect, conf.Version)
	}

	ticketKeyExpect := "08a0d852ef494143af613ef32d3c3931" +
		"4758885f7108e9ab021d55f422a454f7c9cd5a53978f48fa1063eadcdc06878f"
	if conf.SessionTicketKey != ticketKeyExpect {
		t.Errorf("wrong session ticket key (expect :%s, actual: %s)",
			ticketKeyExpect, conf.SessionTicketKey)
	}
}

func TestSessionTicketKeyConfLoad2(t *testing.T) {
	_, err := SessionTicketKeyConfLoad("./testdata/session_ticket_key.data2")
	if err == nil {
		t.Errorf("shuold found err while loading conf")
	}

	_, err = SessionTicketKeyConfLoad("./testdata/session_ticket_key.data3")
	if err == nil {
		t.Errorf("shuold found err while loading conf")
	}
}

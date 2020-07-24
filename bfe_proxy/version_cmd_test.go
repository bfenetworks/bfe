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

// Copyright (c) pires.
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

package bfe_proxy

import (
	"testing"
)

func TestLocal(t *testing.T) {
	b := byte(LOCAL)
	if ProtocolVersionAndCommand(b).IsUnspec() {
		t.Fail()
	}
	if !ProtocolVersionAndCommand(b).IsLocal() {
		t.Fail()
	}
	if ProtocolVersionAndCommand(b).IsProxy() {
		t.Fail()
	}
	if ProtocolVersionAndCommand(b).toByte() != b {
		t.Fail()
	}
}

func TestProxy(t *testing.T) {
	b := byte(PROXY)
	if ProtocolVersionAndCommand(b).IsUnspec() {
		t.Fail()
	}
	if ProtocolVersionAndCommand(b).IsLocal() {
		t.Fail()
	}
	if !ProtocolVersionAndCommand(b).IsProxy() {
		t.Fail()
	}
	if ProtocolVersionAndCommand(b).toByte() != b {
		t.Fail()
	}
}

func TestInvalidProtocolVersion(t *testing.T) {
	if !ProtocolVersionAndCommand(0x00).IsUnspec() {
		t.Fail()
	}
}

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
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	bfe_access_pb3 "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
)

func TestB2logMsgGen(t *testing.T) {
	pbMsg := &bfe_access_pb3.BfeLog{
		Product:   BFE_PRODUCT_ID,
		LogType:   LOG_TYPE_REQ,
		LogTag:    proto.String("req_unittest"),
		Logid:     proto.Uint64(12345),
		Timestamp: proto.Uint64(uint64(time.Now().Unix())),
	}

	msg, err := b2logMsgGen(pbMsg)
	if err != nil {
		t.Errorf("b2logMsgGen() error: %v", err)
	}
	if len(msg) == 0 {
		t.Error("b2logMsgGen() returns empty msg")
	}
}

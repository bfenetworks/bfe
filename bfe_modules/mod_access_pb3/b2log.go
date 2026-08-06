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
	"fmt"

	"github.com/bfenetworks/go-lib/log/log4go"
	"github.com/bfenetworks/bfe-access-pb/b2log"

	"google.golang.org/protobuf/proto"

	bfe_access_pb3 "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
)

// generate b2log msg from pb msg
func b2logMsgGen(pbMsg *bfe_access_pb3.BfeLog) ([]byte, error) {
	msize := proto.Size(pbMsg)

	// initialize buffer
	buf, err := log4go.NewBuffer(b2log.HEADER_SIZE + msize)
	if err != nil {
		return nil, fmt.Errorf("log4go.NewBuffer():%s", err.Error())
	}

	// write b2log header
	b2log.HeaderWrite(buf, msize)

	// marshal pb message to buf
	rawBytes, err := proto.Marshal(pbMsg)
	if err != nil {
		return nil, fmt.Errorf("proto.Marshal():%s", err.Error())
	}
	n := copy(buf[b2log.HEADER_SIZE:], rawBytes)

	// get binary log
	b2logMsg := buf[:b2log.HEADER_SIZE+n]

	return b2logMsg, nil
}

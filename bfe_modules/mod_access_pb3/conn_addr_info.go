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
	"net"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_util/net_util"
	"google.golang.org/protobuf/proto"

	bfe_access_pb3 "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
)

func ConnAddrInfoGen(session *bfe_basic.Session) *bfe_access_pb3.ConnAddrInfo {
	info := new(bfe_access_pb3.ConnAddrInfo)

	// bfe ip
	info.BfeIp = proto.Uint32(0)
	localIp := session.Connection.LocalAddr().(*net.TCPAddr).IP.To4()
	ip, err := net_util.IPv4ToUint32(localIp)
	if err == nil {
		info.BfeIp = proto.Uint32(ip)
	}

	// sock src ip
	info.SockSrcIp = proto.Uint32(0)
	cip, err := net_util.IPv4ToUint32(session.RemoteAddr.IP.To4())
	if err == nil {
		info.SockSrcIp = proto.Uint32(cip)
	}

	// whether sock src ip is trusted
	info.IsTrustSrcIp = proto.Bool(session.TrustSource())

	// vip
	if session.Vip != nil {
		vip4 := session.Vip.To4()
		if vip4 != nil {
			if vip, err := net_util.IPv4ToUint32(vip4); err == nil {
				info.Vip = proto.Uint32(vip)
			}
		} else {
			info.Vip6 = proto.String(session.Vip.String())
		}
	}

	return info
}

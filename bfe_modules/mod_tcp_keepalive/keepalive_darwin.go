// Copyright (c) 2021 The BFE Authors.
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

package mod_tcp_keepalive

import (
	"os"
	"syscall"
)

// from netinet/tcp.h (OS X 10.9.4)
const (
	_TCP_KEEPINTVL = 0x101 /* interval between keepalives */
	_TCP_KEEPCNT   = 0x102 /* number of keepalives before close */
)

func setIdle(fd int, secs int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPALIVE, secs))
}

func setCount(fd int, n int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, _TCP_KEEPCNT, n))
}

func setInterval(fd int, secs int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, _TCP_KEEPINTVL, secs))
}

func setNonblock(fd int) error {
	return os.NewSyscallError("setsockopt", syscall.SetNonblock(fd, true))
}

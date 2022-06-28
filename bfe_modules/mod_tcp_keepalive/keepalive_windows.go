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
	"github.com/baidu/go-lib/log"
)

func setIdle(fd int, secs int) error {
	if openDebug {
		log.Logger.Debug("mod[mod_tcp_keepalive] setIdle not implemented")
	}

	return nil
}

func setCount(fd int, n int) error {
	if openDebug {
		log.Logger.Debug("mod[mod_tcp_keepalive] setCount not implemented")
	}

	return nil
}

func setInterval(fd int, secs int) error {
	if openDebug {
		log.Logger.Debug("mod[mod_tcp_keepalive] setInterval not implemented")
	}

	return nil
}

func setNonblock(fd int) error {
	if openDebug {
		log.Logger.Debug("mod[mod_tcp_keepalive] setNonblock not implemented")
	}

	return nil
}

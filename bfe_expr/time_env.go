// Copyright (c) 2025 The BFE Authors.
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

/* time_env.go - time functions
/*
DESCRIPTION
	Now / InTimeRange / InPeriodicRange，语义对齐旧原语 bfe_time_range /
	bfe_periodic_time_range 及其 BfeTimeFetcher（支持 X-Bfe-Debug-Time）。
*/

package bfe_expr

import (
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_util"
)

// debugTimeHeader 调试时间头：存在时 Now() 取该头指定的时间而非当前时间
const debugTimeHeader = "X-Bfe-Debug-Time"

type TimeEnv struct {
	req *bfe_basic.Request
}

// Now 返回当前时间（UTC）；请求带 X-Bfe-Debug-Time 头时取该调试时间。
// 注意与 expr 内置 now() 区分：内置函数不受调试头影响，请统一使用 Now()。
func (e *TimeEnv) Now() time.Time {
	if e.req != nil && e.req.HttpRequest != nil {
		if values, ok := e.req.HttpRequest.Header[debugTimeHeader]; ok && len(values) > 0 {
			if debugTime, err := bfe_util.ParseTime(values[0]); err == nil {
				return debugTime
			}
		}
	}
	return time.Now().In(time.UTC)
}

// InTimeRange 判断 Now() 是否在 [start, end] 内（闭区间）。
// 时间格式与旧 bfe_time_range 相同（如 "2024-01-01 00:00:00 +0800"）；格式非法返回 false。
func (e *TimeEnv) InTimeRange(start, end string) bool {
	startTime, err := bfe_util.ParseTime(start)
	if err != nil {
		return false
	}
	endTime, err := bfe_util.ParseTime(end)
	if err != nil {
		return false
	}
	now := e.Now()
	return !now.Before(startTime) && !now.After(endTime)
}

// InPeriodicRange 判断当前时刻是否落在每天周期时段内（闭区间）。
// 时间格式与旧 bfe_periodic_time_range 相同（hhmmssZ，Z 为时区）。
func (e *TimeEnv) InPeriodicRange(start, end string) bool {
	startTs, offset1, err := bfe_util.ParseTimeOfDay(start)
	if err != nil {
		return false
	}
	endTs, offset2, err := bfe_util.ParseTimeOfDay(end)
	if err != nil {
		return false
	}
	if offset1 != offset2 {
		return false
	}

	startSec := startTs.Hour()*3600 + startTs.Minute()*60 + startTs.Second()
	endSec := endTs.Hour()*3600 + endTs.Minute()*60 + endTs.Second()
	if startSec > endSec {
		return false
	}

	now := e.Now().In(time.FixedZone("zone", offset1))
	nowSec := now.Hour()*3600 + now.Minute()*60 + now.Second()
	return nowSec >= startSec && nowSec <= endSec
}

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

package bfe_util

import (
	"fmt"
	"strings"
	"time"
)

var (
	// TimeZoneMap reference: https://en.wikipedia.org/wiki/List_of_military_time_zones
	TimeZoneMap = map[string]int{ // alphabet => time offset
		"Y": -12 * 3600, // UTC-12
		"X": -11 * 3600,
		"W": -10 * 3600,
		"V": -9 * 3600,
		"U": -8 * 3600,
		"T": -7 * 3600,
		"S": -6 * 3600,
		"R": -5 * 3600,
		"Q": -4 * 3600,
		"P": -3 * 3600,
		"O": -2 * 3600,
		"N": -1 * 3600, // UTC-1
		"Z": 0,         // UTC
		"A": 1 * 3600,  // UTC-1
		"B": 2 * 3600,
		"C": 3 * 3600,
		"D": 4 * 3600,
		"E": 5 * 3600,
		"F": 6 * 3600,
		"G": 7 * 3600,
		"H": 8 * 3600,
		"I": 9 * 3600,
		"K": 10 * 3600,
		"L": 11 * 3600,
		"M": 12 * 3600, // UTC+12
	}
)

// ParseTime returns a time in UTC+0 time zone.
// Note: currently, we do not use time.LoadLocation func, because it has two issues:
// 1. abusing opening files: https://github.com/golang/go/issues/24844
// 2. the version of zone info file in all machines may differ.
func ParseTime(timeStr string) (time.Time, error) {
	format := "%14s%s"
	var prefixTimeStr, zone string
	_, err := fmt.Sscanf(timeStr, format, &prefixTimeStr, &zone)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time string:%s, err:%s", timeStr, err.Error())
	}
	tm, err := time.Parse("20060102150405", prefixTimeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time string:%s, err:%s", timeStr, err.Error())
	}
	offset, ok := TimeZoneMap[strings.ToUpper(zone)]
	if !ok {
		return time.Time{}, fmt.Errorf("invalid zone:%s", zone)
	}
	return tm.Add(time.Duration(-offset) * time.Second), nil
}

// ParseTimeOfDay parser time string of a day
// timeStr: hhmmssZ, Z represents timezone
// return parsed time and offset of timezone
func ParseTimeOfDay(timeStr string) (time.Time, int, error) {
	format := "%6s%s"
	var prefixTimeStr, zone string
	_, err := fmt.Sscanf(timeStr, format, &prefixTimeStr, &zone)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid time string:%s, err:%s", timeStr, err.Error())
	}
	ts, err := time.Parse("15:04:05", fmt.Sprintf("%s:%s:%s", timeStr[0:2], timeStr[2:4], timeStr[4:6]))
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("time format invalid, err:%s", err.Error())
	}
	offset, ok := TimeZoneMap[strings.ToUpper(zone)]
	if !ok {
		return time.Time{}, 0, fmt.Errorf("invalid zone:%s", zone)
	}
	return ts, offset, nil
}

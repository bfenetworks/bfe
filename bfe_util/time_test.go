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
	"reflect"
	"testing"
	"time"
)

func TestPaseTime(t *testing.T) {
	tm, err := ParseTime("20190204200000H")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !reflect.DeepEqual(tm, time.Date(2019, 2, 4, 12, 0, 0, 0, time.UTC)) {
		t.Fatal("Expect equal")
	}
	tm, err = ParseTime("20190204200000h")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !reflect.DeepEqual(tm, time.Date(2019, 2, 4, 12, 0, 0, 0, time.UTC)) {
		t.Fatal("Expect equal")
	}
	_, err = ParseTime("20190204200000")
	if err == nil {
		t.Fatal("Expect an error")
	}
	expectError := "invalid time string:20190204200000, err:EOF"
	if err.Error() != expectError {
		t.Fatalf("Execpt error:%s, got:%s", expectError, err.Error())
	}
	_, err = ParseTime("2019020420000o")
	if err == nil {
		t.Fatal("Expect an error")
	}
	expectError = "invalid time string:2019020420000o, err:EOF"
	if err.Error() != expectError {
		t.Fatalf("Execpt error:%s, got:%s", expectError, err.Error())
	}
	_, err = ParseTime("201902042000000")
	if err == nil {
		t.Fatal("Expect an error")
	}
	expectError = "invalid zone:0"
	if err.Error() != expectError {
		t.Fatalf("Execpt error:%s, got:%s", expectError, err.Error())
	}
	_, err = ParseTime("20190204200000J")
	if err == nil {
		t.Fatal("Expect an error")
	}
	expectError = "invalid zone:J"
	if err.Error() != expectError {
		t.Fatalf("Execpt error:%s, got:%s", expectError, err.Error())
	}
}

func TestParseTime(t *testing.T) {
	tm, _, err := ParseTimeOfDay("200000H")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !reflect.DeepEqual(tm, time.Date(0000, 1, 1, 20, 0, 0, 0, time.UTC)) {
		t.Fatal("Expect equal")
	}
	tm, offset, err := ParseTimeOfDay("200000h")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !reflect.DeepEqual(tm, time.Date(0000, 1, 1, 20, 0, 0, 0, time.UTC)) {
		t.Fatal("Expect equal")
	}
	if offset != 8*3600 {
		t.Fatalf("offset error, %d expected", 8*3600)
	}
	_, _, err = ParseTimeOfDay("200000")
	if err == nil {
		t.Fatal("Expect an error")
	}
	expectError := "invalid time string:200000, err:EOF"
	if err.Error() != expectError {
		t.Fatalf("Execpt error:%s, got:%s", expectError, err.Error())
	}
	_, _, err = ParseTimeOfDay("20000o")
	if err == nil {
		t.Fatal("Expect an error")
	}
	expectError = "invalid time string:20000o, err:EOF"
	if err.Error() != expectError {
		t.Fatalf("Execpt error:%s, got:%s", expectError, err.Error())
	}
	_, _, err = ParseTimeOfDay("2000000")
	if err == nil {
		t.Fatal("Expect an error")
	}
	expectError = "invalid zone:0"
	if err.Error() != expectError {
		t.Fatalf("Execpt error:%s, got:%s", expectError, err.Error())
	}
	_, _, err = ParseTimeOfDay("200000J")
	if err == nil {
		t.Fatal("Expect an error")
	}
	expectError = "invalid zone:J"
	if err.Error() != expectError {
		t.Fatalf("Execpt error:%s, got:%s", expectError, err.Error())
	}
}

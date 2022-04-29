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

package json

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
)

// just copy struct from gslb
type HashConf struct {
	HashStrategy  *int
	HashHeader    *string
	SessionSticky *bool
}

// copy this struct from gslb
type GslbBasicTestConf struct {
	CrossRetry *int // retry cross sub clusters
	RetryMax   *int // inner cluster retry
	HashConf   *HashConf

	BalanceMode *string // balanceMode, default WRR
}

func TestNewDecoder(t *testing.T) {
	var gb1 GslbBasicTestConf
	var gb2 GslbBasicTestConf
	file1, err := os.Open("./testdata/gb.json")
	if err != nil {
		t.FailNow()
	}
	file2, err := os.Open("./testdata/gb.json")
	if err != nil {
		t.FailNow()
	}
	newDecoder := NewDecoder(file1)
	nativeDecoder := json.NewDecoder(file2)
	defer file1.Close()
	defer file2.Close()
	newDecoder.Decode(&gb1)
	nativeDecoder.Decode(&gb2)
	if !reflect.DeepEqual(gb1, gb2) {
		t.Errorf("NewEncoder() = %v, want %v", gb1, gb2)
	}
}

func TestMarshal(t *testing.T) {
	s := struct {
		Hello string
		World string
	}{
		Hello: "hello",
		World: "World",
	}
	ret1, err1 := Marshal(s)
	ret2, err2 := json.Marshal(s)
	if err1 != nil || err2 != nil {
		t.Errorf("Marshal Error1=%v, Error2=%v", err1, err2)
	}
	if !reflect.DeepEqual(ret1, ret2) {
		t.Errorf("Marshal() = %v, want %v", ret1, ret2)
	}
}

func TestMarshalIndent(t *testing.T) {
	s := struct {
		Hello string
		World string
	}{
		Hello: "hello",
		World: "World",
	}
	ret1, err1 := MarshalIndent(s, "", " ")
	ret2, err2 := json.MarshalIndent(s, "", " ")
	if err1 != nil || err2 != nil {
		t.Errorf("MarshalIndent Error1=%v, Error2=%v", err1, err2)
	}
	if !reflect.DeepEqual(ret1, ret2) {
		t.Errorf("MarshalIndent() = %v, want %v", string(ret1), string(ret2))
	}
}

func TestUnmarshal(t *testing.T) {
	var gb1 GslbBasicTestConf
	var gb2 GslbBasicTestConf
	file, _ := os.Open("./testdata/gb.json")
	bytes, _ := ioutil.ReadAll(file)
	Unmarshal(bytes, &gb1)
	json.Unmarshal(bytes, &gb2)
	if !reflect.DeepEqual(gb1, gb2) {
		t.Errorf("Unmarshal() = %v, want %v", gb1, gb2)
	}
}

func TestMarshalToString(t *testing.T) {
	s := struct {
		Hello string
		World string
	}{
		Hello: "Hello",
		World: "World",
	}
	var want = "{\"Hello\":\"Hello\",\"World\":\"World\"}"
	ret, _ := MarshalToString(s)
	if ret != want {
		t.Errorf("MarshalToString() = %v, want %v", ret, want)
	}
}

func TestUnmarshalFromString(t *testing.T) {
	type testCase struct {
		Hello string
		World string
	}
	var s1 testCase
	s2 := testCase{
		Hello: "Hello",
		World: "World",
	}
	var want = "{\"Hello\":\"Hello\",\"World\":\"World\"}"
	UnmarshalFromString(want, &s1)
	if !reflect.DeepEqual(s1, s2) {
		t.Errorf("UnmarshalFromString() = %v, want %v", s1, s2)
	}

}

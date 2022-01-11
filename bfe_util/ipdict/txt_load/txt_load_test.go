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

package txt_load

import (
	"net"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/ipdict"
)

// test for normal line
func TestCheckSplit_Case0(t *testing.T) {
	var startIP, endIP net.IP
	var line string
	var err error

	line = "1.1.1.1 2.2.2.2"
	startIP, endIP, err = checkSplit(line, " ")
	if !startIP.Equal(net.ParseIP("1.1.1.1")) ||
		!endIP.Equal(net.ParseIP("2.2.2.2")) ||
		err != nil {
		t.Error("TestCheckSplit():", err)
	}

	line = "1.1.1.1"
	startIP, endIP, err = checkSplit(line, " ")
	if !startIP.Equal(net.ParseIP("1.1.1.1")) ||
		!endIP.Equal(net.ParseIP("1.1.1.1")) ||
		err != nil {
		t.Error("TestCheckSplit():", err)
	}

	line = "1.1.1.1  2.2.2.2"
	startIP, endIP, err = checkSplit(line, " ")
	if !startIP.Equal(net.ParseIP("1.1.1.1")) ||
		!endIP.Equal(net.ParseIP("2.2.2.2")) ||
		err != nil {
		t.Error("TestCheckSplit():", err)
	}

	line = "1.1.1.1  \t\t  2.2.2.2"
	startIP, endIP, err = checkSplit(line, " ")
	if !startIP.Equal(net.ParseIP("1.1.1.1")) ||
		!endIP.Equal(net.ParseIP("2.2.2.2")) ||
		err != nil {
		t.Error("TestCheckSplit():", err)
	}
	line = "1::1  \t\t  1::FFFF"
	startIP, endIP, err = checkSplit(line, " ")
	if !startIP.Equal(net.ParseIP("1::1")) ||
		!endIP.Equal(net.ParseIP("1::FFFF")) ||
		err != nil {
		t.Error("TestCheckSplit():", err)
	}
}

// test for abnormal line
func TestCheckSplit_Case1(t *testing.T) {
	var line string
	var err error

	line = "1.1.1.1 a"
	_, _, err = checkSplit(line, " ")
	if err == nil {
		t.Errorf("TestCheckSplit(): line %s err", line)
	}

	line = "a 1.1.1.1"
	_, _, err = checkSplit(line, " ")
	if err == nil {
		t.Errorf("TestCheckSplit(): line %s err", line)
	}

	line = "a b"
	_, _, err = checkSplit(line, " ")
	if err == nil {
		t.Errorf("TestCheckSplit(): line %s err", line)
	}

	line = "1.1.1.1 2.2.2.2 a"
	_, _, err = checkSplit(line, " ")
	if err == nil {
		t.Errorf("TestCheckSplit(): line %s err", line)
	}
	line = "1::1 1::FFFF a"
	_, _, err = checkSplit(line, " ")
	if err == nil {
		t.Errorf("TestCheckSplit(): line %s err", line)
	}
}

// test for normal line
func TestCheckLine_Case0(t *testing.T) {
	var startIP, endIP net.IP
	var line string
	var err error

	line = "1.1.1.1 2.2.2.2"
	startIP, endIP, err = checkLine(line)
	if !startIP.Equal(net.ParseIP("1.1.1.1")) ||
		!endIP.Equal(net.ParseIP("2.2.2.2")) ||
		err != nil {
		t.Error("TestCheckLine():", err)
	}

	line = "1.1.1.1"
	startIP, endIP, err = checkLine(line)
	if !startIP.Equal(net.ParseIP("1.1.1.1")) ||
		!endIP.Equal(net.ParseIP("1.1.1.1")) ||
		err != nil {
		t.Error("TestCheckLine():", err)
	}

	line = "1.1.1.1  2.2.2.2"
	startIP, endIP, err = checkLine(line)
	if !startIP.Equal(net.ParseIP("1.1.1.1")) ||
		!endIP.Equal(net.ParseIP("2.2.2.2")) ||
		err != nil {
		t.Error("TestCheckLine():", err)
	}

	line = "1.1.1.1  \t\t  2.2.2.2"
	startIP, endIP, err = checkLine(line)
	if !startIP.Equal(net.ParseIP("1.1.1.1")) ||
		!endIP.Equal(net.ParseIP("2.2.2.2")) ||
		err != nil {
		t.Error("TestCheckLine():", err)
	}

	line = "1.1.1.1\t\t   \t\t2.2.2.2"
	startIP, endIP, err = checkLine(line)
	if !startIP.Equal(net.ParseIP("1.1.1.1")) ||
		!endIP.Equal(net.ParseIP("2.2.2.2")) ||
		err != nil {
		t.Error("TestCheckLine():", err)
	}
	line = "1::1\t\t   \t\t1::FFFF"
	startIP, endIP, err = checkLine(line)
	if !startIP.Equal(net.ParseIP("1::1")) ||
		!endIP.Equal(net.ParseIP("1::FFFF")) ||
		err != nil {
		t.Error("TestCheckLine():", err)
	}
	line = "192.168.1.1/20"
	startIP, endIP, err = checkLine(line)
	if !startIP.Equal(net.ParseIP("192.168.0.0"))||
		!endIP.Equal(net.ParseIP("192.168.15.255"))||
		err != nil {
		t.Error("TestCheckLine():", err)
	}
	line = "fdbd:ff1:ce00:443:8f5:1f05:2f9d:b6d0/20"
	startIP, endIP, err = checkLine(line)
	if !startIP.Equal(net.ParseIP("fdbd:0000:0000:0000:0000:0000:0000:0000"))||
		!endIP.Equal(net.ParseIP("fdbd:0fff:ffff:ffff:ffff:ffff:ffff:ffff"))||
		err != nil {
		t.Error("TestCheckLine():", err)
	}
}

func TestNewTxtFileLoader(t *testing.T) {
	fileName := "./testdata/ipdict.conf"

	f := NewTxtFileLoader(fileName)

	if f.fileName != fileName {
		t.Errorf("TestNewTxtFileLoader(): fileName %s != %s", f.fileName, fileName)
	}
}

// file not exist case
func TestLoad_Case0(t *testing.T) {
	fileName := "./testdata/no_exist.conf"
	fileLoader := NewTxtFileLoader(fileName)
	_, err := fileLoader.CheckAndLoad("")

	if err == nil {
		t.Errorf("TestCheckAndLoad(): err is not nill")
	}
}

// ip format error case
func TestLoad_Case1(t *testing.T) {
	fileName := "./testdata/ipdict.conf1"
	fileLoader := NewTxtFileLoader(fileName)
	_, err := fileLoader.CheckAndLoad("")

	if err == nil {
		t.Errorf("TestCheckAndLoad(): err is not nill")
	}
}

// startIP > endIP error case
func TestLoad_Case2(t *testing.T) {
	fileName := "./testdata/ipdict.conf2"
	fileLoader := NewTxtFileLoader(fileName)
	_, err := fileLoader.CheckAndLoad("")

	if err == nil {
		t.Errorf("TestCheckAndLoad(): err is not nill")
	}
}

// line format error case
func TestLoad_Case3(t *testing.T) {
	fileName := "./testdata/ipdict.conf3"
	fileLoader := NewTxtFileLoader(fileName)
	_, err := fileLoader.CheckAndLoad("")

	if err == nil {
		t.Errorf("TestCheckAndLoad(): err is not nill")
	}
}

// normal case
func TestLoad_Case4(t *testing.T) {
	fileName := "./testdata/ipdict.conf4"
	fileLoader := NewTxtFileLoader(fileName)
	_, err := fileLoader.CheckAndLoad("")

	if err != nil {
		t.Errorf("TestCheckAndLoad(): err is %s", err.Error())
	}
}

func TestLoad_Case5(t *testing.T) {
	table := ipdict.NewIPTable()

	fileName := "./testdata/ipdict.conf5"
	fileLoader := NewTxtFileLoader(fileName)
	ipItems, err := fileLoader.CheckAndLoad("")
	if err != nil {
		t.Errorf("TestCheckAndLoad(): err is not nill %s", err.Error())
	}

	if ipItems.Length() != 6 || ipItems.Version != "" {
		t.Errorf("ipItems.Length should be 6 version should be nil")
	}

	table.Update(ipItems)
	if table.Search(net.ParseIP("1.1.1.1")) {
		t.Error("TestCheckAndLoad(); 1.1.1.1 should not in table")
	}

	if !table.Search(net.ParseIP("220.1.2.194")) {
		t.Error("TestCheckAndLoad(); 220.1.2.194 not in table")
	}
	if !table.Search(net.ParseIP("1::2")) {
		t.Error("TestCheckAndLoad(); 1::2 not in table")
	}
}

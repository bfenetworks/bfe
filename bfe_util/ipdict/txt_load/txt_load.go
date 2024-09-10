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
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/ipdict"
)

var (
	// file version not change, needn't load the file
	ErrNoNeedUpdate = errors.New("Version no change no need update")
	// line num of file larger than maxline configured
	ErrMaxLineExceed = errors.New("Max line exceed")
	// wrong meta info
	ErrWrongMetaInfo = errors.New("Wrong meta info")
)

type TxtFileLoader struct {
	fileName string
	maxLine  int
}

func NewTxtFileLoader(fileName string) *TxtFileLoader {
	f := new(TxtFileLoader)
	f.fileName = fileName
	f.maxLine = -1
	return f
}

// set max line num
func (f *TxtFileLoader) SetMaxLine(maxLine int) {
	f.maxLine = maxLine
}

/*
   checkSplit checks line split format
   legal start ip and end ip is separated by space[s]/tab[s]
*/
func checkSplit(line string, sep string) (net.IP, net.IP, error) {
	var startIPStr, endIPStr string
	var startIP, endIP net.IP

	segments := strings.SplitN(line, sep, 2)
	segLen := len(segments)

	// Segments[0] : start ip string
	// Segments[1] : end ip string(start ip string instead when no end ip string found)
	if segLen == 1 {
		startIPStr, endIPStr = segments[0], segments[0]
	} else if len(segments) == 2 {
		startIPStr = strings.Trim(segments[0], " \t")
		endIPStr = strings.Trim(segments[1], " \t")
	} else {
		return nil, nil, fmt.Errorf("checkSplit(): err, line is: %s", line)
	}

	// startIPStr format err
	if startIP = net.ParseIP(startIPStr); startIP == nil {
		return nil, nil, fmt.Errorf("checkSplit(): line %s format err", line)
	}

	// endIPStr format err
	if endIP = net.ParseIP(endIPStr); endIP == nil {
		return nil, nil, fmt.Errorf("checkSplit(): line %s format err", line)
	}

	return startIP, endIP, nil
}

// checkLine checks line format
func checkLine(line string) (net.IP, net.IP, error) {
	var startIP, endIP net.IP
	var err error

	// check cidr format first
	startIP, endIP, err = checkCIDR(line)
	if startIP != nil && endIP != nil && err == nil {
		return startIP, endIP, nil
	}

	// check space split segment
	startIP, endIP, err = checkSplit(line, " ")
	if err != nil {
		// check tab split segment
		startIP, endIP, err = checkSplit(line, "\t")
		if err != nil {
			return nil, nil, fmt.Errorf("checkLine(): err, %s", err.Error())
		}
	}

	return startIP, endIP, err
}

// checkCIDR check cidr format
// legal format is [ipv4|ipv6]/xxx
func checkCIDR(line string) (net.IP, net.IP, error) {
	_, netIP, err := net.ParseCIDR(line)
	if err != nil {
		return nil, nil, err
	}
	return netIP.IP, getLastIPAddress(netIP), nil
}

/* check Version num and load IP txt file to IP items in memory */
func (f TxtFileLoader) CheckAndLoad(curVersion string) (*ipdict.IPItems, error) {
	var startIP, endIP net.IP

	fileName := f.fileName
	// get file Version and lineNum
	metaInfo, err := getFileInfo(fileName)
	if err != nil {
		return nil, fmt.Errorf("loadFile(): %s %s", fileName, err.Error())
	}
	newVersion := metaInfo.Version
	singleIPNum := metaInfo.SingleIPNum
	pairIPNum := metaInfo.PairIPNum

	// if singleIPNum + pairIPNum > maxLine
	// use maxline for singleIPNum and pairIPNum(protect malloc failed)
	// but the dict will still cut off by maxLine
	if f.maxLine != -1 && singleIPNum+pairIPNum > f.maxLine {
		singleIPNum = f.maxLine
		pairIPNum = f.maxLine
	}

	// check version
	if newVersion == curVersion && newVersion != "" {
		return nil, ErrNoNeedUpdate
	}

	// init counter for singleIP & pairIP
	singleIPCounter := 0
	pairIPCounter := 0
	lineCounter := 0
	// open file
	file, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("loadFile(): %s, %s", fileName, err.Error())
	}
	defer file.Close()
	// create ipItems
	ipItems, err := ipdict.NewIPItems(singleIPNum, pairIPNum)
	if err != nil {
		return nil, err
	}
	// scan the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {

		// Remove all leading and trailing spaces and tabs
		line := strings.Trim(scanner.Text(), " \t")
		//Line begins with "#" is considered as a comment
		if strings.HasPrefix(line, "#") || len(line) == 0 {
			continue
		}

		// Check line format
		startIP, endIP, err = checkLine(line)
		if err != nil {
			return nil, fmt.Errorf("loadFile(): err, %s", err.Error())
		}

		// insert start ip and end ip into dict
		if bytes.Equal(startIP, endIP) {
			// startIp == endIP insert single
			err = ipItems.InsertSingle(startIP)
			singleIPCounter += 1
		} else {
			err = ipItems.InsertPair(startIP, endIP)
			pairIPCounter += 1
		}
		if err != nil {
			return nil, fmt.Errorf("loadFile(): err, %s", err.Error())
		}

		// check if lineCounter > maxLine or not
		lineCounter += 1
		if f.maxLine != -1 && lineCounter > f.maxLine {
			//sort dict
			ipItems.Sort()
			ipItems.Version = newVersion
			return ipItems, ErrMaxLineExceed
		}

		// if ipcounter > max ipnum
		if singleIPCounter > singleIPNum || pairIPCounter > pairIPNum {
			//sort dict
			ipItems.Sort()
			ipItems.Version = newVersion
			return ipItems, ErrMaxLineExceed
		}
	}

	err = scanner.Err()
	// Scan meets error
	if err != nil {
		return nil, fmt.Errorf("loadFile(): err, %s", err.Error())
	}

	// Load succ, sort dict
	ipItems.Sort()
	ipItems.Version = newVersion
	return ipItems, nil
}

// getLastIPAddress
// get last ip address for cidr
// 192.168.1.1/20 last address is 192.168.15.255
const (
	ipv4BitsLen = 32
	ipv6BitsLen = 128
)

func getLastIPAddress(ipNet *net.IPNet) net.IP {
	if ipNet == nil {
		return nil
	}
	ones, bits := ipNet.Mask.Size()
	lastIP := make(net.IP, 4) // default set ipv4

	switch bits {
	case ipv6BitsLen:
		ipv6Count := (&big.Int{}).Lsh(big.NewInt(1), uint(bits-ones))
		startIPBytes := (&big.Int{}).SetBytes(ipNet.IP)

		lastIP = (&big.Int{}).Add(startIPBytes, big.NewInt(0).Sub(ipv6Count, big.NewInt(1))).Bytes()
	case ipv4BitsLen:
		ipv4Count := uint32(1 << (bits - ones))
		startIPBytes := binary.BigEndian.Uint32(ipNet.IP)

		binary.BigEndian.PutUint32(lastIP, startIPBytes+ipv4Count-1)
	default:
		return nil
	}
	return lastIP
}

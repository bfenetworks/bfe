// Copyright (c) 2019 Baidu, Inc.
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

package access_log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

import (
	"github.com/baidu/go-lib/log/log4go"
)

// logDirCreate check and create dir if nonexist
func logDirCreate(logDir string) error {
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		err = os.MkdirAll(logDir, 0777)
		if err != nil {
			return err
		}
	}
	return nil
}

// prefix2Name generate fileName from prefix
func prefix2Name(prefix string) string {
	return prefix + ".log"
}

// LoggerInit initialize logger. Log file name is prefix.log
func LoggerInit(prefix string, logDir string, when string, backupCount int) (log4go.Logger, error) {
	fileName := prefix2Name(prefix)
	return LoggerInit2(fileName, logDir, when, backupCount)
}

// LoggerInit2 initialize logger. Log file name is fileName
func LoggerInit2(fileName, logDir, when string, backupCount int) (log4go.Logger, error) {
	accessDefaultFormat := "%M"
	return LoggerInitWithFormat2(fileName, logDir, when, backupCount, accessDefaultFormat)
}

// LoggerInit3 initialize logger. FilePath should be provided
func LoggerInit3(filePath, when string, backupCount int) (log4go.Logger, error) {
	logDir, fileName := filepath.Split(filePath)
	return LoggerInit2(fileName, logDir, when, backupCount)
}

// LoggerInitWithFormat initialize logger. Format should be provided
func LoggerInitWithFormat(prefix, logDir, when string, backupCount int,
	format string) (log4go.Logger, error) {
	fileName := prefix2Name(prefix)
	return LoggerInitWithFormat2(fileName, logDir, when, backupCount, format)
}

// LoggerInitWithFormat2 is similar to LoggerInit, instead of prefix, fileName should be provided.
func LoggerInitWithFormat2(fileName, logDir, when string, backupCount int,
	format string) (log4go.Logger, error) {
	var logger log4go.Logger
	// check value of when is valid
	if !log4go.WhenIsValid(when) {
		log4go.Error("LoggerInitWithFormat(): invalid value of when(%s)", when)
		return logger, fmt.Errorf("invalid value of when: %s", when)
	}
	// change when to upper
	when = strings.ToUpper(when)
	// check, and create dir if nonexist
	if err := logDirCreate(logDir); err != nil {
		log4go.Error("Init(), in logDirCreate(%s)", logDir)
		return logger, err
	}
	// create logger
	logger = make(log4go.Logger)
	// create file writer for all log
	fullPath := filepath.Join(logDir, fileName)
	logWriter := log4go.NewTimeFileLogWriter(fullPath, when, backupCount)
	if logWriter == nil {
		return logger, fmt.Errorf("error in log4go.NewTimeFileLogWriter(%s)", fullPath)
	}
	logWriter.SetFormat(format)
	logger.AddFilter("log", log4go.INFO, logWriter)
	return logger, nil
}

// LoggerInitWithSvr initialize logger with remote log server.
func LoggerInitWithSvr(progName string, loggerName string,
	network string, svrAddr string) (log4go.Logger, error) {
	var logger log4go.Logger
	// create file writer for all log
	name := fmt.Sprintf("%s_%s", progName, loggerName)
	// create logger
	logger = make(log4go.Logger)
	logWriter := log4go.NewPacketWriter(name, network, svrAddr, log4go.LogFormat)
	if logWriter == nil {
		return nil, fmt.Errorf("error in log4go.NewPacketWriter(%s)", name)
	}
	logger.AddFilter(name, log4go.INFO, logWriter)
	return logger, nil
}

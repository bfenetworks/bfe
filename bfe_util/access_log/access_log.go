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

package access_log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bfenetworks/bfe/bfe_util"
)

import (
	"github.com/baidu/go-lib/log/log4go"
)

type LogConfig struct {
	// Log directly to a single file (eg. /dev/stdout)
	LogFile string // log file path

	// Log with rotation under specified directory
	LogPrefix   string // log file prefix
	LogDir      string // log file dir
	RotateWhen  string // rotate time
	BackupCount int    // log file backup number
}

func (cfg *LogConfig) Check(confRoot string) error {
	if cfg.LogFile != "" {
		if cfg.LogPrefix != "" || cfg.LogDir != "" || cfg.RotateWhen != "" || cfg.BackupCount > 0 {
			return fmt.Errorf(`ModAccess.LogPrefix, ModAccess.LogDir, ModAccess.RotateWhen and ModAccess.BackupCount cannot be set when ModAccess.LogFile is set`)
		}
		cfg.LogFile = bfe_util.ConfPathProc(cfg.LogFile, confRoot)
	} else {
		if cfg.LogPrefix == "" {
			return fmt.Errorf("ModAccess.LogPrefix is empty")
		}

		if cfg.LogDir == "" {
			return fmt.Errorf("ModAccess.LogDir is empty")
		}
		cfg.LogDir = bfe_util.ConfPathProc(cfg.LogDir, confRoot)

		if !log4go.WhenIsValid(cfg.RotateWhen) {
			return fmt.Errorf("ModAccess.RotateWhen invalid: %s", cfg.RotateWhen)
		}

		if cfg.BackupCount <= 0 {
			return fmt.Errorf("ModAccess.BackupCount should > 0: %d", cfg.BackupCount)
		}

	}
	return nil
}

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
func LoggerInit(c LogConfig) (log4go.Logger, error) {
	if c.LogFile != "" {
		accessDefaultFormat := "%M"
		return loggerInitWithFilePath(c.LogFile, accessDefaultFormat)
	} else {
		fileName := prefix2Name(c.LogPrefix)
		return LoggerInit2(fileName, c.LogDir, c.RotateWhen, c.BackupCount)
	}
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
func LoggerInitWithFormat(c LogConfig, format string) (log4go.Logger, error) {
	fileName := prefix2Name(c.LogPrefix)
	return LoggerInitWithFormat2(fileName, c.LogDir, c.RotateWhen, c.BackupCount, format)
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

// loggerInitWithFilePath initialize logger with a single file name and output logs to file simply.
func loggerInitWithFilePath(filePath, format string) (log4go.Logger, error) {
	// create logger
	var logger = make(log4go.Logger)
	logWriter := log4go.NewFileLogWriter(filePath, false)
	if logWriter == nil {
		return nil, fmt.Errorf("error in log4go.NewFileLogWriter(%s)", filePath)
	}
	logWriter.SetFormat(format)
	logger.AddFilter("log", log4go.INFO, logWriter)
	return logger, nil
}

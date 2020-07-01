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

package mod_access

import (
	"fmt"
	"strings"
)

import (
	"github.com/RackSec/srslog"
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/log/log4go"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/access_log"
)

type LoggerWrapper struct {
	log4go.Logger
	syslogger *srslog.Writer
}

func LoggerInit(conf *ConfModAccess) (*LoggerWrapper, error) {
	var logger LoggerWrapper
	l, err := access_log.LoggerInit(conf.Log.LogPrefix, conf.Log.LogDir,
		conf.Log.RotateWhen, conf.Log.BackupCount)
	if err != nil {
		return nil, err
	}

	logger.Logger = l
	w, err := srslog.Dial(conf.SysLog.Network, conf.SysLog.Addr, srslog.LOG_INFO, conf.SysLog.Tag)
	if err != nil {
		log.Logger.Warn("dial syslog failed:%v, ignore syslog", err)
	} else {
		w.SetFormatter(srslog.RFC5424Formatter)
		logger.syslogger = w
	}
	return &logger, nil
}

func (l *LoggerWrapper) Info(arg0 interface{}, args ...interface{}) {
	l.Logger.Info(arg0, args)
	if l.syslogger != nil {
		var msg string
		switch first := arg0.(type) {
		case string:
			// Use the string as a format string
			msg = fmt.Sprintf(first, args...)
		case []byte:
			msg = string(first)
		case func() string:
			// Log the closure (no other arguments used)
			msg = first()
		default:
			// Build a format string so that it will be similar to Sprint
			msg = fmt.Sprintf(fmt.Sprint(first)+strings.Repeat(" %v", len(args)), args...)
		}
		l.syslogger.Info(msg)
	}
}

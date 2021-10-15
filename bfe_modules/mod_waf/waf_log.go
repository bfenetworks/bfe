// Copyright (c) 2020 The BFE Authors.
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
package mod_waf

import (
	"fmt"
)

import (
	"github.com/baidu/go-lib/log/log4go"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/access_log"
)

type wafLogger struct {
	log log4go.Logger // wrapper log info
}

func NewWafLogger() *wafLogger {
	return new(wafLogger)
}

func (wf *wafLogger) Init(conf *ConfModWaf) error {
	var err error
	// WAF LOG Demo:[2020/08/25 13:40:58 CST] [INFO] [69613] {"Rule":"RuleBashCmd","Type":"Block","Hit":true ...
	logFormatter := "[%D %T] [%L] [%P] %M"
	wf.log, err = access_log.LoggerInitWithFormat(conf.Log, logFormatter)
	if err != nil {
		return fmt.Errorf("WafLogger.Init(): create logger error:%v", err)
	}
	return nil
}

func (wl *wafLogger) DumpLog(v interface{}) {
	wl.log.Info("%s", v)
}

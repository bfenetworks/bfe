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
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/baidu/go-lib/queue"
	"github.com/bfenetworks/bfe/bfe_modules/mod_waf/waf_rule"
)

func TestNewWafWorker(t *testing.T) {
	tests := []struct {
		name string
		want *wafWorker
	}{
		{
			name: "normal",
			want: new(wafWorker),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewWafWorker(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewWafWorker() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_wafWorker_Init(t *testing.T) {
	type fields struct {
		concurrency  int
		checkJobList *queue.Queue
		jobCallback  func(interface{})
		wafTable     *waf_rule.WafRuleTable
	}
	type args struct {
		config   *ConfModWaf
		callback func(interface{})
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "",
			fields: fields{
				concurrency:  50,
				checkJobList: &queue.Queue{},
				jobCallback:  func(interface{}) { fmt.Print("not implemented") },
				wafTable:     &waf_rule.WafRuleTable{},
			},
			args: args{
				config: &ConfModWaf{
					Basic: struct {
						ProductRulePath string
						Concurrency     int
					}{
						ProductRulePath: "",
						Concurrency:     50,
					},
					Log: struct {
						LogPrefix   string
						LogDir      string
						RotateWhen  string
						BackupCount int
					}{
						LogPrefix:   "",
						LogDir:      "",
						RotateWhen:  "",
						BackupCount: 0,
					},
				},
				callback: func(interface{}) { fmt.Print("not implemented") },
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ww := &wafWorker{
				concurrency:  tt.fields.concurrency,
				checkJobList: tt.fields.checkJobList,
				jobCallback:  tt.fields.jobCallback,
				wafTable:     tt.fields.wafTable,
			}
			if err := ww.Init(tt.args.config, tt.args.callback); (err != nil) != tt.wantErr {
				t.Errorf("wafWorker.Init() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_wafWorker_pushAsyncJob(t *testing.T) {
	ww := NewWafWorker()
	callback := func(a interface{}) {
		if v, ok := a.(*wafJob); ok {
			v.Rule = "finished"
		}
	}
	conf := &ConfModWaf{
		Basic: struct {
			ProductRulePath string
			Concurrency     int
		}{
			ProductRulePath: "",
			Concurrency:     50,
		},
		Log: struct {
			LogPrefix   string
			LogDir      string
			RotateWhen  string
			BackupCount int
		}{
			LogPrefix:   "",
			LogDir:      "",
			RotateWhen:  "",
			BackupCount: 0,
		},
	}
	ww.Init(conf, callback)
	wj := &wafJob{
		Rule: "RuleSQLInjection",
		Type: "Block",
		Hit:  false,
		RuleRequest: &waf_rule.RuleRequestInfo{
			Method:     "GET",
			Version:    "",
			Headers:    map[string][]string{},
			Uri:        "/img",
			UriUnquote: "/img",
			UriParsed: &url.URL{
				Scheme:     "http",
				Opaque:     "",
				User:       nil,
				Host:       "",
				Path:       "",
				RawPath:    "",
				ForceQuery: false,
				RawQuery:   "",
				Fragment:   "",
			},
			QueryValues: map[string][]string{},
		},
	}
	ww.pushAsyncJob(wj)
	time.Sleep(time.Duration(2) * time.Second)
	if wj.Rule != "finished" {
		t.Errorf("pushAsyncJob() err=rule: %s", wj.Rule)
	}

}

func Test_wafWorker_doSyncJob(t *testing.T) {
	ww := NewWafWorker()
	callback := func(a interface{}) {
		if v, ok := a.(*wafJob); ok {
			v.Rule = "finished"
		}
	}
	conf := &ConfModWaf{
		Basic: struct {
			ProductRulePath string
			Concurrency     int
		}{
			ProductRulePath: "",
			Concurrency:     50,
		},
		Log: struct {
			LogPrefix   string
			LogDir      string
			RotateWhen  string
			BackupCount int
		}{
			LogPrefix:   "",
			LogDir:      "",
			RotateWhen:  "",
			BackupCount: 0,
		},
	}
	ww.Init(conf, callback)
	wj := &wafJob{
		Rule: "RuleSQLInjection",
		Type: "Block",
		Hit:  false,
		RuleRequest: &waf_rule.RuleRequestInfo{
			Method:     "GET",
			Version:    "",
			Headers:    map[string][]string{},
			Uri:        "/img",
			UriUnquote: "/img",
			UriParsed: &url.URL{
				Scheme:     "http",
				Opaque:     "",
				User:       nil,
				Host:       "",
				Path:       "",
				RawPath:    "",
				ForceQuery: false,
				RawQuery:   "",
				Fragment:   "",
			},
			QueryValues: map[string][]string{},
		},
	}
	_, err := ww.doSyncJob(wj)
	if err != nil {
		t.Errorf("doSyncJob err[%v]", err)
	}
	if wj.Rule != "finished" {
		t.Errorf("doSyncJob() err=rule: %s", wj.Rule)
	}
}

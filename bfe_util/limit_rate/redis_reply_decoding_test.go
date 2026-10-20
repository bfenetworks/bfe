// Copyright (c) 2026 The BFE Authors.
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

package limit_rate

import (
	"errors"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

// 本文件针对 agent 对 Redis（Lua）返回值的解码分支做表驱动测试。
//
// 真实 Redis 经 redigo 返回的 Lua 数字一律是 int64，因此 string / []byte /
// 非法类型这些分支在 miniredis 或真 Redis 上都走不到。但这些分支正是"上游库
// 换了编码方式就静默算错"的风险点，用 fake 客户端把每种编码都跑一遍。

// fakeScript 回放预置的脚本返回值。
type fakeScript struct {
	reply interface{}
	err   error
}

func (s *fakeScript) Run(key string, args ...interface{}) (interface{}, error) {
	return s.reply, s.err
}

// fakeClient 只实现 NewScript；其余方法通过内嵌 nil 接口满足签名，
// 一旦被调用会立即 panic，可及时发现用例走错了路径。
type fakeClient struct {
	redis_client.Client
	script *fakeScript
}

func (c *fakeClient) NewScript(string) redis_client.RedisScript { return c.script }

func newFakeAgent(reply interface{}, err error) *RedisLRAgent {
	return NewRedisLRAgent(&fakeClient{script: &fakeScript{reply: reply, err: err}})
}

func TestQpmReplyDecoding(t *testing.T) {
	tests := []struct {
		name        string
		reply       interface{}
		wantTime    int64
		wantAllowed bool
		wantTat     float64
		wantErr     string
	}{
		{
			name:        "Int64",
			reply:       []interface{}{int64(1700000000), int64(1), int64(3), int64(0)},
			wantTime:    1700000000,
			wantAllowed: true,
			wantTat:     3,
		},
		{
			name:        "Float64",
			reply:       []interface{}{float64(1700000000), float64(0), float64(7), float64(0)},
			wantTime:    1700000000,
			wantAllowed: false,
			wantTat:     7,
		},
		{
			name: "String",
			// currentTime 支持 string，但 isAllowed 只有 int64/float64/[]byte 分支。
			// 该不对称是既有实现现状（见方案文档 §4.4 待确认项），此处固化。
			reply:   []interface{}{"1700000000", "1", "3", "0"},
			wantErr: "invalid isAllowed type",
		},
		{
			name:        "Bytes",
			reply:       []interface{}{[]byte("1700000000"), []byte("1"), []byte("3"), []byte("0")},
			wantTime:    1700000000,
			wantAllowed: true,
			wantTat:     3,
		},
		{
			name:    "WrongLength",
			reply:   []interface{}{int64(1)},
			wantErr: "unexpected lua return type",
		},
		{
			name:    "InvalidTimeType",
			reply:   []interface{}{true, int64(1), int64(3), int64(0)},
			wantErr: "invalid currentTime type",
		},
		{
			name:    "BadTimeString",
			reply:   []interface{}{"abc", int64(1), int64(3), int64(0)},
			wantErr: "parse currentTime string error",
		},
		{
			name:    "BadTimeBytes",
			reply:   []interface{}{[]byte("abc"), int64(1), int64(3), int64(0)},
			wantErr: "parse currentTime bytes error",
		},
		{
			name:    "InvalidAllowedType",
			reply:   []interface{}{int64(1), true, int64(3), int64(0)},
			wantErr: "invalid isAllowed type",
		},
		{
			name:    "BadAllowedBytes",
			reply:   []interface{}{int64(1), []byte("x"), int64(3), int64(0)},
			wantErr: "parse isAllowed bytes error",
		},
		{
			name:    "InvalidTatType",
			reply:   []interface{}{int64(1), int64(1), true, int64(0)},
			wantErr: "invalid tat type",
		},
		{
			name:    "BadTatBytes",
			reply:   []interface{}{int64(1), int64(1), []byte("x"), int64(0)},
			wantErr: "parse tat bytes error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTime, gotAllowed, gotTat, err := newFakeAgent(tt.reply, nil).QpmCheck("k", 60, 10, 1)
			assertDecoded(t, err, tt.wantErr)
			if err != nil {
				return
			}
			if gotTime != tt.wantTime || gotAllowed != tt.wantAllowed || gotTat != tt.wantTat {
				t.Errorf("got (time=%d allowed=%v tat=%v), want (time=%d allowed=%v tat=%v)",
					gotTime, gotAllowed, gotTat, tt.wantTime, tt.wantAllowed, tt.wantTat)
			}
		})
	}
}

func TestConAcquireReplyDecoding(t *testing.T) {
	tests := []struct {
		name        string
		reply       interface{}
		wantTime    int64
		wantAllowed bool
		wantCount   int64
		wantErr     string
	}{
		{
			name:        "Int64",
			reply:       []interface{}{int64(1700000000), int64(1), int64(2)},
			wantTime:    1700000000,
			wantAllowed: true,
			wantCount:   2,
		},
		{
			name: "String",
			// 同 QpmCheck：currentTime 支持 string，isAllowed 不支持。
			reply:   []interface{}{"1700000000", "0", "5"},
			wantErr: "invalid isAllowed type",
		},
		{
			name:        "Bytes",
			reply:       []interface{}{[]byte("1700000000"), []byte("1"), []byte("1")},
			wantTime:    1700000000,
			wantCount:   1,
			wantAllowed: true,
		},
		{
			name:    "WrongLength",
			reply:   []interface{}{int64(1), int64(1)},
			wantErr: "unexpected lua return type",
		},
		{
			name:    "InvalidTimeType",
			reply:   []interface{}{true, int64(1), int64(1)},
			wantErr: "invalid currentTime type",
		},
		{
			name:    "InvalidAllowedType",
			reply:   []interface{}{int64(1), true, int64(1)},
			wantErr: "invalid isAllowed type",
		},
		{
			name:    "InvalidCountType",
			reply:   []interface{}{int64(1), int64(1), true},
			wantErr: "invalid curCount type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTime, gotAllowed, gotCount, err := newFakeAgent(tt.reply, nil).ConAcquire("k", 2, 60)
			assertDecoded(t, err, tt.wantErr)
			if err != nil {
				return
			}
			if gotTime != tt.wantTime || gotAllowed != tt.wantAllowed || gotCount != tt.wantCount {
				t.Errorf("got (time=%d allowed=%v count=%d), want (time=%d allowed=%v count=%d)",
					gotTime, gotAllowed, gotCount, tt.wantTime, tt.wantAllowed, tt.wantCount)
			}
		})
	}
}

func TestConReleaseReplyDecoding(t *testing.T) {
	tests := []struct {
		name      string
		reply     interface{}
		wantCount int64
		wantErr   string
	}{
		{name: "Int64", reply: []interface{}{int64(3)}, wantCount: 3},
		{
			name:    "String",
			reply:   []interface{}{"3"},
			wantErr: "invalid curCount type",
		},
		{name: "Bytes", reply: []interface{}{[]byte("3")}, wantCount: 3},
		{name: "WrongLength", reply: []interface{}{int64(1), int64(2)}, wantErr: "unexpected lua return type"},
		{name: "InvalidType", reply: []interface{}{true}, wantErr: "invalid curCount type"},
		{name: "BadBytes", reply: []interface{}{[]byte("x")}, wantErr: "parse curCount bytes error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCount, err := newFakeAgent(tt.reply, nil).ConRelease("k", 60)
			assertDecoded(t, err, tt.wantErr)
			if err == nil && gotCount != tt.wantCount {
				t.Errorf("curCount = %d, want %d", gotCount, tt.wantCount)
			}
		})
	}
}

func TestCheckAndConsumeTokenReplyDecoding(t *testing.T) {
	ok := []interface{}{int64(1700000000), int64(1), int64(1), int64(40), int64(90)}

	tests := []struct {
		name       string
		reply      interface{}
		wantBucket int64
		wantTotal  int64
		wantFinal  bool
		wantErr    string
	}{
		{name: "Int64", reply: ok, wantBucket: 40, wantTotal: 90, wantFinal: true},
		{
			name:    "String",
			reply:   []interface{}{"1700000000", "1", "1", "40", "90"},
			wantErr: "invalid isBucketAllowed type",
		},
		{
			name:       "Bytes",
			reply:      []interface{}{[]byte("1700000000"), []byte("1"), []byte("1"), []byte("40"), []byte("90")},
			wantBucket: 40,
			wantTotal:  90,
			wantFinal:  true,
		},
		{
			name:      "BucketRejected",
			reply:     []interface{}{int64(1), int64(0), int64(1), int64(0), int64(90)},
			wantTotal: 90,
		},
		{name: "WrongLength", reply: []interface{}{int64(1)}, wantErr: "unexpected lua return type"},
		{name: "InvalidTimeType", reply: []interface{}{true, int64(1), int64(1), int64(1), int64(1)}, wantErr: "invalid currentTime type"},
		{name: "InvalidBucketAllowedType", reply: []interface{}{int64(1), true, int64(1), int64(1), int64(1)}, wantErr: "invalid isBucketAllowed type"},
		{name: "InvalidWholeAllowedType", reply: []interface{}{int64(1), int64(1), true, int64(1), int64(1)}, wantErr: "invalid isWholeAllowed type"},
		{name: "InvalidBucketRemainingType", reply: []interface{}{int64(1), int64(1), int64(1), true, int64(1)}, wantErr: "invalid currentBucketRemaining type"},
		{name: "InvalidTotalRemainingType", reply: []interface{}{int64(1), int64(1), int64(1), int64(1), true}, wantErr: "invalid totalRemaining type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, bucketLeft, totalLeft, final, err :=
				newFakeAgent(tt.reply, nil).CheckAndConsumeToken("k", 100, 60, 50, 10, 10)
			assertDecoded(t, err, tt.wantErr)
			if err != nil {
				return
			}
			if bucketLeft != tt.wantBucket || totalLeft != tt.wantTotal || final != tt.wantFinal {
				t.Errorf("got (bucketLeft=%d totalLeft=%d final=%v), want (%d, %d, %v)",
					bucketLeft, totalLeft, final, tt.wantBucket, tt.wantTotal, tt.wantFinal)
			}
		})
	}
}

func TestUpdateConsumeTokenReplyDecoding(t *testing.T) {
	tests := []struct {
		name    string
		reply   interface{}
		wantVal int64
		wantErr string
	}{
		{name: "Int64", reply: []interface{}{int64(110)}, wantVal: 110},
		{
			name:    "String",
			reply:   []interface{}{"110"},
			wantErr: "invalid newBucketValue type",
		},
		{name: "Bytes", reply: []interface{}{[]byte("110")}, wantVal: 110},
		{name: "WrongLength", reply: []interface{}{int64(1), int64(2)}, wantErr: "unexpected lua return type"},
		{name: "InvalidType", reply: []interface{}{true}, wantErr: "invalid newBucketValue type"},
		{name: "BadBytes", reply: []interface{}{[]byte("x")}, wantErr: "parse newBucketValue bytes error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, err := newFakeAgent(tt.reply, nil).UpdateConsumeToken("k", 1700000000, 10, 100)
			assertDecoded(t, err, tt.wantErr)
			if err == nil && gotVal != tt.wantVal {
				t.Errorf("newBucketValue = %d, want %d", gotVal, tt.wantVal)
			}
		})
	}
}

// 脚本执行错误原样上传，不做包装。
func TestAgentPropagatesScriptError(t *testing.T) {
	wantErr := errors.New("script boom")
	agent := newFakeAgent(nil, wantErr)

	if _, _, _, err := agent.QpmCheck("k", 60, 10, 1); !errors.Is(err, wantErr) {
		t.Errorf("QpmCheck err = %v, want %v", err, wantErr)
	}
	if _, _, _, err := agent.ConAcquire("k", 1, 60); !errors.Is(err, wantErr) {
		t.Errorf("ConAcquire err = %v, want %v", err, wantErr)
	}
	if _, err := agent.ConRelease("k", 60); !errors.Is(err, wantErr) {
		t.Errorf("ConRelease err = %v, want %v", err, wantErr)
	}
	if _, _, _, _, _, _, err := agent.CheckAndConsumeToken("k", 100, 60, 50, 10, 1); !errors.Is(err, wantErr) {
		t.Errorf("CheckAndConsumeToken err = %v, want %v", err, wantErr)
	}
	if _, err := agent.UpdateConsumeToken("k", 1700000000, 10, 1); !errors.Is(err, wantErr) {
		t.Errorf("UpdateConsumeToken err = %v, want %v", err, wantErr)
	}
}

// assertDecoded 校验解码错误：期望为空时要求无错，否则要求包含指定关键字。
func assertDecoded(t *testing.T, err error, wantErr string) {
	t.Helper()
	if wantErr == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", wantErr)
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), wantErr)
	}
}

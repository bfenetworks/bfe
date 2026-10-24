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

package bfe_basic

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// NewSession 初始化时间、连接与上下文。
func TestNewSession(t *testing.T) {
	session := newSession()

	if session.StartTime.IsZero() {
		t.Error("StartTime should be set")
	}
	if session.Connection == nil {
		t.Error("Connection should be set")
	}
	if session.RemoteAddr == nil || session.RemoteAddr.String() != "10.0.0.1:54321" {
		t.Errorf("RemoteAddr = %v, want 10.0.0.1:54321", session.RemoteAddr)
	}
	if session.Use100Continue {
		t.Error("Use100Continue should default to false")
	}
	if session.GetVip() != nil {
		t.Errorf("Vip = %v, want nil", session.GetVip())
	}

	// 上下文已初始化，可直接写入
	session.SetContext("k", "v")
	if got := session.GetContext("k"); got != "v" {
		t.Errorf("GetContext = %v, want v", got)
	}

	// conn 为 nil 时不 panic
	if nilSession := NewSession(nil); nilSession.RemoteAddr != nil {
		t.Errorf("RemoteAddr = %v, want nil", nilSession.RemoteAddr)
	}
}

// 请求计数：累加、覆盖、活跃数增减。
func TestSessionReqNum(t *testing.T) {
	session := newSession()

	if got := session.IncReqNum(1); got != 1 {
		t.Errorf("IncReqNum = %d, want 1", got)
	}
	if got := session.IncReqNum(2); got != 3 {
		t.Errorf("IncReqNum = %d, want 3", got)
	}
	if got := session.ReqNum(); got != 3 {
		t.Errorf("ReqNum = %d, want 3", got)
	}

	session.SetReqNum(10)
	if got := session.ReqNum(); got != 10 {
		t.Errorf("ReqNum after set = %d, want 10", got)
	}

	if got := session.IncReqNumActive(1); got != 1 {
		t.Errorf("IncReqNumActive = %d, want 1", got)
	}
	if got := session.IncReqNumActive(-1); got != 0 {
		t.Errorf("IncReqNumActive(-1) = %d, want 0", got)
	}
	session.SetReqNumActive(5)
	if got := session.ReqNumActive(); got != 5 {
		t.Errorf("ReqNumActive = %d, want 5", got)
	}
}

// 读写字节数：返回与上次的差值，回退时返回 0。
func TestSessionByteCounters(t *testing.T) {
	session := newSession()

	if got := session.UpdateReadTotal(100); got != 100 {
		t.Errorf("first UpdateReadTotal = %d, want 100", got)
	}
	if got := session.UpdateReadTotal(150); got != 50 {
		t.Errorf("second UpdateReadTotal = %d, want 50", got)
	}
	// 计数回退（如新连接复用）不应返回负值
	if got := session.UpdateReadTotal(120); got != 0 {
		t.Errorf("regressed UpdateReadTotal = %d, want 0", got)
	}
	if got := session.ReadTotal(); got != 120 {
		t.Errorf("ReadTotal = %d, want 120", got)
	}

	if got := session.UpdateWriteTotal(64); got != 64 {
		t.Errorf("first UpdateWriteTotal = %d, want 64", got)
	}
	if got := session.UpdateWriteTotal(32); got != 0 {
		t.Errorf("regressed UpdateWriteTotal = %d, want 0", got)
	}
	if got := session.WriteTotal(); got != 32 {
		t.Errorf("WriteTotal = %d, want 32", got)
	}
}

// 连接级错误：写入后可按 (msg, err) 读回。
func TestSessionError(t *testing.T) {
	session := newSession()

	if msg, err := session.GetError(); msg != "" || err != nil {
		t.Errorf("GetError = (%q, %v), want empty", msg, err)
	}

	wantErr := errors.New("test error")
	session.SetError(wantErr, "boom")
	msg, err := session.GetError()
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
	if msg != "boom" {
		t.Errorf("msg = %q, want boom", msg)
	}
}

// 上下文：写入、读取、清空。
func TestSessionContext(t *testing.T) {
	session := newSession()

	session.SetContext("k1", 1)
	session.SetContext(2, "v2")
	if got := session.GetContext("k1"); got != 1 {
		t.Errorf("GetContext(k1) = %v, want 1", got)
	}
	if got := session.GetContext(2); got != "v2" {
		t.Errorf("GetContext(2) = %v, want v2", got)
	}

	session.ClearContext()
	if got := session.GetContext("k1"); got != nil {
		t.Errorf("GetContext after clear = %v, want nil", got)
	}

	// 清空后仍可继续写入
	session.SetContext("k3", 3)
	if got := session.GetContext("k3"); got != 3 {
		t.Errorf("GetContext(k3) = %v, want 3", got)
	}
}

// 可信来源标记。
func TestSessionTrustSource(t *testing.T) {
	session := newSession()

	if session.TrustSource() {
		t.Error("TrustSource should default to false")
	}

	session.SetTrustSource(true)
	if !session.TrustSource() {
		t.Error("TrustSource should be true after set")
	}

	session.SetTrustSource(false)
	if session.TrustSource() {
		t.Error("TrustSource should be false after reset")
	}
}

// Finish 记录结束时间与耗时。
func TestSessionFinish(t *testing.T) {
	session := newSession()
	time.Sleep(time.Millisecond)

	session.Finish()

	if session.EndTime.IsZero() {
		t.Error("EndTime should be set")
	}
	if session.Overhead <= 0 {
		t.Errorf("Overhead = %v, want > 0", session.Overhead)
	}
	if session.Overhead != session.EndTime.Sub(session.StartTime) {
		t.Error("Overhead should equal EndTime - StartTime")
	}
}

// String 输出会话 id。
func TestSessionString(t *testing.T) {
	session := newSession()
	session.SessionId = "abc123"

	if got := session.String(); !strings.Contains(got, "abc123") {
		t.Errorf("String() = %q, want it to contain abc123", got)
	}
}
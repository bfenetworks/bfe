// Copyright (c) 2025 The BFE Authors.
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

/* program_test.go - tests for expression compile & run */

package bfe_expr

import (
	"testing"
)

func TestCompileAndMatch(t *testing.T) {
	c, err := Compile(`ReqMethod() in ["GET", "POST"] && ReqPath() startsWith "/api"`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	req := newTestRequest()
	if !c.Match(req) {
		t.Errorf("should match GET /api/user")
	}

	req.HttpRequest.Method = "PUT"
	if c.Match(req) {
		t.Errorf("should not match PUT")
	}

	req.HttpRequest.Method = "GET"
	req.HttpRequest.URL.Path = "/web"
	if c.Match(req) {
		t.Errorf("should not match /web")
	}
}

func TestCompileRejectsNonBool(t *testing.T) {
	// 漏写 ; false：非 bool 结果，编译期必须报错
	if _, err := Compile(`UsrVarSetInt("p", 42)`); err == nil {
		t.Errorf("compile should reject non-bool result")
	}
	if _, err := Compile(`ReqMethod()`); err == nil {
		t.Errorf("compile should reject non-bool result")
	}
}

func TestCompileRejectsUnknownName(t *testing.T) {
	if _, err := Compile(`NoSuchFunc() == 1`); err == nil {
		t.Errorf("compile should reject unknown function")
	}
}

func TestSeedRulePattern(t *testing.T) {
	// 种子规则：写入生效、Cond 为 false
	seed, err := Compile(`UsrVarSetInt("p", 42); false`)
	if err != nil {
		t.Fatalf("Compile seed rule: %v", err)
	}

	req := newTestRequest()
	if seed.Match(req) {
		t.Errorf("seed rule should not match")
	}
	if got := GetEnv(req).UsrVarInt("p"); got != 42 {
		t.Errorf("seed rule should set p=42, got %d", got)
	}

	// 消费规则：读到同一个 p
	consumer, err := Compile(`UsrVarInt("p") < 100`)
	if err != nil {
		t.Fatalf("Compile consumer: %v", err)
	}
	if !consumer.Match(req) {
		t.Errorf("consumer should match with shared p")
	}
}

func TestRuntimeErrorTreatedAsFalse(t *testing.T) {
	// int("abc") 运行期求值错误 → 条件按 false 处理，不 panic
	c := MustCompile(`int(ReqQuery("uid")) < 10`)

	req := newTestRequest()
	req.HttpRequest.URL.RawQuery = "uid=abc"
	if c.Match(req) {
		t.Errorf("runtime error should be treated as false")
	}

	// 注意：CachedQuery 按请求缓存，换个新请求验证正常路径
	req2 := newTestRequest()
	req2.HttpRequest.URL.RawQuery = "uid=5"
	if !c.Match(req2) {
		t.Errorf("should match uid=5")
	}
}

func TestHashGrayReleaseExpression(t *testing.T) {
	c := MustCompile(`Hash(ReqCip()) < 10000`) // 全量
	req := newTestRequest()
	if !c.Match(req) {
		t.Errorf("full bucket range should always match")
	}
}

func TestWriteAndJudgeInOne(t *testing.T) {
	c := MustCompile(`UsrVarSetInt("uid2", int(ReqQuery("uid") ?? "0")) > 100`)

	req := newTestRequest()
	if c.Match(req) {
		t.Errorf("uid=100 should not be > 100")
	}
	if got := GetEnv(req).UsrVarInt("uid2"); got != 100 {
		t.Errorf("write-and-judge should set uid2=100, got %d", got)
	}

	req2 := newTestRequest()
	req2.HttpRequest.URL.RawQuery = "uid=200"
	if !c.Match(req2) {
		t.Errorf("uid=200 should be > 100")
	}
}

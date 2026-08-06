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

package mod_body_process

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewContentAudit(t *testing.T) {
	caf, err := NewContentAudit("http://127.0.0.1:9999", false)
	if err != nil {
		t.Fatalf("NewContentAudit failed: %s", err)
	}
	if caf == nil {
		t.Fatal("expected ContentAudit")
	}
	if !strings.HasSuffix(caf.url, "/text-filter") {
		t.Errorf("expected filter URL, got %s", caf.url)
	}

	caf, _ = NewContentAudit("http://127.0.0.1:9999/", true)
	if !strings.HasSuffix(caf.url, "/text-replace") {
		t.Errorf("expected replace URL, got %s", caf.url)
	}
}

func TestGetAuditDataRawEvent(t *testing.T) {
	e := RawEvent([]byte("hello"))
	data, err := GetAuditData(&e)
	if err != nil {
		t.Fatalf("GetAuditData failed: %s", err)
	}
	if string(data) != "hello" {
		t.Errorf("unexpected data: %s", string(data))
	}
}

func TestGetAuditDataSSEEvent(t *testing.T) {
	e := &SSEEvent{DataLines: [][]byte{[]byte("hello")}, endstyle: "\n"}
	data, err := GetAuditData(e)
	if err != nil {
		t.Fatalf("GetAuditData failed: %s", err)
	}
	if string(data) != "data: hello\n\n" {
		t.Errorf("unexpected data: %s", string(data))
	}
}

func TestGetAuditDataNil(t *testing.T) {
	_, err := GetAuditData(nil)
	if err == nil {
		t.Error("expected error for nil event")
	}
}

func TestGetAuditDataUnsupported(t *testing.T) {
	_, err := GetAuditData(unsupportedEvent{})
	if err == nil {
		t.Error("expected error for unsupported event type")
	}
}

func TestSetAuditDataRawEvent(t *testing.T) {
	e := RawEvent([]byte("hello"))
	if err := SetAuditData(&e, []byte("world")); err != nil {
		t.Fatalf("SetAuditData failed: %s", err)
	}
	if string(e) != "world" {
		t.Errorf("unexpected data: %s", string(e))
	}
}

func TestSetAuditDataSSEEvent(t *testing.T) {
	e := &SSEEvent{DataLines: [][]byte{[]byte("hello")}}
	if err := SetAuditData(e, []byte("world")); err != nil {
		t.Fatalf("SetAuditData failed: %s", err)
	}
	if string(e.GetData()) != "world" {
		t.Errorf("unexpected data: %s", string(e.GetData()))
	}
}

func TestSetAuditDataNil(t *testing.T) {
	if err := SetAuditData(nil, []byte("x")); err == nil {
		t.Error("expected error for nil event")
	}
}

func TestSetAuditDataUnsupported(t *testing.T) {
	if err := SetAuditData(unsupportedEvent{}, []byte("x")); err == nil {
		t.Error("expected error for unsupported event type")
	}
}

func TestContentAuditProcessPass(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		if !strings.Contains(string(body), "txt=") {
			t.Errorf("expected txt param in body, got %s", string(body))
		}
		w.Write([]byte(`{"RiskLevel":"PASS"}`))
	}))
	defer server.Close()

	caf, _ := NewContentAudit(server.URL, false)
	events := []Event{newRawEvent("safe text")}
	out, err := caf.Process(events)
	if err != nil {
		t.Fatalf("Process failed: %s", err)
	}
	if len(out) != 1 {
		t.Errorf("expected 1 event, got %d", len(out))
	}
}

func TestContentAuditProcessReject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"RiskLevel":"REJECT"}`))
	}))
	defer server.Close()

	caf, _ := NewContentAudit(server.URL, false)
	events := []Event{newRawEvent("bad text")}
	_, err := caf.Process(events)
	if err == nil {
		t.Error("expected reject error")
	}
}

func TestContentAuditProcessReplace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ResultText":"replaced text"}`))
	}))
	defer server.Close()

	caf, _ := NewContentAudit(server.URL, true)
	e := newRawEvent("original")
	events := []Event{e}
	out, err := caf.Process(events)
	if err != nil {
		t.Fatalf("Process failed: %s", err)
	}
	if string(out[0].ToBytes()) != "replaced text" {
		t.Errorf("unexpected replaced text: %s", string(out[0].ToBytes()))
	}
}

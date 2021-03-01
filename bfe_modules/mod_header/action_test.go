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

package mod_header

import (
	"net"
	"net/url"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_net/textproto"
)

// fake tcp Connection
type fakeConn struct {
	net.Conn
}

func (fc fakeConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("1.1.1.1"), Port: 22}
}

func makeBasicRequest(requestURL string) *bfe_basic.Request {
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Connection = fakeConn{}
	req.HttpRequest, _ = bfe_http.NewRequest("GET", requestURL, nil)

	return req
}

func makeActionFileList() ActionFileList {
	cmdSet := "REQ_HEADER_SET"
	cmdAdd := "REQ_HEADER_ADD"
	cmdDel := "REQ_HEADER_DEL"

	a1 := ActionFile{&cmdSet, []string{"header1", "value1"}}
	a2 := ActionFile{&cmdAdd, []string{"header2", "value2"}}
	a3 := ActionFile{&cmdDel, []string{"header2"}}

	return []ActionFile{a1, a2, a3}
}

func makeAbnormalActionFileList() ActionFileList {
	cmdSet := "REQ_HEADER_SET"
	cmdAdd := "REQ_HEADER_ADD"
	cmdDel := "REQ_HEADER_DEL"

	a1 := ActionFile{&cmdSet, []string{"header1", "value1", "header"}}
	a2 := ActionFile{&cmdAdd, []string{"header2", "value2"}}
	a3 := ActionFile{&cmdDel, []string{"header3"}}

	return []ActionFile{a1, a2, a3}
}

func TestHeaderActionsDo_Case1(t *testing.T) {
	req := makeBasicRequest("http://www.example.org")

	cmdMod := "REQ_HEADER_MOD"
	action := Action{Cmd: cmdMod, Params: []string{"SCHEME_SET", "Referer", "https"}}

	req.HttpRequest.Header.Add("Referer", "http://www.example.org/index.html")
	HeaderActionsDo(req, 0, []Action{action})

	referer := req.HttpRequest.Header.Get("Referer")
	refererURL, err := url.Parse(referer)
	if err != nil {
		t.Error(err)
	}

	if refererURL.Scheme != "https" {
		t.Errorf("scheme should be https while %s", refererURL.Scheme)
	}

}

func TestHeaderActionsDo_Case2(t *testing.T) {
	req := makeBasicRequest("http://www.example.org")

	cmdMod := "REQ_HEADER_MOD"
	action := Action{Cmd: cmdMod, Params: []string{"QUERY_ADD", "Referer", "foo", "bar"}}

	req.HttpRequest.Header.Add("Referer", "http://www.example.org/index.html")
	HeaderActionsDo(req, 0, []Action{action})

	referer := req.HttpRequest.Header.Get("referer")
	refererURL, err := url.Parse(referer)
	if err != nil {
		t.Error(err)
	}

	query := refererURL.Query()
	if query["foo"][0] != "bar" {
		t.Error("url should have query foo=bar")
	}

	if referer != "http://www.example.org/index.html?foo=bar" {
		t.Error("referer is wrong")
	}
}

func TestHeaderActionsDo_Case3(t *testing.T) {
	req := makeBasicRequest("http://www.example.org")

	cmdMod := "REQ_HEADER_MOD"
	action := Action{Cmd: cmdMod, Params: []string{"QUERY_ADD", "Referer", "foo", "bar"}}

	req.HttpRequest.Header.Add("Referer", "http://www.example.org/index.html?a=b")
	HeaderActionsDo(req, 0, []Action{action})

	referer := req.HttpRequest.Header.Get("referer")
	refererURL, err := url.Parse(referer)
	if err != nil {
		t.Error(err)
	}

	query := refererURL.Query()
	if query["foo"][0] != "bar" {
		t.Error("url should have query foo=bar")
	}

	if referer != "http://www.example.org/index.html?a=b&foo=bar" {
		t.Error("referer is wrong")
	}
}

func TestHeaderActionsDo_Case4(t *testing.T) {
	req := makeBasicRequest("http://www.example.org")

	cmdMod := "REQ_HEADER_MOD"
	action := Action{Cmd: cmdMod, Params: []string{"QUERY_ADD", "Referer", "foo", "bar"}}

	req.HttpRequest.Header.Add("Referer", "http://www.example.org/index.html?foo=b")
	HeaderActionsDo(req, 0, []Action{action})

	referer := req.HttpRequest.Header.Get("referer")
	refererURL, err := url.Parse(referer)
	if err != nil {
		t.Error(err)
	}

	query := refererURL.Query()
	if query["foo"][0] != "b" {
		t.Error("url should have query foo=bar")
	}

	exceptReferer := "http://www.example.org/index.html?foo=b&foo=bar"
	if referer != exceptReferer {
		t.Errorf("referer should be [%s] while [%s]", exceptReferer, referer)
	}
}

func TestHeaderActionsDo_Case5(t *testing.T) {
	req := makeBasicRequest("http://www.example.org")

	cmdMod := "REQ_HEADER_RENAME"
	action := Action{Cmd: cmdMod, Params: []string{"OriginalKey", "NewKey"}}
	expectVal := "TestCase"

	req.HttpRequest.Header.Add("OriginalKey", expectVal)
	HeaderActionsDo(req, 0, []Action{action})

	value := req.HttpRequest.Header.Get("NewKey")
	if value != expectVal {
		t.Errorf("header rename newkey want[%s] got[%s]", expectVal, value)
	}

	value = req.HttpRequest.Header.Get("OriginalKey")
	if value != "" {
		t.Errorf("header rename originalkey want[%s] got[%s]", "", value)
	}
}

func TestActionsConvert(t *testing.T) {
	cmdSet := "REQ_HEADER_SET"
	cmdAdd := "REQ_HEADER_ADD"
	cmdDel := "REQ_HEADER_DEL"

	actionFileList := makeActionFileList()
	actionList, err := actionsConvert(actionFileList)

	if actionList == nil || err != nil {
		t.Error("actionsConvert failed")
	}

	if actionList[0].Cmd != cmdSet {
		t.Errorf("actionsConvert failed for Cmd: %s", cmdSet)
	}
	if actionList[0].Params[0] != "Header1" ||
		actionList[0].Params[1] != "value1" {
		t.Errorf("actionsConvert failed for Params: %s, %s", "header1", "value1")
	}

	if actionList[1].Cmd != cmdAdd {
		t.Errorf("actionsConvert failed for Cmd: %s", cmdAdd)
	}

	if actionList[1].Params[0] != "Header2" ||
		actionList[1].Params[1] != "value2" {
		t.Errorf("actionsConvert failed for Params: %s, %s", "header2", "value2")
	}

	if actionList[2].Cmd != cmdDel {
		t.Errorf("actionsConvert failed for Cmd: %s", cmdDel)
	}
	if actionList[2].Params[0] != "Header2" {
		t.Errorf("actionsConvert failed for Params: %s", "header2")
	}
}

func TestActionFileListCheck(t *testing.T) {
	actionFileList := makeActionFileList()
	err := ActionFileListCheck(&actionFileList)
	if err != nil {
		t.Error("ActionFileListCheck failed. Expect Success.")
	}

	abnormalActionFileList := makeAbnormalActionFileList()
	err = ActionFileListCheck(&abnormalActionFileList)
	if err == nil {
		t.Error("ActionFileListCheck failed. Expect Failure.")
	}
}

func TestHeaderActionsDo(t *testing.T) {
	actionFileList := makeActionFileList()
	actionList, err := actionsConvert(actionFileList)

	if err != nil {
		t.Errorf("actionsConvert failed")
	}

	req := makeBasicRequest("http://www.example.org")
	HeaderActionsDo(req, ReqHeader, actionList)

	if req.HttpRequest.Header.Get("header1") != "value1" {
		t.Error("headerActionsDo failed. Expect True")
	}

	//header2 is deleted by action HEADER_DEL
	if req.HttpRequest.Header.Get("header2") != "" {
		t.Error("headerActionsDo failed. Expect True")
	}
}

func TestSetScheme(t *testing.T) {
	u := "www.example.org/s?wo%21rd=%21"
	in := "http://" + u
	out := "https://" + u

	uri := setScheme(in, "https")
	if uri != out {
		t.Errorf("uri should be %s, while %s", out, uri)
	}
}

func BenchmarkHeaderConvertCase1(b *testing.B) {
	b.ResetTimer()
	header := "X-Ssl-Request"

	for i := 0; i < b.N; i++ {
		textproto.CanonicalMIMEHeaderKey(header)
	}
}

func BenchmarkHeaderConvertCase2(b *testing.B) {
	b.ResetTimer()
	header := "X-Ssl-Request"
	header = textproto.CanonicalMIMEHeaderKey(header)

	for i := 0; i < b.N; i++ {
		textproto.CanonicalMIMEHeaderKey(header)
	}
}

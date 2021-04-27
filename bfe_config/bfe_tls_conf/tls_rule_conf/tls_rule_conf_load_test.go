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

package tls_rule_conf

import (
	"reflect"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_tls"
)

func TestTlsRuleConfLoad(t *testing.T) {
	confActual, err := TlsRuleConfLoad("./testdata/tls_rule.data")
	if err != nil {
		t.Errorf("found err while loading config %s", err.Error())
	}

	confExpect := BfeTlsRuleConf{}
	confExpect.Version = "20141112095308"
	confExpect.Config = make(TlsRuleMap)
	confExpect.Config["pb"] = &TlsRuleConf{
		CertName:   "*.example.com",
		NextProtos: []string{"spdy/3.1", "http/1.1"},
		VipConf:    []string{"1.0.0.1", "1.0.0.2"},
		Grade:      "A",
	}
	confExpect.Config["pn"] = &TlsRuleConf{
		CertName: "*.example.com",
		VipConf:  []string{"1.0.0.3"},
		Grade:    "B",
	}
	confExpect.Config["pz"] = &TlsRuleConf{
		CertName:   "*.example.com",
		NextProtos: []string{"spdy/3.1;rate=50", "http/1.1"},
		VipConf:    []string{"1.0.0.4"},
		Grade:      "B",
	}
	confExpect.Config["pt"] = &TlsRuleConf{
		CertName:   "*.example.com",
		NextProtos: []string{"h2;rate=30", "spdy/3.1;rate=50", "http/1.1"},
		VipConf:    []string{"1.0.0.5"},
		Grade:      "B",
	}
	confExpect.Config["pm"] = &TlsRuleConf{
		CertName:   "*.baidu.com",
		NextProtos: []string{"stream;level=2;pp=1"},
		VipConf:    []string{"1.0.0.6"},
		Grade:      "B",
	}

	if !reflect.DeepEqual(confExpect, confActual) {
		t.Errorf("config expect %v, actual %v", confExpect, confActual)
	}
}

func TestTlsRuleConfLoad2(t *testing.T) {
	invalidConfs := []string{
		"./testdata/tls_rule_conf.data2",
		"./testdata/tls_rule_conf.data3",
		"./testdata/tls_rule_conf.data4",
		"./testdata/tls_rule_conf.data5",
		"./testdata/tls_rule_conf.data6",
		"./testdata/tls_rule_conf.data7",
		"./testdata/tls_rule_conf.data8",
	}
	for _, file := range invalidConfs {
		_, err := TlsRuleConfLoad(file)
		if err == nil {
			t.Errorf("should found err while loading config %s", file)
		}
	}
}

func TestTlsRuleConfLoad3(t *testing.T) {
	file := "./testdata/tls_rule.data9"

	conf, err := TlsRuleConfLoad(file)
	if err != nil {
		t.Errorf("should have no error, not %v", err)
	}

	for product, tlsConf := range conf.Config {
		if product == "pb" && tlsConf.Grade != bfe_tls.GradeC {
			t.Errorf("product[%s] tls security grade err, should be %s, not %s",
				product, bfe_tls.GradeC, tlsConf.Grade)
		} else if product == "pn" && tlsConf.Grade != bfe_tls.GradeC {
			t.Errorf("product[%s] tls security grade err, should be %s, not %s",
				product, bfe_tls.GradeC, tlsConf.Grade)
		}
	}
}

func TestTlsRuleConfLoad4(t *testing.T) {
	file := "./testdata/tls_rule.data10"

	conf, err := TlsRuleConfLoad(file)
	if err != nil {
		t.Errorf("should have no error, not %v", err)
	}

	paCfg, ok := conf.Config["pa"]
	if !ok {
		t.Errorf("should contain product pa")
	}

	if !paCfg.ClientAuth || paCfg.ClientCAName != "pa" {
		t.Errorf("ClientAuth should be true and ClientCAName should be pa")
	}

	clientCAMap, err := ClientCALoad(conf.Config, "./testdata/client_cas")
	if err != nil {
		t.Errorf("in ClientCALoad() :%v", err)
	}

	_, ok = clientCAMap["pa"]
	if !ok {
		t.Errorf("clientCAMap should contain pa CAs")
	}
}

func TestTlsRuleConfLoad5(t *testing.T) {
	file := "./testdata/tls_rule.data11"

	conf, err := TlsRuleConfLoad(file)
	if err != nil {
		t.Errorf("should have no error, not %v", err)
	}

	paCfg, ok := conf.Config["pa"]
	if !ok {
		t.Errorf("should contain product pa")
	}

	if !paCfg.ClientAuth || paCfg.ClientCAName != "pa2" {
		t.Errorf("ClientAuth should be true and ClientCAName should be pa")
	}

	_, err = ClientCALoad(conf.Config, "./testdata/client_cas")
	if err == nil {
		t.Errorf("should have error")
	}
}

func TestTlsRuleConfLoad6(t *testing.T) {
	file := "./testdata/tls_rule.data12"

	confActual, err := TlsRuleConfLoad(file)
	if err != nil {
		t.Errorf("should have no error, not %v", err)
	}

	confExpect := BfeTlsRuleConf{}
	confExpect.Version = "1"
	confExpect.Config = make(TlsRuleMap)
	confExpect.Config["pb"] = &TlsRuleConf{
		CertName:   "*.example.com",
		NextProtos: []string{"spdy/3.1", "http/1.1"},
		VipConf:    []string{"ff02::1"},
		Grade:      "A",
	}
	confExpect.Config["pn"] = &TlsRuleConf{
		CertName: "*.example.com",
		VipConf:  []string{"fc00:1:a000:b00:0:527:127:ab"},
		Grade:    "B",
	}
	confExpect.Config["pz"] = &TlsRuleConf{
		CertName:   "*.example.com",
		NextProtos: []string{"spdy/3.1;rate=50", "http/1.1"},
		VipConf:    []string{"2001:0:1111:a:b0:0:9000:200"},
		Grade:      "B",
	}
	confExpect.Config["pw"] = &TlsRuleConf{
		CertName:   "*.example.com",
		NextProtos: []string{"h2;rate=30", "spdy/3.1;rate=50", "http/1.1"},
		VipConf:    []string{"::1"},
		Grade:      "B",
	}

	if !reflect.DeepEqual(confExpect, confActual) {
		t.Errorf("config expect %v, actual %v", confExpect, confActual)
	}
}

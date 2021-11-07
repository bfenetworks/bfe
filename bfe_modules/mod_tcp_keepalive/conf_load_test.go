/* conf_mod_tcp_keepalive_test.go - test for conf_mod_tcp_keepalive.go */
/*
modification history
--------------------
2021/9/8, by Yu Hui, create
*/
/*
DESCRIPTION
*/

package mod_tcp_keepalive

import (
	"testing"
)

func TestConfModTcpKeepAlive_1(t *testing.T) {
	config, err := ConfLoad("./testdata/mod_tcp_keepalive.conf", "")
	if err != nil {
		t.Errorf("TcpKeepAlive ConfLoad() error: %s", err.Error())
		return
	}

	if config.Basic.DataPath != "../data/mod_tcp_keepalive/tcp_keepalive.data" {
		t.Error("DataPath should be ../data/mod_tcp_keepalive/tcp_keepalive.data")
		return
	}

	if config.Log.OpenDebug != true {
		t.Error("Log.OpenDebug shoule be true")
		return
	}
}

func TestConfModTcpKeepAlive_2(t *testing.T) {
	// invalid value
	config, err := ConfLoad("./testdata/mod_tcp_keepalive_2.conf", "")
	if err != nil {
		t.Errorf("CondLoad() error: %s", err.Error())
	}

	// use default value
	if config.Basic.DataPath != "mod_tcp_keepalive/tcp_keepalive.data" {
		t.Error("DataPath shoule be mod_tcp_keepalive/tcp_keepalive.data")
		return
	}
}

/* keepalive_table_test.go - test for keepalive_table.go */
/*
modification history
--------------------
2021/9/7, by Yu Hui, create
*/
/*
DESCRIPTION
*/

package mod_tcp_keepalive

import (
	"testing"
)

func TestKeepAliveTable(t *testing.T) {
	table := NewKeepAliveTable()
	path := "./testdata/tcp_keepalive.data"
	data, err := KeepAliveDataLoad(path)
	if err != nil {
		t.Errorf("KeepAliveDataLoad(%s) error: %v", path, err)
		return
	}

	table.Update(data)
	if len(table.productRules) != 2 {
		t.Errorf("table.Update error: rules length should be 2 but %d", len(table.productRules))
		return
	}

	_, ok := table.Search("product1")
	if !ok {
		t.Errorf("table.Search error: product1 should exist")
		return
	}
}

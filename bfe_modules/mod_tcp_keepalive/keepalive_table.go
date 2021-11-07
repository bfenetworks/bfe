/* product_rule_table.go - table for storing product tcp keepalive rules */
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
	"sync"
)

type KeepAliveTable struct {
	lock         sync.RWMutex
	version      string
	productRules ProductRules
}

func NewKeepAliveTable() *KeepAliveTable {
	t := new(KeepAliveTable)
	t.productRules = make(ProductRules)

	return t
}

func (t *KeepAliveTable) Update(data ProductRuleData) {
	t.lock.Lock()
	t.version = data.Version
	t.productRules = data.Config
	t.lock.Unlock()
}

func (t *KeepAliveTable) Search(product string) (KeepAliveRules, bool) {
	t.lock.RLock()
	productRules := t.productRules
	t.lock.RUnlock()

	rules, ok := productRules[product]
	return rules, ok
}

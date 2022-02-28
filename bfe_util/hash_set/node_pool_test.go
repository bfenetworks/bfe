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

package hash_set

import (
	"bytes"
	"testing"
)

func TestAddItem(t *testing.T) {
	np := newNodePool(32, 32*32, false)

	Item := []byte("keyForTest")
	node, err := np.add(-1, Item)

	// normal case 1
	if err != nil {
		t.Errorf("err is not nil %s", err.Error())
	}
	if node != 0 {
		t.Errorf("node should be 0 %d", node)
	}
	if !bytes.Equal(np.element(0), Item) {
		t.Error("element is wrong")
	}

	// normal case 2
	node, err = np.add(0, Item)
	if err != nil {
		t.Errorf("err is not nil %s", err.Error())
	}
	if node != 1 {
		t.Errorf("node should be 0 %d", node)
	}
	if !bytes.Equal(np.element(1), Item) {
		t.Error("element is wrong")
	}
	if !np.exist(0, Item) {
		t.Error("should find in this list")
	}
	if np.compare(Item, 1) != 0 {
		t.Error("should find in this list")
	}

}

func TestDelItem(t *testing.T) {
	np := newNodePool(32, 32*32, false)

	Item1 := []byte("ItemForTest1")
	Item2 := []byte("ItemForTest2")
	np.add(-1, Item1)
	np.add(0, Item1)
	np.add(1, Item2)
	if np.array[1].next != 0 {
		t.Error("1 should link to 0")
	}
	if np.array[0].next != -1 {
		t.Error("0 should link to -1")
	}

	// del at list
	if np.del(1, Item1) != 0 {
		t.Error("del should return newhead")
	}

	// del head
	if np.del(0, Item2) != 0 {
		t.Error("del should return newhead")
	}
}

func TestGetFreeNode(t *testing.T) {
	np := newNodePool(32, 32*32, false)

	//case1
	node, err := np.getFreeNode()
	if node != 0 || err != nil {
		t.Error("get node error")
	}
	//case after recycleNode
	np.recycleNode(3)
	node, err = np.getFreeNode()
	if node != 3 || err != nil {
		t.Error("get node error")
	}

}

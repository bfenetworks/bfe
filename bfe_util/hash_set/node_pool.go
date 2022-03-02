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
	"fmt"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/byte_pool"
)

/* hash node */
type hashNode struct {
	next int32 // link to the next node
}

/* a list of hash node */
type nodePool struct {
	array []hashNode //node array

	freeNode int32 // manage the freeNode of nodePool
	capacity int   // capacity of nodePool
	length   int   // length of nodePool

	pool byte_pool.IBytePool // reference to []byte pool
}

/*
 * create a new nodePool
 *
 * PARAMS:
 *  - elemNum: max num of elements
 *  - elemSize: max size of each element of hashSet
 *
 * RETURNS:
 *  - pointer to nodePool
 */
func newNodePool(elemNum, elemSize int, isFixedKeylen bool) *nodePool {
	np := new(nodePool)

	// make and init node array
	np.array = make([]hashNode, elemNum)
	for i := 0; i < elemNum-1; i += 1 {
		np.array[i].next = int32(i + 1) // link to the next node
	}
	np.array[elemNum-1].next = -1 //initial value == -1, means end of the list

	np.freeNode = 0 //free node start from 0
	np.capacity = elemNum
	np.length = 0

	if isFixedKeylen {
		np.pool = byte_pool.NewFixedBytePool(elemNum, elemSize)
	} else {
		np.pool = byte_pool.NewBytePool(elemNum, elemSize)
	}
	return np
}

/*
 * add
 *  - add key into the list starting from head
 *  - return the new headNode
 *
 * PARAMS:
 *  - head: first node of the list
 *  - key: []byte type
 *
 * RETURNS:
 *  - (newHead, nil), success, new headNode of the list
 *  - (-1, error), if fail
 */
func (np *nodePool) add(head int32, key []byte) (int32, error) {
	// get a bucket from freeNode List
	node, err := np.getFreeNode()
	if err != nil {
		return -1, err
	}

	np.array[node].next = head
	//set the node with key
	np.pool.Set(node, key)

	np.length += 1
	return node, nil
}

/*
 * del
 *  - remove the key([]byte) in the given list
 *  - return the new head of the list
 *
 * PARAMS:
 *  - head: int, the first node of the list
 *  - key: []byte, the key need to be del
 *
 * RETURNS:
 *  - newHead int, the new head node of the list
 */
func (np *nodePool) del(head int32, key []byte) int32 {
	var newHead int32
	// check at the head of List
	if np.compare(key, head) == 0 {
		newHead = np.array[head].next
		np.recycleNode(head) //recycle the node
		return newHead
	}

	// check at the list
	pindex := head
	for {
		index := np.array[pindex].next
		if index == -1 {
			break
		}
		if np.compare(key, index) == 0 {
			np.array[pindex].next = np.array[index].next
			np.recycleNode(index) //recycle the node
			return head
		}
		pindex = index
	}
	return head
}

/* del the node, add the node into freeNode list */
func (np *nodePool) recycleNode(node int32) {
	index := np.freeNode
	np.freeNode = node
	np.array[node].next = index
	np.length -= 1
}

/* check if the key exist in the list */
func (np *nodePool) exist(head int32, key []byte) bool {
	for index := head; index != -1; index = np.array[index].next {
		if np.compare(key, index) == 0 {
			return true
		}
	}
	return false
}

/* get a free node from freeNode list */
func (np *nodePool) getFreeNode() (int32, error) {
	if np.freeNode == -1 {
		return -1, fmt.Errorf("NodePool: no more node to use")
	}

	// return freeNode and make freeNode = freeNode.next
	node := np.freeNode
	np.freeNode = np.array[node].next
	np.array[node].next = -1

	return node, nil
}

/* get node num in use of nodePool */
func (np *nodePool) elemNum() int {
	return np.length
}

/* check if the node Pool is full */
func (np *nodePool) full() bool {
	return np.length >= np.capacity
}

/* compare the given key with index node */
func (np *nodePool) compare(key []byte, i int32) int {
	element := np.element(i)
	return bytes.Compare(key, element)
}

/* get the element of the giving index*/
func (np *nodePool) element(i int32) []byte {
	return np.pool.Get(i)
}

/* get the space allocate for each element */
func (np *nodePool) elemSize() int {
	return np.pool.MaxElemSize()
}

/* check whtether the key is legal for the set */
func (np *nodePool) validateKey(key []byte) error {
	if len(key) <= np.elemSize() {
		return nil
	}
	return fmt.Errorf("element len[%d] > bucketSize[%d]", len(key), np.elemSize())
}

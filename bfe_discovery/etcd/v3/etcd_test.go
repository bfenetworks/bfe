// Copyright (c) 2019 Baidu, Inc.
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

package v3

import (
	"bytes"
	"context"
	"testing"
	"time"
)

import (
	"github.com/baidu/bfe/bfe_discovery"
)

func TestEtcdv3(t *testing.T) {
	store, err := New(&bfe_discovery.Config{
		Addrs:       []string{"127.0.0.1:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	// put key
	err = store.Put(ctx, "bfe_key", []byte("bfe_key"), &bfe_discovery.WriteOptions{
		TTL: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	// put dir
	err = store.Put(ctx, "bfe_key_dir/aaa", []byte("aaa"), &bfe_discovery.WriteOptions{
		TTL: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = store.Put(ctx, "bfe_key_dir/bbb", []byte("bbb"), &bfe_discovery.WriteOptions{
		TTL: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	// get
	kv, err := store.Get(context.Background(), "bfe_key", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(kv.Value) != "bfe_key" {
		t.Fatal("unexpected store.Get")
	}

	// list
	kvs, err := store.List(context.Background(), "bfe_key_dir", nil)
	if err != nil {
		t.Fatal(err)
	}

	if kvs[0].Key != "bfe_key_dir" && !bytes.Equal(kvs[0].Value, []byte("bbb")) {
		t.Fatal("unexpected store.List")
	}
	if kvs[0].Key != "bfe_key_dir" && !bytes.Equal(kvs[1].Value, []byte("aaa")) {
		t.Fatal("unexpected store.List")
	}

	// exist && delete
	exist, err := store.Exist(context.Background(), "bfe_key")
	if err != nil {
		t.Fatal(err)
	}
	if !exist {
		t.Fatal("unexpected store.Exist")
	}
	err = store.Delete(context.Background(), "bfe_key")
	if err != nil {
		t.Fatal(err)
	}
	exist, err = store.Exist(context.Background(), "bfe_key")
	if err != nil {
		t.Fatal(err)
	}
	if exist {
		t.Fatal("unexpected store.Exist")
	}

	cancel()

}

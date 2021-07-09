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
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

import (
	etcd "go.etcd.io/etcd/clientv3"
)

import (
	discovery "github.com/baidu/bfe/bfe_discovery"
)

const (
	PathPrefix = "bfe"
)

type Etcdv3 struct {
	cli  *etcd.Client
	once sync.Once
}

func init() {
	discovery.Register(discovery.BACKENDETCDV3, New)
}

func New(config *discovery.Config) (discovery.Store, error) {
	var err error
	e := &Etcdv3{}

	// defautl config
	if config == nil {
		config = &discovery.Config{
			Addrs:       []string{"127.0.0.1:2379"},
			DialTimeout: 5 * time.Second,
			PathPrefix:  PathPrefix,
		}
	}

	e.cli, err = etcd.New(etcd.Config{
		Endpoints:   config.Addrs,
		DialTimeout: config.DialTimeout,

		Username: config.Username,
		Password: config.Password,
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Etcdv3) Get(ctx context.Context, key string, options *discovery.ReadOptions) (*discovery.KVPair, error) {
	var err error
	var resp *etcd.GetResponse

	if options != nil && !options.Consistency {
		resp, err = e.cli.Get(ctx, key)
	} else {
		resp, err = e.cli.Get(ctx, key, etcd.WithSerializable())
	}
	if err != nil {
		return nil, err
	}

	if resp.Count == 0 {
		return nil, fmt.Errorf("%s(%s)", discovery.ErrKeyNotFound, key)
	}

	kvPair := &discovery.KVPair{
		Key:   string(resp.Kvs[0].Key),
		Value: resp.Kvs[0].Value,
	}

	return kvPair, nil
}

func (e *Etcdv3) List(ctx context.Context, key string, options *discovery.ReadOptions) ([]*discovery.KVPair, error) {
	var err error
	var resp *etcd.GetResponse

	if options != nil && !options.Consistency {
		resp, err = e.cli.Get(ctx, key, etcd.WithPrefix(), etcd.WithSort(etcd.SortByKey, etcd.SortDescend))
	} else {
		resp, err = e.cli.Get(ctx, key, etcd.WithSerializable(), etcd.WithPrefix(), etcd.WithSort(etcd.SortByKey, etcd.SortDescend))
	}
	if err != nil {
		return nil, err
	}

	if resp.Count == 0 {
		return nil, fmt.Errorf("%s(%s)", discovery.ErrKeyNotFound, key)
	}

	var kvPair []*discovery.KVPair
	for _, kv := range resp.Kvs {
		kvPair = append(kvPair, &discovery.KVPair{
			Key:   string(kv.Key),
			Value: kv.Value,
		})
	}

	return kvPair, nil
}

func (e *Etcdv3) Put(ctx context.Context, key string, value []byte, options *discovery.WriteOptions) error {
	var err error

	if options != nil && options.TTL > 0 {
		grantResp, err := e.cli.Grant(context.Background(), int64(options.TTL/time.Second))
		if err != nil {
			return err
		}
		_, err = e.cli.Put(ctx, key, string(value), etcd.WithLease(grantResp.ID))
	} else {
		_, err = e.cli.Put(ctx, key, string(value))
	}
	if err != nil {
		return err
	}

	return nil
}

func (e *Etcdv3) Delete(ctx context.Context, key string) error {
	var err error

	resp, err := e.cli.Delete(ctx, key)
	if err != nil {
		return err
	}

	if resp.Deleted == 0 {
		return fmt.Errorf("%s(%s)", discovery.ErrKeyNotFound, key)
	}

	return nil
}

func (e *Etcdv3) Exist(ctx context.Context, key string) (bool, error) {
	var err error

	_, err = e.Get(ctx, key, nil)
	if err != nil {
		// TODO Custom errors package, we need Contains func
		if strings.Contains(err.Error(), discovery.ErrKeyNotFound.Error()) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (e *Etcdv3) Watch(ctx context.Context, key string, options *discovery.ReadOptions) (<-chan *discovery.KVPair, error) {
	respCh := make(chan *discovery.KVPair)

	go func() {
		defer close(respCh)

		rch := e.cli.Watch(ctx, key)
		for wresp := range rch {
			for _, ev := range wresp.Events {
				respCh <- &discovery.KVPair{
					Key:   string(ev.Kv.Key),
					Value: ev.Kv.Value,
				}
			}
		}
	}()

	return respCh, nil
}

func (e *Etcdv3) WatchList(ctx context.Context, key string, options *discovery.ReadOptions) (<-chan []*discovery.KVPair, error) {
	respCh := make(chan []*discovery.KVPair)

	go func() {
		defer close(respCh)

		// TODO maybe we need revision on watch
		rch := e.cli.Watch(ctx, key, etcd.WithPrefix())
		for wresp := range rch {

			list := make([]*discovery.KVPair, len(wresp.Events))
			for i, ev := range wresp.Events {
				list[i] = &discovery.KVPair{
					Key:   string(ev.Kv.Key),
					Value: ev.Kv.Value,
				}

				respCh <- list
			}
		}
	}()

	return respCh, nil
}

func (e *Etcdv3) Close() {
	e.once.Do(func() {
		e.cli.Close()
	})
}

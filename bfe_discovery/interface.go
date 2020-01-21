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

package bfe_discovery

import (
	"context"
	"time"
	"fmt"
)


type Store interface {
	Get(ctx context.Context, key string, options *ReadOptions) (*KVPair, error)
	List(ctx context.Context, key string, options *ReadOptions) ([]*KVPair, error)
	Put(ctx context.Context, key string, value []byte, options *WriteOptions) error
	Delete(ctx context.Context, key string) error
	Exist(ctx context.Context, key string) (bool,error)
	Watch(ctx context.Context, key string, options *ReadOptions) (<-chan *KVPair, error)
	WatchList(ctx context.Context, key string, options *ReadOptions) (<-chan []*KVPair, error)
	Close()
}

func NewStore(b BackendType, config *Config) (Store, error)  {
	if init, ok := Backend[b]; ok {
		return init(config)
	}
	return nil, fmt.Errorf("%s, only support %s", ErrBackendNonSupported, SupportedBackend())
}

type KVPair struct {
	Key string
	Value []byte
}

type ReadOptions struct {
	// Etcdv3 --consistency args
	Consistency bool
}

type WriteOptions struct {
	IsDir bool // Etcdv2
	TTL   time.Duration
}

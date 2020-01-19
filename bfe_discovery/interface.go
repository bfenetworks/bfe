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

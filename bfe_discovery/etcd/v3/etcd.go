package v3

import (
	"time"
	"sync"
	"context"
	"fmt"
	"strings"
)

import (
	etcd "go.etcd.io/etcd/clientv3"
)

import (
	discovery "github.com/baidu/bfe/bfe_discovery"
)

type Etcdv3 struct {
	cli *etcd.Client
	once sync.Once
}

func init()  {
	discovery.Register(discovery.BACKENDETCDV3, New)
}

func New(config *discovery.Config) (discovery.Store, error)  {
	var err error
	e := &Etcdv3{}

	// defautl config
	if config == nil {
		config = &discovery.Config{
			Addrs: []string{"127.0.0.1:2379"},
			DialTimeout: 5 * time.Second,
		}
	}

	e.cli, err = etcd.New(etcd.Config{
		Endpoints:   config.Addrs,
		DialTimeout: config.DialTimeout,

		Username: config.Username,
		Password:config.Password,
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
	}else {
		resp, err = e.cli.Get(ctx, key, etcd.WithSerializable())
	}
	if err != nil {
		return  nil, err
	}

	if resp.Count == 0 {
		return nil, fmt.Errorf("%s(%s)",discovery.ErrKeyNotFound, key)
	}

	kvPair := &discovery.KVPair{
		Key:string(resp.Kvs[0].Key),
		Value:resp.Kvs[0].Value,
	}

	return kvPair, nil
}

func (e *Etcdv3) List(ctx context.Context, key string, options *discovery.ReadOptions) ([]*discovery.KVPair, error) {
	var err error
	var resp *etcd.GetResponse

	if options != nil && !options.Consistency {
		resp, err = e.cli.Get(ctx, key, etcd.WithPrefix(), etcd.WithSort(etcd.SortByKey, etcd.SortDescend))
	}else {
		resp, err = e.cli.Get(ctx, key, etcd.WithSerializable(), etcd.WithPrefix(), etcd.WithSort(etcd.SortByKey, etcd.SortDescend))
	}
	if err != nil {
		return  nil, err
	}

	if resp.Count == 0 {
		return nil, fmt.Errorf("%s(%s)",discovery.ErrKeyNotFound, key)
	}

	var kvPair []*discovery.KVPair
	for _, kv := range resp.Kvs {
		kvPair = append(kvPair, &discovery.KVPair{
			Key: string(kv.Key),
			Value:kv.Value,
		})
	}

	return kvPair, nil
}

func (e *Etcdv3) Put(ctx context.Context, key string, value []byte, options *discovery.WriteOptions) error {
	var err error

	if options != nil && options.TTL > 0{
		grantResp, err := e.cli.Grant(context.Background(), int64(options.TTL/time.Second))
		if err != nil {
			return err
		}
		_, err = e.cli.Put(ctx, key, string(value), etcd.WithLease(grantResp.ID))
	}else {
		_, err = e.cli.Put(ctx, key, string(value))
	}
	if err != nil {
		return  err
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
		return fmt.Errorf("%s(%s)",discovery.ErrKeyNotFound, key)
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

func (e *Etcdv3)Close()  {
	e.once.Do(func() {
		e.cli.Close()
	})
}
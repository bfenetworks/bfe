package bfe_discovery

import (
	"sort"
	"strings"
	"errors"
)

var (
	ErrBackendNonSupported = errors.New("bfe_discovery Backend Non Supported")

	ErrKeyNotFound = errors.New("Key not found")
)

type BackendType string

const (
	BACKENDETCD BackendType = "etcd"
	BACKENDETCDV3 BackendType = "etcdv3"
	BACKENDCONSUL BackendType = "consul"
	BACKENDZK BackendType = "zk"
)

type InitFunc func(config *Config) (Store, error)

var (
	Backend = make(map[BackendType]InitFunc)
)

func Register(b BackendType, f InitFunc)  {
	Backend[b] = f
}

func SupportedBackend() string {
	keys := make([]string, 0, len(Backend))
	for k := range Backend {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
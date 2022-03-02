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

// an implementation of tls.ServerSessionCache

package bfe_server

import (
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/gomodule/redigo/redis"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_conf"
	"github.com/bfenetworks/bfe/bfe_util/bns"
)

type ServerSessionCache struct {
	Servers     []string     // tcp address for redis servers
	serversLock sync.RWMutex // lock for servers
	bnsClient   *bns.Client  // name client

	ConnectTimeout time.Duration // connect timeout (ms)
	ReadTimeout    time.Duration // read timeout (ms)
	WriteTimeout   time.Duration // write timeout (ms)
	KeyPrefix      string        // prefix for cache key

	SessionExpire int          // expire time for tls session state (s)
	MaxIdle       int          // max idle connections in pool
	pool          *redis.Pool  // connection pool to redis server
	poolLock      sync.RWMutex // lock for pool

	state *ProxyState // state for session cache
}

func NewServerSessionCache(conf bfe_conf.ConfigSessionCache, state *ProxyState) (
	c *ServerSessionCache) {
	var err error
	c = new(ServerSessionCache)

	// get address list of redis servers
	c.bnsClient = bns.NewClient()
	if c.Servers, err = c.bnsClient.GetInstancesAddr(conf.Servers); err != nil {
		log.Logger.Warn("ServerSessionCache: get instance for %s error (%s)", conf.Servers, err)
		c.Servers = make([]string, 0)
	}

	c.ConnectTimeout = time.Duration(conf.ConnectTimeout) * time.Millisecond
	c.ReadTimeout = time.Duration(conf.ReadTimeout) * time.Millisecond
	c.WriteTimeout = time.Duration(conf.WriteTimeout) * time.Millisecond
	c.KeyPrefix = conf.KeyPrefix

	c.SessionExpire = conf.SessionExpire
	c.MaxIdle = conf.MaxIdle
	c.pool = &redis.Pool{
		MaxIdle: c.MaxIdle,
		Dial:    c.dial,
	}

	c.state = state
	go c.checkServerInstance(conf.Servers)

	return c
}

func (c *ServerSessionCache) dial() (redis.Conn, error) {
	c.state.SessionCacheConn.Inc(1)

	// choose a random server
	c.serversLock.RLock()
	if len(c.Servers) == 0 {
		c.serversLock.RUnlock()
		return nil, fmt.Errorf("no available connection in pool")
	}
	server := c.Servers[rand.Intn(len(c.Servers))]
	c.serversLock.RUnlock()

	// create connection to server
	conn, err := redis.Dial("tcp", server,
		redis.DialConnectTimeout(c.ConnectTimeout),
		redis.DialReadTimeout(c.ReadTimeout),
		redis.DialWriteTimeout(c.WriteTimeout))
	if err != nil {
		log.Logger.Debug("ServerSessionCache:dail() to %s err(%v)", server, err)
		c.state.SessionCacheConnFail.Inc(1)
		return nil, err
	}
	return conn, err
}

// Put saves sessionState to session cache.
func (c *ServerSessionCache) Put(sessionKey string, sessionState []byte) (err error) {
	c.state.SessionCacheSet.Inc(1)
	sessionKey = fmt.Sprintf("%s:%s", c.KeyPrefix, sessionKey)

	c.poolLock.RLock()
	pool := c.pool
	c.poolLock.RUnlock()

	// get connection from pool
	conn := pool.Get()
	defer conn.Close()

	// save session state to redis
	conn.Send("SET", sessionKey, sessionState)
	conn.Send("EXPIRE", sessionKey, c.SessionExpire)
	conn.Flush()
	if _, err = conn.Receive(); err != nil {
		log.Logger.Debug("ServerSessionCache:put() sessionState %v", err)
		c.state.SessionCacheSetFail.Inc(1)
		return err
	}
	if _, err = conn.Receive(); err != nil {
		log.Logger.Debug("ServerSessionCache:put() sessionState %v", err)
		c.state.SessionCacheSetFail.Inc(1)
		return err
	}

	log.Logger.Debug("ServerSessionCache:put() sessionState success (%s: %x)",
		sessionKey, sessionState)
	return nil
}

// Get gets sessionState from session cache.
func (c *ServerSessionCache) Get(sessionKey string) ([]byte, bool) {
	c.state.SessionCacheGet.Inc(1)
	sessionKey = fmt.Sprintf("%s:%s", c.KeyPrefix, sessionKey)

	c.poolLock.RLock()
	pool := c.pool
	c.poolLock.RUnlock()

	// get connection from pool
	conn := pool.Get()
	defer conn.Close()

	// get session state from redis
	sessionParam, err := conn.Do("GET", sessionKey)
	if err != nil {
		log.Logger.Debug("ServerSessionCache:get() sessionState %v", err)
		if err != redis.ErrNil {
			c.state.SessionCacheGetFail.Inc(1)
		} else {
			c.state.SessionCacheMiss.Inc(1)
		}
		return nil, false
	}

	sessionState, ok := sessionParam.([]byte)
	if !ok {
		c.state.SessionCacheTypeNotBytes.Inc(1)
		log.Logger.Debug("ServerSessionCache:get() sessionState type not []byte(%s: %T)",
			sessionKey, sessionParam)
		return nil, false
	}

	log.Logger.Debug("ServerSessionCache:get() sessionState success (%s: %x)",
		sessionKey, sessionState)
	c.state.SessionCacheHit.Inc(1)
	return sessionState, true
}

func (c *ServerSessionCache) UpdateServers(servers []string) {
	c.serversLock.Lock()
	c.Servers = servers
	c.serversLock.Unlock()
}

func (c *ServerSessionCache) UpdatePool(pool *redis.Pool) *redis.Pool {
	c.poolLock.Lock()
	oldPool := c.pool
	c.pool = pool
	c.poolLock.Unlock()

	return oldPool
}

func (c *ServerSessionCache) checkServerInstance(name string) {
	for {
		time.Sleep(10 * time.Second)

		// check addresses of redis servers
		servers, err := c.bnsClient.GetInstancesAddr(name)
		if err != nil {
			log.Logger.Warn("ServerSessionCache: get instance address %s", err.Error())
			continue
		}
		if len(servers) == 0 {
			log.Logger.Warn("ServerSessionCache: no address configured for %v", name)
			c.state.SessionCacheNoInstance.Inc(1)
			continue
		}
		if reflect.DeepEqual(servers, c.Servers) {
			continue
		}

		// update addresses of redis servers
		log.Logger.Debug("ServerSessionCache: update instances %s", servers)
		c.UpdateServers(servers)

		// update connection pool
		pool := &redis.Pool{
			MaxIdle: c.MaxIdle,
			Dial:    c.dial,
		}
		oldPool := c.UpdatePool(pool)
		oldPool.Close()
	}
}

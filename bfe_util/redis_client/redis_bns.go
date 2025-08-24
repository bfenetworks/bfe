/* redis_bns.go - redis client with bns support*/
/*
DESCRIPTION
    redis client with bns support

Usage:
    bnsName := "bfe-tc.bfe.tc" // bns of redis server
    maxIdle := 10              // max Idle connection
    connectTimeout := 10       // connection timeout in ms
    readTimeout    := 10       // read redis server timeout in ms
    writeTimeout   := 10       // write redis server timeout in ms
    redisClient :=  redis_bns.NewRedisClient(bnsName, maxIdle, connectTimeout, readTimeout, writeTimeout)

    // setex/get/incr/decr
    redisClient.Setex("key", "val", expireTime)
    redisClient.Get("key")
    redisClient.Incr("key", expireTime)
    redisClient.Decr("key")
*/

package redis_client

import (
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/delay_counter"
	"github.com/baidu/go-lib/web-monitor/module_state2"
	"github.com/spaolacci/murmur3"
	"github.com/gomodule/redigo/redis"
	"github.com/bfenetworks/bfe/bfe_util/bns"
)

var (
	// default bns update interval 10s
	DfBnsUpdateInterval = 60 * time.Second
	// max value for one redis cluster weight
	MaxWeightValue = 100
	// min value for one redis cluster weight
	MinWeightValue = 1
	// max value for all redis clusters weight sum
	MaxWeightSum = 10000

	RedisGetBnsInstanceErr  = "REDIS_GET_BNS_INSTANCE_ERR"
	RedisNoBnsInstance      = "REDIS_NO_BNS_INSTANCE"
	RedisBnsInstanceChanged = "REDIS_BNS_INSTANCE_CHANGED"
)

type RedisClient struct {
	ConnectTimeout time.Duration // connect timeout (ms)
	ReadTimeout    time.Duration // read timeout (ms)
	WriteTimeout   time.Duration // write timeout (ms)

	Password string // password, ignore if no password

	MaxIdle   int  // max idle conenctions in pool
	MaxActive int  // max active connections in pool
	Wait      bool // if pool meet MaxActive limit, and Wait is true, wait for a connection return to pool

	redisClusters        []redisCluster // redisCluster list, offset is redisClusterId
	redisClusterSlotSize uint64         // hash slot size, the value is the sum of each bns`s weight
	redisClusterSlotMap  []int          // offset: hash slot, value: redisClusterId

	// stateDelegate StateDelegate              // state delegate, this can be nil
	moduleState2  *module_state2.State       // state in format to module_state2
	delay         *delay_counter.DelayRecent // delay counter for reids
	connDelay     *delay_counter.DelayRecent // delay counter for connect to redis
}

type redisCluster struct {
	bns         string       // bns for redis cluster
	weight      int          // weight for current cluster
	redisClient *RedisClient // associate redis client pointer

	serversLock sync.RWMutex // lock for servers
	Servers     []string     // tcp address for redis servers
	pool        *redis.Pool  // connection pool to redis server
	poolLock    sync.RWMutex // lock for pool
}

type RedisClusterConf struct {
	bns    string // bns for redis cluster
	weight int    // weight for current cluster
}

func ParseRedisBnsConf(serviceConfRawStr string) ([]RedisClusterConf, error) {
	// 0.1 trim space in serviceConf string
	serviceConf := strings.Replace(serviceConfRawStr, " ", "", -1)

	// 0.2 check empty string
	if len(serviceConfRawStr) == 0 {
		return []RedisClusterConf{}, fmt.Errorf("service conf is empty string")
	}

	// 1. simple condition: serviceConf is just a bns
	if !strings.Contains(serviceConf, ",") && !strings.Contains(serviceConf, "|") {
		return []RedisClusterConf{{bns: serviceConf, weight: 1}}, nil
	}

	// 2. the other condition: serviceConf is a batch of bns name with weight
	// 2.1 parse and check confList from serviceConf string
	confStrList := strings.Split(serviceConf, "|")
	if len(confStrList) == 0 {
		return []RedisClusterConf{}, fmt.Errorf("split redis serviceConf(%s) err", serviceConf)
	}
	clusterSize := len(confStrList)
	confList := make([]RedisClusterConf, clusterSize)
	for i, confStr := range confStrList {
		confElements := strings.Split(confStr, ",")
		if len(confElements) != 2 {
			return []RedisClusterConf{}, fmt.Errorf("split redis serviceConf(%s) by ',' length err", confStr)
		}

		confList[i].bns = confElements[0]

		weightElements := strings.Split(confElements[1], ":")
		if len(weightElements) != 2 {
			return []RedisClusterConf{}, fmt.Errorf("split redis serviceConf(%s) weightStr(%s) by ':' length err",
				confStr, confElements[1])
		}
		if weightElements[0] != "weight" {
			return []RedisClusterConf{}, fmt.Errorf("split redis serviceConf(%s) weightStr(%s) by ':' find no 'weight'",
				confStr, confElements[1])
		}
		weight, err := strconv.Atoi(weightElements[1])
		if err != nil {
			return []RedisClusterConf{}, fmt.Errorf("check redis serviceConf(%s) weight(%s) err(%s)",
				confStr, weightElements[1], err.Error())
		}
		if weight > MaxWeightValue || weight < MinWeightValue {
			return []RedisClusterConf{}, fmt.Errorf("check redis serviceConf(%s) weight(%s) err, weight should be [%d, %d])",
				confStr, weightElements[1], MinWeightValue, MaxWeightValue)
		}
		confList[i].weight = weight
	}

	// 2.2 check bns name conflict and weight sum
	weightSum := 0
	bnsConflictChecker := make(map[string]bool)
	for _, conf := range confList {
		if _, ok := bnsConflictChecker[conf.bns]; ok {
			return []RedisClusterConf{},
				fmt.Errorf("check redis serviceConf(%s) err: bns(%s) conflict", serviceConf, conf.bns)
		}
		bnsConflictChecker[conf.bns] = true
		weightSum = weightSum + conf.weight
	}
	if weightSum > MaxWeightSum {
		return []RedisClusterConf{}, fmt.Errorf("check redis serviceConf(%s) err: weight sum overlimit(%d)",
			serviceConf, MaxWeightSum)
	}

	return confList, nil
}

// NewRedisClient(): create a new redisClient with bns support
// Notice:
//    - if resolve bns error, c.Servers will be empty.
// Params:
//    - serviceConf: string, bns name or a batch of bns name with weight of redis server
//    - maxIdle: int, max idle connections in connection pool
//    - ct: int, connect redis server timeout, in ms
//    - rt: int, read redis server timeout, in ms
//    - wt: int, write redis server timeout, in ms
// Returns:
//    - *redisClient: a new redis client
func NewRedisClient1(serviceConf string, maxIdle int, ct, rt, wt int) *RedisClient {
	return NewRedisBnsClient(&Options{
		ServiceConf:    serviceConf,
		MaxIdle:        maxIdle,
		ConnTimeoutMs:  ct,
		ReadTimeoutMs:  rt,
		WriteTimeoutMs: wt,
	})
}

// NewRedisClient2(): create a new redisClient with bns support
// Notice:
//    - if resolve bns error, c.Servers will be empty.
// Params:
//    - serviceConf: string, bns name or a batch of bns name with weight of redis server
//    - maxIdle: int, max idle connections in connection pool
//    - maxActive: int, max active connections in connection pool
//    - wait: bool, if wait is true and pool at the maxActive limit,
//                  command waits for a connection return to the pool
//    - ct: int, connect redis server timeout, in ms
//    - rt: int, read redis server timeout, in ms
//    - wt: int, write redis server timeout, in ms
// Returns:
//    - *redisClient: a new redis client
func NewRedisClient2(serviceConf string, maxIdle, maxActive int, wait bool, ct, rt, wt int) *RedisClient {
	return NewRedisBnsClient(&Options{
		ServiceConf:    serviceConf,
		MaxIdle:        maxIdle,
		ConnTimeoutMs:  ct,
		ReadTimeoutMs:  rt,
		WriteTimeoutMs: wt,
		MaxActive:      maxActive,
		Wait:           wait,
	})
}

func (opts *Options) Format() error {
	serviceConf, err := ParseRedisBnsConf(opts.ServiceConf)
	if err != nil {
		return fmt.Errorf("parse redis service conf %s err %s", opts.ServiceConf, err.Error())
	}

	opts.clusterList = serviceConf
	return nil
}

// NewRedisBnsClient(): create a new redisClient with bns support
// Notice:
//    - if resolve bns error, c.Servers will be empty.
// Returns:
//    - *redisClient: a new redis client
func NewRedisBnsClient(opts *Options) *RedisClient {
	err := opts.Format()
	if err != nil {
		log.Logger.Warn(err.Error())
		return nil
	}

	redisClusterConfList := opts.clusterList

	// create RedisClient
	c := &RedisClient{
		Password: opts.Password,

		// timeout in ms
		ConnectTimeout: time.Duration(opts.ConnTimeoutMs) * time.Millisecond,
		ReadTimeout:    time.Duration(opts.ReadTimeoutMs) * time.Millisecond,
		WriteTimeout:   time.Duration(opts.WriteTimeoutMs) * time.Millisecond,

		// max idle connection
		MaxIdle: opts.MaxIdle,

		// max active connection
		MaxActive: opts.MaxActive,
		Wait:      opts.Wait,

		// module state
		// stateDelegate: nil,
		moduleState2:  nil,
		delay:         nil,
		connDelay:     nil,
	}

	// create redis clusters
	c.redisClusterSlotSize = 0
	c.redisClusters = make([]redisCluster, len(redisClusterConfList))
	for i, redisClusterConf := range redisClusterConfList {
		c.redisClusters[i].bns = redisClusterConf.bns
		c.redisClusters[i].weight = redisClusterConf.weight
		c.redisClusters[i].redisClient = c

		c.redisClusterSlotSize = c.redisClusterSlotSize + uint64(redisClusterConf.weight)

		c.redisClusters[i].Servers, err = bns.NewClient().GetInstancesAddr(redisClusterConf.bns)
		if err != nil {
			log.Logger.Warn("get instance for %s err %s", redisClusterConf.bns, err.Error())
		}

		c.redisClusters[i].pool = &redis.Pool{
			MaxIdle:   c.MaxIdle,
			MaxActive: c.MaxActive,
			Wait:      c.Wait,
			Dial:      c.redisClusters[i].dial,
		}
	}

	// set redisClusterSlotMap
	slotIndex := 0
	c.redisClusterSlotMap = make([]int, c.redisClusterSlotSize)
	for id := range c.redisClusters {
		for count := 0; count < c.redisClusters[id].weight; count++ {
			c.redisClusterSlotMap[slotIndex] = id
			slotIndex++
		}
	}

	// goroutine to update bns
	go c.checkServerInstance()

	return c
}

// set state delegate to redisClient
// func (c *RedisClient) SetStateDelegate(delegate StateDelegate) {
// 	c.stateDelegate = delegate
// }

// set state of module_state2 to redisClient
func (c *RedisClient) SetModuleState2(state *module_state2.State) {
	c.moduleState2 = state
}

// set delay counter to redisClient
func (c *RedisClient) SetDelay(delayCounter *delay_counter.DelayRecent) {
	c.delay = delayCounter
}

// set conn delay counter to redisClient
func (c *RedisClient) SetConnDelay(delayCounter *delay_counter.DelayRecent) {
	c.connDelay = delayCounter
}

// judge and set module_state2 by state string
func (c *RedisClient) incrModuleState2(state string) {
	if c.moduleState2 != nil {
		c.moduleState2.Inc(state, 1)
	}
}

// judge and set delay counter
func (c *RedisClient) setDelayState(delay *delay_counter.DelayRecent, start time.Time) {
	if delay != nil {
		delay.AddBySub(start, time.Now())
	}
}

// dial choose a random server from redisCluster.Servers and connect
func (c *redisCluster) dial() (redis.Conn, error) {
	c.redisClient.incrModuleState2(RedisConn)

	// choose a random server
	c.serversLock.RLock()
	if len(c.Servers) == 0 {
		c.serversLock.RUnlock()
		return nil, fmt.Errorf("no available connnection in pool")
	}
	server := c.Servers[rand.Intn(len(c.Servers))]
	c.serversLock.RUnlock()

	// create connection to server
	conn, err := redis.DialTimeout("tcp",
		server,
		c.redisClient.ConnectTimeout,
		c.redisClient.ReadTimeout,
		c.redisClient.WriteTimeout)
	if err != nil {
		c.redisClient.incrModuleState2(RedisConnFail)
		return nil, err
	}

	if password := c.redisClient.Password; password != "" {
		if _, err := conn.Do("AUTH", password); err != nil {
			c.redisClient.incrModuleState2(RedisAuthFail)
			conn.Close()
			return nil, err
		}
	}

	return conn, nil
}

func (c *redisCluster) UpdateServers(servers []string) {
	c.serversLock.Lock()
	c.Servers = servers
	c.serversLock.Unlock()
}

func (c *redisCluster) UpdatePool(pool *redis.Pool) *redis.Pool {
	c.poolLock.RLock()
	oldPool := c.pool
	c.pool = pool
	c.poolLock.RUnlock()

	return oldPool
}

// ActiveConnNum returns the num of active connextions
func (c *RedisClient) ActiveConnNum() int {
	activeCountSum := 0
	for id := range c.redisClusters {
		c.redisClusters[id].poolLock.RLock()
		activeCountSum += c.redisClusters[id].pool.ActiveCount()
		c.redisClusters[id].poolLock.RUnlock()
	}

	return activeCountSum
}

// Setex(): save key:value to redis server, and set expire time
// Params:
//    - key: string
//    - value: []byte
//    - expire: int, expire time in second
// Returns:
//    - nil, if success, otherwise return error
//save sessionState to session cache
func (c *RedisClient) Setex(key string, value []byte, expire int) (err error) {
	c.incrModuleState2(RedisSetex)

	// get a connection
	conn := c.getConnByKey(key)
	defer conn.Close()

	procStart := time.Now()
	// send setex cmd
	conn.Send("SETEX", key, expire, value)
	conn.Flush()
	if _, err = conn.Receive(); err != nil {
		c.incrModuleState2(RedisSetexFail)
		return err
	}

	c.setDelayState(c.delay, procStart)
	return nil
}

// get value from redis
func (c *RedisClient) Get(key string) (interface{}, error) {
	c.incrModuleState2(RedisGet)

	// get connection from pool
	conn := c.getConnByKey(key)
	defer conn.Close()

	procStart := time.Now()
	// get session state from redis
	value, err := conn.Do("GET", key)
	// redigo may return both value and err is nil
	if value == nil && err == nil {
		c.incrModuleState2(RedisGetMiss)
		return nil, redis.ErrNil
	}
	// handle err is not nil
	if err != nil {
		if err != redis.ErrNil {
			c.incrModuleState2(RedisGetFail)
		} else {
			c.incrModuleState2(RedisGetMiss)
		}
		return nil, err
	}

	c.setDelayState(c.delay, procStart)
	c.incrModuleState2(RedisGetHit)
	return value, nil
}

// get value from redis
func (c *RedisClient) GetInt64(key string) (int64, error) {
	c.incrModuleState2(RedisGet)

	// get connection from pool
	conn := c.getConnByKey(key)
	defer conn.Close()

	procStart := time.Now()
	// get session state from redis
	value, err := redis.Int64(conn.Do("GET", key))
	// handle err is not nil
	if err != nil {
		if err != redis.ErrNil {
			c.incrModuleState2(RedisGetFail)
		} else {
			c.incrModuleState2(RedisGetMiss)
		}
		return 0, err
	}

	c.setDelayState(c.delay, procStart)
	c.incrModuleState2(RedisGetHit)
	return value, nil
}

// set expire to redis
func (c *RedisClient) Expire(key string, expire int) error {
	c.incrModuleState2(RedisExpire)

	// get connection from pool
	conn := c.getConnByKey(key)
	defer conn.Close()

	procStart := time.Now()
	// get session state from redis
	_, err := conn.Do("EXPIRE", key, expire)
	if err != nil {
		c.incrModuleState2(RedisExpireFail)
		return err
	}

	c.setDelayState(c.delay, procStart)
	return nil
}

// incr key to redis
func (c *RedisClient) Incr(key string) (int64, error) {
	c.incrModuleState2(RedisIncr)

	// get connection from pool
	conn := c.getConnByKey(key)
	defer conn.Close()

	procStart := time.Now()
	// send incr & expire cmd
	conn.Send("INCR", key)
	conn.Flush()
	// get result from incr cmd
	count, err := redis.Int64(conn.Receive())
	if err != nil {
		c.incrModuleState2(RedisIncrFail)
		return count, err
	}

	c.setDelayState(c.delay, procStart)
	return count, nil
}

// incr key to redis
func (c *RedisClient) IncrBy(key string, delta int64) (int64, error) {
	c.incrModuleState2(RedisIncr)

	// get connection from pool
	conn := c.getConnByKey(key)
	defer conn.Close()

	procStart := time.Now()
	// send incr & expire cmd
	conn.Send("INCRBY", key, delta)
	conn.Flush()
	// get result from incr cmd
	count, err := redis.Int64(conn.Receive())
	if err != nil {
		c.incrModuleState2(RedisIncrFail)
		return count, err
	}

	c.setDelayState(c.delay, procStart)
	return count, nil
}

// incr and expire key to redis
func (c *RedisClient) IncrAndExpire(key string, expire int) (int64, error) {
	c.incrModuleState2(RedisIncr)

	// get connection from pool
	conn := c.getConnByKey(key)
	defer conn.Close()

	procStart := time.Now()
	// send incr & expire cmd
	conn.Send("INCR", key)
	conn.Send("EXPIRE", key, expire)
	conn.Flush()
	// get result from incr cmd
	count, err := redis.Int64(conn.Receive())
	if err != nil {
		c.incrModuleState2(RedisIncrFail)
		return count, err
	}

	// get result from expire cmd
	if _, err = conn.Receive(); err != nil {
		c.incrModuleState2(RedisExpireFail)
		return count, err
	}

	c.setDelayState(c.delay, procStart)
	return count, nil
}

/*
do redis pipeline incr command, filter the keys by redis cluster id
set countList and errList as return value, only modify the members which belong to current cluster id
param:
	keyList []string	total key list, this function only use the members which belog to current cluster id
	countList *[]int64		count list return value, only modify the members which belong to current cluster id
	errList *[]error		error return value, only modify the member with the offset is current cluster id
*/
func (c *RedisClient) pincrByRedisClusterId(keyList []string,
	clusterId int,
	countList *[]int64,
	errList *[]error) {
	var err error
	var count int64

	// get a sub list for the keys belong to current cluster id
	subKeyList := make([]string, 0)
	for i := 0; i < len(keyList); i++ {
		if c.getClusterIdByKey(keyList[i]) == clusterId {
			subKeyList = append(subKeyList, keyList[i])
		}
	}

	// if there is no sub keylist for current cluster id, just return
	if len(subKeyList) == 0 {
		return
	}

	// get connection from pool
	conn := c.getConnByClusterId(clusterId)
	defer conn.Close()

	// send by pipeline
	subCountList := make([]int64, len(subKeyList))
	for i := range subKeyList {
		c.incrModuleState2(RedisIncr)

		// send incr cmd
		if err = conn.Send("INCR", subKeyList[i]); err != nil {
			c.incrModuleState2(RedisSendFail)
			goto ret
		}
	}

	// flush
	if err = conn.Flush(); err != nil {
		c.incrModuleState2(RedisFlushFail)
		goto ret
	}

	// receive values
	for i := range subKeyList {
		// get result from incr cmd
		if count, err = redis.Int64(conn.Receive()); err != nil {
			c.incrModuleState2(RedisIncrFail)
			goto ret
		}

		// append to countList
		subCountList[i] = count
	}

ret:
	if err == nil {
		subIndex := 0
		for i := 0; i < len(keyList); i++ {
			if c.getClusterIdByKey(keyList[i]) == clusterId {
				(*countList)[i] = subCountList[subIndex]
				subIndex++
			}
		}
	}
	(*errList)[clusterId] = err
}

// PIncr incr keys in pipeline mode, seprate keyList by clusterId and do pincr concurrently
func (c *RedisClient) PIncr(keyList []string) ([]int64, error) {
	var err error
	var totalErrStr string
	errList := make([]error, len(c.redisClusters))
	procStart := time.Now()

	if len(keyList) == 0 {
		return []int64{}, fmt.Errorf("len err: keyList(%d)", len(keyList))
	}
	countList := make([]int64, len(keyList), len(keyList))

	// run pincr seprate by cluseter id concurrently
	for id := range c.redisClusters {
		c.pincrByRedisClusterId(keyList,
			id,
			&countList,
			&errList)
	}

	// wait for each response
	for redisClusterId := 0; redisClusterId < len(c.redisClusters); redisClusterId++ {
		if errList[redisClusterId] != nil {
			totalErrStr += fmt.Sprintf("redisClusterId(%d) pincr err(%s), ",
				redisClusterId, errList[redisClusterId].Error())
		}
	}

	// hanele error response
	if totalErrStr != "" {
		err = fmt.Errorf(totalErrStr)
	}

	c.setDelayState(c.delay, procStart)
	return countList, err
}

// decr key to redis
func (c *RedisClient) Decr(key string) (int64, error) {
	c.incrModuleState2(RedisDecr)

	// get connection from pool
	conn := c.getConnByKey(key)
	defer conn.Close()

	procStart := time.Now()
	// send decr cmd
	conn.Send("DECR", key)
	conn.Flush()
	// get result from decr cmd
	count, err := redis.Int64(conn.Receive())
	if err != nil {
		c.incrModuleState2(RedisDecrFail)
		return count, err
	}

	c.setDelayState(c.delay, procStart)
	return count, nil
}

// get a connection from connection pool by redis key
// todo: change this to private function
func (c *RedisClient) getConnByKey(key string) redis.Conn {
	return c.getConnByClusterId(c.getClusterIdByKey(key))
}

// get redis cluster id by redis key
func (c *RedisClient) getClusterIdByKey(key string) int {
	slot := getHash([]byte(key), c.redisClusterSlotSize)
	return c.redisClusterSlotMap[slot]
}

// get a connection from connection pool by cluster id
func (c *RedisClient) getConnByClusterId(clusterId int) redis.Conn {
	procStart := time.Now()

	// get connection pool
	c.redisClusters[clusterId].poolLock.RLock()
	pool := c.redisClusters[clusterId].pool
	c.redisClusters[clusterId].poolLock.RUnlock()

	// get connection from pool
	conn := pool.Get()

	c.setDelayState(c.connDelay, procStart)
	return conn
}

// update bns
func (c *RedisClient) checkServerInstance() {
	for {
		time.Sleep(DfBnsUpdateInterval)

		for id := range c.redisClusters {
			// check addresses of redis servers
			servers, err := bns.NewClient().GetInstancesAddr(c.redisClusters[id].bns)
			if err != nil {
				c.incrModuleState2(RedisGetBnsInstanceErr)
				continue
			}
			if len(servers) == 0 {
				c.incrModuleState2(RedisNoBnsInstance)
				continue
			}
			if reflect.DeepEqual(servers, c.redisClusters[id].Servers) {
				continue
			}

			// update addresses of redis servers
			c.redisClusters[id].UpdateServers(servers)

			// counter bns instance changed
			c.incrModuleState2(RedisBnsInstanceChanged)

			// update connection pool
			pool := &redis.Pool{
				MaxIdle:   c.MaxIdle,
				MaxActive: c.MaxActive,
				Wait:      c.Wait,
				Dial:      c.redisClusters[id].dial,
			}
			oldPool := c.redisClusters[id].UpdatePool(pool)
			oldPool.Close()
		}
	}
}

func getHash(value []byte, base uint64) int {
	var hash uint64

	if value == nil {
		hash = uint64(rand.Uint32())
	} else {
		hash = murmur3.Sum64(value)
	}

	return int(hash % base)
}

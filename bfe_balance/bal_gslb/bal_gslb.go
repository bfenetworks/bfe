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

// sub-cluster level load balance using gslb

package bal_gslb

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bfenetworks/go-lib/log"
	"github.com/bfenetworks/go-lib/web-monitor/metrics"

	bal_backend "github.com/bfenetworks/bfe/bfe_balance/backend"
	"github.com/bfenetworks/bfe/bfe_balance/bal_slb"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/gslb_conf"
	"github.com/bfenetworks/bfe/bfe_util/epp"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	DefaultRetryMax      = 3 // default max retries in assigned sub cluster
	DefaultCrossRetryMax = 1 // default max retries in other sub cluster, if retries in assigned sub cluster fail
	REQ_CTX_EPP          = "epp_ctx"
)

type BalanceGslb struct {
	lock sync.Mutex

	name        string         // name of cluster, e.g., "news"
	subClusters SubClusterList // list of sub cluster

	totalWeight int  // sum weight of sub clusters that has a weight >0
	single      bool // only one sub cluster?
	avail       int  // if single is true, avail is index of avail sub cluster

	retryMax    int                   // max retries in assigned sub cluster
	crossRetry  int                   // max retries in other sub cluster, if all retry within assigned sub cluster fail
	hashConf    cluster_conf.HashConf // gslb hash conf
	BalanceMode string                // balanceMode, WRR or WLC, defined in cluster_conf

	// EPP related
	eppMu      sync.Mutex
	eppRt      *eppRuntime   // current EPP runtime, nil if not in EPP mode
	eppRetired []*eppRuntime // old runtimes pending graceful close
	eppBreaker *eppBreaker   // circuit breaker of EPP path, nil if not in EPP mode
}

func NewBalanceGslb(name string) *BalanceGslb {
	bal := new(BalanceGslb)
	bal.name = name

	bal.retryMax = DefaultRetryMax
	bal.crossRetry = DefaultCrossRetryMax
	defaultStrategy := cluster_conf.ClientIpOnly
	defaultSessionSticky := false
	bal.hashConf = cluster_conf.HashConf{
		HashStrategy:  &defaultStrategy,
		SessionSticky: &defaultSessionSticky,
	}
	bal.BalanceMode = cluster_conf.BalanceModeWrr

	return bal
}

func (bal *BalanceGslb) SetGslbBasic(gslbBasic cluster_conf.GslbBasicConf) {
	bal.lock.Lock()

	bal.crossRetry = *gslbBasic.CrossRetry
	bal.retryMax = *gslbBasic.RetryMax
	bal.hashConf = *gslbBasic.HashConf
	bal.BalanceMode = *gslbBasic.BalanceMode

	bal.lock.Unlock()

	// init or close EPP runtime according to balance mode
	if gslbBasic.BalanceMode != nil && strings.ToUpper(*gslbBasic.BalanceMode) == cluster_conf.BalanceModeEPP {
		if gslbBasic.EPPAddr != nil && len(*gslbBasic.EPPAddr) > 0 {
			if err := bal.initEPP(*gslbBasic.EPPAddr, gslbBasic); err != nil {
				log.Logger.Error("initEPP failed: %v", err)
			}
		} else {
			// EPP mode without address, ensure EPP is closed
			bal.closeEPP()
		}
	} else {
		// non-EPP mode, ensure EPP is closed
		bal.closeEPP()
	}
}

func (bal *BalanceGslb) SetSlowStart(backendConf cluster_conf.BackendBasic) {
	bal.lock.Lock()

	for _, sub := range bal.subClusters {
		sub.setSlowStart(*backendConf.SlowStartTime)
	}

	bal.lock.Unlock()
}

// eppRetireGrace is the time in-flight requests may keep using connections
// of a retired EPP runtime before they are closed. It is a var (not const)
// so tests can shorten it.
var eppRetireGrace = 60 * time.Second

// buildEPPRuntimeConf extracts EPP runtime parameters from gslb basic conf,
// applying defaults for unset (nil) sections.
func buildEPPRuntimeConf(gslbBasic cluster_conf.GslbBasicConf) eppRuntimeConf {
	conf := eppRuntimeConf{
		connectTimeout:   cluster_conf.DefaultEPPConnectTimeoutValue(),
		callTimeout:      cluster_conf.DefaultEPPCallTimeoutValue(),
		checkInterval:    cluster_conf.DefaultEPPCheckIntervalValue(),
		failThreshold:    cluster_conf.DefaultEPPFailThreshold,
		cooldown:         cluster_conf.DefaultEPPCooldownValue(),
		successThreshold: cluster_conf.DefaultEPPSuccessThreshold,
	}

	if gslbBasic.EPPCheck != nil {
		conf.checkDisabled = gslbBasic.EPPCheck.Disabled
		conf.checkInterval = gslbBasic.EPPCheck.CheckIntervalDuration()
		if gslbBasic.EPPCheck.FailThreshold != nil {
			conf.failThreshold = *gslbBasic.EPPCheck.FailThreshold
		}
		conf.cooldown = gslbBasic.EPPCheck.CooldownDuration()
		if gslbBasic.EPPCheck.SuccessThreshold != nil {
			conf.successThreshold = *gslbBasic.EPPCheck.SuccessThreshold
		}
	}

	if gslbBasic.EPPTimeout != nil {
		conf.connectTimeout = gslbBasic.EPPTimeout.ConnectDuration()
		conf.callTimeout = gslbBasic.EPPTimeout.CallDuration()
	}

	// Compat: nil EPPTLS keeps legacy behavior (skip certificate verification).
	// Only an explicitly configured EPPTLS enables verification.
	if gslbBasic.EPPTLS != nil {
		conf.tlsInsecure = gslbBasic.EPPTLS.Insecure
		conf.tlsCAFile = gslbBasic.EPPTLS.CAFile
	} else {
		conf.tlsInsecure = true
	}

	return conf
}

// buildEPPBreakerConf extracts EPP breaker parameters from gslb basic conf,
// applying defaults for unset fields.
func buildEPPBreakerConf(gslbBasic cluster_conf.GslbBasicConf) eppBreakerConf {
	conf := eppBreakerConf{
		windowSize:       cluster_conf.DefaultEPPBreakerWindowSize,
		minVolume:        cluster_conf.DefaultEPPBreakerMinVolume,
		errorRatePercent: cluster_conf.DefaultEPPBreakerErrorRatePercent,
		openTimeout:      cluster_conf.DefaultEPPBreakerOpenTimeoutValue(),
	}

	if gslbBasic.EPPBreaker != nil {
		conf.disabled = gslbBasic.EPPBreaker.Disabled
		if gslbBasic.EPPBreaker.WindowSize != nil {
			conf.windowSize = *gslbBasic.EPPBreaker.WindowSize
		}
		if gslbBasic.EPPBreaker.MinVolume != nil {
			conf.minVolume = *gslbBasic.EPPBreaker.MinVolume
		}
		if gslbBasic.EPPBreaker.ErrorRatePercent != nil {
			conf.errorRatePercent = *gslbBasic.EPPBreaker.ErrorRatePercent
		}
		conf.openTimeout = gslbBasic.EPPBreaker.OpenTimeoutDuration()
	}

	return conf
}

// initEPP initializes or refreshes EPP runtime with given addresses.
// If only check/timeout/tls parameters changed (address table unchanged),
// they are applied to the existing runtime so in-flight requests are not
// interrupted; address table change swaps in a new runtime and the old one
// is closed after a grace period.
func (bal *BalanceGslb) initEPP(addrs []string, gslbBasic cluster_conf.GslbBasicConf) error {
	conf := buildEPPRuntimeConf(gslbBasic)
	breakerConf := buildEPPBreakerConf(gslbBasic)

	bal.eppMu.Lock()
	defer bal.eppMu.Unlock()

	if len(addrs) == 0 {
		bal.closeEPPLocked()
		return nil
	}

	// create or refresh the circuit breaker (EPP path only)
	if bal.eppBreaker == nil {
		bal.eppBreaker = newEppBreaker(bal.name, breakerConf)
	} else {
		bal.eppBreaker.updateConf(breakerConf)
	}

	if bal.eppRt != nil && bal.eppRt.sameAddrs(addrs) {
		bal.eppRt.updateConf(conf)
		return nil
	}

	rt, err := newEPPRuntime(bal.name, addrs, conf)
	if err != nil {
		return err
	}

	old := bal.eppRt
	bal.eppRt = rt
	if old != nil {
		bal.retireEPPLocked(old)
	}
	return nil
}

// retireEPPLocked stops the health check of an old runtime and closes its
// connections after eppRetireGrace, so in-flight requests are not interrupted.
// Caller must hold bal.eppMu.
func (bal *BalanceGslb) retireEPPLocked(rt *eppRuntime) {
	rt.stopProbes()
	bal.eppRetired = append(bal.eppRetired, rt)
	time.AfterFunc(eppRetireGrace, func() {
		rt.closeConns()
	})
}

// closeEPP closes and clears EPP runtime if exists.
func (bal *BalanceGslb) closeEPP() {
	bal.eppMu.Lock()
	defer bal.eppMu.Unlock()
	bal.closeEPPLocked()
}

// closeEPPLocked closes current and retired EPP runtimes.
// Caller must hold bal.eppMu.
func (bal *BalanceGslb) closeEPPLocked() {
	if bal.eppRt != nil {
		bal.eppRt.stopProbes()
		bal.eppRt.closeConns()
		bal.eppRt = nil
	}
	for _, rt := range bal.eppRetired {
		rt.stopProbes()
		rt.closeConns()
	}
	bal.eppRetired = nil
	bal.eppBreaker = nil
}

// getEPPRt returns current EPP runtime, or nil if not in EPP mode.
func (bal *BalanceGslb) getEPPRt() *eppRuntime {
	bal.eppMu.Lock()
	defer bal.eppMu.Unlock()
	return bal.eppRt
}

// chooseBackendFromEPP calls EPP service to get target backend address.
// The first message carries "llm-d.ai/inference-pool" metadata so that EPP
// can route the stream to the cell of this cluster. If the chosen address
// fails with a retryable error (cell draining / unknown pool / transport),
// the following addresses are tried in order for this request only (one
// pass); this does not affect the health check state machine.
func (bal *BalanceGslb) chooseBackendFromEPP(req *bfe_basic.Request) (addr string, c *epp.EppClient, err error) {
	rt := bal.getEPPRt()
	if rt == nil {
		return "", nil, fmt.Errorf("no epp client")
	}

	n := rt.addrCount()
	if n == 0 {
		return "", nil, fmt.Errorf("no epp address")
	}

	// inject pool metadata: {"llm-d.ai": {"inference-pool": <cluster name>}}
	md, err := structpb.NewStruct(map[string]any{"inference-pool": bal.name})
	if err != nil {
		return "", nil, err
	}
	metadata := &corev3.Metadata{
		FilterMetadata: map[string]*structpb.Struct{"llm-d.ai": md},
	}

	hasBody := req.OutRequest.ContentLength != 0
	callTimeout := rt.callTimeout()
	start := rt.activeIndex()

	// record the final outcome of this request's EPP call(s) for metrics
	attempted := false
	defer func() {
		if attempted {
			recordEPPCall(bal.name, err)
		}
	}()

	var lastErr error
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		conn := rt.connFor(idx)
		if conn == nil {
			lastErr = fmt.Errorf("epp conn %s not ready", rt.addrAt(idx))
			continue
		}

		attempted = true
		addrinfo, eppClient, callErr := bal.callEPP(conn, metadata, req, hasBody, callTimeout)
		if callErr == nil {
			return addrinfo, eppClient, nil
		}
		lastErr = callErr
		if !isEPPRetryable(callErr) {
			// e.g. missing inference-pool metadata is a BFE defect, do not retry
			break
		}
		log.Logger.Info("EPP addr %s failed (%v), try next address", rt.addrAt(idx), callErr)
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no epp address available")
	}
	return "", nil, lastErr
}

// callEPP talks to one EPP address: opens a stream on the shared connection,
// sends RequestHeaders (+RequestBody), receives scheduling decision from
// dynamic metadata.
func (bal *BalanceGslb) callEPP(conn *grpc.ClientConn, metadata *corev3.Metadata,
	req *bfe_basic.Request, hasBody bool, callTimeout time.Duration) (string, *epp.EppClient, error) {
	if conn == nil {
		return "", nil, fmt.Errorf("epp conn not ready")
	}

	eppClient, err := epp.NewEppClient(conn, callTimeout)
	if err != nil {
		return "", nil, err
	}

	reqMsg := &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: epp.BuildEnvoyGRPCHeaders(req.OutRequest.Header, true, !hasBody),
		},
		MetadataContext: metadata,
	}

	if err := eppClient.Send(reqMsg); err != nil {
		eppClient.Close()
		return "", nil, err
	}

	if hasBody {
		var body []byte
		bodyAccessor, _ := req.OutRequest.GetBodyAccessor()
		if bodyAccessor != nil {
			body, _ = bodyAccessor.GetBytes()
		}

		reqMsg := &extprocv3.ProcessingRequest{
			Request: &extprocv3.ProcessingRequest_RequestBody{
				RequestBody: &extprocv3.HttpBody{
					Body:        body,
					EndOfStream: true,
				},
			},
		}

		if err := eppClient.Send(reqMsg); err != nil {
			eppClient.Close()
			return "", nil, err
		}
	}

	// receive response with deadline for first message round trip
	resp, err := eppClient.RecvTimeout(callTimeout)
	if err != nil {
		eppClient.Close()
		return "", nil, err
	}

	var addrinfo string
	// Try to inspect dynamic_metadata
	if md := resp.GetDynamicMetadata(); md != nil {
		if v, ok := md.Fields["envoy.lb"]; ok {
			// try to get x-gateway-destination-endpoint
			if m := v.GetStructValue(); m != nil {
				if val, found := m.Fields["x-gateway-destination-endpoint"]; found {
					if s := val.GetStringValue(); s != "" {
						// may be comma separated list, pick first
						parts := strings.Split(s, ",")
						addrinfo = strings.TrimSpace(parts[0])
					}
				}
			}
		}
	}

	if hasBody && resp.Response != nil && resp.GetRequestHeaders() != nil {
		// receive response for request body
		resp, err = eppClient.RecvTimeout(callTimeout)
		// the response should be resp.GetRequestBody()
		if err != nil {
			eppClient.Close()
			return "", nil, err
		}
	}

	if addrinfo != "" {
		return addrinfo, eppClient, nil
	}

	eppClient.Close()
	return "", nil, fmt.Errorf("no endpoint from epp")
}

// isEPPRetryable reports whether an EPP error allows retrying the request on
// the next address (per-request retry; does not change health check state):
//   - Unavailable / cell is not serving (cell draining): try backup instance
//   - Internal / unknown inference pool: EPP has no cell of this cluster
func isEPPRetryable(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}

	switch st.Code() {
	case codes.Unavailable:
		return true
	case codes.Internal:
		msg := st.Message()
		return strings.Contains(msg, "unknown inference pool") ||
			strings.Contains(msg, "cell is not serving") ||
			strings.Contains(msg, "draining")
	default:
		return false
	}
}

// Init initializes gslb cluster with config
func (bal *BalanceGslb) Init(gslbConf gslb_conf.GslbClusterConf) error {
	totalWeight := 0

	for subClusterName, weight := range gslbConf {
		subCluster := newSubCluster(subClusterName)
		subCluster.weight = weight

		if weight > 0 {
			totalWeight += weight
		}

		// add sub-cluster to cluster
		bal.subClusters = append(bal.subClusters, subCluster)
	}

	if totalWeight == 0 {
		// should never be here, as ClusterCheck return true
		log.Logger.Critical("gslb total weight = 0 [%s]", bal.name)
		return fmt.Errorf("gslb total weight = 0 [%s]", bal.name)
	}

	bal.totalWeight = totalWeight

	// sort list to guarantee same order, since map iteration is not in order
	sort.Sort(SubClusterListSorter{bal.subClusters})
	availNum := 0
	for index, sub := range bal.subClusters {
		if sub.weight > 0 {
			bal.avail = index
			availNum += 1
		}
	}
	bal.single = (availNum == 1)

	return nil
}

func (bal *BalanceGslb) BackendInit(clusterBackend cluster_table_conf.ClusterBackend) error {
	bal.lock.Lock()

	for _, subCluster := range bal.subClusters {
		if backend, ok := clusterBackend[subCluster.Name]; ok {
			subCluster.init(backend)
		}
	}

	bal.lock.Unlock()
	return nil
}

// Reload reloads gslb config
func (bal *BalanceGslb) Reload(gslbConf gslb_conf.GslbClusterConf) error {
	bal.lock.Lock()
	defer bal.lock.Unlock()

	// create new SubClusterList
	var subListNew SubClusterList

	// create a map to record exist subCluster in gslbConf
	subExist := make(map[string]bool)

	// go through existing sub cluster, and doing update
	for i := 0; i < len(bal.subClusters); i++ {
		sub := bal.subClusters[i]

		// find new conf of sub in gslbConf
		weight, ok := gslbConf[sub.Name]

		if ok {
			// exist in new conf
			sub.weight = weight

			// add sub cluster to subListNew
			subListNew = append(subListNew, sub)
		} else {
			// release sub_cluster
			sub.release()
			log.Logger.Info("release subcluster %s", sub.Name)
		}

		// record in the map of subExist
		subExist[sub.Name] = true
	}

	// go through gslbConf, and doing init for those not in subExist
	for subName, weight := range gslbConf {
		_, ok := subExist[subName]

		if !ok {
			// create new sub cluster
			sub := newSubCluster(subName)
			sub.weight = weight

			// add sub cluster to subListNew
			subListNew = append(subListNew, sub)
		}
	}

	// sort list
	sort.Sort(SubClusterListSorter{subListNew})

	// calc total_weight
	totalWeight := 0
	availableNum := 0
	lastAvailIndex := 0

	for index, sub := range subListNew {
		if sub.weight > 0 {
			totalWeight += sub.weight
			availableNum += 1
			lastAvailIndex = index
		}
	}

	if totalWeight == 0 {
		// should never be here, as ClusterCheck return true
		log.Logger.Critical("gslb total weight = 0 [%s]", bal.name)
		return fmt.Errorf("gslb total weight = 0 [%s]", bal.name)
	}

	bal.totalWeight = totalWeight

	if availableNum == 1 {
		bal.single = true
		bal.avail = lastAvailIndex
	} else {
		bal.single = false
	}

	// update gslb.subClusters
	bal.subClusters = subListNew

	return nil
}

func (bal *BalanceGslb) BackendReload(clusterBackend cluster_table_conf.ClusterBackend) error {
	bal.lock.Lock()

	for _, subCluster := range bal.subClusters {
		if backend, ok := clusterBackend[subCluster.Name]; ok {
			subCluster.update(backend)
		}
	}

	bal.lock.Unlock()

	return nil
}

func (bal *BalanceGslb) Release() {
	bal.lock.Lock()

	// go through all sub clusters
	for i := 0; i < len(bal.subClusters); i++ {
		// release sub_cluster
		bal.subClusters[i].release()
	}

	bal.lock.Unlock()

	// stop EPP health check and close EPP connections
	bal.closeEPP()
}

// getHashKey returns hash key according hash strategy
func (bal *BalanceGslb) getHashKey(req *bfe_basic.Request) []byte {
	var clientIP net.IP
	var hashKey []byte

	if req.ClientAddr != nil {
		clientIP = req.ClientAddr.IP
	} else {
		clientIP = nil
	}

	switch *bal.hashConf.HashStrategy {
	case cluster_conf.ClientIdOnly:
		hashKey = getHashKeyByHeader(req, *bal.hashConf.HashHeader)

	case cluster_conf.ClientIpOnly:
		hashKey = clientIP

	case cluster_conf.ClientIdPreferred:
		hashKey = getHashKeyByHeader(req, *bal.hashConf.HashHeader)
		if hashKey == nil {
			hashKey = clientIP
		}

	case cluster_conf.RequestURI:
		hashKey = []byte(req.HttpRequest.RequestURI)
	}

	// if hashKey is empty, use random value
	if len(hashKey) == 0 {
		hashKey = make([]byte, 8)
		binary.BigEndian.PutUint64(hashKey, rand.Uint64())
	}

	return hashKey
}

func getHashKeyByHeader(req *bfe_basic.Request, header string) []byte {
	if val := req.HttpRequest.Header.Get(header); len(val) > 0 {
		return []byte(val)
	}

	if cookieKey, ok := cluster_conf.GetCookieKey(header); ok {
		if cookie, ok := req.Cookie(cookieKey); ok {
			return []byte(cookie.Value)
		}
	}

	return nil
}

func (bal *BalanceGslb) BalanceEpp(req *bfe_basic.Request) (*bal_backend.BfeBackend, error) {
	var backend *bal_backend.BfeBackend
	var err error

	bal.lock.Lock()
	defer bal.lock.Unlock()

	// still in-cluster selection
	if req.RetryTime > bal.retryMax {
		// for epp only check in-cluster.
		state.ErrBkRetryTooMany.Inc(1)
		state.ErrEppFallbackLocal.Inc(1)
		recordEPPFallbackLocal(bal.name)
		// Note: not modify req.ErrCode to just record last error
		return nil, bfe_basic.ErrBkRetryTooMany
	}

	// circuit breaker: too many recent EPP failures, short-circuit to
	// local balance without calling EPP at all
	bal.eppMu.Lock()
	breaker := bal.eppBreaker
	bal.eppMu.Unlock()
	if breaker != nil && !breaker.allow() {
		state.ErrEppFallbackLocal.Inc(1)
		recordEPPFallbackLocal(bal.name)
		return nil, fmt.Errorf("epp breaker open")
	}

	// If BalanceMode == EPP, try to get backend from EPP service first
	addrinfo, eppClient, err := bal.chooseBackendFromEPP(req)
	if breaker != nil {
		breaker.record(err == nil)
	}
	if err != nil {
		state.ErrEppFallbackLocal.Inc(1)
		recordEPPFallbackLocal(bal.name)
		return nil, fmt.Errorf("EPP no decision: %v", err)
	}
	if addrinfo != "" {
		req.SetContext(REQ_CTX_EPP, eppClient)
		// try to find backend in subclusters
		for _, sub := range bal.subClusters {
			if sub == nil {
				continue
			}
			bk, berr := sub.backends.LookUpBackend(addrinfo)
			if berr == nil && bk != nil {
				req.Backend.SubclusterName = sub.Name
				return bk, nil
			}
		}
		// not found: log and make a temporary backend
		log.Logger.Info("EPP returned addr %s not found in local backends", addrinfo)
		backend = bal_backend.NewBfeBackendByAddrinfo("EPP_temp", addrinfo, addrinfo)
		return backend, nil
	}
	return nil, fmt.Errorf("EPP no decision")
}

// Balance selects a backend for given request.
func (bal *BalanceGslb) Balance(req *bfe_basic.Request) (*bal_backend.BfeBackend, error) {
	var backend *bal_backend.BfeBackend
	var current *SubCluster
	var err error
	var balAlgor int

	bal.lock.Lock()
	defer bal.lock.Unlock()

	if req.RetryTime > (bal.retryMax + bal.crossRetry) {
		// both in-cluster and cross-cluster retry failed.
		state.ErrBkRetryTooMany.Inc(1)
		// Note: req.ErrCode is not modified to ErrBkRetryTooMany, to record last error msg
		return nil, bfe_basic.ErrBkRetryTooMany
	}

	// if there is backend info in req's context, bfe will try to lookup backend info;
	if bal.isContextWithBackend(req) {
		backend, _, err := bal.LookupStickyBackend(req)
		if err == nil {
			return backend, err
		}
	}

	// select balance mode
	switch bal.BalanceMode {
	case cluster_conf.BalanceModeWlc:
		balAlgor = bal_slb.WlcSmooth
	default:
		balAlgor = bal_slb.WrrSmooth
	}

	// If use sticky session feature, bfe bind a user's session to a specific backend.
	// All requests from the user during the session are sent to the same backend.
	if *bal.hashConf.SessionSticky {
		balAlgor = bal_slb.WrrSticky
	}

	hashKey := bal.getHashKey(req)

	// subCluster-level balance
	current, err = bal.subClusterBalance(hashKey)
	if err != nil {
		// no sub cluster available
		state.ErrBkNoSubCluster.Inc(1)
		req.ErrCode = bfe_basic.ErrBkNoSubCluster
		return nil, bfe_basic.ErrBkNoSubCluster
	}
	req.Backend.SubclusterName = current.Name
	log.Logger.Debug("sub cluster=[%s],total_weight=[%d]",
		current.Name, bal.totalWeight)

	// after get the distribution subcluster

	// black hole
	if current.sType == TypeGslbBlackhole {
		state.ErrGslbBlackhole.Inc(1)
		req.ErrCode = bfe_basic.ErrGslbBlackhole
		return nil, bfe_basic.ErrGslbBlackhole
	}

	// still in-cluster selection
	if req.RetryTime <= bal.retryMax {
		backend, err = current.balance(balAlgor, hashKey)
		if err == nil {
			return backend, nil
		} else {
			// fail to get backend from current sub-cluster
			state.ErrBkNoBackend.Inc(1)
			log.Logger.Info("gslb.Balance():no backend(in cluster):cluster[%s], sub[%s], err[%s]",
				bal.name, current.Name, err.Error())
			req.ErrMsg = fmt.Sprintf("cluster[%s], sub[%s], err[%s]", bal.name, current.Name, err.Error())
			// Note: all backends down in current sub-cluster, may cross retry
			req.RetryTime = bal.retryMax
		}
	}

	// check if cross retry is disabled
	if bal.crossRetry <= 0 {
		req.ErrCode = bfe_basic.ErrBkNoBackend
		return nil, bfe_basic.ErrBkNoBackend
	}

	// in-cluster selection failed, select from cross-cluster
	log.Logger.Debug("start cross-cluster selection , retry = %d", req.RetryTime)
	if req.Stat != nil {
		req.Stat.IsCrossCluster = true
	}

	current, err = bal.randomSelectExclude(current)
	if err != nil {
		state.ErrBkNoSubClusterCross.Inc(1)
		req.ErrCode = bfe_basic.ErrBkNoSubClusterCross
		return nil, bfe_basic.ErrBkNoSubClusterCross
	}
	req.Backend.SubclusterName = current.Name

	backend, err = current.balance(balAlgor, hashKey)
	if err == nil {
		return backend, nil
	}

	// fail to get backend from current sub-cluster
	state.ErrBkNoBackend.Inc(1)
	req.ErrCode = bfe_basic.ErrBkNoBackend
	req.ErrMsg = fmt.Sprintf("cluster[%s], sub[%s], err[%s]", bal.name, current.Name, err.Error())
	log.Logger.Info("gslb.Balance():no backend(cross cluster):cluster[%s], sub[%s], err[%s]",
		bal.name, current.Name, err.Error())

	return backend, bfe_basic.ErrBkCrossRetryBalance
}

// subClusterBalance selects one sub cluster.
func (bal *BalanceGslb) subClusterBalance(value []byte) (*SubCluster, error) {
	var subCluster *SubCluster
	var w int

	if bal == nil {
		return subCluster, fmt.Errorf("gslb is nil")
	}

	if bal.totalWeight == 0 {
		return subCluster, fmt.Errorf("totalWeight is 0")
	}

	if bal.single {
		return bal.subClusters[bal.avail], nil
	}

	w = bal_slb.GetHash(value, uint(bal.totalWeight))

	for i := 0; i < len(bal.subClusters); i++ {
		subCluster = bal.subClusters[i]
		if subCluster.weight <= 0 {
			continue
		}
		w -= subCluster.weight
		// got it
		if w < 0 {
			break
		}
	}

	return subCluster, nil
}

// randomSelectExclude randomly selects a sub cluster, exclude exclude_sub_cluster, gslb blackhole.
func (bal *BalanceGslb) randomSelectExclude(excludeCluster *SubCluster) (*SubCluster, error) {
	var i int
	var subCluster *SubCluster

	available := 0

	for i = 0; i < len(bal.subClusters); i++ {
		subCluster = bal.subClusters[i]
		if subCluster != excludeCluster && subCluster.weight >= 0 &&
			subCluster.sType != TypeGslbBlackhole {
			available++
		}
	}

	if available == 0 {
		return subCluster, fmt.Errorf("no sub cluster available")
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	n := int(r.Int31()) % available

	for i = 0; i < len(bal.subClusters); i++ {
		subCluster = bal.subClusters[i]
		if subCluster != excludeCluster && subCluster.weight >= 0 &&
			subCluster.sType != TypeGslbBlackhole {
			if n == 0 {
				return subCluster, nil
			} else {
				n--
			}
		}
	}

	// never reach here
	return subCluster, fmt.Errorf("randomSelectExclude():should not reach here")
}

func (bal *BalanceGslb) isContextWithBackend(req *bfe_basic.Request) bool {
	_, ok := req.Context[bfe_basic.SessionStickyBackendKey]
	return ok
}

// lookup session sticky backend from request's context
func (bal *BalanceGslb) LookupStickyBackend(req *bfe_basic.Request) (*bal_backend.BfeBackend, *SubCluster, error) {
	if _, ok := req.Context[bfe_basic.SessionStickyBackendKey]; !ok {
		return nil, nil, fmt.Errorf("no SessionStickyBackendKey in contexts")

	}
	val, matched := req.Context[bfe_basic.SessionStickyBackendKey].(*bfe_basic.SessionStickyBackend)
	if !matched {
		return nil, nil, fmt.Errorf("Context type unmatched: %s", bal.BalanceMode)
	}
	var subCluster *SubCluster
	for _, s := range bal.subClusters {
		if s.Name == *val.SubCluster && s.weight > 0 {
			subCluster = s
			break
		}
	}
	if subCluster == nil {
		return nil, nil, fmt.Errorf("no subCluster, want: %s", *val.SubCluster)
	}

	addrInfo := fmt.Sprintf("%s:%d", *val.Addr, *val.Port)

	backend, err := subCluster.backends.LookUpBackend(addrInfo)
	if err != nil {
		return backend, subCluster, err
	}

	req.Backend.SubclusterName = subCluster.Name

	return backend, subCluster, nil

}

func (bal *BalanceGslb) SubClusterNum() int {
	return len(bal.subClusters)
}

type BalErrState struct {
	ErrBkNoSubCluster      *metrics.Counter
	ErrBkNoSubClusterCross *metrics.Counter
	ErrBkNoBackend         *metrics.Counter
	ErrBkRetryTooMany      *metrics.Counter
	ErrGslbBlackhole       *metrics.Counter

	// EPP related (flat counters; labeled cluster-dimension metrics
	// are exposed via /monitor/epp_metrics, see epp_metrics.go)
	ErrEppFailover        *metrics.Counter // EPP failover to backup address
	ErrEppFailback        *metrics.Counter // EPP failback to higher priority address
	ErrEppFallbackLocal   *metrics.Counter // EPP failure, fallback to local balance
	ErrEppBreakerOpen     *metrics.Counter // breaker transitioned to open
	ErrEppBreakerHalfOpen *metrics.Counter // breaker transitioned to half-open
	ErrEppBreakerClosed   *metrics.Counter // breaker transitioned to closed
}

var state BalErrState

func GetBalErrState() *BalErrState {
	return &state
}

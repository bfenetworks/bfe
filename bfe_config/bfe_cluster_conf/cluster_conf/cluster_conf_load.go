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

// load cluster conf from json file

package cluster_conf

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bfenetworks/go-lib/log"
	"github.com/bfenetworks/go-lib/quota"

	"github.com/bfenetworks/bfe/bfe_tls"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

// RetryLevels
const (
	RetryConnect = 0 // retry if connect backend fail
	RetryGet     = 1 // retry if forward GET request fail (plus RetryConnect)
)

// DefaultTimeout
const (
	DefaultReadClientTimeout      = 30000
	DefaultWriteClientTimeout     = 60000
	DefaultReadClientAgainTimeout = 60000
)

// HashStrategy for subcluster-level load balance (GSLB).
// Note:
//   - CLIENTID is a special request header which represents a unique client,
//     eg. baidu id, passport id, device id etc.
const (
	ClientIdOnly      = iota // use CLIENTID to hash
	ClientIpOnly             // use CLIENTIP to hash
	ClientIdPreferred        // use CLIENTID to hash, otherwise use CLIENTIP
	RequestURI               // use request URI to hash
)

// BALANCE_MODE used for GslbBasicConf.
const (
	BalanceModeWrr = "WRR" // weighted round robin
	BalanceModeWlc = "WLC" // weighted least connection
	BalanceModeEPP = "EPP" // balance by epp
)

const (
	// AnyStatusCode is a special status code used in health-check.
	// If AnyStatusCode is used, any status code is accepted for health-check response.
	AnyStatusCode = 0
)

const (
	HostType_HOST        = "HOST"
	HostType_Instance_IP = "Instance_IP"
)

// BackendCheck is conf of backend check
type BackendCheck struct {
	Schem      *string // protocol for health check (HTTP/HTTPS/TCP)
	Uri        *string // uri used in health check
	Host       *string // if check request use special host header
	HostType   *string // extending the type of Host.
	StatusCode *int    // default value is 200
	// StatusCodeRange Legal configuration items:
	//	(1) One of "3xx", "4xx", "5xx"
	//	(2) Specific HTTP status codes
	//	(3) Combinations of the above (1) or (2) connected by the "|" symbol, for example: "3xx", "4xx", "5xx", "503" | "4xx", "501" | "409"
	StatusCodeRange *string
	FailNum         *int // unhealthy threshold (consecutive failures of check request)
	SuccNum         *int // healthy threshold (consecutive successes of normal request)
	CheckTimeout    *int // timeout for health check, in ms
	CheckInterval   *int // interval of health check, in ms
}

// FCGIConf are FastCGI related configurations
type FCGIConf struct {
	EnvVars map[string]string // the vars which will send to backend
	Root    string            // the server root
}

// BackendBasic is conf of backend basic
type BackendBasic struct {
	Protocol                 *string // backend protocol
	TimeoutConnSrv           *int    // timeout for connect backend, in ms
	TimeoutResponseHeader    *int    // timeout for read header from backend, in ms
	MaxIdleConnsPerHost      *int    // max idle conns for each backend
	MaxConnsPerHost          *int    // max conns for each backend (zero means unrestricted)
	RetryLevel               *int    // retry level if request fail
	SlowStartTime            *int    // time for backend increases the weight to the full value, in seconds
	OutlierDetectionHttpCode *string // outlier detection http status code
	// protocol specific configurations
	FCGIConf *FCGIConf
}

// BackendHTTPS is conf of backend https
type BackendHTTPS struct {
	// confs
	RSHost               *string   // real server hostname
	RSInsecureSkipVerify *bool     // whether to skip verify cert of real server
	RSCAList             *[]string // real server CA Cert list. nil/empty - use system ca list; not empty - completely use RSCAList and do not use system list
	BFECertFile          *string   // BFE Cert file
	BFEKeyFile           *string   // Privatekey file of BFECert

	// caches
	rsCAList *x509.CertPool       // cache RSCAList
	bfeCert  *bfe_tls.Certificate // cache BFECertFile & BFEKeyFile
	protocol string               // protocol of backend https
}

// AIKey represents a single API key for AI service
type AIKey struct {
	Name   string // identifier
	Key    string // API key value
	Weight int    // weight for weighted random selection, [0,100]
}

// AIKeyPolicy represents routing/retry policy for AI keys
type AIKeyPolicy struct {
	Strategy            string // "weighted_random" only in this version
	MaxRetries          int    // total retry budget within one aiClusterInvoke call
	RetryBackoffInitial int    // ms
	RetryBackoffMax     int    // ms

	// Session-level API-Key affinity based on Redis + ClientKeyId
	SessionAffinity              bool   // default false
	SessionAffinityTTL           int    // Redis binding TTL in seconds, default 300
	SessionAffinityRedisPrefix   string // Redis key prefix, default "bfe:ai:key_affinity"
	SessionAffinityPenaltyEnable bool   // skip Keys recently returned 429/401/403, default true
}

// TimeRange defines a single time range within a week for a pricing tier.
// Weekdays uses Go's weekday convention: 0=Sunday, 1=Monday, ..., 6=Saturday.
// An empty Weekdays slice means the range applies every day.
type TimeRange struct {
	Weekdays []int  // 0=Sunday, 1=Monday ... 6=Saturday; empty means every day
	Start    string // "HH:MM"
	End      string // "HH:MM", must be > Start; cross-midnight ranges should be split
}

// PriceTier defines a named pricing tier (e.g., "peak") with its time ranges.
type PriceTier struct {
	Name       string      // tier name; initially only "peak" is supported
	TimeRanges []TimeRange // hit any range means the request belongs to this tier
}

// PriceMap is a map of price keys to their numeric values (yuan per unit).
// Prices may use more than 8 decimal places and are serialized with the
// default JSON encoder, which may emit scientific notation (e.g. 1.5e-6).
type PriceMap map[string]float64

// TierPriceMap is a map of tier names to PriceMap values.
type TierPriceMap map[string]map[string]float64

// ModelPrice represents a single model pricing entry in AIConf.ModelTable
type ModelPrice struct {
	Provider            string
	Model               string
	BaseModel           string
	Mode                string
	Capabilities        []string
	SupportedParameters []string
	Limits              map[string]interface{}
	Prices              PriceMap     // default prices (yuan per unit, float64)
	TierPrices          TierPriceMap // tier name -> price table
	Metadata            map[string]interface{}
}

// ModelTable represents the cost/pricing table for a cluster
type ModelTable struct {
	Currency string // fixed "RMB" in v0.4
	TimeZone string // default "Asia/Shanghai"
	Tiers    []PriceTier
	Models   []ModelPrice

	// priceIndex is built at config load time: model -> mode -> *ModelPrice
	priceIndex map[string]map[string]*ModelPrice
	// tierIndex is built at config load time: tier name -> *PriceTier
	tierIndex map[string]*PriceTier
	// tz is parsed from TimeZone at config load time.
	tz *time.Location
}

type AIConf struct {
	Type         int                // type of LLM service, reserved for future use. should be 0 now.
	ModelMapping *map[string]string // model mapping, key is model name in req, value is model name in backend
	Provider     string             // provider name in model_prices
	Keys         []AIKey            // multiple API keys; empty means no key injection
	KeyPolicy    *AIKeyPolicy       // key selection & retry policy
	ModelTable   *ModelTable        // pricing table, auto-filled by InnerAPI

	// MatchPrefix defines the provider/model prefix this cluster matches.
	// Must end with '/' to avoid matching model names themselves.
	MatchPrefix string `json:"MatchPrefix,omitempty"`
	// StripPrefix controls whether to strip MatchPrefix from the request model
	// field before forwarding to the backend.
	StripPrefix bool `json:"StripPrefix"`

	// ModelProtocols lists the model access protocols supported by the cluster's provider.
	// It comes from ai-gateway-api provider.model_protocols, e.g. ["openai"], ["anthropic"],
	// ["openai", "anthropic"]. Empty defaults to ["openai"] for backward compatibility.
	ModelProtocols []string
}

const (
	PriceInputCostPerToken           = "input_cost_per_token"
	PriceOutputCostPerToken          = "output_cost_per_token"
	PriceCacheReadInputTokenCost     = "cache_read_input_token_cost"
	PriceCacheCreationInputTokenCost = "cache_creation_input_token_cost"
	PriceInputCostPerAudioToken      = "input_cost_per_audio_token"
	PriceOutputCostPerAudioToken     = "output_cost_per_audio_token"
	PriceOutputCostPerImage          = "output_cost_per_image"
	PriceInputCostPerImageToken      = "input_cost_per_image_token"
	PriceOutputCostPerVideo          = "output_cost_per_video"
)

func (conf *BackendHTTPS) GetProtocol() string {
	return conf.protocol
}

// GetRSCAList : cache the RSCAList in memory,
func (conf *BackendHTTPS) GetRSCAList() (*x509.CertPool, error) {
	return conf.rsCAList, nil
}

// SetRSCAList : just for unit test
func (conf *BackendHTTPS) SetRSCAList(cal *x509.CertPool) {
	conf.rsCAList = cal
}

// GetBFECert : cache the cert in memory
func (conf *BackendHTTPS) GetBFECert() (bfe_tls.Certificate, error) {
	if conf.bfeCert == nil {
		return bfe_tls.Certificate{}, errors.New("BFECert not found.")
	}
	return *conf.bfeCert, nil
}

// SetBFECert : just for unit test
func (conf *BackendHTTPS) SetBFECert(cert *bfe_tls.Certificate) {
	conf.bfeCert = cert
}

// CheckBFECertAndKey : check the BFECertFile and BFEKeyFile
func (conf *BackendHTTPS) CheckBFECertAndKey() error {
	var (
		certPem, keyPem []byte
		cert            bfe_tls.Certificate
		err             error = nil
	)
	conf.bfeCert = nil
	if conf.BFECertFile != nil && *conf.BFECertFile != "" {
		if certPem, err = os.ReadFile(*conf.BFECertFile); err != nil {
			return err
		}
	}
	if conf.BFEKeyFile != nil && *conf.BFEKeyFile != "" {
		if keyPem, err = os.ReadFile(*conf.BFEKeyFile); err != nil {
			return err
		}
	}
	if certPem == nil || keyPem == nil {
		return nil
	} else if cert, err = bfe_tls.X509KeyPair(certPem, keyPem); err != nil {
		return err
	}
	conf.bfeCert = &cert
	return nil
}

type HashConf struct {
	// HashStrategy is hash strategy for subcluster-level load balance.
	// ClientIdOnly, ClientIpOnly, ClientIdPreferred, RequestURI.
	HashStrategy *int

	// HashHeader is an optional request header which represents a unique client.
	// format for speicial cookie header is "Cookie:Key".
	// eg, Dueros-Device-Id, Cookie:BAIDUID, Cookie:PASSPORTID, etc
	HashHeader *string

	// SessionSticky enable sticky session (ensures that all requests
	// from the user during the session are sent to the same backend)
	SessionSticky *bool
}

// Default values for EPP related conf (see GslbBasicConf).
const (
	DefaultEPPCheckInterval           = "2s"
	DefaultEPPFailThreshold           = 3
	DefaultEPPCooldown                = "45s"
	DefaultEPPSuccessThreshold        = 2
	DefaultEPPConnectTimeout          = "500ms"
	DefaultEPPCallTimeout             = "3s"
	DefaultEPPBreakerWindowSize       = 100
	DefaultEPPBreakerMinVolume        = 20
	DefaultEPPBreakerErrorRatePercent = 50
	DefaultEPPBreakerOpenTimeout      = "30s"
)

// Parsed forms of default EPP conf values.
func DefaultEPPCheckIntervalValue() time.Duration {
	d, _ := time.ParseDuration(DefaultEPPCheckInterval)
	return d
}

func DefaultEPPCooldownValue() time.Duration {
	d, _ := time.ParseDuration(DefaultEPPCooldown)
	return d
}

func DefaultEPPConnectTimeoutValue() time.Duration {
	d, _ := time.ParseDuration(DefaultEPPConnectTimeout)
	return d
}

func DefaultEPPCallTimeoutValue() time.Duration {
	d, _ := time.ParseDuration(DefaultEPPCallTimeout)
	return d
}

func DefaultEPPBreakerOpenTimeoutValue() time.Duration {
	d, _ := time.ParseDuration(DefaultEPPBreakerOpenTimeout)
	return d
}

// EPPCheckConf is health check and failover hysteresis conf for EPP,
// only effective when BalanceMode is EPP.
type EPPCheckConf struct {
	Disabled bool // disable background health check (not recommended in production)

	CheckInterval    *string // duration string, health check probe interval
	FailThreshold    *int    // consecutive probe fails of active addr before failover
	Cooldown         *string // duration string, no failback to a failed addr within cooldown
	SuccessThreshold *int    // consecutive probe successes before failback to higher priority addr
}

// CheckIntervalDuration returns probe interval, default if unset or unparsable.
func (c *EPPCheckConf) CheckIntervalDuration() time.Duration {
	return parseEPPDuration(c.CheckInterval, DefaultEPPCheckInterval)
}

// CooldownDuration returns failover cooldown, default if unset or unparsable.
func (c *EPPCheckConf) CooldownDuration() time.Duration {
	return parseEPPDuration(c.Cooldown, DefaultEPPCooldown)
}

// EPPTimeoutConf is timeout conf for EPP calls, only effective when BalanceMode is EPP.
type EPPTimeoutConf struct {
	Connect *string // duration string, timeout for establishing gRPC connection/stream
	Call    *string // duration string, timeout for first message (RequestHeaders Send+Recv)
}

// ConnectDuration returns connect timeout, default if unset or unparsable.
func (c *EPPTimeoutConf) ConnectDuration() time.Duration {
	return parseEPPDuration(c.Connect, DefaultEPPConnectTimeout)
}

// CallDuration returns first-message call timeout, default if unset or unparsable.
func (c *EPPTimeoutConf) CallDuration() time.Duration {
	return parseEPPDuration(c.Call, DefaultEPPCallTimeout)
}

// EPPTLSConf is transport security conf for EPP connections,
// only effective when BalanceMode is EPP.
type EPPTLSConf struct {
	Insecure bool   // true = skip certificate verification (testing only)
	CAFile   string // CA certificate file for verifying EPP server, required if Insecure is false
}

// EPPBreakerConf is circuit breaker conf of the EPP path, only effective
// when BalanceMode is EPP. Breaker stops going to EPP entirely (falling back
// to local balance) when the error rate of recent calls is too high,
// complementing address-level failover.
type EPPBreakerConf struct {
	Disabled bool // true = disable circuit breaker

	WindowSize       *int    // sliding window size (recent call results), default 100
	MinVolume        *int    // min calls in window before evaluating, default 20
	ErrorRatePercent *int    // error rate threshold (percent), default 50
	OpenTimeout      *string // duration string, OPEN duration before HALF-OPEN probing, default "30s"
}

// OpenTimeoutDuration returns open timeout, default if unset or unparsable.
func (c *EPPBreakerConf) OpenTimeoutDuration() time.Duration {
	return parseEPPDuration(c.OpenTimeout, DefaultEPPBreakerOpenTimeout)
}

func parseEPPDuration(v *string, def string) time.Duration {
	s := def
	if v != nil && *v != "" {
		s = *v
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		d, _ = time.ParseDuration(def)
	}
	return d
}

// GslbBasicConf is basic conf for Gslb
type GslbBasicConf struct {
	CrossRetry *int // retry cross sub clusters
	RetryMax   *int // inner cluster retry
	HashConf   *HashConf

	BalanceMode *string         // balanceMode, default WRR
	EPPAddr     *[]string       // EPP addresses, ordered primary/backup, effective when BalanceMode is EPP
	EPPCheck    *EPPCheckConf   // EPP health check and failover hysteresis
	EPPTimeout  *EPPTimeoutConf // EPP call timeouts
	EPPTLS      *EPPTLSConf     // EPP transport security
	EPPBreaker  *EPPBreakerConf // EPP circuit breaker
}

// ClusterBasicConf is basic conf for cluster.
type ClusterBasicConf struct {
	TimeoutReadClient      *int // timeout for read client body in ms
	TimeoutWriteClient     *int // timeout for write response to client
	TimeoutReadClientAgain *int // timeout for read client again in ms

	ReqWriteBufferSize  *int  // write buffer size for request in byte
	ReqFlushInterval    *int  // interval to flush request in ms. if zero, disable periodic flush
	ResFlushInterval    *int  // interval to flush response in ms. if zero, disable periodic flush
	CancelOnClientClose *bool // cancel blocking operation on server if client connection disconnected

	DisableHostHeader  *bool // disable host header when forward to backend
	DisableHealthCheck *bool // disable health check for backend
}

// ClusterConf is conf of cluster.
type ClusterConf struct {
	BackendConf  *BackendBasic     // backend's basic conf
	CheckConf    *BackendCheck     // how to check backend
	GslbBasic    *GslbBasicConf    // gslb basic conf for cluster
	ClusterBasic *ClusterBasicConf // basic conf for cluster
	HTTPSConf    *BackendHTTPS     // backend's https conf
	AIConf       *AIConf           // ai conf for cluster
}

type ClusterToConf map[string]ClusterConf

// BfeClusterConf is conf of all bfe cluster.
type BfeClusterConf struct {
	Version *string // version of config
	Config  *ClusterToConf
}

// BackendHTTPS is https conf of backend.
func BackendHTTPSCheck(protocol *string, conf *BackendHTTPS) error {
	if protocol == nil || *protocol != "https" {
		return nil
	}
	conf.protocol = *protocol
	if conf.RSInsecureSkipVerify == nil || !*conf.RSInsecureSkipVerify {
		if conf.RSCAList != nil && len(*conf.RSCAList) > 0 {
			rootCAs := x509.NewCertPool()
			for _, caFp := range *conf.RSCAList {
				chain, err := os.ReadFile(caFp)
				log.Logger.Debug("load: ca_fp=%s, err=%v", caFp, err)
				if err != nil {
					return err
				}
				var certs []*x509.Certificate
				block, rest := pem.Decode(chain)
				for block != nil {
					if block.Type == "CERTIFICATE" {
						cert, err := x509.ParseCertificate(block.Bytes)
						if err != nil {
							return err
						}
						certs = append(certs, cert)
					}
					if len(rest) > 0 {
						block, rest = pem.Decode(rest)
					} else {
						break
					}
				}
				for _, crt := range certs {
					rootCAs.AddCert(crt)
				}
			}
			conf.rsCAList = rootCAs
		}
	}
	return conf.CheckBFECertAndKey()
}

// BackendBasicCheck check BackendBasic config.
func BackendBasicCheck(conf *BackendBasic) error {
	if conf.Protocol == nil {
		defaultProtocol := "http"
		conf.Protocol = &defaultProtocol
	}
	*conf.Protocol = strings.ToLower(*conf.Protocol)
	switch *conf.Protocol {
	case "http", "tcp", "ws", "fcgi", "h2c", "https":
	default:
		return fmt.Errorf("protocol only support http/tcp/ws/fcgi/h2c/https, but is:%s", *conf.Protocol)
	}

	if conf.TimeoutConnSrv == nil {
		defaultTimeConnSrv := 2000
		conf.TimeoutConnSrv = &defaultTimeConnSrv
	}

	if conf.TimeoutResponseHeader == nil {
		defaultTimeoutResponseHeader := 60000
		conf.TimeoutResponseHeader = &defaultTimeoutResponseHeader
	}

	if conf.MaxIdleConnsPerHost == nil {
		defaultIdle := 2
		conf.MaxIdleConnsPerHost = &defaultIdle
	}

	if conf.MaxConnsPerHost == nil || *conf.MaxConnsPerHost < 0 {
		defaultConns := 0
		conf.MaxConnsPerHost = &defaultConns
	}

	if conf.RetryLevel == nil {
		retryLevel := RetryConnect
		conf.RetryLevel = &retryLevel
	}

	if conf.OutlierDetectionHttpCode == nil {
		outlierDetectionCode := ""
		conf.OutlierDetectionHttpCode = &outlierDetectionCode
	} else {
		httpCode := *conf.OutlierDetectionHttpCode
		httpCode = strings.ToLower(httpCode)
		conf.OutlierDetectionHttpCode = &httpCode
	}

	if conf.SlowStartTime == nil {
		defaultSlowStartTime := 0
		conf.SlowStartTime = &defaultSlowStartTime
	}

	if conf.FCGIConf == nil {
		defaultFCGIConf := new(FCGIConf)
		defaultFCGIConf.EnvVars = make(map[string]string)
		defaultFCGIConf.Root = ""
		conf.FCGIConf = defaultFCGIConf
	}

	return nil
}

// checkStatusCode checks status code
func checkStatusCode(statusCode int) error {
	// Note: meaning for status code
	//  - 100~599: for status code of that value
	//  - 0b00001: for 1xx; 0b00010: for 2xx; ... ; 0b10000: for 5xx
	//  - 0b00110: for 2xx or 3xx
	//  - 0: for any status code

	// normal status code
	if statusCode >= 100 && statusCode <= 599 {
		return nil
	}

	// special status code
	if statusCode >= 0 && statusCode <= 31 {
		return nil
	}

	return errors.New("status code should be 100~599 (normal), 0~31 (special)")
}

// convertStatusCode convert status code to string
func convertStatusCode(statusCode int) string {
	// normal status code
	if statusCode >= 100 && statusCode <= 599 {
		return fmt.Sprintf("%d", statusCode)
	}

	// any status code
	if statusCode == AnyStatusCode {
		return "ANY"
	}

	// wildcard status code
	if statusCode >= 1 && statusCode <= 31 {
		var codeStr string
		for i := 0; i < 5; i++ {
			if statusCode>>uint(i)&1 != 0 {
				codeStr += fmt.Sprintf("%dXX ", i+1)
			}
		}
		return codeStr
	}

	return fmt.Sprintf("INVALID %d", statusCode)
}

func checkStatusCodeRange(sp *string) error {
	if sp == nil || *sp == "" {
		return nil
	}
	s := *sp
	s = strings.ReplaceAll(s, " ", "")
	validPattern := "^[0-9xX|]+$"
	matched, err := regexp.MatchString(validPattern, s)
	if err != nil {
		return err
	}
	rtnErr := fmt.Errorf("StatusCodeRange format error : %s", s)
	if !matched {
		return rtnErr
	}
	parts := strings.Split(s, "|")
	for _, part := range parts {
		if len(part) != 3 { // xxx|xxx|xxx
			return rtnErr
		}
	}
	return nil

}

func MatchStatusCodeRange(statusCode string, statusCodeRange string) (bool, error) {
	if statusCodeRange == "" {
		return true, nil
	}
	statusCodeRange = strings.ReplaceAll(statusCodeRange, " ", "")
	ranges := strings.Split(statusCodeRange, "|")
	for _, rangeStr := range ranges {
		pattern := strings.ReplaceAll(rangeStr, "x", `\d`)
		pattern = fmt.Sprintf("^%s$", pattern)
		match, err := regexp.MatchString(pattern, statusCode)
		if err != nil {
			return false, err
		}
		if match {
			return true, nil
		}
	}
	return false, fmt.Errorf("not match: statusCode=%s, statusCodeRange=%s", statusCode, statusCodeRange)
}

func MatchStatusCode(statusCodeGet int, statusCodeExpect int) (bool, error) {
	// for normal status code
	if statusCodeExpect >= 100 && statusCodeExpect <= 599 {
		if statusCodeGet == statusCodeExpect {
			return true, nil
		}
	}

	// for any status code
	if statusCodeExpect == AnyStatusCode {
		return true, nil
	}

	// for wildcard status code
	if statusCodeExpect >= 1 && statusCodeExpect <= 31 {
		statusCodeWildcard := 1 << uint(statusCodeGet/100-1) // eg. 2xx is 0b00010, 3xx is 0b00100
		if statusCodeExpect&statusCodeWildcard != 0 {
			return true, nil
		}
	}

	return false, fmt.Errorf("response statusCode[%d], while expect[%s]",
		statusCodeGet, convertStatusCode(statusCodeExpect))
}

// BackendCheckCheck check BackendCheck config.
func BackendCheckCheck(conf *BackendCheck) error {
	if conf.Schem == nil {
		// set default schem to http
		schem := "http"
		conf.Schem = &schem
	} else if *conf.Schem != "http" && *conf.Schem != "https" && *conf.Schem != "tls" && *conf.Schem != "tcp" {
		return errors.New("Schem for BackendCheck should be http/https/tls/tcp")
	}

	if conf.Uri == nil {
		uri := "/health_check"
		conf.Uri = &uri
	}

	if conf.Host == nil {
		host := ""
		conf.Host = &host
	}

	if conf.HostType == nil {
		hostType := HostType_HOST
		conf.HostType = &hostType
	}

	if conf.StatusCode == nil {
		statusCode := 0
		conf.StatusCode = &statusCode
	}

	if conf.FailNum == nil {
		failNum := 5
		conf.FailNum = &failNum
	}

	if conf.CheckInterval == nil {
		checkInterval := 1000
		conf.CheckInterval = &checkInterval
	}

	if conf.SuccNum == nil {
		succNum := 1
		conf.SuccNum = &succNum
	}

	if *conf.Schem == "http" || *conf.Schem == "https" {
		if !strings.HasPrefix(*conf.Uri, "/") {
			return errors.New("Uri should be start with '/'")
		}

		if err := checkStatusCode(*conf.StatusCode); err != nil {
			return err
		}

		if err := checkStatusCodeRange(conf.StatusCodeRange); err != nil {
			return err
		}
	}

	if *conf.SuccNum < 1 {
		return errors.New("SuccNum should be bigger than 0")
	}

	return nil
}

// GslbBasicConfCheck check GslbBasicConf config.
func GslbBasicConfCheck(conf *GslbBasicConf) error {
	if conf.CrossRetry == nil {
		defaultCrossRetry := 0
		conf.CrossRetry = &defaultCrossRetry
	}

	if conf.RetryMax == nil {
		defaultRetryMax := 2
		conf.RetryMax = &defaultRetryMax
	}

	if conf.HashConf == nil {
		conf.HashConf = &HashConf{}
	}

	if conf.BalanceMode == nil {
		defaultBalMode := BalanceModeWrr
		conf.BalanceMode = &defaultBalMode
	}

	if err := HashConfCheck(conf.HashConf); err != nil {
		return err
	}

	// check balanceMode
	*conf.BalanceMode = strings.ToUpper(*conf.BalanceMode)
	switch *conf.BalanceMode {
	case BalanceModeWrr:
	case BalanceModeWlc:
	case BalanceModeEPP:
		if conf.EPPAddr == nil || len(*conf.EPPAddr) == 0 {
			return errors.New("EPPAddr is nil or empty")
		}
		if err := checkEPPAddrs(*conf.EPPAddr); err != nil {
			return err
		}
		if conf.EPPCheck == nil {
			conf.EPPCheck = &EPPCheckConf{}
		}
		if conf.EPPTimeout == nil {
			conf.EPPTimeout = &EPPTimeoutConf{}
		}
		if conf.EPPBreaker == nil {
			conf.EPPBreaker = &EPPBreakerConf{}
		}
		if err := EPPCheckConfCheck(conf.EPPCheck); err != nil {
			return err
		}
		if err := EPPTimeoutConfCheck(conf.EPPTimeout); err != nil {
			return err
		}
		if err := EPPBreakerConfCheck(conf.EPPBreaker); err != nil {
			return err
		}
		if err := EPPTLSConfCheck(conf.EPPTLS); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported bal mode %s", *conf.BalanceMode)
	}

	return nil
}

// checkEPPAddrs validates EPP address list: each element must be host:port,
// and duplicate addresses are rejected (primary/backup must be different instances).
func checkEPPAddrs(addrs []string) error {
	seen := make(map[string]bool)
	for _, addr := range addrs {
		host, port, err := net.SplitHostPort(addr)
		if err != nil || host == "" || port == "" {
			return fmt.Errorf("EPPAddr element %q is not valid host:port", addr)
		}
		if seen[addr] {
			return fmt.Errorf("EPPAddr element %q is duplicated", addr)
		}
		seen[addr] = true
	}
	return nil
}

// EPPCheckConfCheck checks EPPCheckConf, filling defaults for unset fields.
func EPPCheckConfCheck(conf *EPPCheckConf) error {
	if conf == nil {
		return nil
	}

	if conf.CheckInterval == nil {
		s := DefaultEPPCheckInterval
		conf.CheckInterval = &s
	}
	if conf.FailThreshold == nil {
		v := DefaultEPPFailThreshold
		conf.FailThreshold = &v
	}
	if conf.Cooldown == nil {
		s := DefaultEPPCooldown
		conf.Cooldown = &s
	}
	if conf.SuccessThreshold == nil {
		v := DefaultEPPSuccessThreshold
		conf.SuccessThreshold = &v
	}

	if _, err := time.ParseDuration(*conf.CheckInterval); err != nil {
		return fmt.Errorf("EPPCheck.CheckInterval %q is not valid duration: %v", *conf.CheckInterval, err)
	}
	if _, err := time.ParseDuration(*conf.Cooldown); err != nil {
		return fmt.Errorf("EPPCheck.Cooldown %q is not valid duration: %v", *conf.Cooldown, err)
	}
	checkInterval, _ := time.ParseDuration(*conf.CheckInterval)
	cooldown, _ := time.ParseDuration(*conf.Cooldown)
	if checkInterval <= 0 {
		return errors.New("EPPCheck.CheckInterval should be positive")
	}
	if *conf.FailThreshold < 1 {
		return errors.New("EPPCheck.FailThreshold should be >= 1")
	}
	if cooldown <= 0 {
		return errors.New("EPPCheck.Cooldown should be positive")
	}
	if *conf.SuccessThreshold < 1 {
		return errors.New("EPPCheck.SuccessThreshold should be >= 1")
	}

	return nil
}

// EPPTimeoutConfCheck checks EPPTimeoutConf, filling defaults for unset fields.
func EPPTimeoutConfCheck(conf *EPPTimeoutConf) error {
	if conf == nil {
		return nil
	}

	if conf.Connect == nil {
		s := DefaultEPPConnectTimeout
		conf.Connect = &s
	}
	if conf.Call == nil {
		s := DefaultEPPCallTimeout
		conf.Call = &s
	}

	if _, err := time.ParseDuration(*conf.Connect); err != nil {
		return fmt.Errorf("EPPTimeout.Connect %q is not valid duration: %v", *conf.Connect, err)
	}
	if _, err := time.ParseDuration(*conf.Call); err != nil {
		return fmt.Errorf("EPPTimeout.Call %q is not valid duration: %v", *conf.Call, err)
	}
	connect, _ := time.ParseDuration(*conf.Connect)
	call, _ := time.ParseDuration(*conf.Call)
	if connect <= 0 {
		return errors.New("EPPTimeout.Connect should be positive")
	}
	if call <= 0 {
		return errors.New("EPPTimeout.Call should be positive")
	}

	return nil
}

// EPPBreakerConfCheck checks EPPBreakerConf, filling defaults for unset fields.
func EPPBreakerConfCheck(conf *EPPBreakerConf) error {
	if conf == nil {
		return nil
	}

	if conf.WindowSize == nil {
		v := DefaultEPPBreakerWindowSize
		conf.WindowSize = &v
	}
	if conf.MinVolume == nil {
		v := DefaultEPPBreakerMinVolume
		conf.MinVolume = &v
	}
	if conf.ErrorRatePercent == nil {
		v := DefaultEPPBreakerErrorRatePercent
		conf.ErrorRatePercent = &v
	}
	if conf.OpenTimeout == nil {
		s := DefaultEPPBreakerOpenTimeout
		conf.OpenTimeout = &s
	}

	if *conf.WindowSize < 1 {
		return errors.New("EPPBreaker.WindowSize should be >= 1")
	}
	if *conf.MinVolume < 1 {
		return errors.New("EPPBreaker.MinVolume should be >= 1")
	}
	if *conf.MinVolume > *conf.WindowSize {
		return errors.New("EPPBreaker.MinVolume should not be bigger than WindowSize")
	}
	if *conf.ErrorRatePercent < 1 || *conf.ErrorRatePercent > 100 {
		return errors.New("EPPBreaker.ErrorRatePercent should be in [1, 100]")
	}
	openTimeout, err := time.ParseDuration(*conf.OpenTimeout)
	if err != nil {
		return fmt.Errorf("EPPBreaker.OpenTimeout %q is not valid duration: %v", *conf.OpenTimeout, err)
	}
	if openTimeout <= 0 {
		return errors.New("EPPBreaker.OpenTimeout should be positive")
	}

	return nil
}

// EPPTLSConfCheck checks EPPTLSConf.
func EPPTLSConfCheck(conf *EPPTLSConf) error {
	if conf == nil {
		// Compat: existing deployments upgraded to this version do not configure
		// EPPTLS at all. Rejecting them at load time would break rolling upgrades,
		// so a nil EPPTLS keeps the legacy behavior (skip certificate verification)
		// and only logs a warning to prompt migration. Certificate verification is
		// enabled only when EPPTLS is explicitly configured.
		log.Logger.Warn("EPPTLS not configured, EPP connections skip certificate verification (legacy behavior), please configure EPPTLS")
		return nil
	}

	if !conf.Insecure && conf.CAFile == "" {
		return errors.New("EPPTLS.CAFile is required when Insecure is false")
	}
	if !conf.Insecure {
		if _, err := os.Stat(conf.CAFile); err != nil {
			return fmt.Errorf("EPPTLS.CAFile %q is not readable: %v", conf.CAFile, err)
		}
	}

	return nil
}

// HashConfCheck check HashConf config.
func HashConfCheck(conf *HashConf) error {
	if conf.HashStrategy == nil {
		defaultStrategy := ClientIpOnly
		conf.HashStrategy = &defaultStrategy
	}

	if conf.SessionSticky == nil {
		defaultSessionSticky := false
		conf.SessionSticky = &defaultSessionSticky
	}

	if *conf.HashStrategy != ClientIdOnly &&
		*conf.HashStrategy != ClientIpOnly &&
		*conf.HashStrategy != ClientIdPreferred &&
		*conf.HashStrategy != RequestURI {
		return fmt.Errorf("HashStrategy[%d] must be [%d], [%d], [%d] or [%d]",
			*conf.HashStrategy, ClientIdOnly, ClientIpOnly, ClientIdPreferred, RequestURI)
	}
	if *conf.HashStrategy == ClientIdOnly || *conf.HashStrategy == ClientIdPreferred {
		if conf.HashHeader == nil || len(*conf.HashHeader) == 0 {
			return errors.New("no HashHeader")
		}
		if cookieKey, ok := GetCookieKey(*conf.HashHeader); ok && len(cookieKey) == 0 {
			return errors.New("invalid HashHeader")
		}
	}

	return nil
}

// ClusterBasicConfCheck check ClusterBasicConf.
func ClusterBasicConfCheck(conf *ClusterBasicConf) error {
	if conf.TimeoutReadClient == nil {
		timeoutReadClient := DefaultReadClientTimeout
		conf.TimeoutReadClient = &timeoutReadClient
	}

	if conf.TimeoutWriteClient == nil {
		timeoutWriteClient := DefaultWriteClientTimeout
		conf.TimeoutWriteClient = &timeoutWriteClient
	}

	if conf.TimeoutReadClientAgain == nil {
		timeoutReadClientAgain := DefaultReadClientAgainTimeout
		conf.TimeoutReadClientAgain = &timeoutReadClientAgain
	}

	if conf.ReqWriteBufferSize == nil {
		reqWriteBufferSize := 512
		conf.ReqWriteBufferSize = &reqWriteBufferSize
	}

	if conf.ReqFlushInterval == nil {
		reqFlushInterval := 0
		conf.ReqFlushInterval = &reqFlushInterval
	}

	if conf.ResFlushInterval == nil {
		resFlushInterval := -1
		conf.ResFlushInterval = &resFlushInterval
	}

	if conf.CancelOnClientClose == nil {
		cancelOnClientClose := false
		conf.CancelOnClientClose = &cancelOnClientClose
	}

	if conf.DisableHostHeader == nil {
		disableHostHeader := false
		conf.DisableHostHeader = &disableHostHeader
	}

	if conf.DisableHealthCheck == nil {
		disableHealthCheck := false
		conf.DisableHealthCheck = &disableHealthCheck
	}

	return nil
}

// ClusterConfCheck check ClusterConf.
func ClusterConfCheck(conf *ClusterConf) error {
	var err error

	// check BackendConf
	if conf.BackendConf == nil {
		conf.BackendConf = &BackendBasic{}
	}
	err = BackendBasicCheck(conf.BackendConf)
	if err != nil {
		return fmt.Errorf("BackendConf:%s", err.Error())
	}

	// check CheckConf
	if conf.CheckConf == nil {
		conf.CheckConf = &BackendCheck{}
	}
	err = BackendCheckCheck(conf.CheckConf)
	if err != nil {
		return fmt.Errorf("CheckConf:%s", err.Error())
	}

	// check GslbBasic
	if conf.GslbBasic == nil {
		conf.GslbBasic = &GslbBasicConf{}
	}
	err = GslbBasicConfCheck(conf.GslbBasic)
	if err != nil {
		return fmt.Errorf("GslbBasic:%s", err.Error())
	}

	// check ClusterBasic
	if conf.ClusterBasic == nil {
		conf.ClusterBasic = &ClusterBasicConf{}
	}
	err = ClusterBasicConfCheck(conf.ClusterBasic)
	if err != nil {
		return fmt.Errorf("ClusterBasic:%s", err.Error())
	}

	// check AIConf
	if conf.AIConf != nil {
		err = AIConfCheck(conf.AIConf)
		if err != nil {
			return fmt.Errorf("AIConf:%s", err.Error())
		}
	}

	return nil
}

// AIConfCheck checks AIConf config.
func AIConfCheck(conf *AIConf) error {
	if conf.ModelTable != nil {
		if err := ModelTableCheck(conf.ModelTable); err != nil {
			return fmt.Errorf("ModelTable:%s", err.Error())
		}
	}

	if conf.StripPrefix {
		if conf.MatchPrefix == "" {
			return fmt.Errorf("MatchPrefix is required when StripPrefix is true")
		}
		if !strings.HasSuffix(conf.MatchPrefix, "/") {
			return fmt.Errorf("MatchPrefix must end with '/'")
		}
	}

	if conf.KeyPolicy != nil {
		if err := AIKeyPolicyCheck(conf.KeyPolicy); err != nil {
			return fmt.Errorf("KeyPolicy:%s", err.Error())
		}
	}

	return nil
}

// AIKeyPolicyCheck checks and fills defaults for AIKeyPolicy.
func AIKeyPolicyCheck(policy *AIKeyPolicy) error {
	if policy.SessionAffinityTTL < 0 {
		return fmt.Errorf("SessionAffinityTTL must be > 0")
	}
	if policy.SessionAffinityTTL == 0 {
		policy.SessionAffinityTTL = 600
	}
	if policy.SessionAffinityRedisPrefix == "" {
		policy.SessionAffinityRedisPrefix = "bfe:ai:key_affinity"
	}
	if policy.Strategy == "" {
		policy.Strategy = "weighted_random"
	}
	if policy.RetryBackoffInitial < 0 {
		return fmt.Errorf("RetryBackoffInitial must be >= 0")
	}
	if policy.RetryBackoffMax < 0 {
		return fmt.Errorf("RetryBackoffMax must be >= 0")
	}
	return nil
}

// containsInt reports whether vals contains target.
func containsInt(vals []int, target int) bool {
	for _, v := range vals {
		if v == target {
			return true
		}
	}
	return false
}

// minutesFromHHMM parses "HH:MM" into minutes since midnight.
func minutesFromHHMM(s string) (int, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time format %s, expected HH:MM", s)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid hour in %s", s)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid minute in %s", s)
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("time %s out of range", s)
	}
	return hour*60 + minute, nil
}

// timeRangesOverlap reports whether two time ranges overlap.
// Both ranges are within the same day and use half-open intervals [start, end).
func timeRangesOverlap(a, b TimeRange) bool {
	if len(a.Weekdays) > 0 && len(b.Weekdays) > 0 {
		sharedWeekday := false
		for _, wd := range a.Weekdays {
			if containsInt(b.Weekdays, wd) {
				sharedWeekday = true
				break
			}
		}
		if !sharedWeekday {
			return false
		}
	}
	aStart, _ := minutesFromHHMM(a.Start)
	aEnd, _ := minutesFromHHMM(a.End)
	bStart, _ := minutesFromHHMM(b.Start)
	bEnd, _ := minutesFromHHMM(b.End)
	return aStart < bEnd && bStart < aEnd
}

// validateTimeRanges checks a list of time ranges for a single tier.
func validateTimeRanges(tierName string, ranges []TimeRange) error {
	if len(ranges) == 0 {
		return fmt.Errorf("tier %s has no time_ranges", tierName)
	}
	for i, tr := range ranges {
		for _, wd := range tr.Weekdays {
			if wd < 0 || wd > 6 {
				return fmt.Errorf("tier %s time_ranges[%d].weekdays contains invalid weekday %d", tierName, i, wd)
			}
		}
		start, err := minutesFromHHMM(tr.Start)
		if err != nil {
			return fmt.Errorf("tier %s time_ranges[%d].start: %v", tierName, i, err)
		}
		end, err := minutesFromHHMM(tr.End)
		if err != nil {
			return fmt.Errorf("tier %s time_ranges[%d].end: %v", tierName, i, err)
		}
		if end <= start {
			return fmt.Errorf("tier %s time_ranges[%d].end must be greater than start", tierName, i)
		}
		for j := 0; j < i; j++ {
			if timeRangesOverlap(ranges[i], ranges[j]) {
				return fmt.Errorf("tier %s time_ranges[%d] overlaps with time_ranges[%d]", tierName, i, j)
			}
		}
	}
	return nil
}

// ModelTableCheck checks and initializes ModelTable.
// It converts float prices to fixed-point integers and builds priceIndex.
func ModelTableCheck(table *ModelTable) error {
	if table == nil {
		return nil
	}

	if table.Currency != quota.UnitRMB {
		return fmt.Errorf("currency must be %s", quota.UnitRMB)
	}

	if table.TimeZone == "" {
		table.TimeZone = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(table.TimeZone)
	if err != nil {
		return fmt.Errorf("invalid TimeZone %s: %v", table.TimeZone, err)
	}
	table.tz = loc

	table.tierIndex = make(map[string]*PriceTier)
	for i := range table.Tiers {
		tier := &table.Tiers[i]
		if tier.Name == "" {
			return errors.New("tier name is empty")
		}
		if tier.Name != "peak" {
			return fmt.Errorf("unsupported tier name %s, only 'peak' is allowed", tier.Name)
		}
		if err := validateTimeRanges(tier.Name, tier.TimeRanges); err != nil {
			return err
		}
		table.tierIndex[tier.Name] = tier
	}

	table.priceIndex = make(map[string]map[string]*ModelPrice)

	for i := range table.Models {
		price := &table.Models[i]

		if price.Model == "" {
			return errors.New("model is empty")
		}
		if price.Mode == "" {
			return errors.New("mode is empty")
		}

		input := price.Prices[PriceInputCostPerToken]
		output := price.Prices[PriceOutputCostPerToken]
		cacheRead := price.Prices[PriceCacheReadInputTokenCost]
		cacheWrite := price.Prices[PriceCacheCreationInputTokenCost]
		audioInput := price.Prices[PriceInputCostPerAudioToken]
		audioOutput := price.Prices[PriceOutputCostPerAudioToken]
		outputCostPerImage := price.Prices[PriceOutputCostPerImage]
		inputImageToken := price.Prices[PriceInputCostPerImageToken]
		outputCostPerVideo := price.Prices[PriceOutputCostPerVideo]
		if input < 0 || output < 0 || cacheRead < 0 || cacheWrite < 0 ||
			audioInput < 0 || audioOutput < 0 || outputCostPerImage < 0 ||
			inputImageToken < 0 || outputCostPerVideo < 0 {
			return fmt.Errorf("negative price for model %s", price.Model)
		}

		for tierName, tierPriceMap := range price.TierPrices {
			if tierName != "peak" {
				return fmt.Errorf("unsupported tier name %s in TierPrices for model %s, only 'peak' is allowed", tierName, price.Model)
			}
			for key, val := range tierPriceMap {
				if val < 0 {
					return fmt.Errorf("negative tier price %s for model %s tier %s", key, price.Model, tierName)
				}
			}
		}

		if table.priceIndex[price.Model] == nil {
			table.priceIndex[price.Model] = make(map[string]*ModelPrice)
		}
		if table.priceIndex[price.Model][price.Mode] != nil {
			return fmt.Errorf("duplicate model %s mode %s", price.Model, price.Mode)
		}
		table.priceIndex[price.Model][price.Mode] = price
	}

	return nil
}

// ActiveTierName returns the name of the tier active at the given time.
// If no tier matches, it returns an empty string (fallback to default prices).
func (table *ModelTable) ActiveTierName(now time.Time) string {
	if table == nil || len(table.Tiers) == 0 || table.tz == nil {
		return ""
	}
	t := now.In(table.tz)
	wd := int(t.Weekday())
	cur := t.Hour()*60 + t.Minute()

	for i := range table.Tiers {
		tier := &table.Tiers[i]
		for _, tr := range tier.TimeRanges {
			if len(tr.Weekdays) > 0 && !containsInt(tr.Weekdays, wd) {
				continue
			}
			start, _ := minutesFromHHMM(tr.Start)
			end, _ := minutesFromHHMM(tr.End)
			if start <= cur && cur < end {
				return tier.Name
			}
		}
	}
	return ""
}

// GetPrice returns the float64 price (yuan per unit) for the given tier and key.
// If tier is empty or the tier/key is not configured, it falls back to default Prices.
// Prices stay float64 until a billing item is converted via quota.CalcCostUnits
// at request time, so prices with more than 8 decimal places keep their precision.
func (p *ModelPrice) GetPrice(tier, key string) float64 {
	if tier != "" && p.TierPrices != nil {
		if tierMap, ok := p.TierPrices[tier]; ok {
			if v, ok := tierMap[key]; ok {
				return v
			}
		}
	}
	return p.Prices[key]
}

// LookupModelPrice looks up a model price entry by model and mode.
// It returns nil if not found.
func LookupModelPrice(table *ModelTable, model, mode string) *ModelPrice {
	if table == nil || table.priceIndex == nil {
		return nil
	}
	idx, ok := table.priceIndex[model]
	if !ok {
		return nil
	}
	return idx[mode]
}

// ClusterToConfCheck check ClusterToConf.
func ClusterToConfCheck(conf ClusterToConf) error {
	for clusterName, clusterConf := range conf {
		err := ClusterConfCheck(&clusterConf)
		if err != nil {
			return fmt.Errorf("conf for %s:%s", clusterName, err.Error())
		}
		conf[clusterName] = clusterConf
	}
	return nil
}

func prevCheckBfeClusterConf(conf *BfeClusterConf, fn func() error) error {
	if conf == nil {
		return errors.New("nil BfeClusterConf")
	}
	if conf.Version == nil {
		return errors.New("no Version")
	}

	if conf.Config == nil {
		return errors.New("no Config")
	}
	return fn()
}

// ClusterToConfBackendHTTPSCheck check ClusterToConf.HTTPSConf
func ClusterToConfBackendHTTPSCheck(conf *BfeClusterConf) error {
	return prevCheckBfeClusterConf(conf, func() error {
		for clusterName, clusterConf := range *conf.Config {
			// "HTTPSConf" does not have strict required fields, so it should be allowed to be empty.
			if clusterConf.HTTPSConf == nil {
				clusterConf.HTTPSConf = new(BackendHTTPS)
			}
			err := BackendHTTPSCheck(clusterConf.BackendConf.Protocol, clusterConf.HTTPSConf)
			if err != nil {
				return fmt.Errorf("BackendHTTPS: clusterName=%s, err=%s", clusterName, err.Error())
			}
		}
		return nil
	})
}

// BfeClusterConfCheck check integrity of config
func BfeClusterConfCheck(conf *BfeClusterConf) error {
	return prevCheckBfeClusterConf(conf, func() error {
		err := ClusterToConfCheck(*conf.Config)
		if err != nil {
			return fmt.Errorf("BfeClusterConf.Config:%s", err.Error())
		}
		return nil
	})
}

func GetCookieKey(header string) (string, bool) {
	i := strings.Index(header, ":")
	if i < 0 {
		return "", false
	}
	return strings.TrimSpace(header[i+1:]), true
}

func (conf *BfeClusterConf) LoadAndCheck(filename string) (string, error) {
	/* open the file    */
	file, err := os.Open(filename)

	if err != nil {
		return "", err
	}

	/* decode the file  */
	decoder := json.NewDecoder(file)
	defer file.Close()

	if err := decoder.Decode(&conf); err != nil {
		return "", err
	}

	/* check conf */
	if err := BfeClusterConfCheck(conf); err != nil {
		return "", err
	}

	if err := ClusterToConfBackendHTTPSCheck(conf); err != nil {
		return "", fmt.Errorf("BfeClusterConf.Config.HTTPSConf:%s", err.Error())
	}

	return *(conf.Version), nil
}

// ClusterConfLoad load config of cluster conf from file
func ClusterConfLoad(filename string) (BfeClusterConf, error) {
	var config BfeClusterConf
	if _, err := config.LoadAndCheck(filename); err != nil {
		return config, fmt.Errorf("%s", err)
	}

	return config, nil
}

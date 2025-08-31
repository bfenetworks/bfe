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
	"os"
	"regexp"
	"strings"

	"github.com/baidu/go-lib/log"

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

type AIConf struct {
	Type               int                // type of LLM service, reserved for future use. should be 0 now.
	ModelMapping       *map[string]string // model mapping, key is model name in req, value is model name in backend
	Key                *string            // API key for AI service
}

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

// GslbBasicConf is basic conf for Gslb
type GslbBasicConf struct {
	CrossRetry *int // retry cross sub clusters
	RetryMax   *int // inner cluster retry
	HashConf   *HashConf

	BalanceMode *string // balanceMode, default WRR
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
}

// ClusterConf is conf of cluster.
type ClusterConf struct {
	BackendConf  *BackendBasic     // backend's basic conf
	CheckConf    *BackendCheck     // how to check backend
	GslbBasic    *GslbBasicConf    // gslb basic conf for cluster
	ClusterBasic *ClusterBasicConf // basic conf for cluster
	HTTPSConf    *BackendHTTPS     // backend's https conf
	AIConf             *AIConf         // ai conf for cluster
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
	default:
		return fmt.Errorf("unsupported bal mode %s", *conf.BalanceMode)
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

	return nil
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

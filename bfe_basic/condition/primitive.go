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

// primitive condition implementation

package condition

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

import (
	"github.com/baidu/bfe/bfe_basic"
	"github.com/baidu/bfe/bfe_basic/condition/parser"
	"github.com/baidu/bfe/bfe_util/net_util"
	"github.com/spaolacci/murmur3"
)

const (
	HashMatcherBucketSize = 10000 // default hash bucket size for hash value matcher
)

type Fetcher interface {
	Fetch(req *bfe_basic.Request) (interface{}, error)
}

type Matcher interface {
	Match(interface{}) bool
}

// DefaultTrueCond always return true
type DefaultTrueCond struct{}

func (dt DefaultTrueCond) Match(req *bfe_basic.Request) bool {
	return true
}

type PrimitiveCond struct {
	name    string
	node    *parser.CallExpr
	fetcher Fetcher
	matcher Matcher
}

func (p *PrimitiveCond) String() string {
	return p.node.String()
}

func (p *PrimitiveCond) Match(req *bfe_basic.Request) bool {
	if req == nil || req.Session == nil || req.HttpRequest == nil {
		return false
	}

	fetched, err := p.fetcher.Fetch(req)
	if err != nil {
		return false
	}

	r := p.matcher.Match(fetched)
	return r
}

type HostFetcher struct{}

func (hf *HostFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpRequest == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	// ignore optional port in Host
	host := strings.SplitN(req.HttpRequest.Host, ":", 2)[0]
	return host, nil
}

type ProtoFetcher struct{}

func (pf *ProtoFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpRequest == nil || req.Session == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	return req.Protocol(), nil
}

type MethodFetcher struct{}

func (mf *MethodFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpRequest == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	return req.HttpRequest.Method, nil
}

type PortFetcher struct{}

func (pf *PortFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpRequest == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	port := "80"
	i := strings.Index(req.HttpRequest.Host, ":")
	if i > 0 {
		port = req.HttpRequest.Host[i+1:]
	}

	return port, nil
}

type TagFetcher struct {
	key string
}

func (tf *TagFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	if req.Tags.TagTable == nil {
		return nil, nil
	}

	return req.Tags.TagTable[tf.key], nil
}

type HasTagMatcher struct {
	value string
}

func (tm *HasTagMatcher) Match(v interface{}) bool {
	tags, ok := v.([]string)
	if !ok {
		return false
	}

	for _, t := range tags {
		tag := strings.Split(t, ":")[0]
		if tag == tm.value {
			return true
		}
	}

	return false
}

type UrlFetcher struct{}

func (uf *UrlFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpRequest == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	return req.HttpRequest.RequestURI, nil
}

type PathFetcher struct{}

func (pf *PathFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpRequest == nil || req.HttpRequest.URL == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	return req.HttpRequest.URL.Path, nil
}

type QueryKeyInFetcher struct {
	keys []string
}

func (qf *QueryKeyInFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpRequest == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	for _, key := range qf.keys {
		if _, ok := req.CachedQuery()[key]; ok {
			return true, nil
		}
	}
	return false, nil
}

type QueryKeyPrefixInFetcher struct {
	keys []string
}

func (qf *QueryKeyPrefixInFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpRequest == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	ok := false
	for k := range req.CachedQuery() {
		if prefix_in(k, qf.keys) {
			ok = true
			break
		}
	}
	return ok, nil
}

type QueryValueFetcher struct {
	key string
}

// Fetch gets first query value for the given name
func (q *QueryValueFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpRequest == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	return req.CachedQuery().Get(q.key), nil
}

type QueryExistMatcher struct{}

func (m *QueryExistMatcher) Match(req *bfe_basic.Request) bool {
	query := req.CachedQuery()

	return len(query) != 0
}

type CookieKeyInFetcher struct {
	keys []string
}

func (c *CookieKeyInFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpRequest == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	for _, key := range c.keys {
		if _, ok := req.Cookie(key); ok {
			return true, nil
		}
	}

	return false, nil
}

type CookieValueFetcher struct {
	key string
}

func (c *CookieValueFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpRequest == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	cookie, ok := req.Cookie(c.key)
	if !ok {
		return nil, fmt.Errorf("fetcher: cookie not found")
	}

	return cookie.Value, nil
}

type HeaderKeyInFetcher struct {
	keys []string
}

func (r *HeaderKeyInFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpRequest == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	for _, key := range r.keys {
		if val := req.HttpRequest.Header.Get(key); val != "" {
			return true, nil
		}
	}

	return false, nil

}

type HeaderValueFetcher struct {
	key string
}

func (r *HeaderValueFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpRequest == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	return req.HttpRequest.Header.Get(r.key), nil
}

type BypassMatcher struct{}

func (b *BypassMatcher) Match(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}

	return false
}

type InMatcher struct {
	patterns []string
	foldCase bool
}

func (im *InMatcher) Match(v interface{}) bool {
	vs, ok := v.(string)
	if !ok {
		return false
	}

	if im.foldCase {
		vs = strings.ToUpper(vs)
	}

	return in(vs, im.patterns)
}

type ExactMatcher struct {
	pattern  string
	foldCase bool
}

func (em *ExactMatcher) Match(v interface{}) bool {
	vs, ok := v.(string)
	if !ok {
		return false
	}
	if em.foldCase {
		vs = strings.ToUpper(vs)
	}
	return vs == em.pattern
}

func NewExactMatcher(pattern string, foldCase bool) *ExactMatcher {
	p := pattern

	if foldCase {
		p = strings.ToUpper(p)
	}

	return &ExactMatcher{
		pattern:  p,
		foldCase: foldCase,
	}
}

func toUpper(patterns []string) []string {
	upper := make([]string, len(patterns))

	for i, v := range patterns {
		upper[i] = strings.ToUpper(v)
	}

	return upper
}

func NewInMatcher(patterns string, foldCase bool) *InMatcher {
	p := strings.Split(patterns, "|")

	if foldCase {
		p = toUpper(p)
	}

	sort.Strings(p)

	return &InMatcher{
		patterns: p,
		foldCase: foldCase,
	}
}

type IpInMatcher struct {
	patterns []net.IP
}

func (m *IpInMatcher) Match(v interface{}) bool {
	ip, ok := v.(net.IP)
	if !ok {
		return false
	}
	ip = ip.To16()
	for _, p := range m.patterns {
		if p.Equal(ip) {
			return true
		}
	}
	return false
}

func NewIpInMatcher(patterns string) (*IpInMatcher, error) {
	p := []net.IP{}
	ips := strings.Split(patterns, "|")
	for _, ipStr := range ips {
		// Note: net.ParseIP will return ip with 16 bytes
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP addr string:%s", ipStr)
		}
		p = append(p, ip)
	}
	return &IpInMatcher{
		patterns: p,
	}, nil
}

type PrefixInMatcher struct {
	patterns []string
	foldCase bool
}

func (p *PrefixInMatcher) Match(v interface{}) bool {
	vs, ok := v.(string)
	if !ok {
		return false
	}

	if p.foldCase {
		vs = strings.ToUpper(vs)
	}

	return prefix_in(vs, p.patterns)
}

func NewPrefixInMatcher(patterns string, foldCase bool) *PrefixInMatcher {
	p := strings.Split(patterns, "|")

	if foldCase {
		p = toUpper(p)
	}

	return &PrefixInMatcher{
		patterns: p,
		foldCase: foldCase,
	}
}

type SuffixInMatcher struct {
	patterns []string
	foldCase bool
}

func (p *SuffixInMatcher) Match(v interface{}) bool {
	vs, ok := v.(string)
	if !ok {
		return false
	}

	if p.foldCase {
		vs = strings.ToUpper(vs)
	}

	return suffix_in(vs, p.patterns)
}

func NewSuffixInMatcher(patterns string, foldCase bool) *SuffixInMatcher {
	p := strings.Split(patterns, "|")

	if foldCase {
		p = toUpper(p)
	}

	return &SuffixInMatcher{
		patterns: p,
		foldCase: foldCase,
	}
}

type RegMatcher struct {
	regex *regexp.Regexp
}

func (p *RegMatcher) Match(v interface{}) bool {
	vs, ok := v.(string)
	if !ok {
		return false
	}

	return p.regex.MatchString(vs)
}

func NewRegMatcher(regex *regexp.Regexp) *RegMatcher {
	return &RegMatcher{
		regex: regex,
	}
}

func in(v string, patterns []string) bool {
	i := sort.SearchStrings(patterns, v)
	return i < len(patterns) && patterns[i] == v
}

func prefix_in(v string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.HasPrefix(v, pattern) {
			return true
		}
	}

	return false
}

func suffix_in(v string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.HasSuffix(v, pattern) {
			return true
		}
	}

	return false
}

func contains(v, pattern string) bool {
	return strings.Contains(v, pattern)
}

type UAFetcher struct{}

func (uaf *UAFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpRequest == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	return req.HttpRequest.Header.Get("User-Agent"), nil
}

type ResHeaderKeyInFetcher struct {
	keys []string
}

func (r *ResHeaderKeyInFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpResponse == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	for _, key := range r.keys {
		if val := req.HttpResponse.Header.Get(key); val != "" {
			return true, nil
		}
	}

	return false, nil

}

type ResHeaderValueFetcher struct {
	key string
}

func (r *ResHeaderValueFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpResponse == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	return req.HttpResponse.Header.Get(r.key), nil
}

type ResCodeFetcher struct{}

func (rf *ResCodeFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.HttpResponse == nil {
		return nil, fmt.Errorf("fetcher: nil pointer")
	}

	return strconv.Itoa(req.HttpResponse.StatusCode), nil
}

type TrustedCIpMatcher struct{}

func (m *TrustedCIpMatcher) Match(req *bfe_basic.Request) bool {
	return req.Session.IsTrustIP
}

type SecureProtoMatcher struct{}

func (m *SecureProtoMatcher) Match(req *bfe_basic.Request) bool {
	return req.Session.IsSecure
}

// CIPFetcher fetches client addr
type CIPFetcher struct{}

func (ip *CIPFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.ClientAddr == nil {
		return nil, fmt.Errorf("fetcher: no clientAddr")
	}

	return req.ClientAddr.IP, nil
}

// SIPFetcher fetches remote socket addr
type SIPFetcher struct{}

func (ip *SIPFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil {
		return nil, fmt.Errorf("fetcher: no req")
	}

	ses := req.Session
	if ses == nil || ses.RemoteAddr == nil {
		return nil, fmt.Errorf("fetcher: no socket ip")
	}

	return ses.RemoteAddr.IP, nil
}

// VIPFetcher fetches vip addr
type VIPFetcher struct{}

func (ip *VIPFetcher) Fetch(req *bfe_basic.Request) (interface{}, error) {
	if req == nil || req.Session.Vip == nil {
		return nil, fmt.Errorf("fetcher: no vip")
	}

	return req.Session.Vip, nil
}

type IPMatcher struct {
	startIP net.IP
	endIP   net.IP
}

func NewIPMatcher(sIPStr string, eIPStr string) (*IPMatcher, error) {
	// convert ipStr to uint32
	sIP := net.ParseIP(sIPStr)
	if sIP == nil {
		return nil, fmt.Errorf("invalid IP addr string:%s", sIPStr)
	}

	eIP := net.ParseIP(eIPStr)
	if eIP == nil {
		return nil, fmt.Errorf("invalid IP addr string:%s", eIPStr)
	}

	if net_util.IsIPv4Address(sIPStr) != net_util.IsIPv4Address(eIPStr) {
		return nil, fmt.Errorf("startIP[%s] and endIP[%s] has different addr type(IPv4/IPv6)", sIPStr, eIPStr)
	}

	// endIP must >= startIP
	if bytes.Compare(eIP, sIP) < 0 {
		return nil, fmt.Errorf("startIP[%s] must <= endIP[%s]", sIPStr, eIPStr)
	}

	return &IPMatcher{
		startIP: sIP,
		endIP:   eIP,
	}, nil
}

func (ip *IPMatcher) Match(v interface{}) bool {
	ipAddr, ok := v.(net.IP)
	if !ok {
		return false
	}
	ipAddr = ipAddr.To16()

	if bytes.Compare(ipAddr, ip.startIP) < 0 {
		return false
	}

	if bytes.Compare(ipAddr, ip.endIP) > 0 {
		return false
	}

	return true
}

type HostMatcher struct {
	patterns []string
}

func (hm *HostMatcher) Match(v interface{}) bool {
	vs, ok := v.(string)
	if !ok {
		return false
	}

	vs = strings.ToUpper(vs)

	return in(vs, hm.patterns)
}

func checkHostAndToUpper(patterns []string) ([]string, error) {
	upper := make([]string, len(patterns))

	for i, v := range patterns {
		// port shoud not be included in host
		if strings.Contains(v, ":") {
			return nil, fmt.Errorf("port shoud not be included in host(%s)", v)
		}

		upper[i] = strings.ToUpper(v)
	}

	return upper, nil
}

func NewHostMatcher(patterns string) (*HostMatcher, error) {
	p := strings.Split(patterns, "|")

	p, err := checkHostAndToUpper(p)
	if err != nil {
		return nil, err
	}

	sort.Strings(p)

	return &HostMatcher{
		patterns: p,
	}, nil
}

type ContainMatcher struct {
	patterns []string
	foldCase bool
}

func NewContainMatcher(patterns string, foldCase bool) *ContainMatcher {
	p := strings.Split(patterns, "|")

	if foldCase {
		p = toUpper(p)
	}

	return &ContainMatcher{
		patterns: p,
		foldCase: foldCase,
	}
}

func contain(v string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(v, pattern) {
			return true
		}
	}

	return false
}

func (cm *ContainMatcher) Match(v interface{}) bool {
	vs, ok := v.(string)
	if !ok {
		return false
	}

	if cm.foldCase {
		vs = strings.ToUpper(vs)
	}

	return contain(vs, cm.patterns)
}

type HashValueMatcher struct {
	buckets     []bool
	insensitive bool
}

func (matcher *HashValueMatcher) Match(v interface{}) bool {
	var rawValue string

	switch v.(type) {
	case string:
		rawValue = v.(string)
	case net.IP:
		rawValue = v.(net.IP).String()
	default:
		return false
	}

	value := rawValue
	if matcher.insensitive {
		value = strings.ToLower(rawValue)
	}

	bucket := GetHash([]byte(value), HashMatcherBucketSize)
	return matcher.buckets[bucket]
}

// setHashBuckets returns the result of inserting one section of hash bucket number to buckets table
// section is one section of bucket number. e.g.: "20" or "0-99"
// buckets is destination bucket table to be inserted
func setHashBuckets(section string, buckets *[]bool) error {
	// split numbers
	start, end, err := parserHashSectionConf(section)
	if err != nil {
		return err
	}

	// set buckets
	for i := start; i <= end; i++ {
		(*buckets)[i] = true
	}

	return nil
}

// parserHashSectionConf returns start number, end number and parse result
func parserHashSectionConf(section string) (int, int, error) {
	// split numbers
	numbers := strings.Split(section, "-")
	if len(numbers) == 0 || len(numbers) > 2 {
		return 0, 0, fmt.Errorf("hash value section %s length error", section)
	}

	// checkt numbers
	var start, end int
	for i, numberRawStr := range numbers {
		numberStr := strings.Replace(numberRawStr, " ", "", -1)
		number, err := strconv.Atoi(numberStr)
		if err != nil {
			return 0, 0, fmt.Errorf("hash value check section %s number %s err %s",
				section, numberStr, err.Error())
		}

		if number < 0 || number >= HashMatcherBucketSize {
			return 0, 0, fmt.Errorf("hash value check section %s number %s overlimit",
				section, numberStr)
		}

		if i == 0 {
			start = number
			end = number
		}

		if i == 1 {
			end = number
			if end < start {
				return 0, 0, fmt.Errorf("hash value check section %s err, start is larger", section)
			}
		}
	}

	return start, end, nil
}

func NewHashMatcher(patterns string, insensitive bool) (*HashValueMatcher, error) {
	buckets := make([]bool, HashMatcherBucketSize)

	sections := strings.Split(patterns, "|")
	for _, section := range sections {
		if err := setHashBuckets(section, &buckets); err != nil {
			return nil, err
		}
	}

	return &HashValueMatcher{
		buckets:     buckets,
		insensitive: insensitive,
	}, nil
}

func GetHash(value []byte, base uint) int {
	var hash uint64

	if value == nil {
		hash = uint64(rand.Uint32())
	} else {
		hash = murmur3.Sum64(value)
	}

	return int(hash % uint64(base))
}

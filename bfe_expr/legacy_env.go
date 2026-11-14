// Copyright (c) 2025 The BFE Authors.
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

/* legacy_env.go - legacy condition primitives implemented as Env methods
/*
DESCRIPTION
	旧条件原语以 Env 方法形式实现（PascalCase），配合 legacyPatcher 的标识符重写，
	使旧风格条件串可直接在新引擎（expr）中编译执行，语义与旧实现（primitive.go）对齐。

	移植约定（对齐 primitive.go，对拍单测以旧引擎为 oracle）：
	- 列表分隔大部分为 SplitPatterns（'|' 分隔、'\|' 转义、段内 Unquote），
	  少数为 plain strings.Split（req_host_in、hash 区间、key_in 系列），逐项照原实现
	- fold 语义统一为 ToUpper（usr_variable_value_in 除外，用 ToLower）
	- 哈希桶空间 HashMatcherBucketSize=10000，空输入每次调用取新随机桶（旧语义）
	- req_vip_in 为单 IP 精确匹配，不支持 CIDR（旧语义如此）
*/

package bfe_expr

import (
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/bfenetworks/bfe/bfe_util/murmur3"
)

// HashMatcherBucketSize 旧哈希原语的桶空间
const HashMatcherBucketSize = 10000

// LegacyEnv 旧原语方法集，方法内通过 env 调用新取数方法实现
type LegacyEnv struct {
	env *Env
}

// ---------- 内部辅助 ----------

// splitLegacyPatterns 等价旧 SplitPatterns：'|' 分隔、'\|' 转义为字面 '|'、段内做 Unquote
func splitLegacyPatterns(patterns string) []string {
	segs := make([]string, 0, 4)
	var cur strings.Builder
	bs := []byte(patterns)
	for i := 0; i < len(bs); i++ {
		if bs[i] == '\\' && i+1 < len(bs) {
			if bs[i+1] == '|' {
				cur.WriteByte('|')
			} else {
				cur.WriteByte(bs[i])
				cur.WriteByte(bs[i+1])
			}
			i++
			continue
		}
		if bs[i] == '|' {
			segs = append(segs, unquoteSeg(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(bs[i])
	}
	segs = append(segs, unquoteSeg(cur.String()))
	return segs
}

// unquoteSeg 等价旧 UnescapePattern；失败时按原文返回（旧实现为加载期报错，运行期降级处理）
func unquoteSeg(seg string) string {
	if s, err := strconv.Unquote("\"" + seg + "\""); err == nil {
		return s
	}
	return seg
}

func toUpperList(patterns []string) []string {
	out := make([]string, len(patterns))
	for i, p := range patterns {
		out[i] = strings.ToUpper(p)
	}
	return out
}

// inMatch 精确匹配；fold 时 value 与 patterns 均已预转大写
func inMatch(v string, patterns []string) bool {
	for _, p := range patterns {
		if v == p {
			return true
		}
	}
	return false
}

func prefixMatch(v string, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	return false
}

func suffixMatch(v string, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasSuffix(v, p) {
			return true
		}
	}
	return false
}

func containMatch(v string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(v, p) {
			return true
		}
	}
	return false
}

// regexCache 正则编译缓存（旧实现为加载期编译；运行期调用经此缓存避免重复编译）
var regexCache sync.Map // pattern(string) -> *regexp.Regexp

func regexMatch(v string, pattern string) bool {
	re, ok := regexCache.Load(pattern)
	if !ok {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		re, _ = regexCache.LoadOrStore(pattern, compiled)
	}
	return re.(*regexp.Regexp).MatchString(v)
}

// hashSectionMatch 等价旧 HashValueMatcher + GetHash：
// sections "0-99|500-599"；空输入每次调用取新随机桶（旧语义，非 Hash() 的每请求稳定语义）
func (l *LegacyEnv) hashSectionMatch(value string, sections string, insensitive bool) bool {
	bucket := uint32(0)
	if value != "" {
		if insensitive {
			value = strings.ToLower(value)
		}
		bucket = murmur3.Sum32([]byte(value)) % HashMatcherBucketSize
	} else {
		bucket = l.env.HashEnv.rng.Uint32() % HashMatcherBucketSize
	}

	for _, section := range strings.Split(sections, "|") {
		start, end, ok := parseHashSection(section)
		if !ok {
			continue
		}
		if int(bucket) >= start && int(bucket) <= end {
			return true
		}
	}
	return false
}

// parseHashSection 等价旧 parserHashSectionConf："20" 或 "0-99"
func parseHashSection(section string) (int, int, bool) {
	numbers := strings.Split(strings.Replace(section, " ", "", -1), "-")
	if len(numbers) == 0 || len(numbers) > 2 {
		return 0, 0, false
	}
	start, err := strconv.Atoi(numbers[0])
	if err != nil || start < 0 || start >= HashMatcherBucketSize {
		return 0, 0, false
	}
	end := start
	if len(numbers) == 2 {
		end, err = strconv.Atoi(numbers[1])
		if err != nil || end < 0 || end >= HashMatcherBucketSize || end < start {
			return 0, 0, false
		}
	}
	return start, end, true
}

// ipInExactMatch 等价旧 IpInMatcher：单 IP 精确匹配列表（plain split，无 CIDR）
func ipInExactMatch(ip string, patterns string) bool {
	ipAddr := net.ParseIP(ip)
	if ipAddr == nil {
		return false
	}
	for _, p := range strings.Split(patterns, "|") {
		if pattern := net.ParseIP(p); pattern != nil && pattern.Equal(ipAddr) {
			return true
		}
	}
	return false
}

// ---------- 旧原语方法（命名对应 legacyNameMap） ----------

func (l *LegacyEnv) DefaultT() bool { return true }

func (l *LegacyEnv) ReqCipTrusted() bool { return l.env.SesTrusted() }

// ReqVipIn 旧 req_vip_in：单 IP 精确匹配，不支持 CIDR
func (l *LegacyEnv) ReqVipIn(patterns string) bool {
	return ipInExactMatch(l.env.ReqVip(), patterns)
}

func (l *LegacyEnv) ReqProtoMatch(pattern string) bool {
	return strings.ToUpper(l.env.ReqProto()) == strings.ToUpper(pattern)
}

func (l *LegacyEnv) ReqProtoSecure() bool { return l.env.SesSecure() }

// ReqHostIn 旧 req_host_in：plain split + ToUpper 精确匹配
func (l *LegacyEnv) ReqHostIn(patterns string) bool {
	return inMatch(strings.ToUpper(l.env.ReqHost()), toUpperList(strings.Split(patterns, "|")))
}

func (l *LegacyEnv) ReqHostRegmatch(pattern string) bool {
	return regexMatch(l.env.ReqHost(), pattern)
}

func (l *LegacyEnv) ReqHostTagIn(patterns string) bool {
	return inMatch(strings.ToUpper(l.env.RouteHostTag()),
		toUpperList(splitLegacyPatterns(patterns)))
}

func (l *LegacyEnv) ReqHostSuffixIn(patterns string) bool {
	return suffixMatch(strings.ToUpper(l.env.ReqHost()),
		toUpperList(splitLegacyPatterns(patterns)))
}

func (l *LegacyEnv) ReqPathIn(patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqPath()
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return inMatch(v, p)
}

func (l *LegacyEnv) ReqPathPrefixIn(patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqPath()
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return prefixMatch(v, p)
}

func (l *LegacyEnv) ReqPathSuffixIn(patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqPath()
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return suffixMatch(v, p)
}

func (l *LegacyEnv) ReqPathContain(patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqPath()
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return containMatch(v, p)
}

func (l *LegacyEnv) ReqPathRegmatch(pattern string) bool {
	return regexMatch(l.env.ReqPath(), pattern)
}

// ReqPathElementPrefixIn 旧 req_path_element_prefix_in：
// 模式与路径都补尾 "/"（不重复补），fold 转大写后做前缀匹配
func (l *LegacyEnv) ReqPathElementPrefixIn(patterns string, fold bool) bool {
	p := strings.Split(patterns, "|")
	for i, v := range p {
		if !strings.HasSuffix(v, "/") {
			v += "/"
		}
		if fold {
			v = strings.ToUpper(v)
		}
		p[i] = v
	}
	v := l.env.ReqPath()
	if !strings.HasSuffix(v, "/") {
		v += "/"
	}
	if fold {
		v = strings.ToUpper(v)
	}
	return prefixMatch(v, p)
}

func (l *LegacyEnv) ReqQueryKeyPrefixIn(keys string) bool {
	for _, k := range strings.Split(keys, "|") {
		for _, qk := range l.env.ReqQueryKeys() {
			if strings.HasPrefix(qk, k) {
				return true
			}
		}
	}
	return false
}

func (l *LegacyEnv) ReqQueryKeyIn(keys string) bool {
	queryKeys := make(map[string]bool)
	for _, qk := range l.env.ReqQueryKeys() {
		queryKeys[qk] = true
	}
	for _, k := range strings.Split(keys, "|") {
		if queryKeys[k] {
			return true
		}
	}
	return false
}

func (l *LegacyEnv) ReqQueryExist() bool {
	return len(l.env.ReqQueryKeys()) > 0
}

func (l *LegacyEnv) ReqQueryValueIn(key, patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqQuery(key)
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return inMatch(v, p)
}

func (l *LegacyEnv) ReqQueryValuePrefixIn(key, patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqQuery(key)
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return prefixMatch(v, p)
}

func (l *LegacyEnv) ReqQueryValueSuffixIn(key, patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqQuery(key)
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return suffixMatch(v, p)
}

func (l *LegacyEnv) ReqQueryValueRegmatch(key, pattern string) bool {
	return regexMatch(l.env.ReqQuery(key), pattern)
}

func (l *LegacyEnv) ReqQueryValueHashIn(key, sections string, insensitive bool) bool {
	return l.hashSectionMatch(l.env.ReqQuery(key), sections, insensitive)
}

func (l *LegacyEnv) ReqQueryValueContain(key, patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqQuery(key)
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return containMatch(v, p)
}

func (l *LegacyEnv) ReqUrlRegmatch(pattern string) bool {
	return regexMatch(l.env.ReqUri(), pattern)
}

func (l *LegacyEnv) ReqCookieKeyIn(keys string) bool {
	for _, k := range strings.Split(keys, "|") {
		if l.env.ReqCookie(k) != "" {
			return true
		}
	}
	return false
}

func (l *LegacyEnv) ReqCookieValueIn(key, patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqCookie(key)
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return inMatch(v, p)
}

func (l *LegacyEnv) ReqCookieValuePrefixIn(key, patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqCookie(key)
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return prefixMatch(v, p)
}

func (l *LegacyEnv) ReqCookieValueSuffixIn(key, patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqCookie(key)
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return suffixMatch(v, p)
}

func (l *LegacyEnv) ReqCookieValueHashIn(key, sections string, insensitive bool) bool {
	return l.hashSectionMatch(l.env.ReqCookie(key), sections, insensitive)
}

func (l *LegacyEnv) ReqCookieValueContain(key, patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqCookie(key)
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return containMatch(v, p)
}

func (l *LegacyEnv) ReqPortIn(patterns string) bool {
	return inMatch(l.env.ReqPort(), splitLegacyPatterns(patterns))
}

// ReqTagMatch 旧 req_tag_match：tag 按 ":" 取首段比较
func (l *LegacyEnv) ReqTagMatch(key, value string) bool {
	for _, t := range l.env.ReqTags(key) {
		if strings.Split(t, ":")[0] == value {
			return true
		}
	}
	return false
}

func (l *LegacyEnv) ReqUaRegmatch(pattern string) bool {
	return regexMatch(l.env.ReqHeader("User-Agent"), pattern)
}

func (l *LegacyEnv) ReqHeaderKeyIn(keys string) bool {
	for _, k := range strings.Split(keys, "|") {
		if l.env.ReqHeader(k) != "" {
			return true
		}
	}
	return false
}

func (l *LegacyEnv) ReqHeaderValueIn(key, patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqHeader(key)
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return inMatch(v, p)
}

func (l *LegacyEnv) ReqHeaderValuePrefixIn(key, patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqHeader(key)
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return prefixMatch(v, p)
}

func (l *LegacyEnv) ReqHeaderValueSuffixIn(key, patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqHeader(key)
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return suffixMatch(v, p)
}

func (l *LegacyEnv) ReqHeaderValueRegmatch(key, pattern string) bool {
	return regexMatch(l.env.ReqHeader(key), pattern)
}

func (l *LegacyEnv) ReqHeaderValueHashIn(key, sections string, insensitive bool) bool {
	return l.hashSectionMatch(l.env.ReqHeader(key), sections, insensitive)
}

func (l *LegacyEnv) ReqHeaderValueContain(key, patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqHeader(key)
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return containMatch(v, p)
}

func (l *LegacyEnv) ReqMethodIn(patterns string) bool {
	return inMatch(strings.ToUpper(l.env.ReqMethod()),
		toUpperList(splitLegacyPatterns(patterns)))
}

func (l *LegacyEnv) ReqCipRange(start, end string) bool {
	return l.env.IpRange(l.env.ReqCip(), start, end)
}

func (l *LegacyEnv) ReqCipHashIn(sections string) bool {
	return l.hashSectionMatch(l.env.ReqCip(), sections, false)
}

func (l *LegacyEnv) ReqVipRange(start, end string) bool {
	return l.env.IpRange(l.env.ReqVip(), start, end)
}

// ResCodeIn 旧 res_code_in：状态码转字符串后精确匹配（字符串比较语义）
func (l *LegacyEnv) ResCodeIn(patterns string) bool {
	return inMatch(strconv.Itoa(l.env.ResCode()), splitLegacyPatterns(patterns))
}

func (l *LegacyEnv) ResHeaderKeyIn(keys string) bool {
	for _, k := range strings.Split(keys, "|") {
		if l.env.ResHeader(k) != "" {
			return true
		}
	}
	return false
}

func (l *LegacyEnv) ResHeaderValueIn(key, patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ResHeader(key)
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return inMatch(v, p)
}

func (l *LegacyEnv) SesVipRange(start, end string) bool {
	return l.env.IpRange(l.env.ReqVip(), start, end)
}

func (l *LegacyEnv) SesSipRange(start, end string) bool {
	return l.env.IpRange(l.env.ReqSip(), start, end)
}

func (l *LegacyEnv) SesTlsSniIn(patterns string) bool {
	return inMatch(strings.ToUpper(l.env.SesSni()),
		toUpperList(splitLegacyPatterns(patterns)))
}

// SesTlsClientCaIn 旧 ses_tls_client_ca_in
func (l *LegacyEnv) SesTlsClientCaIn(patterns string) bool {
	return inMatch(l.env.SesTlsClientCa(), splitLegacyPatterns(patterns))
}

// ReqContextValueIn 旧 req_context_value_in：Context 值须为 string，否则 false
func (l *LegacyEnv) ReqContextValueIn(key, patterns string, fold bool) bool {
	val, ok := l.env.ReqContext(key).(string)
	if !ok {
		return false
	}
	p := splitLegacyPatterns(patterns)
	if fold {
		val, p = strings.ToUpper(val), toUpperList(p)
	}
	return inMatch(val, p)
}

func (l *LegacyEnv) BfeTimeRange(start, end string) bool {
	return l.env.InTimeRange(start, end)
}

// BfePeriodicTimeRange 旧 bfe_periodic_time_range：period 非空在旧实现为加载期报错，运行期按 false
func (l *LegacyEnv) BfePeriodicTimeRange(start, end, period string) bool {
	if period != "" {
		return false
	}
	return l.env.InPeriodicRange(start, end)
}

// ReqBodyContain 旧 req_body_contain（预留实现，legacyNameMap 暂不映射）
func (l *LegacyEnv) ReqBodyContain(patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqBody()
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return containMatch(v, p)
}

func (l *LegacyEnv) ReqBodyJsonIn(path, patterns string, fold bool) bool {
	p := splitLegacyPatterns(patterns)
	v := l.env.ReqBodyJson(path)
	if fold {
		v, p = strings.ToUpper(v), toUpperList(p)
	}
	return inMatch(v, p)
}

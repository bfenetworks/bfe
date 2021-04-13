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

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bfe_http

import (
	"bytes"
	"net"
	"strconv"
	"strings"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
)

// This implementation is done according to RFC 6265:
//
//    http://tools.ietf.org/html/rfc6265

// A Cookie represents an HTTP cookie as sent in the Set-Cookie header of an
// HTTP response or the Cookie header of an HTTP request.
type Cookie struct {
	Name       string
	Value      string
	Path       string
	Domain     string
	Expires    time.Time
	RawExpires string

	// MaxAge=0 means no 'Max-Age' attribute specified.
	// MaxAge<0 means delete cookie now, equivalently 'Max-Age: 0'
	// MaxAge>0 means Max-Age attribute present and given in seconds
	MaxAge   int
	Secure   bool
	HttpOnly bool
	SameSite SameSite
	Raw      string
	Unparsed []string // Raw text of unparsed attribute-value pairs
}

type CookieMap map[string]*Cookie

// SameSite allows a server to define a cookie attribute making it impossible for
// the browser to send this cookie along with cross-site requests. The main
// goal is to mitigate the risk of cross-origin information leakage, and provide
// some protection against cross-site request forgery attacks.
//
// See https://tools.ietf.org/html/draft-ietf-httpbis-cookie-same-site-00 for details.
type SameSite int

const (
	SameSiteDefaultMode SameSite = iota + 1
	SameSiteLaxMode
	SameSiteStrictMode
	SameSiteNoneMode
)

// if disableSanitize is true, when read cookie value, no sanitize is performed
var disableSanitize = false

func SetDisableSanitize(disabled bool) {
	disableSanitize = disabled
}

// CookieMapGet parse cookies(slice) to req.Route.CookieMap(map)
func CookieMapGet(cookies []*Cookie) CookieMap {
	cookieMap := make(CookieMap, len(cookies))

	for _, cookie := range cookies {
		// if more than one cookies have the same key,
		// only the first cookie will be saved in CookieMap
		_, ok := cookieMap[cookie.Name]

		if !ok {
			cookieMap[cookie.Name] = cookie
		}
	}
	return cookieMap
}

func (cm CookieMap) Get(key string) (*Cookie, bool) {
	if cm == nil {
		return nil, false
	}

	val, ok := cm[key]
	return val, ok
}

// http://tools.ietf.org/html/rfc6265#section-5.2.1 only specify an algorithm to parse a cookie-date
// According to https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Set-Cookie, Expires format should be HTTP-date
// Preferred syntax: <day-name>, <day> <month> <year> <hour>:<minute>:<second> GMT
// However, there are some obsolete formats, specified in https://tools.ietf.org/html/rfc7231#section-7.1.1.2
func parseExpireTime(val string) (time.Time, bool) {
	// preferred format
	exptime, err := time.Parse(TimeFormat, val)
	if err == nil {
		return exptime.UTC(), true
	}
	exptime, err = time.Parse("Mon, 02-Jan-06 15:04:05 GMT", val)
	if err == nil {
		return exptime.UTC(), true
	}
	exptime, err = time.Parse(time.RFC1123, val)
	if err == nil {
		return exptime.UTC(), true
	}
	// set-cookie expire format of php version: Sun, 07-May-2028 10:53:08 GMT
	exptime, err = time.Parse("Mon, 02-Jan-2006 15:04:05 MST", val)
	if err == nil {
		return exptime.UTC(), true
	}
	return time.Time{}, false
}

// readSetCookies parses all "Set-Cookie" values from
// the header h and returns the successfully parsed Cookies.
func readSetCookies(h Header) []*Cookie {
	cookies := []*Cookie{}
	for _, line := range h["Set-Cookie"] {
		parts := strings.Split(strings.TrimSpace(line), ";")
		if len(parts) == 1 && parts[0] == "" {
			continue
		}
		parts[0] = strings.TrimSpace(parts[0])
		j := strings.Index(parts[0], "=")
		if j < 0 {
			continue
		}
		name, value := parts[0][:j], parts[0][j+1:]
		if !isCookieNameValid(name) {
			continue
		}
		value, success := parseCookieValue(value)
		if !success {
			continue
		}
		c := &Cookie{
			Name:  name,
			Value: value,
			Raw:   line,
		}
		for i := 1; i < len(parts); i++ {
			parts[i] = strings.TrimSpace(parts[i])
			if len(parts[i]) == 0 {
				continue
			}

			attr, val := parts[i], ""
			if j := strings.Index(attr, "="); j >= 0 {
				attr, val = attr[:j], attr[j+1:]
			}
			lowerAttr := strings.ToLower(attr)
			parseCookieValueFn := parseCookieValue
			if lowerAttr == "expires" {
				parseCookieValueFn = parseCookieExpiresValue
			}
			val, success = parseCookieValueFn(val)
			if !success {
				c.Unparsed = append(c.Unparsed, parts[i])
				continue
			}
			switch lowerAttr {
			case "samesite":
				lowerVal := strings.ToLower(val)
				switch lowerVal {
				case "lax":
					c.SameSite = SameSiteLaxMode
				case "strict":
					c.SameSite = SameSiteStrictMode
				case "none":
					c.SameSite = SameSiteNoneMode
				default:
					c.SameSite = SameSiteDefaultMode
				}
				continue
			case "secure":
				c.Secure = true
				continue
			case "httponly":
				c.HttpOnly = true
				continue
			case "domain":
				c.Domain = val
				// TODO: Add domain parsing
				continue
			case "max-age":
				secs, err := strconv.Atoi(val)
				if err != nil || secs != 0 && val[0] == '0' {
					break
				}
				if secs <= 0 {
					c.MaxAge = -1
				} else {
					c.MaxAge = secs
				}
				continue
			case "expires":
				c.RawExpires = val
				// if parse failed, will add to Unparsed
				exp, ok := parseExpireTime(val)
				if ok {
					c.Expires = exp
					continue
				}
			case "path":
				c.Path = val
				// TODO: Add path parsing
				continue
			}
			c.Unparsed = append(c.Unparsed, parts[i])
		}
		cookies = append(cookies, c)
	}
	return cookies
}

// SetCookie adds a Set-Cookie header to the provided ResponseWriter's headers.
func SetCookie(w ResponseWriter, cookie *Cookie) {
	w.Header().Add("Set-Cookie", cookie.String())
}

// String returns the serialization of the cookie for use in a Cookie
// header (if only Name and Value are set) or a Set-Cookie response
// header (if other fields are set).
func (c *Cookie) String() string {
	var b bytes.Buffer
	b.WriteString(sanitizeCookieName(c.Name))
	b.WriteRune('=')
	b.WriteString(sanitizeCookieValue(c.Value))
	if len(c.Path) > 0 {
		b.WriteString("; Path=")
		b.WriteString(sanitizeCookiePath(c.Path))
	}
	if len(c.Domain) > 0 {
		if validCookieDomain(c.Domain) {
			// A c.Domain containing illegal characters is not
			// sanitized but simply dropped which turns the cookie
			// into a host-only cookie. A leading dot is okay
			// but won't be sent.
			d := c.Domain
			if d[0] == '.' {
				d = d[1:]
			}
			b.WriteString("; Domain=")
			b.WriteString(d)
		} else {
			log.Logger.Warn("net/http: invalid Cookie.Domain %q; dropping domain attribute",
				c.Domain)
		}
	}
	if validCookieExpires(c.Expires) {
		b.WriteString("; Expires=")
		b2 := b.Bytes()
		b.Reset()
		b.Write(c.Expires.UTC().AppendFormat(b2, TimeFormat))
	}
	if c.MaxAge > 0 {
		b.WriteString("; Max-Age=")
		b2 := b.Bytes()
		b.Reset()
		b.Write(strconv.AppendInt(b2, int64(c.MaxAge), 10))
	} else if c.MaxAge < 0 {
		b.WriteString("; Max-Age=0")
	}
	if c.HttpOnly {
		b.WriteString("; HttpOnly")
	}
	if c.Secure {
		b.WriteString("; Secure")
	}
	switch c.SameSite {
	case SameSiteDefaultMode:
		b.WriteString("; SameSite")
	case SameSiteNoneMode:
		b.WriteString("; SameSite=None")
	case SameSiteLaxMode:
		b.WriteString("; SameSite=Lax")
	case SameSiteStrictMode:
		b.WriteString("; SameSite=Strict")
	}
	return b.String()
}

// validCookieExpires returns whether v is a valid cookie expires-value.
func validCookieExpires(t time.Time) bool {
	// IETF RFC 6265 Section 5.1.1.5, the year must not be less than 1601
	return t.Year() >= 1601
}

// readCookies parses all "Cookie" values from the header h and
// returns the successfully parsed Cookies.
//
// if filter isn't empty, only cookies of that name are returned
func readCookies(h Header, filter string) []*Cookie {
	lines := h["Cookie"]
	if len(lines) == 0 {
		return []*Cookie{}
	}

	cookies := make([]*Cookie, 0, len(lines)+strings.Count(lines[0], ";"))

	for _, line := range lines {
		line = strings.TrimSpace(line)

		var part string
		for len(line) > 0 {
			if splitIndex := strings.Index(line, ";"); splitIndex > 0 {
				part, line = line[:splitIndex], line[splitIndex+1:]
			} else {
				part, line = line, ""
			}
			part = strings.TrimSpace(part)
			if len(part) == 0 {
				continue
			}
			name, val := part, ""
			if j := strings.Index(name, "="); j >= 0 {
				name, val = name[:j], name[j+1:]
			}
			if !isCookieNameValid(name) {
				continue
			}
			if filter != "" && filter != name {
				continue
			}
			val, success := parseCookieValue(val)
			if !success {
				continue
			}
			cookies = append(cookies, &Cookie{Name: name, Value: val})
		}
	}
	return cookies
}

// validCookieDomain returns whether v is a valid cookie domain-value.
func validCookieDomain(v string) bool {
	if isCookieDomainName(v) {
		return true
	}
	if net.ParseIP(v) != nil && !strings.Contains(v, ":") {
		return true
	}
	return false
}

// isCookieDomainName returns whether s is a valid domain name or a valid
// domain name with a leading dot '.'.  It is almost a direct copy of
// package net's isDomainName.
func isCookieDomainName(s string) bool {
	if len(s) == 0 {
		return false
	}
	if len(s) > 255 {
		return false
	}

	if s[0] == '.' {
		// A cookie a domain attribute may start with a leading dot.
		s = s[1:]
	}
	last := byte('.')
	ok := false // Ok once we've seen a letter.
	partlen := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		default:
			return false
		case 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z':
			// No '_' allowed here (in contrast to package net).
			ok = true
			partlen++
		case '0' <= c && c <= '9':
			// fine
			partlen++
		case c == '-':
			// Byte before dash cannot be dot.
			if last == '.' {
				return false
			}
			partlen++
		case c == '.':
			// Byte before dot cannot be dot, dash.
			if last == '.' || last == '-' {
				return false
			}
			if partlen > 63 || partlen == 0 {
				return false
			}
			partlen = 0
		}
		last = c
	}
	if last == '-' || partlen > 63 {
		return false
	}

	return ok
}

var cookieNameSanitizer = strings.NewReplacer("\n", "-", "\r", "-")

func sanitizeCookieName(n string) string {
	return cookieNameSanitizer.Replace(n)
}

// http://tools.ietf.org/html/rfc6265#section-4.1.1
// cookie-value      = *cookie-octet / ( DQUOTE *cookie-octet DQUOTE )
// cookie-octet      = %x21 / %x23-2B / %x2D-3A / %x3C-5B / %x5D-7E
//           ; US-ASCII characters excluding CTLs,
//           ; whitespace DQUOTE, comma, semicolon,
//           ; and backslash
func sanitizeCookieValue(v string) string {
	if disableSanitize {
		return v
	}

	return sanitizeOrWarn("Cookie.Value", validCookieValueByte, v)
}

func validCookieValueByte(b byte) bool {
	return 0x20 < b && b < 0x7f && b != '"' && b != ',' && b != ';' && b != '\\'
}

// path-av           = "Path=" path-value
// path-value        = <any CHAR except CTLs or ";">
func sanitizeCookiePath(v string) string {
	return sanitizeOrWarn("Cookie.Path", validCookiePathByte, v)
}

func validCookiePathByte(b byte) bool {
	return 0x20 <= b && b < 0x7f && b != ';'
}

func sanitizeOrWarn(fieldName string, valid func(byte) bool, v string) string {
	ok := true
	for i := 0; i < len(v); i++ {
		if valid(v[i]) {
			continue
		}
		log.Logger.Warn("net/http: invalid byte %q in %s; dropping invalid bytes",
			v[i], fieldName)
		ok = false
		break
	}
	if ok {
		return v
	}
	buf := make([]byte, 0, len(v))
	for i := 0; i < len(v); i++ {
		if b := v[i]; valid(b) {
			buf = append(buf, b)
		}
	}
	return string(buf)
}

func unquoteCookieValue(v string) string {
	if len(v) > 1 && v[0] == '"' && v[len(v)-1] == '"' {
		return v[1 : len(v)-1]
	}
	return v
}

func isCookieByte(c byte) bool {
	switch {
	case c == 0x21, 0x23 <= c && c <= 0x2b, 0x2d <= c && c <= 0x3a,
		0x3c <= c && c <= 0x5b, 0x5d <= c && c <= 0x7e:
		return true
	}
	return false
}

func isCookieExpiresByte(c byte) (ok bool) {
	return isCookieByte(c) || c == ',' || c == ' '
}

func parseCookieValue(raw string) (string, bool) {
	if disableSanitize {
		return raw, true
	}

	return parseCookieValueUsing(raw, isCookieByte)
}

func parseCookieExpiresValue(raw string) (string, bool) {
	return parseCookieValueUsing(raw, isCookieExpiresByte)
}

func parseCookieValueUsing(raw string, validByte func(byte) bool) (string, bool) {
	raw = unquoteCookieValue(raw)
	for i := 0; i < len(raw); i++ {
		if !validByte(raw[i]) {
			return "", false
		}
	}
	return raw, true
}

func isCookieNameValid(raw string) bool {
	return strings.IndexFunc(raw, isNotToken) < 0
}

package mod_sdk_nsfocus_waf

import (
	"net/http"
	"slices"
)

// checkSensitiveHeader check if header is sensitive
func checkSensitiveHeader(key string) bool {
	// list of sensitive headers
	sensitiveHeaders := []string{
		"authorization",
		"www-authenticate",
		"cookie",
		"cookie2",
		"proxy-authenticate",
		"proxy-authorization",
	}
	// check if key is in sensitive headers
	key = http.CanonicalHeaderKey(key)
	return slices.Contains(sensitiveHeaders, key)
}

// copyHeaders copy headers from src to dst
func copyHeaders(src http.Header, dst http.Header, sensitive bool) {
	// copy headers for each key
	for k, vv := range src {
		// skip sensitive headers
		if sensitive && checkSensitiveHeader(k) {
			continue
		}
		// set header
		dst[k] = vv
	}
}

// createReqFromHTTPReq creates a new HTTP request from an existing HTTP request
func createReqFromHTTPReq(originReq *http.Request) (*http.Request, error) {
	// copy new url
	newURL := *originReq.URL
	// set new url, nsfocus waf server only support http
	newURL.Scheme = "http"
	newURL.Host = originReq.Host
	// set header
	newReq := &http.Request{
		Method: originReq.Method,
		URL:    &newURL,
		Header: make(http.Header),
		Body:   originReq.Body,
		Host:   originReq.Host,
	}
	// copy headers, all headers except sensitive headers
	copyHeaders(originReq.Header, newReq.Header, false)
	return newReq, nil
}

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

package mod_header

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_net/textproto"
)

type ActionFile struct {
	Cmd    *string // command of action
	Params []string
}

type Action struct {
	Cmd    string   // command of action (for header set/add/del)
	Params []string // params of action ([header, value] or [header])
}

type ActionFileList []ActionFile

func ActionFileCheck(conf ActionFile) error {
	// check command
	if conf.Cmd == nil {
		return errors.New("no Cmd")
	}

	// validate command, and get how many params should exist for each command
	switch *conf.Cmd {
	case "REQ_HEADER_SET",
		"REQ_HEADER_ADD",
		"REQ_HEADER_RENAME",
		"RSP_HEADER_SET",
		"RSP_HEADER_ADD",
		"RSP_HEADER_RENAME":

		// header and value
		if len(conf.Params) != 2 {
			return fmt.Errorf("num of params:[ok:2, now:%d]", len(conf.Params))
		}
	case "REQ_HEADER_DEL",
		"RSP_HEADER_DEL":
		// header
		if len(conf.Params) != 1 {
			return fmt.Errorf("num of params:[ok:1, now:%d]", len(conf.Params))
		}

	case "REQ_HEADER_MOD",
		"RSP_HEADER_MOD":

		// check params for req/rsp_header_mod. eg.
		// - REQ_HEADER_MOD: [scheme_set, referer, http]
		// - RSP_HEADER_MOD: [scheme_set, location, https]
		// - REQ_HEADER_MOD: [query_add, referer, key, value]
		if err := checkHeaderModParams(conf.Params); err != nil {
			return fmt.Errorf("checkHeaderModParams: %s", err.Error())
		}

	case ReqCookieSet:
		if len(conf.Params) != 2 {
			return fmt.Errorf("num of params:[ok:2, now:%d]", len(conf.Params))
		}

	case ReqCookieDel:
		if len(conf.Params) != 1 {
			return fmt.Errorf("num of params:[ok:1, now:%d]", len(conf.Params))
		}

	case RspCookieDel:
		if len(conf.Params) != 3 {
			return fmt.Errorf("num of params:[ok:1, now:%d]", len(conf.Params))
		}

	case RspCookieSet:
		if len(conf.Params) != 8 {
			return fmt.Errorf("num of params:[ok:6, now:%d]", len(conf.Params))
		}
		if _, err := time.Parse(time.RFC1123, conf.Params[4]); err != nil {
			return fmt.Errorf("expires format error, should be RFC1123 format")
		}
		if _, err := strconv.Atoi(conf.Params[5]); err != nil {
			return fmt.Errorf("type of max age should be int")
		}
		if _, err := strconv.ParseBool(conf.Params[6]); err != nil {
			return fmt.Errorf("type of http only should be bool")
		}
		if _, err := strconv.ParseBool(conf.Params[7]); err != nil {
			return fmt.Errorf("type of secure should be bool")
		}

	default:
		return fmt.Errorf("invalid cmd:%s", *conf.Cmd)
	}

	for _, p := range conf.Params {
		if len(p) == 0 {
			return errors.New("empty Params")
		}
	}

	return nil
}

func checkHeaderModParams(params []string) error {
	// - REQ_HEADER_MOD: [scheme_set, referer, http]
	// - RSP_HEADER_MOD: [scheme_set, location, https]
	// - REQ_HEADER_MOD: [query_add, referer, key, value]
	if len(params) != 3 && len(params) != 4 {
		return fmt.Errorf("num of params:[ok:3 or 4, now:%d]", len(params))
	}

	headerModCmd := strings.ToUpper(params[0])
	headerKey := textproto.CanonicalMIMEHeaderKey(params[1])

	switch headerModCmd {
	case "SCHEME_SET":
		// scheme_set must have 3 params
		if len(params) != 3 {
			return fmt.Errorf("scheme_set should have 3 params, now: %d", len(params))
		}

		// only referer/location support scheme_set
		if headerKey != "Referer" && headerKey != "Location" {
			return fmt.Errorf("scheme_set only support referer/location, now: %s", headerKey)
		}

		// scheme_set only support http/https
		if params[2] != "http" && params[2] != "https" {
			return fmt.Errorf("scheme_set only support http/https, now: %s", params[2])
		}

	case "QUERY_ADD":
		// query_add must have 4 params
		if len(params) != 4 {
			return fmt.Errorf("query_add should have 4 params, now: %d", len(params))
		}

		// only referer/location support query_add
		if headerKey != "Referer" && headerKey != "Location" {
			return fmt.Errorf("query_add only support referer/location, now: %s", headerKey)
		}

	default:
		return fmt.Errorf("invalid headerModCmd:%s", headerModCmd)
	}

	return nil
}

func ActionFileListCheck(conf *ActionFileList) error {
	for index, action := range *conf {
		err := ActionFileCheck(action)
		if err != nil {
			return fmt.Errorf("ActionFileList:%d, %s", index, err.Error())
		}
	}

	return nil
}

// expectPercent returns index of '%', otherwise return
// last index of str
func expectPercent(str string) int {
	index := 0
	for _, c := range str {
		if c != '%' {
			index++
			continue
		}
		break
	}

	return index
}

const variableCharset = "abcdefghijklmnopqrstuvwxyz0123456789_"

func expectVariableParam(str string) int {
	index := 0
	// variable param, variable now only has
	// characters [a-z] and '_'
	for _, c := range str {
		if !strings.Contains(variableCharset, string(c)) {
			break
		}
		index++
	}

	return index
}

// splitParam splits string "__bsi=%bfe_ssl_info;max-age=3600;
// domain=%bfe_domain_auto" to separate strings
// "__bsi="
// "%bfe_ssl_info"
// ";max-age=3600"
func splitParam(param string) []string {
	params := make([]string, 0)
	paramBegin := 0
	index := 0

	for {
		paramBegin = index
		if paramBegin >= len(param) {
			break
		}
		// normal text
		if param[index] != '%' {
			index++
			index += expectPercent(param[index:])
		} else if index++; index < len(param) {
			if param[index] == '%' {
				// escape text
				index++
				index += expectPercent(param[index:])
			} else {
				// variable param
				index += expectVariableParam(param[index:])
			}
		}

		params = append(params, param[paramBegin:index])
	}

	return params
}

func preProcessParams(param string) ([]string, error) {
	/* second param has three schema:
		 * bfe_vip:   no '%' prefix
	     * %bfe_vip:  has '%' prefix
	     * %%bfe_vip: has '%%' prefix
		 * here, we only process second schema, such as %bfe_vip
		 * other two, return directly
	*/

	params := splitParam(param)

	for _, p := range params {

		//header value variable
		if strings.HasPrefix(p, "%") && !strings.HasPrefix(p, "%%") {
			varHeaderVal := strings.ToLower(p)
			if _, found := VariableHandlers[varHeaderVal[1:]]; !found {
				// not valid header value variable
				err := fmt.Errorf("command's second param is not valid: %s", p)
				return params, err
			}
		}
	}

	return params, nil
}

func getHeaderValue(req *bfe_basic.Request, action Action) string {
	var value string

	if len(action.Params) <= 1 {
		return ""
	}

	// concatenate sepate strings to a whole one
	// if a variable param, decode it first
	for _, p := range action.Params[1:] {
		if strings.HasPrefix(p, "%") {
			if handler, found := VariableHandlers[p[1:]]; found {
				value += handler(req)
			} else {
				value += p[1:]
			}
		} else {
			value += p
		}
	}

	return value
}

// modify header value by action.
func modHeaderValue(value string, action Action) string {
	var headerVal string
	headerModCmd := action.Params[0]

	// set scheme
	if headerModCmd == "SCHEME_SET" {
		// action.Params[2]: scheme
		headerVal = setScheme(value, action.Params[2])
	}

	// add query
	if headerModCmd == "QUERY_ADD" {
		// action.Params[2]: key
		// action.Params[3]: value
		headerVal = addQuery(value, action.Params[2], action.Params[3])
	}

	return headerVal
}

// setScheme set uri scheme.
func setScheme(uri string, scheme string) string {
	var i int

	// check uri start by http:// or https://
	if !(strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://")) {
		return uri
	}

	// find scheme end
	if i = strings.Index(uri, ":"); i == -1 {
		// return raw uri
		return uri
	}

	// uri = scheme + rest
	uri = scheme + uri[i:]

	return uri
}

// addQuery add query for uri.
func addQuery(uri string, key string, val string) string {
	// parse uri
	u, err := url.Parse(uri)
	if err != nil {
		// return raw uri
		return uri
	}

	if u.RawQuery == "" {
		u.RawQuery = key + "=" + val
	} else {
		u.RawQuery = u.RawQuery + "&" + key + "=" + val
	}

	return u.String()
}

func actionConvert(actionFile ActionFile) (Action, error) {
	action := Action{}
	action.Cmd = *actionFile.Cmd

	switch action.Cmd {
	case "REQ_HEADER_SET", "REQ_HEADER_ADD",
		"RSP_HEADER_SET", "RSP_HEADER_ADD":
		// - REQ_HEADER_SET: [referer,  http://bfe.baidu.com]
		// - RSP_HEADER_SET: [set-cookie, __bsi=%bfe_ssl_info; max-age=3600;]
		key := textproto.CanonicalMIMEHeaderKey(actionFile.Params[0])
		values, err := preProcessParams(actionFile.Params[1])
		if err != nil {
			return action, err
		}

		// append key values
		action.Params = append(action.Params, key)
		action.Params = append(action.Params, values...)
	case "REQ_HEADER_RENAME", "RSP_HEADER_RENAME":
		originalKey := textproto.CanonicalMIMEHeaderKey(actionFile.Params[0])
		newKey := textproto.CanonicalMIMEHeaderKey(actionFile.Params[1])
		action.Params = append(action.Params, originalKey)
		action.Params = append(action.Params, newKey)
	case "REQ_HEADER_DEL", "RSP_HEADER_DEL":
		// - REQ_HEADER_DEL: [referer]
		// - RSP_HEADER_DEL: [location]
		key := textproto.CanonicalMIMEHeaderKey(actionFile.Params[0])

		// append key
		action.Params = append(action.Params, key)

	case "REQ_HEADER_MOD", "RSP_HEADER_MOD":
		// - REQ_HEADER_MOD: [scheme_set, referer, http]
		// - RSP_HEADER_MOD: [scheme_set, location, https]
		// - REQ_HEADER_MOD: [query_add, referer, key, value]
		cmd := strings.ToUpper(actionFile.Params[0])
		key := textproto.CanonicalMIMEHeaderKey(actionFile.Params[1])

		action.Params = append(action.Params, cmd)
		action.Params = append(action.Params, key)
		action.Params = append(action.Params, actionFile.Params[2:]...)

	case ReqCookieSet, ReqCookieDel,
		RspCookieSet, RspCookieDel:
		action.Params = actionFile.Params

	default:
		return action, fmt.Errorf("invalid cmd:%s", action.Cmd)
	}

	return action, nil
}

func actionsConvert(actionFiles ActionFileList) ([]Action, error) {
	actions := make([]Action, 0)

	for _, actionFile := range actionFiles {
		action, err := actionConvert(actionFile)
		if err != nil {
			return actions, err
		}
		actions = append(actions, action)
	}

	return actions, nil
}

func HeaderActionDo(h *bfe_http.Header, cmd string, headerName string, value string) {
	switch cmd {
	// insert or modify
	case "HEADER_SET", "HEADER_MOD":
		headerSet(h, headerName, value)
	// append
	case "HEADER_ADD":
		headerAdd(h, headerName, value)
	// delete
	case "HEADER_DEL":
		headerDel(h, headerName)
	case "HEADER_RENAME":
		headerRename(h, headerName, value)
	}
}

func getHeader(req *bfe_basic.Request, headerType int) (h *bfe_http.Header) {
	switch headerType {
	case ReqHeader:
		h = &req.HttpRequest.Header
	case RspHeader:
		h = &req.HttpResponse.Header
	}

	return h
}

func processHeader(req *bfe_basic.Request, headerType int, action Action) {
	var key string
	var value string
	var cmd string

	h := getHeader(req, headerType)

	cmd = action.Cmd[4:]

	switch cmd {
	case "HEADER_MOD":
		key = action.Params[1]
		// get header value
		if value = h.Get(key); value == "" {
			// if req do not have this header, continue
			return
		}
		// mod header value
		value = modHeaderValue(value, action)
	case "HEADER_RENAME":
		originalKey, newKey := action.Params[0], action.Params[1]
		if h.Get(originalKey) == "" || h.Get(newKey) != "" {
			return
		}
		key, value = originalKey, newKey
	default:
		key = action.Params[0]
		value = getHeaderValue(req, action)
	}

	// trim action.Cmd prefix REQ_ and RSP_
	HeaderActionDo(h, cmd, key, value)
}

func processCookie(req *bfe_basic.Request, headerType int, action Action) {
	if headerType == ReqHeader {
		ReqCookieActionDo(req, action)
		return
	}
	RspCookieActionDo(req, action)
}

func HeaderActionsDo(req *bfe_basic.Request, headerType int, actions []Action) {
	for _, action := range actions {
		if strings.Contains(action.Cmd, "HEADER") {
			processHeader(req, headerType, action)
		} else {
			processCookie(req, headerType, action)
		}
	}
}

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

package mod_errors

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"strconv"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_util"
)

const (
	RETURN   = "RETURN"
	REDIRECT = "REDIRECT"
)

type ActionFile struct {
	Cmd    *string  // command of action
	Params []string // params of action
}

type Action struct {
	Cmd string // command of action

	// for RETURN action
	StatusCode  int    // status code
	ContentType string // content type
	ContentData string // content data

	// for REDIRECT action
	RedirectUrl string
}

type ActionFileList []ActionFile

// ActionFileCheck check ActionFile, return nil if check ok
func ActionFileCheck(conf ActionFile) error {
	// check command
	if conf.Cmd == nil {
		return errors.New("no Cmd")
	}

	// check params
	switch *conf.Cmd {
	case RETURN:
		// check number of params: statusCode, contentType, contentData
		if len(conf.Params) != 3 {
			return fmt.Errorf("params num should be 3 (%d)", len(conf.Params))
		}

		// check response code
		responseCode, err := strconv.Atoi(conf.Params[0])
		if err != nil {
			return fmt.Errorf("Params[0]: invalid status code:%s", conf.Params[0])
		}
		if bfe_http.StatusTextGet(responseCode) == "" {
			return fmt.Errorf("params[0]: invalid status code:%s", conf.Params[0])
		}

		codeClass := responseCode / 100
		if codeClass != 2 && codeClass != 4 && codeClass != 5 {
			return fmt.Errorf("params[0]: status code should be 2XX/4XX/5XX:%s", conf.Params[0])
		}

		// check content data
		if err := bfe_util.CheckStaticFile(conf.Params[2], MaxPageSize); err != nil {
			return fmt.Errorf("params[2] err:%s", err.Error())
		}

	case REDIRECT:
		if len(conf.Params) != 1 {
			return fmt.Errorf("params num should be 1 (%d)", len(conf.Params))
		}
		if _, err := url.Parse(conf.Params[0]); err != nil {
			return fmt.Errorf("invalid url: %s", err)
		}

	default:
		return fmt.Errorf("invalid command: %s", *conf.Cmd)
	}

	return nil
}

// ActionFileListCheck check ActionFileList, return nil if check ok
func ActionFileListCheck(actionList *ActionFileList) error {
	if len(*actionList) != 1 {
		return fmt.Errorf("ActionFileList: should contain 1 actions")
	}

	actions := *actionList
	if err := ActionFileCheck(actions[0]); err != nil {
		return fmt.Errorf("ActionFileList: %s", err)
	}

	return nil
}

func actionConvert(actionFile ActionFile) Action {
	action := Action{}
	action.Cmd = *actionFile.Cmd

	switch action.Cmd {
	case RETURN:
		// convert status code
		action.StatusCode, _ = strconv.Atoi(actionFile.Params[0])

		// convert content type
		action.ContentType = actionFile.Params[1]

		// convert content data
		rawData, _ := ioutil.ReadFile(actionFile.Params[2])
		action.ContentData = string(rawData)

	case REDIRECT:
		action.RedirectUrl = actionFile.Params[0]
	}

	return action
}

func actionsConvert(actionFiles ActionFileList) []Action {
	actions := make([]Action, 0)
	for _, actionFile := range actionFiles {
		action := actionConvert(actionFile)
		actions = append(actions, action)
	}
	return actions
}

func ErrorsActionsDo(req *bfe_basic.Request, actions []Action) {
	action := actions[0] // should contain 1 action
	switch action.Cmd {
	case RETURN:
		doReturn(req, action)
	case REDIRECT:
		doRedirect(req, action)
	}
}

func doReturn(req *bfe_basic.Request, action Action) {
	prepareResponse(req)
	res := req.HttpResponse
	res.StatusCode = action.StatusCode
	if len(action.ContentType) != 0 {
		res.Header.Set("Content-Type", action.ContentType)
	}

	content := action.ContentData
	res.Header.Set("Content-Length", strconv.Itoa(len(content)))
	res.Body = ioutil.NopCloser(strings.NewReader(content))
}

func doRedirect(req *bfe_basic.Request, action Action) {
	prepareResponse(req)
	res := req.HttpResponse
	res.StatusCode = 302
	res.ContentLength = 0
	res.Header.Set("Location", action.RedirectUrl)
	res.Body = bfe_http.EofReader
}

func prepareResponse(req *bfe_basic.Request) {
	res := req.HttpResponse
	if res.Body != nil {
		res.Body.Close()
	}

	res.Header = make(bfe_http.Header)
	res.Header.Set("Server", "bfe")
	res.Trailer = nil
	res.TransferEncoding = nil
	res.TLS = nil
}

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

package mod_static

import (
	"fmt"
	"mime"
	"net/url"
	"os"
	"strings"
	"time"
)

import (
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	ModStatic = "mod_static"
)

var (
	openDebug     = false
	unixEpochTime = time.Unix(0, 0)
)

type ModuleStaticState struct {
	FileBrowseSize             *metrics.Counter
	FileBrowseCount            *metrics.Counter
	FileBrowseNotExist         *metrics.Counter
	FileBrowseContentTypeError *metrics.Counter
	FileBrowseFallbackDefault  *metrics.Counter
	FileCurrentOpened          *metrics.Gauge
}

type ModuleStatic struct {
	name          string
	state         ModuleStaticState
	metrics       metrics.Metrics
	conf          *ConfModStatic
	ruleTable     *StaticRuleTable
	mimeTypeTable *MimeTypeTable
}

func NewModuleStatic() *ModuleStatic {
	m := new(ModuleStatic)
	m.name = ModStatic
	m.metrics.Init(&m.state, ModStatic, 0)
	m.ruleTable = NewStaticRuleTable()
	m.mimeTypeTable = NewMimeTypeTable()
	return m
}

func (m *ModuleStatic) Name() string {
	return m.name
}

func (m *ModuleStatic) loadConfData(query url.Values) error {
	path := query.Get("path")
	if path == "" {
		path = m.conf.Basic.DataPath
	}

	conf, err := StaticConfLoad(path)
	if err != nil {
		return fmt.Errorf("error in StaticConfLoad(%s): %v", path, err)
	}

	m.ruleTable.Update(conf)
	return nil
}

func (m *ModuleStatic) loadMimeType(query url.Values) error {
	var err error
	path := query.Get("path")
	if path == "" {
		path = m.conf.Basic.MimeTypePath
	}

	conf, err := MimeTypeConfLoad(path)
	if err != nil {
		return fmt.Errorf("error in MimeTypeConfLoad(%s): %v", path, err)
	}
	m.mimeTypeTable.Update(conf)

	return nil
}

func (m *ModuleStatic) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleStatic) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleStatic) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

func (m *ModuleStatic) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:                              m.loadConfData,
		fmt.Sprintf("%s.mime_type", m.name): m.loadMimeType,
	}
	return handlers
}

func errorStatusCode(err error) int {
	if os.IsNotExist(err) {
		return bfe_http.StatusNotFound
	}
	if os.IsPermission(err) {
		return bfe_http.StatusForbidden
	}

	return bfe_http.StatusInternalServerError
}

func (m *ModuleStatic) openStaticFile(req *bfe_http.Request, root string,
	defaultFile string) (*staticFile, error) {
	filename := req.URL.Path

	// check accept encoding
	encodingList := make([]string, 0)
	if m.conf.Basic.EnableCompress {
		encodingList = CheckAcceptEncoding(req)
	}

	// try specified file
	file, err := newStaticFile(root, filename, encodingList, m)
	if os.IsNotExist(err) {
		m.state.FileBrowseNotExist.Inc(1)
	}

	// try default file
	if os.IsNotExist(err) || err == errUnexpectedDir {
		if len(defaultFile) != 0 {
			file, err = newStaticFile(root, defaultFile, encodingList, m)
			m.state.FileBrowseFallbackDefault.Inc(1)
		}
	}

	return file, err
}

func (m *ModuleStatic) processContentType(resp *bfe_http.Response, file *staticFile) {
	// get and check mime type
	ctype, ok := m.mimeTypeTable.Search(strings.ToLower(file.extension))
	if !ok {
		ctype = mime.TypeByExtension(file.extension)
	}
	if ctype == "" {
		m.state.FileBrowseContentTypeError.Inc(1)
		return
	}

	// set Content-Type header
	resp.Header.Set("Content-Type", ctype)
}

func (m *ModuleStatic) processContentEncoding(resp *bfe_http.Response, file *staticFile) {
	switch file.encoding {
	case EncodeGzip, EncodeBrotil:
		resp.Header.Set("Content-Encoding", file.encoding)
	default:
		return
	}
}

func (m *ModuleStatic) processContentLength(resp *bfe_http.Response, file *staticFile) {
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", file.Size()))
}

func (m *ModuleStatic) processLastModified(resp *bfe_http.Response, file *staticFile) {
	// get and check mod time
	t := file.ModTime()
	if t.IsZero() || t.Equal(unixEpochTime) {
		return
	}

	// set Last-Modified header
	resp.Header.Set("Last-Modified", t.UTC().Format(bfe_http.TimeFormat))
}

func (m *ModuleStatic) createRespFromStaticFile(req *bfe_basic.Request,
	rule *StaticRule) *bfe_http.Response {
	resp := bfe_basic.CreateInternalResp(req, bfe_http.StatusOK)
	root := rule.Action.Params[0]
	defaultFile := rule.Action.Params[1]

	// check request method
	httpRequest := req.HttpRequest
	if httpRequest.Method != "GET" && httpRequest.Method != "HEAD" {
		resp.StatusCode = bfe_http.StatusMethodNotAllowed
		return resp
	}

	// open static file
	file, err := m.openStaticFile(httpRequest, root, defaultFile)
	if err != nil {
		resp.StatusCode = errorStatusCode(err)
		return resp
	}
	m.state.FileBrowseSize.Inc(uint(file.Size()))

	// prepare response
	m.processContentType(resp, file)
	m.processContentEncoding(resp, file)
	m.processContentLength(resp, file)
	m.processLastModified(resp, file)

	if httpRequest.Method != "HEAD" {
		resp.Body = file
	} else {
		file.Close()
	}

	return resp
}

func (m *ModuleStatic) staticFileHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	rules, ok := m.ruleTable.Search(req.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn, nil
	}

	for _, rule := range *rules {
		if !rule.Cond.Match(req) {
			continue
		}

		switch rule.Action.Cmd {
		case ActionBrowse:
			m.state.FileBrowseCount.Inc(1)
			return bfe_module.BfeHandlerResponse, m.createRespFromStaticFile(req, &rule)
		default: // never come here
			return bfe_module.BfeHandlerGoOn, nil
		}
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleStatic) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error
	var cfg *ConfModStatic

	confPath := bfe_module.ModConfPath(cr, m.name)
	if cfg, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err: %v", m.name, err)
	}
	openDebug = cfg.Log.OpenDebug
	m.conf = cfg

	if err = m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %v", err)
	}

	if err = m.loadMimeType(nil); err != nil {
		return fmt.Errorf("err in loadMimeType(): %v", err)
	}

	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.staticFileHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.staticFileHandler): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.monitorHandlers): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(): %v", m.name, err)
	}

	return nil
}

// Copyright (c) 2019 Baidu, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     bfe_http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mod_static

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

import (
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/baidu/bfe/bfe_basic"
	"github.com/baidu/bfe/bfe_http"
	"github.com/baidu/bfe/bfe_module"
)

const (
	ModStatic = "mod_static"
)

var (
	unixEpochTime = time.Unix(0, 0)
)

type ModuleStaticState struct {
	FileBrowseCount    *metrics.Counter
	FileCurrentOpened  *metrics.Counter
	FileBrowseNotExist *metrics.Counter
	FileBrowseSize     *metrics.Counter
}

type ModuleStatic struct {
	name       string
	state      ModuleStaticState
	metrics    metrics.Metrics
	configPath string
	ruleTable  *StaticRuleTable
}

type staticFile struct {
	http.File
	m *ModuleStatic
}

func newStaticFile(root string, filename string, m *ModuleStatic) (*staticFile, error) {
	var err error
	s := new(staticFile)
	s.m = m
	s.File, err = http.Dir(root).Open(filename)
	if err != nil {
		return nil, err
	}

	m.state.FileCurrentOpened.Inc(1)
	return s, nil
}

func (s *staticFile) Close() error {
	err := s.File.Close()
	if err != nil {
		return err
	}

	state := s.m.state
	state.FileCurrentOpened.Inc(-1)
	return nil
}

func NewModuleStatic() *ModuleStatic {
	m := new(ModuleStatic)
	m.name = ModStatic
	m.metrics.Init(&m.state, ModStatic, 0)
	m.ruleTable = NewStaticRuleTable()
	return m
}

func (m *ModuleStatic) Name() string {
	return m.name
}

func (m *ModuleStatic) loadConfData(query url.Values) error {
	path := query.Get("path")
	if path == "" {
		path = m.configPath
	}

	conf, err := StaticConfLoad(path)
	if err != nil {
		return fmt.Errorf("error in StaticConfLoad(%s): %v", path, err)
	}

	m.ruleTable.Update(conf)
	return nil
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

func (m *ModuleStatic) tryDefaultFile(root string, defaultFile string) (*staticFile, error) {
	if len(defaultFile) != 0 {
		return newStaticFile(root, defaultFile, m)
	}
	m.state.FileBrowseNotExist.Inc(1)
	return nil, os.ErrNotExist
}

func isZeroTime(t time.Time) bool {
	return t.IsZero() || t.Equal(unixEpochTime)
}

func setLastModified(resp *bfe_http.Response, modtime time.Time) {
	if !isZeroTime(modtime) {
		resp.Header.Set("Last-Modified", modtime.UTC().Format(bfe_http.TimeFormat))
	}
}

func (m *ModuleStatic) createRespFromStaticFile(req *bfe_basic.Request,
	rule *StaticRule) (resp *bfe_http.Response) {
	resp = bfe_basic.CreateInternalResp(req, bfe_http.StatusOK)
	root := rule.Action.Params[0]
	defaultFile := rule.Action.Params[1]

	httpRequest := req.HttpRequest
	if httpRequest.Method != "GET" && httpRequest.Method != "HEAD" {
		resp.StatusCode = bfe_http.StatusMethodNotAllowed
		return
	}

	reqPath := httpRequest.URL.Path
	file, err := newStaticFile(root, reqPath, m)
	if os.IsNotExist(err) {
		file, err = m.tryDefaultFile(root, defaultFile)
	}
	if err != nil {
		resp.StatusCode = errorStatusCode(err)
		return
	}

	fileInfo, err := file.Stat()
	if err != nil {
		resp.StatusCode = errorStatusCode(err)
		return
	}
	if fileInfo.IsDir() {
		file, err = m.tryDefaultFile(root, defaultFile)
		if err != nil {
			resp.StatusCode = errorStatusCode(err)
			return
		}
	}
	m.state.FileBrowseSize.Inc(int(fileInfo.Size()))

	resp.StatusCode = bfe_http.StatusOK
	setLastModified(resp, fileInfo.ModTime())
	resp.Body = file
	return
}

func (m *ModuleStatic) staticFileHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	rules, ok := m.ruleTable.Search(req.Route.Product)
	if !ok {
		return bfe_module.BFE_HANDLER_GOON, nil
	}

	for _, rule := range *rules {
		if rule.Cond.Match(req) {
			switch rule.Action.Cmd {
			case ActionBrowse:
				m.state.FileBrowseCount.Inc(1)
				return bfe_module.BFE_HANDLER_RESPONSE, m.createRespFromStaticFile(req, &rule)
			default:
				continue
			}
		}
	}

	return bfe_module.BFE_HANDLER_GOON, nil
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
	m.configPath = cfg.Basic.DataPath

	if err = m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %v", err)
	}

	err = cbs.AddFilter(bfe_module.HANDLE_FOUND_PRODUCT, m.staticFileHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.staticFileHandler): %v", m.name, err)
	}

	err = whs.RegisterHandler(web_monitor.WEB_HANDLE_RELOAD, m.name, m.loadConfData)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.loadConfData): %v", m.name, err)
	}

	return nil
}

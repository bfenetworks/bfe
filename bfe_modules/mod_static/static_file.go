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
	"errors"
	"net/http"
	"os"
	"path/filepath"
)

import (
	"github.com/bfenetworks/bfe/bfe_http"
)

const (
	EncodeGzip   = "gzip"
	EncodeBrotil = "br"
)

const (
	FileExtensionGzip   = "gz"
	FileExtensionBrotil = "br"
)

var (
	errUnexpectedDir = errors.New("file type should not be dir")
)

func ConvertEncodeToExt(encoding string) string {
	switch encoding {
	case EncodeGzip:
		return FileExtensionGzip
	case EncodeBrotil:
		return FileExtensionBrotil
	default:
		return encoding
	}
}

func CheckAcceptEncoding(req *bfe_http.Request) []string {
	encodingList := make([]string, 0)
	acceptEncoding := req.Header.GetDirect("Accept-Encoding")
	if bfe_http.HasToken(acceptEncoding, EncodeGzip) {
		encodingList = append(encodingList, EncodeGzip)
	}
	if bfe_http.HasToken(acceptEncoding, EncodeBrotil) {
		encodingList = append(encodingList, EncodeBrotil)
	}

	return encodingList
}

type staticFile struct {
	http.File
	os.FileInfo
	extension string
	encoding  string
	m         *ModuleStatic
}

func newStaticFile(root string, filename string, encodingList []string, m *ModuleStatic) (*staticFile, error) {
	var err error
	s := new(staticFile)
	s.m = m
	s.extension = filepath.Ext(filename)

	for _, encoding := range encodingList {
		ext := ConvertEncodeToExt(encoding)
		if _, err := os.Stat(filepath.Join(root, filename+"."+ext)); err == nil {
			filename = filename + "." + ext
			s.encoding = encoding
			break
		}
	}

	s.File, err = http.Dir(root).Open(filename)
	if err != nil {
		return nil, err
	}

	s.FileInfo, err = s.File.Stat()
	if err != nil {
		s.File.Close()
		return nil, err
	}

	if s.FileInfo.IsDir() {
		s.File.Close()
		return nil, errUnexpectedDir
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
	state.FileCurrentOpened.Dec(1)
	return nil
}

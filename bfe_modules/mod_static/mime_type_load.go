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
	"os"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type MimeType map[string]string

type MimeTypeConf struct {
	Version string
	Config  MimeType
}

func MimeTypeConfCheck(mimeTypeConf MimeTypeConf) error {
	if len(mimeTypeConf.Version) == 0 {
		return fmt.Errorf("no Version")
	}

	return nil
}

func MimeTypeConfConvert(mimeTypeConf *MimeTypeConf) {
	mimeType := make(MimeType)
	for k, v := range mimeTypeConf.Config {
		mimeType[strings.ToLower(k)] = v
	}

	mimeTypeConf.Config = mimeType
}

func MimeTypeConfLoad(filename string) (MimeTypeConf, error) {
	var mimeTypeConf MimeTypeConf

	file, err := os.Open(filename)
	if err != nil {
		return mimeTypeConf, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&mimeTypeConf)
	if err != nil {
		return mimeTypeConf, err
	}

	err = MimeTypeConfCheck(mimeTypeConf)
	if err != nil {
		return mimeTypeConf, err
	}

	MimeTypeConfConvert(&mimeTypeConf)

	return mimeTypeConf, err
}

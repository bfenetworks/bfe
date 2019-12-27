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

package mod_static

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

type MimeType struct {
	sync.Map
}

func (t *MimeType) UnmarshalJSON(data []byte) error {
	var types map[string]string
	if err := json.Unmarshal(data, &types); err != nil {
		return err
	}

	for k, v := range types {
		t.Store(strings.ToLower(k), v)
	}

	return nil
}

func MimeTypeLoad(filename string) (*MimeType, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	var mimeType MimeType
	err = decoder.Decode(&mimeType)
	return &mimeType, err
}

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
	"sync"
)

type MimeTypeTable struct {
	lock     sync.RWMutex
	version  string
	mimeType MimeType
}

func NewMimeTypeTable() *MimeTypeTable {
	t := new(MimeTypeTable)
	t.mimeType = make(MimeType)
	return t
}

func (t *MimeTypeTable) Update(conf MimeTypeConf) {
	t.lock.Lock()
	t.version = conf.Version
	t.mimeType = conf.Config
	t.lock.Unlock()
}

func (t *MimeTypeTable) Search(key string) (string, bool) {
	t.lock.RLock()
	mimeType := t.mimeType
	t.lock.RUnlock()

	value, ok := mimeType[key]
	return value, ok
}

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

package mod_markdown

import (
	"fmt"
)

import (
	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
)

func Render(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, fmt.Errorf("Render(): render empty src")
	}
	dst := render(src)
	return dst, nil
}

func render(src []byte) []byte {
	unsafe := blackfriday.Run(src)
	html := bluemonday.UGCPolicy().SanitizeBytes(unsafe)
	return html
}

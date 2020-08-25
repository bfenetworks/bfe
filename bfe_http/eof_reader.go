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

// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bfe_http

import (
	"io"
	"io/ioutil"
	"strings"
)

// EofReader is a non-nil io.ReadCloser that always returns EOF.
// It embeds a *strings.Reader so it still has a WriteTo method
// and io.Copy won't need a buffer.
var EofReader = &struct {
	*strings.Reader
	io.Closer
}{
	strings.NewReader(""),
	ioutil.NopCloser(nil),
}

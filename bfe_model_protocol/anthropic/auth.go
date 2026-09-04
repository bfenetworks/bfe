// Copyright (c) 2026 The BFE Authors.
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

package anthropic

import (
	"github.com/bfenetworks/bfe/bfe_http"
)

// InjectAuth writes the upstream credential as the x-api-key header. An
// empty key is a no-op.
func (a *Adapter) InjectAuth(outreq *bfe_http.Request, key string) error {
	if key == "" {
		return nil
	}
	outreq.Header.Set("x-api-key", key)
	return nil
}

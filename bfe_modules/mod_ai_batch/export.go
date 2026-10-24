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

package mod_ai_batch

// Cross-module helpers (used by mod_ai_rate_limit). All are fail-open and
// safe to call when the module is not configured.

// batchActiveWindowSec is the in-flight window for active-batch counting:
// the 24h provider completion window.
const batchActiveWindowSec = 86400

// BatchActiveCount returns the number of in-flight batches of the apikey
// (entries older than the 24h completion window are purged first).
func BatchActiveCount(apiKeyId string) int64 {
	m := defaultBatchModule
	if m == nil {
		return 0
	}
	return m.activeCount(apiKeyId, batchActiveWindowSec)
}

// BatchGlobalFileCeilings returns the global hard ceilings
// (maxFileBytes, maxFileLines); 0 or negative means the ceiling is disabled.
func BatchGlobalFileCeilings() (int64, int64) {
	m := defaultBatchModule
	if m == nil {
		return 0, 0
	}
	data := m.getData()
	return data.MaxFileBytes, data.MaxFileLines
}

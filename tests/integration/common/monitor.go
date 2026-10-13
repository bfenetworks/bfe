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

package common

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ModuleMetricsRaw fetches the prometheus export of a module's counters from
// the BFE web monitor, e.g. ModuleMetricsRaw(port, "mod_ai_token_auth").
func ModuleMetricsRaw(port int, module string) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/monitor/%s?format=prometheus", port, module)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("get %s monitor failed: %w", module, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s monitor status = %d, body: %s", module, resp.StatusCode, data)
	}
	return string(data), nil
}

// ModuleMetric returns the value of one module counter (0 when absent).
//
// The web-monitor prometheus export renders the counter field names in
// SNAKE_CASE, so callers pass the Go field name of the counter (e.g.
// "BillingZeroCost", which is exported as
// "mod_ai_token_auth_BILLING_ZERO_COST").
func ModuleMetric(port int, module, field string) (float64, error) {
	data, err := ModuleMetricsRaw(port, module)
	if err != nil {
		return 0, err
	}

	fullName := module + "_" + CamelToSnake(field)
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.LastIndex(line, " ")
		if idx < 0 || !strings.EqualFold(strings.TrimSpace(line[:idx]), fullName) {
			continue
		}
		// The value is the first numeric token after the metric name (a
		// trailing timestamp is tolerated and ignored).
		for _, tok := range strings.Fields(line[idx+1:]) {
			v, perr := strconv.ParseFloat(tok, 64)
			if perr == nil {
				return v, nil
			}
		}
	}
	return 0, nil
}

// WaitForModuleMetric polls a module counter until cond is satisfied or the
// timeout elapses, and returns the last observed value. The monitor handler
// reads the counters on demand, so the poll only guards the race between a
// request finishing and the assertion.
func WaitForModuleMetric(port int, module, field string, timeout time.Duration,
	cond func(float64) bool) (float64, error) {

	deadline := time.Now().Add(timeout)
	var (
		last float64
		err  error
	)
	for {
		last, err = ModuleMetric(port, module, field)
		if err != nil {
			return last, err
		}
		if cond(last) || !time.Now().Before(deadline) {
			return last, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// CamelToSnake converts a CamelCase counter field name to the SNAKE_CASE key
// used by the web-monitor prometheus export (e.g. "UsageUnrecognized" ->
// "USAGE_UNRECOGNIZED").
func CamelToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return strings.ToUpper(b.String())
}

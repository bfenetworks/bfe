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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// SimBinary locates the llm-d-inference-sim binary: $INFERENCE_SIM_BIN, then
// the sibling checkout at <bfe-root>/../llm-d-inference-sim/bin, then
// tests/integration/.sim-bin/. Returns "" when unavailable (tests should
// t.Skip).
func SimBinary(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("INFERENCE_SIM_BIN"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	root, err := locateBFESourceRoot()
	if err != nil {
		t.Fatalf("locate bfe source root failed: %v", err)
	}
	names := []string{"llm-d-inference-sim"}
	if runtime.GOOS == "windows" {
		names = []string{"llm-d-inference-sim.exe", "llm-d-inference-sim"}
	}
	for _, name := range names {
		for _, p := range []string{
			filepath.Join(root, "..", "llm-d-inference-sim", "bin", name),
			filepath.Join(root, "tests", "integration", ".sim-bin", name),
		} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// StartInferenceSim starts one llm-d-inference-sim instance on a free loopback
// port in echo mode with near-zero simulated latency, and waits for its HTTP
// port. It returns the server address (host:port) and a stop function.
//
// If the inference-sim binary is not found the test is skipped (the sim is an
// external optional dependency).
func StartInferenceSim(t *testing.T, logDir, name, model string) (addr string, stop func()) {
	t.Helper()
	return StartInferenceSimWithArgs(t, logDir, name, model)
}

// StartInferenceSimWithArgs is StartInferenceSim with extra simulator
// arguments appended after the standard ones.
func StartInferenceSimWithArgs(t *testing.T, logDir, name, model string, extraArgs ...string) (addr string, stop func()) {
	t.Helper()
	bin := SimBinary(t)
	if bin == "" {
		t.Skip("llm-d-inference-sim binary not found (set INFERENCE_SIM_BIN or build ../llm-d-inference-sim)")
	}

	port, err := FindFreePort()
	if err != nil {
		t.Fatalf("find free port for inference-sim failed: %v", err)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("create log dir failed: %v", err)
	}
	logFile, err := os.Create(filepath.Join(logDir, name+".log"))
	if err != nil {
		t.Fatalf("create sim log file failed: %v", err)
	}

	args := append([]string{
		"--port", fmt.Sprint(port),
		"--model", model,
		"--mode", "echo",
		"--time-to-first-token", "2ms",
		"--inter-token-latency", "1ms",
	}, extraArgs...)
	cmd := exec.Command(bin, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		t.Fatalf("start inference-sim failed: %v", err)
	}

	addr = fmt.Sprintf("127.0.0.1:%d", port)
	if err := WaitForTCP(addr, 30*time.Second); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		logFile.Close()
		t.Fatalf("inference-sim %s not ready: %v (log: %s)", name, err, logFile.Name())
	}

	stop = func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		logFile.Close()
	}
	return addr, stop
}

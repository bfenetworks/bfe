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
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// ProcessEnv manages building and running a real BFE process for integration tests.
type ProcessEnv struct {
	t *testing.T

	// sourceRoot is the absolute path to the bfe module root.
	sourceRoot string
	// binDir is the directory where the compiled BFE binary is cached.
	binDir string
	// workDir is a per-test temporary directory.
	workDir string

	bfeBinaryPath string

	buildOnce sync.Once
}

// NewProcessEnv creates a ProcessEnv for the current test.
func NewProcessEnv(t *testing.T) *ProcessEnv {
	root, err := locateBFESourceRoot()
	if err != nil {
		t.Fatalf("locate bfe source root failed: %v", err)
	}
	return &ProcessEnv{
		t:          t,
		sourceRoot: root,
		binDir:     filepath.Join(root, "tests", "integration", ".integration-test-bin"),
		workDir:    t.TempDir(),
	}
}

// WorkDir returns the per-test temporary directory.
func (p *ProcessEnv) WorkDir() string { return p.workDir }

// SourceRoot returns the absolute path to the bfe source root.
func (p *ProcessEnv) SourceRoot() string { return p.sourceRoot }

// locateBFESourceRoot walks up from this file to find the directory whose
// go.mod declares module "github.com/bfenetworks/bfe".
func locateBFESourceRoot() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot get current file path")
	}
	dir := filepath.Dir(filename)
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(goModPath); err == nil {
			if strings.Contains(string(data), "module github.com/bfenetworks/bfe") {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("cannot locate bfe source root go.mod")
}

// Build compiles the BFE binary if not already cached.
func (p *ProcessEnv) Build() {
	p.buildOnce.Do(func() {
		if err := os.MkdirAll(p.binDir, 0755); err != nil {
			p.t.Fatalf("create bin dir failed: %v", err)
		}

		binName := "bfe"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		binPath := filepath.Join(p.binDir, fmt.Sprintf("bfe-%s-%s-%s", runtime.GOOS, runtime.GOARCH, binName))

		if _, err := os.Stat(binPath); err == nil {
			p.t.Logf("use cached bfe binary: %s", binPath)
			p.bfeBinaryPath = binPath
			return
		}

		p.t.Logf("building bfe binary from %s ...", p.sourceRoot)
		cmd := exec.Command("go", "build", "-o", binPath, ".")
		cmd.Dir = p.sourceRoot
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			p.t.Fatalf("build bfe failed: %v", err)
		}
		p.t.Logf("built bfe -> %s", binPath)
		p.bfeBinaryPath = binPath
	})
}

// StartBFE starts a real BFE process with the given conf root and log dir.
// It returns the HTTP port, the monitor port and a teardown function.
func (p *ProcessEnv) StartBFE(confDir, logDir string) (int, int, func()) {
	httpPort, err := FindFreePort()
	if err != nil {
		p.t.Fatalf("find free port for bfe http failed: %v", err)
	}
	httpsPort, err := FindFreePort()
	if err != nil {
		p.t.Fatalf("find free port for bfe https failed: %v", err)
	}
	monitorPort, err := FindFreePort()
	if err != nil {
		p.t.Fatalf("find free port for bfe monitor failed: %v", err)
	}

	if err := RewriteBFEPorts(filepath.Join(confDir, "bfe.conf"), httpPort, httpsPort, monitorPort); err != nil {
		p.t.Fatalf("rewrite bfe ports failed: %v", err)
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		p.t.Fatalf("create bfe log dir failed: %v", err)
	}

	cmd := exec.Command(p.bfeBinaryPath, "-c", confDir, "-l", logDir, "-s")
	cmd.Dir = confDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		p.t.Fatalf("start bfe failed: %v", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", httpPort)
	if err := WaitForTCP(addr, 30*time.Second); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		p.t.Fatalf("bfe did not start in time: %v", err)
	}

	stop := func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		time.Sleep(50 * time.Millisecond)
	}
	return httpPort, monitorPort, stop
}

// FindFreePort returns a free TCP port on localhost.
func FindFreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// WaitForTCP waits until the given TCP address is reachable.
func WaitForTCP(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}

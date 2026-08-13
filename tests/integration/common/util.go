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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// copyDirContents recursively copies files and directories from src to dst.
func copyDirContents(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return err
			}
			if err := copyDirContents(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// GetBFETotalBytesBodyBuffer queries the BFE monitor endpoint for the current
// total bytes_body buffer size.
func GetBFETotalBytesBodyBuffer(monitorPort int) (int64, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/monitor/server_stat", monitorPort)
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var stat struct {
		TotalBytesBodyBuffer int64 `json:"total_bytes_body_buffer"`
	}
	if err := json.Unmarshal(data, &stat); err != nil {
		return 0, err
	}
	return stat.TotalBytesBodyBuffer, nil
}

// clusterSubName maps a cluster name to the sub-cluster name used in gslb.data.
func clusterSubName(clusterName string) string {
	switch clusterName {
	case "cluster_primary_a":
		return "a"
	case "cluster_primary_b":
		return "b"
	case "cluster_primary_c":
		return "c"
	case "cluster_fallback_1":
		return "fb1"
	case "cluster_fallback_2":
		return "fb2"
	case "cluster_entity_default":
		return "entity"
	case "cluster_global_default":
		return "global"
	case "cluster_holder":
		return "holder"
	case "cluster_multi_key":
		return "multi"
	case "cluster_fallback_ok":
		return "fb"
	case "cluster_rmb":
		return "rmb"
	case "cluster_no_table":
		return "notable"
	case "cluster_fallback_rmb":
		return "fallback_rmb"
	}
	return "sub"
}

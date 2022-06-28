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

package bfe_util

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
)

// CopyFile copy src file to dst file.
// return file length, error
func CopyFile(src, dst string) (int64, error) {
	srcFile, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("open src error %s", err)
	}
	defer srcFile.Close()

	srcFileStat, err := srcFile.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat src error %s", err)
	}

	if !srcFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	// mkdir all dir
	dirPath := path.Dir(dst)
	if err = os.MkdirAll(dirPath, 0755); err != nil {
		return 0, fmt.Errorf("MkdirALl err %s", err.Error())
	}

	// create file
	dstFile, err := os.Create(dst)
	if err != nil {
		return 0, fmt.Errorf("create dst error %s", dst)
	}
	defer dstFile.Close()

	return io.Copy(dstFile, srcFile)
}

// BackupFile backup given file.
func BackupFile(path string, bakPath string) error {
	// write to a temp file
	copyPath := fmt.Sprintf("%s.%d.bak", path, os.Getpid())
	if _, err := CopyFile(path, copyPath); err != nil {
		return err
	}

	// rename temp file
	if err := os.Rename(copyPath, bakPath); err != nil {
		return err
	}

	return nil
}

// CheckStaticFile check local file
func CheckStaticFile(filename string, sizeLimit int64) error {
	stat, err := os.Stat(filename)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return fmt.Errorf("%s is not regular file", filename)
	}
	if stat.Size() > sizeLimit {
		return fmt.Errorf("%s file size too large[> %d]", filename, sizeLimit)
	}
	if _, err := ioutil.ReadFile(filename); err != nil {
		return fmt.Errorf("read %s: %s", filename, err)
	}
	return nil
}

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

package access_log

import (
	"io/ioutil"
	"testing"
)

import (
	"github.com/stretchr/testify/assert"
)

func TestCheckLogConfig(t *testing.T) {
	var c LogConfig
	confRoot, err := ioutil.TempDir("", "test_check_log_config*")
	assert.NoError(t, err)
	c.LogFile = "test_file"
	err = c.Check(confRoot)
	assert.NoError(t, err)
	c.LogDir = "test_dir"
	err = c.Check(confRoot)
	assert.Error(t, err)
}

func TestFileLogger(t *testing.T) {
	var c LogConfig
	c.LogFile = "test_file"
	confRoot, err := ioutil.TempDir("", "test_file_logger*")
	assert.NoError(t, err)
	err = c.Check(confRoot)
	assert.NoError(t, err)
	logger, err := LoggerInit(c)
	assert.NoError(t, err)
	content := "test_log"
	logger.Info(content)
	// close and flush
	logger.Close()
	logContent, err := ioutil.ReadFile(c.LogFile)
	assert.NoError(t, err)
	assert.Equal(t, content+"\n", string(logContent))
}

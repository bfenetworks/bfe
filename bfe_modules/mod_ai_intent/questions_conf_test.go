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

package mod_ai_intent

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testQuestionsValid = `{
  "Version": "2026092601",
  "MinConfidence": 0.6,
  "Questions": [
    {
      "Name": "task_type",
      "Type": "choice",
      "Instructions": "这条请求属于哪类研发任务？",
      "Criteria": {
        "coding": "编写或修改代码",
        "test_writing": "编写测试用例"
      }
    },
    {
      "Name": "complexity",
      "Type": "score",
      "Instructions": "这个任务的复杂度如何？",
      "MinConfidence": 0.7,
      "Levels": [
        {"Name": "simple", "Description": "单步即可完成"},
        {"Name": "medium", "Description": "多步但模式常见"},
        {"Name": "complex", "Description": "需要深入推理"}
      ]
    }
  ]
}`

func writeQuestionsFile(t *testing.T, content string) string {
	dir := t.TempDir()
	path := filepath.Join(dir, "intent_questions.data")
	require.NoError(t, ioutil.WriteFile(path, []byte(content), 0644))
	return path
}

func TestQuestionsConfLoadValid(t *testing.T) {
	path := writeQuestionsFile(t, testQuestionsValid)
	qc := NewQuestionsConf(path)
	require.NoError(t, qc.Load())

	qs := qc.Current()
	require.NotNil(t, qs)
	assert.Equal(t, "2026092601", qs.Version())

	// per-question override wins; others inherit the global gate;
	// unconfigured questions get 0
	assert.Equal(t, 0.6, qs.Threshold("task_type"))
	assert.Equal(t, 0.7, qs.Threshold("complexity"))
	assert.Equal(t, 0.0, qs.Threshold("not_configured"))

	// option validation
	q, ok := qs.Question("task_type")
	require.True(t, ok)
	assert.True(t, q.HasOption("coding"))
	assert.False(t, q.HasOption("nope"))
	assert.False(t, q.HasOption(""))

	// System One request assembly
	so := qs.SystemOneQuestions()
	require.Len(t, so, 2)
	choice, ok := so["task_type"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "choice", choice["type"])
	assert.Equal(t, "这条请求属于哪类研发任务？", choice["instructions"])
	criteria, ok := choice["criteria"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "编写测试用例", criteria["test_writing"])

	score, ok := so["complexity"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "score", score["type"])
	descs, ok := score["criteria"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"单步即可完成", "多步但模式常见", "需要深入推理"}, descs)
}

func TestQuestionsConfValidation(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"missing version", `{"Questions": [{"Name":"q","Type":"choice","Instructions":"i","Criteria":{"a":"b"}}]}`},
		{"empty version", `{"Version":"","Questions": [{"Name":"q","Type":"choice","Instructions":"i","Criteria":{"a":"b"}}]}`},
		{"global min conf too large", `{"Version":"v1","MinConfidence":1.1,"Questions": [{"Name":"q","Type":"choice","Instructions":"i","Criteria":{"a":"b"}}]}`},
		{"global min conf negative", `{"Version":"v1","MinConfidence":-0.1,"Questions": [{"Name":"q","Type":"choice","Instructions":"i","Criteria":{"a":"b"}}]}`},
		{"empty question name", `{"Version":"v1","Questions": [{"Name":"","Type":"choice","Instructions":"i","Criteria":{"a":"b"}}]}`},
		{"duplicated question name", `{"Version":"v1","Questions": [{"Name":"q","Type":"choice","Instructions":"i","Criteria":{"a":"b"}},{"Name":"q","Type":"choice","Instructions":"i","Criteria":{"a":"b"}}]}`},
		{"invalid question type", `{"Version":"v1","Questions": [{"Name":"q","Type":"noul","Instructions":"i","Criteria":{"a":"b"}}]}`},
		{"empty instructions", `{"Version":"v1","Questions": [{"Name":"q","Type":"choice","Instructions":"","Criteria":{"a":"b"}}]}`},
		{"choice with levels", `{"Version":"v1","Questions": [{"Name":"q","Type":"choice","Instructions":"i","Criteria":{"a":"b"},"Levels":[{"Name":"l","Description":"d"}]}]}`},
		{"choice empty criteria", `{"Version":"v1","Questions": [{"Name":"q","Type":"choice","Instructions":"i","Criteria":{}}]}`},
		{"choice option contains separator", `{"Version":"v1","Questions": [{"Name":"q","Type":"choice","Instructions":"i","Criteria":{"a|b":"d"}}]}`},
		{"choice empty option name", `{"Version":"v1","Questions": [{"Name":"q","Type":"choice","Instructions":"i","Criteria":{"":"d"}}]}`},
		{"score with criteria", `{"Version":"v1","Questions": [{"Name":"q","Type":"score","Instructions":"i","Criteria":{"a":"b"},"Levels":[{"Name":"l","Description":"d"}]}]}`},
		{"score empty levels", `{"Version":"v1","Questions": [{"Name":"q","Type":"score","Instructions":"i","Levels":[]}]}`},
		{"score duplicated level name", `{"Version":"v1","Questions": [{"Name":"q","Type":"score","Instructions":"i","Levels":[{"Name":"l","Description":"d"},{"Name":"l","Description":"d2"}]}]}`},
		{"score empty level name", `{"Version":"v1","Questions": [{"Name":"q","Type":"score","Instructions":"i","Levels":[{"Name":"","Description":"d"}]}]}`},
		{"question min conf too large", `{"Version":"v1","Questions": [{"Name":"q","Type":"choice","Instructions":"i","MinConfidence":1.5,"Criteria":{"a":"b"}}]}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeQuestionsFile(t, c.content)
			qc := NewQuestionsConf(path)
			err := qc.Load()
			assert.Error(t, err)
		})
	}
}

func TestQuestionsConfHotReload(t *testing.T) {
	path := writeQuestionsFile(t, testQuestionsValid)
	qc := NewQuestionsConf(path)
	require.NoError(t, qc.Load())

	snap := qc.Current()
	require.NotNil(t, snap)

	// valid new version: applied atomically; the old snapshot stays intact
	v2 := `{
	  "Version": "2026092602",
	  "MinConfidence": 0.4,
	  "Questions": [
	    {"Name": "task_type", "Type": "choice", "Instructions": "i", "Criteria": {"coding": "d"}}
	  ]
	}`
	require.NoError(t, ioutil.WriteFile(path, []byte(v2), 0644))
	require.NoError(t, qc.Load())
	assert.Equal(t, "2026092602", qc.Current().Version())
	assert.Equal(t, 0.4, qc.Threshold("task_type"))
	assert.Equal(t, "2026092601", snap.Version(), "old snapshot must stay immutable")

	// invalid new version: rejected, old version kept
	broken := `{"Version": "2026092603", "Questions": [{"Name": "q"}]}`
	require.NoError(t, ioutil.WriteFile(path, []byte(broken), 0644))
	assert.Error(t, qc.Load())
	assert.Equal(t, "2026092602", qc.Current().Version())

	// unchanged version: no-op even if content differs
	sameVersion := `{
	  "Version": "2026092602",
	  "MinConfidence": 0.9,
	  "Questions": [
	    {"Name": "task_type", "Type": "choice", "Instructions": "i", "Criteria": {"coding": "d"}}
	  ]
	}`
	require.NoError(t, ioutil.WriteFile(path, []byte(sameVersion), 0644))
	require.NoError(t, qc.Load())
	assert.Equal(t, 0.4, qc.Threshold("task_type"), "unchanged version must not be applied")
}

func TestQuestionsConfThresholdFallback(t *testing.T) {
	// no global MinConfidence: default 0.6
	content := `{
	  "Version": "v1",
	  "Questions": [
	    {"Name": "q", "Type": "choice", "Instructions": "i", "Criteria": {"a": "b"}}
	  ]
	}`
	path := writeQuestionsFile(t, content)
	qc := NewQuestionsConf(path)
	require.NoError(t, qc.Load())
	assert.Equal(t, DefaultMinConfidence, qc.Threshold("q"))
	assert.Equal(t, 0.0, qc.Threshold("missing"))
}

func TestQuestionsConfEmptyQuestions(t *testing.T) {
	// empty Questions is the soft switch that disables intent classification
	content := `{"Version": "off1", "MinConfidence": 0.6, "Questions": []}`
	path := writeQuestionsFile(t, content)
	qc := NewQuestionsConf(path)
	require.NoError(t, qc.Load())

	qs := qc.Current()
	require.NotNil(t, qs)
	assert.Equal(t, "off1", qs.Version())
	assert.Empty(t, qs.OrderedQuestions())
	assert.Empty(t, qs.SystemOneQuestions())
	assert.Equal(t, 0.0, qs.Threshold("anything"))

	// hot reload from empty to non-empty (and back) works with version bumps
	on := `{"Version": "on1", "Questions": [{"Name": "q", "Type": "choice", "Instructions": "i", "Criteria": {"a": "b"}}]}`
	require.NoError(t, ioutil.WriteFile(path, []byte(on), 0644))
	require.NoError(t, qc.Load())
	assert.Len(t, qs.OrderedQuestions(), 0, "old snapshot stays immutable")
	assert.Len(t, qc.Current().OrderedQuestions(), 1)

	require.NoError(t, ioutil.WriteFile(path, []byte(content), 0644))
	require.NoError(t, qc.Load())
	assert.Empty(t, qc.Current().OrderedQuestions())
}

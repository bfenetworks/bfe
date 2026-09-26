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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"sync/atomic"

	"github.com/bfenetworks/bfe/bfe_basic"
)

const (
	DefaultMinConfidence = 0.6

	minQuestionOptions = 1
	maxQuestionOptions = 255
)

// noConfidenceOverride marks a question without per-question MinConfidence.
const noConfidenceOverride = -1.0

// question types (kept in sync with bfe_basic.IntentQType*)
const (
	QuestionTypeChoice = bfe_basic.IntentQTypeChoice
	QuestionTypeScore  = bfe_basic.IntentQTypeScore
)

// QuestionLevel is one level of a score question, ordered low to high.
type QuestionLevel struct {
	Name        string `json:"Name"`
	Description string `json:"Description"`
}

// QuestionFile is the JSON DTO of a single question in intent_questions.data.
type QuestionFile struct {
	Name          string            `json:"Name"`
	Type          string            `json:"Type"`
	Instructions  string            `json:"Instructions"`
	Criteria      map[string]string `json:"Criteria"` // choice options: name -> description
	Levels        []QuestionLevel   `json:"Levels"`   // score levels, ordered low to high
	MinConfidence *float64          `json:"MinConfidence"`
}

// QuestionsFile is the JSON DTO of intent_questions.data.
type QuestionsFile struct {
	Version       string         `json:"Version"`
	MinConfidence *float64       `json:"MinConfidence"`
	Questions     []QuestionFile `json:"Questions"`
}

// IntentQuestion is the runtime representation of a question.
type IntentQuestion struct {
	Name          string
	Type          string
	Instructions  string
	Criteria      map[string]string // choice: option name -> description
	LevelNames    []string          // score: level names, ordered low to high
	LevelDescs    []string          // score: level descriptions, ordered low to high
	MinConfidence float64           // per-question gate, noConfidenceOverride to inherit
}

// HasOption reports whether option is a valid option/level name of the question.
func (q *IntentQuestion) HasOption(option string) bool {
	if option == "" {
		return false
	}
	if q.Type == QuestionTypeChoice {
		_, ok := q.Criteria[option]
		return ok
	}
	for _, name := range q.LevelNames {
		if name == option {
			return true
		}
	}
	return false
}

// IntentQuestions is the runtime, immutable snapshot of the questions conf.
// It is swapped atomically on hot reload, so readers never see a partial conf.
type IntentQuestions struct {
	version       string
	minConfidence float64
	order         []string // question names in file order
	questions     map[string]*IntentQuestion
}

func (qs *IntentQuestions) Version() string {
	return qs.version
}

func (qs *IntentQuestions) Question(name string) (*IntentQuestion, bool) {
	q, ok := qs.questions[name]
	return q, ok
}

// OrderedQuestions returns questions in file order (stable request assembly).
func (qs *IntentQuestions) OrderedQuestions() []*IntentQuestion {
	ret := make([]*IntentQuestion, 0, len(qs.order))
	for _, name := range qs.order {
		ret = append(ret, qs.questions[name])
	}
	return ret
}

// Threshold returns the current effective confidence gate of a question:
// per-question override, then the global gate. Unconfigured questions get 0,
// i.e. only the presence of an answer is required.
func (qs *IntentQuestions) Threshold(question string) float64 {
	q, ok := qs.questions[question]
	if !ok {
		return 0
	}
	if q.MinConfidence != noConfidenceOverride {
		return q.MinConfidence
	}
	return qs.minConfidence
}

// SystemOneQuestions assembles the questions field of the decision service
// request: choice questions carry a name->description criteria map; score
// questions carry the ordered level descriptions.
func (qs *IntentQuestions) SystemOneQuestions() map[string]interface{} {
	ret := make(map[string]interface{}, len(qs.questions))
	for _, name := range qs.order {
		q := qs.questions[name]
		switch q.Type {
		case QuestionTypeChoice:
			ret[name] = map[string]interface{}{
				"type":         q.Type,
				"instructions": q.Instructions,
				"criteria":     q.Criteria,
			}
		case QuestionTypeScore:
			ret[name] = map[string]interface{}{
				"type":         q.Type,
				"instructions": q.Instructions,
				"criteria":     q.LevelDescs,
			}
		}
	}
	return ret
}

// validateQuestionsFile validates the parsed file and converts it into the
// runtime representation.
func validateQuestionsFile(f *QuestionsFile) (*IntentQuestions, error) {
	if f.Version == "" {
		return nil, fmt.Errorf("Version is empty")
	}

	minConfidence := DefaultMinConfidence
	if f.MinConfidence != nil {
		if *f.MinConfidence < 0 || *f.MinConfidence > 1 {
			return nil, fmt.Errorf("MinConfidence %f out of range [0,1]", *f.MinConfidence)
		}
		minConfidence = *f.MinConfidence
	}

	if len(f.Questions) == 0 {
		return nil, fmt.Errorf("Questions is empty")
	}

	qs := &IntentQuestions{
		version:       f.Version,
		minConfidence: minConfidence,
		order:         make([]string, 0, len(f.Questions)),
		questions:     make(map[string]*IntentQuestion, len(f.Questions)),
	}

	for i := range f.Questions {
		qf := &f.Questions[i]
		if qf.Name == "" {
			return nil, fmt.Errorf("Questions[%d]: Name is empty", i)
		}
		if _, exist := qs.questions[qf.Name]; exist {
			return nil, fmt.Errorf("Questions[%d]: Name %s duplicated", i, qf.Name)
		}
		if qf.Type != QuestionTypeChoice && qf.Type != QuestionTypeScore {
			return nil, fmt.Errorf("Questions[%d]: invalid Type %s", i, qf.Type)
		}
		if qf.Instructions == "" {
			return nil, fmt.Errorf("Questions[%d]: Instructions is empty", i)
		}
		if qf.MinConfidence != nil && (*qf.MinConfidence < 0 || *qf.MinConfidence > 1) {
			return nil, fmt.Errorf("Questions[%d]: MinConfidence %f out of range [0,1]", i, *qf.MinConfidence)
		}

		q := &IntentQuestion{
			Name:          qf.Name,
			Type:          qf.Type,
			Instructions:  qf.Instructions,
			MinConfidence: noConfidenceOverride,
		}
		if qf.MinConfidence != nil {
			q.MinConfidence = *qf.MinConfidence
		}

		switch qf.Type {
		case QuestionTypeChoice:
			if len(qf.Levels) != 0 {
				return nil, fmt.Errorf("Questions[%d]: Levels not allowed for choice", i)
			}
			n := len(qf.Criteria)
			if n < minQuestionOptions || n > maxQuestionOptions {
				return nil, fmt.Errorf("Questions[%d]: Criteria count %d out of range [%d,%d]",
					i, n, minQuestionOptions, maxQuestionOptions)
			}
			for name := range qf.Criteria {
				if name == "" {
					return nil, fmt.Errorf("Questions[%d]: empty option name", i)
				}
				if strings.Contains(name, "|") {
					return nil, fmt.Errorf("Questions[%d]: option name %s contains '|'", i, name)
				}
			}
			q.Criteria = qf.Criteria
		case QuestionTypeScore:
			if len(qf.Criteria) != 0 {
				return nil, fmt.Errorf("Questions[%d]: Criteria not allowed for score", i)
			}
			n := len(qf.Levels)
			if n < minQuestionOptions || n > maxQuestionOptions {
				return nil, fmt.Errorf("Questions[%d]: Levels count %d out of range [%d,%d]",
					i, n, minQuestionOptions, maxQuestionOptions)
			}
			seen := make(map[string]bool, n)
			q.LevelNames = make([]string, 0, n)
			q.LevelDescs = make([]string, 0, n)
			for j, level := range qf.Levels {
				if level.Name == "" {
					return nil, fmt.Errorf("Questions[%d].Levels[%d]: Name is empty", i, j)
				}
				if seen[level.Name] {
					return nil, fmt.Errorf("Questions[%d].Levels[%d]: Name %s duplicated", i, j, level.Name)
				}
				seen[level.Name] = true
				q.LevelNames = append(q.LevelNames, level.Name)
				q.LevelDescs = append(q.LevelDescs, level.Description)
			}
		}

		qs.questions[q.Name] = q
		qs.order = append(qs.order, q.Name)
	}

	return qs, nil
}

// QuestionsConf holds the current questions conf snapshot with atomic
// replacement, plus its source path for hot reload.
type QuestionsConf struct {
	path    string
	current atomic.Value // *IntentQuestions
}

func NewQuestionsConf(path string) *QuestionsConf {
	qc := &QuestionsConf{path: path}
	qc.current.Store(&IntentQuestions{questions: make(map[string]*IntentQuestion)})
	return qc
}

func (qc *QuestionsConf) Path() string {
	return qc.path
}

// Load reloads the questions conf from the file (or from path if given,
// which also becomes the new source path of the conf). The new conf is
// validated first; a hot reload with an unchanged Version is a no-op.
func (qc *QuestionsConf) Load(path ...string) error {
	if len(path) > 0 && path[0] != "" {
		qc.path = path[0]
	}
	if qc.path == "" {
		return fmt.Errorf("questions conf path is empty")
	}

	data, err := ioutil.ReadFile(qc.path)
	if err != nil {
		return err
	}

	var f QuestionsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("parse questions conf %s err: %s", qc.path, err)
	}

	qs, err := validateQuestionsFile(&f)
	if err != nil {
		return err
	}

	cur := qc.Current()
	if cur != nil && cur.Version() == qs.Version() {
		return nil // unchanged version: nothing to do
	}

	qc.current.Store(qs)
	return nil
}

func (qc *QuestionsConf) Current() *IntentQuestions {
	if v := qc.current.Load(); v != nil {
		return v.(*IntentQuestions)
	}
	return nil
}

// Threshold returns the current effective confidence gate of a question.
func (qc *QuestionsConf) Threshold(question string) float64 {
	if cur := qc.Current(); cur != nil {
		return cur.Threshold(question)
	}
	return 0
}

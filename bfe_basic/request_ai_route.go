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

// ai route result context for request

package bfe_basic

const CtxAiRouteResult = "__REQ_AI_ROUTE_RESULT"

type AiRouteResult struct {
	RouteType string // apikey / entity / global
	Owner     string // route table owner
	RuleName  string // hit rule name
	Targets   []AiRouteTarget
	Fallbacks []AiRouteFallback
}

type AiRouteTarget struct {
	ClusterName string
	Model       string
	Weight      int
}

type AiRouteFallback struct {
	ClusterName string
	Model       string
}

func (r *Request) SetAiRouteResult(result *AiRouteResult) {
	r.SetContext(CtxAiRouteResult, result)
}

func (r *Request) GetAiRouteResult() *AiRouteResult {
	val := r.GetContext(CtxAiRouteResult)
	if val == nil {
		return nil
	}
	result, ok := val.(*AiRouteResult)
	if !ok {
		return nil
	}
	return result
}

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

package bns

import (
	"errors"
	"reflect"
	"testing"
)

func TestGetLocalName(t *testing.T) {
	client := NewClient()

	// local local name conf
	filename := "./testdata/name_conf.data"
	err := LoadLocalNameConf(filename)
	if err != nil {
		t.Errorf("LoadLocalNameConf error: %s", err)
		return
	}

	tests := []struct {
		Name      string
		Err       error
		Instances []Instance
	}{
		{
			"service.c1",
			nil,
			[]Instance{
				{"10.1.1.1", 8080, 10},
				{"10.1.1.2", 8080, 20},
			},
		},
		{
			"service.c2",
			nil,
			[]Instance{
				{"10.2.1.1", 8080, 10},
			},
		},
		{
			"service.c3",
			errors.New("GetInstances fail"),
			nil,
		},
	}

	// run cases
	for i, tt := range tests {
		instances, err := client.GetInstancesInfo(tt.Name)
		if tt.Err == nil {
			if !reflect.DeepEqual(instances, tt.Instances) {
				t.Errorf("case %d expect %v, got %v", i, tt.Instances, instances)
			}
			continue
		}
		if err == nil {
			t.Errorf("case %d expect error", i)
		}
	}
}

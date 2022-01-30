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
	"io/ioutil"
	"os"
	"path"
	"reflect"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/json"
)

// LoadJsonFile loads json content from file, unmarshal to jsonObject.
// check if all field is set if checkNilPointer is true
// check if any field is not pointer type if allowNoPointerField is false
func LoadJsonFile(path string, jsonObject interface{}) error {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(buf, jsonObject); err != nil {
		return err
	}

	return nil
}

// DumpJson dumps json file.
func DumpJson(jsonObject interface{}, filePath string, perm os.FileMode) error {
	buf, err := json.MarshalIndent(jsonObject, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal err %s", err)
	}

	// mkdir all dir
	dirPath := path.Dir(filePath)
	if err = os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("MkdirALl err %s", err.Error())
	}
	return ioutil.WriteFile(filePath, buf, perm)
}

// CheckNilField check if a struct has a nil field
// if allowNoPointerField is false, it also check if fields are all pointers
// if param object is not a struct , return nil
func CheckNilField(object interface{}, allowNoPointerField bool) error {
	v := reflect.ValueOf(object)
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("input is not struct")
	}

	typeOfV := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() != reflect.Ptr {
			if !allowNoPointerField {
				return fmt.Errorf("%s field %s is not a pointer", typeOfV, typeOfV.Field(i).Name)
			}
			continue
		}

		if f.IsNil() {
			return fmt.Errorf("%s field %s is not set", typeOfV, typeOfV.Field(i).Name)
		}
	}
	return nil
}

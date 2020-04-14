// Copyright (c) 2019 Baidu, Inc.
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

package config

import (
	"fmt"
	"os"
	"reflect"
)

// AssertsFile asserts whether a path was a valid file path or not
func AssertsFile(path string) (err error) {
	file, err := os.Stat(path)
	if err != nil {
		return err
	}

	if file.IsDir() {
		return fmt.Errorf("%s not a valid file path", path)
	}

	return nil
}

// MapKeys returns the keys of a map
func MapKeys(m map[string]interface{}) (keys []string) {
	keys = make([]string, 0, len(m))

	for key := range m {
		keys = append(keys, key)
	}

	return keys
}

// MapConvert converts map to struct
func MapConvert(m map[string]interface{}, v interface{}) (err error) {
	dst := reflect.ValueOf(v)

	if dst.Kind() != reflect.Ptr {
		return fmt.Errorf("target should be a pointer to struct")
	}

	dst = reflect.Indirect(dst)

	for i, l := 0, dst.NumField(); i < l; i++ {
		tField := dst.Type().Field(i)
		vField := dst.FieldByName(tField.Name)

		v, ok := m[tField.Name]
		if !(ok && vField.CanSet()) {
			continue
		}

		convertV := reflect.ValueOf(v)
		t0, t1 := convertV.Type(), vField.Type()
		if t0 != t1 {
			return fmt.Errorf("field %s: cannot read type %+v into %+v", tField.Name, t0, t1)
		}

		vField.Set(convertV)
	}
	return nil
}

// Contains returns whether v in target or not
func Contains(target []string, v string) bool {
	for _, v0 := range target {
		if v == v0 {
			return true
		}
	}

	return false
}

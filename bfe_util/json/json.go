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

package json

import (
	"io"
)

import (
	jsoniter "github.com/json-iterator/go"
)

func NewDecoder(reader io.Reader) *jsoniter.Decoder {
	return jsoniter.ConfigCompatibleWithStandardLibrary.NewDecoder(reader)
}

func NewEncoder(writer io.Writer) *jsoniter.Encoder {
	return jsoniter.ConfigCompatibleWithStandardLibrary.NewEncoder(writer)
}

func Marshal(v interface{}) ([]byte, error) {
	return jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(v)
}

func MarshalToString(v interface{}) (string, error) {
	return jsoniter.ConfigCompatibleWithStandardLibrary.MarshalToString(v)

}

func MarshalIndent(v interface{}, prefix, indent string) ([]byte, error) {
	return jsoniter.ConfigCompatibleWithStandardLibrary.MarshalIndent(v, prefix, indent)

}

func UnmarshalFromString(str string, v interface{}) error {
	return jsoniter.UnmarshalFromString(str, v)
}

func Unmarshal(data []byte, v interface{}) error {
	return jsoniter.Unmarshal(data, v)
}

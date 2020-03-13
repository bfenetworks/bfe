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

package mod_auth_jwt

import "testing"
import "errors"

func TestNewTypedError(t *testing.T) {
	rawError := errors.New("")
	if _, ok := rawError.(TypedError); ok {
		t.Error("Raw error type should not be able to be casted as TypedError.")
	}
	err := NewTypedError(int(^uint(0)>>1), rawError)
	if _, ok := interface{}(err).(error); !ok {
		t.Error("TypedError must be error type.")
	}
	errUndefined := NewTypedError(TypeUndefined, rawError)
	if err.Error() != "TypedError: " ||
		errUndefined.Error() != "TypeUndefined: " {
		t.Error("Incorrect type mapping in TypedError.")
	}
}

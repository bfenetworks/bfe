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
	return
}

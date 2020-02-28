package jwk

import (
	"reflect"
	"testing"
)

func TestBase64URLUintDecode(t *testing.T) {
	if _, bigInt, err := Base64URLUintDecode("AA"); err != nil ||
		!bigInt.IsInt64() || bigInt.Uint64() != 0 {
		t.Errorf("decoded value for base64url-encoded string 'AA' should be 0, not %+v", bigInt)
	}
}

func TestKeyCheck(t *testing.T) {
	target := map[string]interface{}{
		"a": 0,
		"b": "",
		"c": false,
	}
	err := KeyCheck(target, map[string]reflect.Kind{
		"a": reflect.Int,
		"b": reflect.String,
		"c": reflect.Bool,
	})
	if err != nil {
		t.Errorf("wrong key check: %s", err)
	}
	err = KeyCheck(target, map[string]reflect.Kind{
		"a": reflect.Bool,
	})
	t.Log(err)
	if err == nil {
		t.Error("wrong key check (type)")
	}
	err = KeyCheck(target, map[string]reflect.Kind{
		"d": reflect.Bool,
	})
	t.Log(err)
	if err == nil {
		t.Error("wrong key check (required)")
	}
}

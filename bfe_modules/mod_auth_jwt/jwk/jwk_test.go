package jwk

import (
	"encoding/json"
	"io/ioutil"
	"testing"
)

func TestNewJWK(t *testing.T) {
	secret, err := ioutil.ReadFile("./../testdata/mod_auth_jwt/secret_test_jws_ES512.key")
	if err != nil {
		t.Fatal(err)
	}
	keyMap := make(map[string]interface{})
	err = json.Unmarshal(secret, &keyMap)
	if err != nil {
		t.Fatal(err)
	}
	jwk, err := NewJWK(keyMap)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(jwk, jwk.Curve)
	if jwk.Kty != EC {
		t.Errorf("expected key type %d, got %d", EC, jwk.Kty)
	}
	if jwk.Curve.Crv != P521 {
		t.Errorf("expected crv value %d, got %d", P521, jwk.Curve.Crv)
	}
}

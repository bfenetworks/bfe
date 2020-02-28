package jwa

import (
	"encoding/json"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"io/ioutil"
	"strings"
	"testing"
)

func TestNewHS512(t *testing.T) {
	secret, err := ioutil.ReadFile("./../testdata/mod_auth_jwt/secret_test_jws_HS512.key")
	token, err := ioutil.ReadFile("./../testdata/mod_auth_jwt/test_jws_HS512.txt")
	if err != nil {
		t.Fatal(err)
	}
	keyMap := make(map[string]interface{})
	if err = json.Unmarshal(secret, &keyMap); err != nil {
		t.Fatal(err)
	}
	mJWK, err := jwk.NewJWK(keyMap)
	if err != nil {
		t.Fatal(err)
	}
	tokens := strings.Split(string(token), ".")
	HS512, err := NewHS512(mJWK)
	if err != nil {
		t.Fatal(err)
	}
	_, err = HS512.Update([]byte(strings.Join(tokens[:2], ".")))
	if err != nil {
		t.Fatal(err)
	}
	sig, _ := jwk.Base64URLDecode(tokens[2])
	if !HS512.Verify(sig) {
		t.Error("wrong signature check result")
	}
}

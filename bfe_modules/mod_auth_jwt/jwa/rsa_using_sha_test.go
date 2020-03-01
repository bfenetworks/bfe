package jwa

import (
	"encoding/json"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"io/ioutil"
	"strings"
	"testing"
)

func TestNewRS256(t *testing.T) {
	secret, err := ioutil.ReadFile("./../testdata/mod_auth_jwt/secret_test_jws_RS256.key")
	token, err := ioutil.ReadFile("./../testdata/mod_auth_jwt/test_jws_RS256.txt")
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
	RS256, err := NewRS256(mJWK)
	if err != nil {
		t.Fatal(err)
	}
	_, err = RS256.Update([]byte(strings.Join(tokens[:2], ".")))
	if err != nil {
		t.Fatal(err)
	}
	sig, _ := jwk.Base64URLDecode(tokens[2])
	if !RS256.Verify(sig) {
		t.Error("wrong signature check result")
	}
}

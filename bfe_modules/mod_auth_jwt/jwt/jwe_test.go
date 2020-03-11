package jwt

import (
	"encoding/json"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"io/ioutil"
	"testing"
)

func TestNewJWE(t *testing.T) {
	token, _ := ioutil.ReadFile("./../testdata/mod_auth_jwt/jwe_valid_1.txt")
	secret, _ := ioutil.ReadFile("./../testdata/mod_auth_jwt/secret_jwe_valid_1.key")
	var keyMap map[string]interface{}
	_ = json.Unmarshal(secret, &keyMap)
	mJWK, _ := jwk.NewJWK(keyMap)
	mJWE, err := NewJWE(string(token), mJWK)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, _ := mJWE.Plaintext()
	t.Log(string(plaintext))
	t.Log(mJWE.Payload)
}

package jwa

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"io/ioutil"
	"strings"
	"testing"
)

func TestNewDir(t *testing.T) {
	token, _ := ioutil.ReadFile(fmt.Sprintf("%s/test_jwe_dir_A128GCM.txt", relativePath))
	secret, _ := ioutil.ReadFile(fmt.Sprintf("%s/secret_test_jwe_dir_A128GCM.key", relativePath))
	eCek, _ := base64.RawURLEncoding.DecodeString(strings.Split(string(token), ".")[1])
	keyMap := make(map[string]interface{})
	_ = json.Unmarshal(secret, &keyMap)
	mJWK, _ := jwk.NewJWK(keyMap)
	context, err := NewDir(mJWK, nil)
	if err != nil {
		t.Fatal(err)
	}
	cek, err := context.Decrypt(eCek)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(cek)
}

package jwt

import (
	"io/ioutil"
	"testing"
)

func TestNewJWE(t *testing.T) {
	token, _ := ioutil.ReadFile("./../testdata/mod_auth_jwt/jwe_valid_1.txt")
	mJWE, err := NewJWE(string(token), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(mJWE, mJWE.InitialVector)
}

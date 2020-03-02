package jwt

import (
	"io/ioutil"
	"testing"
)

func TestNewJWS(t *testing.T) {
	token, _ := ioutil.ReadFile("./../testdata/mod_auth_jwt/test_jws_HS256.txt")
	mJWS, err := NewJWS(string(token), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(mJWS, mJWS.Header.Decoded)
}

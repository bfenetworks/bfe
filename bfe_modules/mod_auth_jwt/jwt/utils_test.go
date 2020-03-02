package jwt

import (
	"io/ioutil"
	"strings"
	"testing"
)

func TestNewBase64URLJson(t *testing.T) {
	token, _ := ioutil.ReadFile("./../testdata/mod_auth_jwt/test_jws_HS256.txt")
	header := strings.Split(string(token), ".")[0]
	obj, err := NewBase64URLJson(header, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(obj)
}

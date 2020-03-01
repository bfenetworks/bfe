package jwa

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"io/ioutil"
	"strings"
	"testing"
)

func TestNewA192KW(t *testing.T) {
	path := "./../testdata/mod_auth_jwt"
	token, _ := ioutil.ReadFile(fmt.Sprintf("%s/test_jwe_A192KW_A128GCM.txt", path))
	secret, _ := ioutil.ReadFile(fmt.Sprintf("%s/secret_test_jwe_A192KW_A128GCM.key", path))
	eCek, _ := base64.RawURLEncoding.DecodeString(strings.Split(string(token), ".")[1])
	keyMap := make(map[string]interface{})
	_ = json.Unmarshal(secret, &keyMap)
	mJWK, _ := jwk.NewJWK(keyMap)
	context, err := NewA192KW(mJWK, nil)
	if err != nil {
		t.Fatal(err)
	}
	cek, err := context.Decrypt(eCek)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(cek)
}

func TestNewA128KW(t *testing.T) {
	mJWK, _ := jwk.NewJWK(map[string]interface{}{
		"kty": "oct",
		"k":   "GawgguFyGrWKav7AX4VKUg",
	})
	cek := []byte{4, 211, 31, 197, 84, 157, 252, 254, 11, 100, 157, 250, 63, 170, 106,
		206, 107, 124, 212, 45, 111, 107, 9, 219, 200, 177, 0, 240, 143, 156,
		44, 207}
	eCek := []byte{232, 160, 123, 211, 183, 76, 245, 132, 200, 128, 123, 75, 190, 216,
		22, 67, 201, 138, 193, 186, 9, 91, 122, 31, 246, 90, 28, 139, 57, 3,
		76, 124, 193, 11, 98, 37, 173, 61, 104, 57}
	context, err := NewA128KW(mJWK, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := context.Decrypt(eCek)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cek, res) {
		t.Error(res)
	}
}

func TestCase(t *testing.T) {
	u, _ := hex.DecodeString("A6A6A6A6A6A6A6A6")
	t.Log(binary.BigEndian.Uint64(u))
	c, _ := hex.DecodeString("1FA68B0A8112B447AEF34BD8FB5A7B829D3E862371D2CFE5")
	k, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F")
	a := binary.BigEndian.Uint64(c[:8])
	ab := make([]byte, 8)
	binary.BigEndian.PutUint64(ab, a)
	t.Log(hex.EncodeToString(ab))
	n := len(c)/8 - 1
	r := make([]uint64, n+1)
	for i := 1; i <= n; i++ {
		r[i] = binary.BigEndian.Uint64(c[i*8 : (i+1)*8])
		t.Logf("R%d = %s", i, hex.EncodeToString(c[i*8:(i+1)*8]))
	}
	decrypter, _ := aes.NewCipher(k)
	for j := 5; j >= 0; j-- {
		for i := n; i >= 1; i-- {
			ab, rb := make([]byte, 8), make([]byte, 8)
			binary.BigEndian.PutUint64(ab, a)
			binary.BigEndian.PutUint64(rb, r[i])
			t.Logf("A = %s, R%d = %s", hex.EncodeToString(ab), i,
				hex.EncodeToString(rb))
			axt := make([]byte, 16)
			binary.BigEndian.PutUint64(axt, a^uint64(n*j+i))
			t.Logf("A xor t = %s", hex.EncodeToString(axt[:8]))
			binary.BigEndian.PutUint64(axt[8:], r[i])
			t.Logf("aes input: %s", hex.EncodeToString(axt))
			b := make([]byte, decrypter.BlockSize())
			decrypter.Decrypt(b, axt)
			t.Logf("aes output: %s", hex.EncodeToString(b))
			a = binary.BigEndian.Uint64(b[:8])
			r[i] = binary.BigEndian.Uint64(b[len(b)-8:])
		}
	}
	ret := make([]byte, (n+1)*8)
	for i := 0; i < n; i++ {
		binary.BigEndian.PutUint64(ret[i*8:(i+1)*8], r[i+1])
	}
	t.Log(hex.EncodeToString(ret))
}

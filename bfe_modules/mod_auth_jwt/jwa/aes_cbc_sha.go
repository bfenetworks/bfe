package jwa

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
)

type ACS struct {
	block  cipher.Block
	signer JWSAlg
}

func (acs *ACS) checkAuthenticationTag(aad, iv, cipherText, tag []byte) bool {
	al := make([]byte, 8) // 64 bit binary aad length
	binary.BigEndian.PutUint64(al, uint64(len(aad)*8))
	hInput := bytes.Join([][]byte{aad, iv, cipherText, al}, nil) // for calculating sign
	_, err := acs.signer.Update(hInput)
	if err != nil {
		return false
	}
	return bytes.HasPrefix(acs.signer.Sign(), tag)
}

func (acs *ACS) Decrypt(iv, aad, cipherText, tag []byte) (msg []byte, err error) {
	if !acs.checkAuthenticationTag(aad, iv, cipherText, tag) {
		return nil, fmt.Errorf("authentication tag check failed")
	}
	defer CatchCryptoPanic(&err) // prevent from panic
	msg = make([]byte, len(cipherText))
	// this may cause panic
	cipher.NewCBCDecrypter(acs.block, iv).CryptBlocks(msg, cipherText)
	// no panic happened, everything is ok
	return msg, nil
}

func NewACS(ekBit, m int, cek []byte) (acs JWEEnc, err error) {
	if len(cek) != ekBit/4 {
		return nil, fmt.Errorf("bad CEK length for A%dCBC-HS%d: %d", ekBit, m, len(cek))
	}
	// cek[:ekBit/8] for mac key
	// cek[ekBit/8:] for encryption key
	mK, eK := cek[:ekBit/8], cek[ekBit/8:]
	block, err := aes.NewCipher(eK)
	if err != nil {
		return nil, err
	}
	mJWK, _ := jwk.NewJWK(map[string]interface{}{
		"kty": "oct",
		"k":   base64.RawURLEncoding.EncodeToString(mK),
	})
	signerName := fmt.Sprintf("HS%d", m)
	signerFactory, ok := JWSAlgSet[signerName]
	if !ok {
		return nil, fmt.Errorf("unsupported signer: %s", signerName)
	}
	signer, err := signerFactory(mJWK)
	if err != nil {
		return nil, err
	}
	return &ACS{block, signer}, nil
}

func NewA128CBCHS256(cek []byte) (acs JWEEnc, err error) {
	return NewACS(128, 256, cek)
}

func NewA192CBCHS384(cek []byte) (acs JWEEnc, err error) {
	return NewACS(192, 384, cek)
}

func NewA256CBCHS512(cek []byte) (acs JWEEnc, err error) {
	return NewACS(256, 512, cek)
}

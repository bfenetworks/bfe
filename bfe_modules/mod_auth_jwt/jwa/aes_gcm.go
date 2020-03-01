package jwa

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
)

type AGCM struct {
	context cipher.AEAD
}

func (agcm *AGCM) Decrypt(iv, aad, cipherText, tag []byte) (msg []byte, err error) {
	cipherText = append(cipherText, tag...)
	return agcm.context.Open(nil, iv, cipherText, aad)
}

func NewAGCM(kBit int, cek []byte) (agcm JWEEnc, err error) {
	if kBit/8 != len(cek) {
		return nil, fmt.Errorf("invalid CEK length for A%dGCM: %d", kBit, len(cek))
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AGCM{gcm}, nil
}

func NewA128GCM(cek []byte) (agcm JWEEnc, err error) {
	return NewAGCM(128, cek)
}

func NewA192GCM(cek []byte) (agcm JWEEnc, err error) {
	return NewAGCM(192, cek)
}

func NewA256GCM(cek []byte) (agcm JWEEnc, err error) {
	return NewAGCM(256, cek)
}

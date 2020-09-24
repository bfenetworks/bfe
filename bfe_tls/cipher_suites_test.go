// Copyright (c) 2019 The BFE Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bfe_tls

import (
	"crypto/aes"
	"crypto/cipher"
	"testing"
)

import (
	"golang.org/x/crypto/chacha20poly1305"
)

func benchamarkAEADSeal(b *testing.B, aead cipher.AEAD, buf []byte) {
	b.SetBytes(int64(len(buf)))

	var nonce [12]byte
	var ad [13]byte
	var out []byte

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out = aead.Seal(out[:0], nonce[:], buf[:], ad[:])
	}
}

func benchamarkAEADOpen(b *testing.B, aead cipher.AEAD, buf []byte) {
	b.SetBytes(int64(len(buf)))

	var nonce [12]byte
	var ad [13]byte
	var ct []byte
	var out []byte

	ct = aead.Seal(ct[:0], nonce[:], buf[:], ad[:])

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, _ = aead.Open(out[:0], nonce[:], ct[:], ad[:])
	}
}

func benchmarkChacha20Poly1305Open(b *testing.B, size int) {
	aead, _ := chacha20poly1305.New(make([]byte, 32))
	benchamarkAEADOpen(b, aead, make([]byte, size))
}

func benchmarkChacha20Poly1305Seal(b *testing.B, size int) {
	aead, _ := chacha20poly1305.New(make([]byte, 32))
	benchamarkAEADSeal(b, aead, make([]byte, size))
}

func benchmarkAES128GCMOpen(b *testing.B, size int) {
	aes, _ := aes.NewCipher(make([]byte, 16))
	aead, _ := cipher.NewGCM(aes)
	benchamarkAEADOpen(b, aead, make([]byte, size))
}

func benchmarkAES128GCMSeal(b *testing.B, size int) {
	aes, _ := aes.NewCipher(make([]byte, 16))
	aead, _ := cipher.NewGCM(aes)
	benchamarkAEADSeal(b, aead, make([]byte, size))
}

func BenchmarkChacha20Poly1305Open_64(b *testing.B) {
	benchmarkChacha20Poly1305Open(b, 64)
}

func BenchmarkChacha20Poly1305Seal_64(b *testing.B) {
	benchmarkChacha20Poly1305Seal(b, 64)
}

func BenchmarkChacha20Poly1305Open_1350(b *testing.B) {
	benchmarkChacha20Poly1305Open(b, 1350)
}

func BenchmarkChacha20Poly1305Seal_1350(b *testing.B) {
	benchmarkChacha20Poly1305Seal(b, 1350)
}

func BenchmarkChacha20Poly1305Open_8K(b *testing.B) {
	benchmarkChacha20Poly1305Open(b, 8*1024)
}

func BenchmarkChacha20Poly1305Seal_8K(b *testing.B) {
	benchmarkChacha20Poly1305Seal(b, 8*1024)
}

func BenchmarkAES128GCMOpen_64(b *testing.B) {
	benchmarkAES128GCMOpen(b, 64)
}

func BenchmarkAES128GCMSeal_64(b *testing.B) {
	benchmarkAES128GCMSeal(b, 64)
}

func BenchmarkAES128GCMOpen_1350(b *testing.B) {
	benchmarkAES128GCMOpen(b, 1350)
}

func BenchmarkAES128GCMSeal_1350(b *testing.B) {
	benchmarkAES128GCMSeal(b, 1350)
}

func BenchmarkAES128GCMOpen_8K(b *testing.B) {
	benchmarkAES128GCMOpen(b, 8*1024)
}

func BenchmarkAES128GCMSeal_8K(b *testing.B) {
	benchmarkAES128GCMSeal(b, 8*1024)
}

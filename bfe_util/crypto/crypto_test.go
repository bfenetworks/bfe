// Copyright (c) 2026 The BFE Authors.
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

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// encryptForTest produces a valid enc$v1$ envelope, mirroring the control
// plane export encryption. BFE itself never encrypts.
func encryptForTest(t *testing.T, plaintext string, key [keyLen]byte, keyID byte) string {
	t.Helper()

	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("cipher.NewGCM: %v", err)
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	raw := make([]byte, 0, keyIDLen+nonceLen+len(plaintext)+tagLen)
	raw = append(raw, keyID)
	raw = append(raw, nonce...)
	raw = gcm.Seal(raw, nonce, []byte(plaintext), nil)

	return Marker + base64.StdEncoding.EncodeToString(raw)
}

func testKey(seed byte) [keyLen]byte {
	var key [keyLen]byte
	for i := range key {
		key[i] = seed + byte(i)
	}
	return key
}

func TestHasMarker(t *testing.T) {
	if !HasMarker(Marker + "AAAA") {
		t.Error("ciphertext should have marker")
	}
	if HasMarker("sk-plaintext") {
		t.Error("plaintext should not have marker")
	}
	if HasMarker("enc$v2$future") {
		t.Error("other versions should not match the v1 marker")
	}
	if HasMarker("") {
		t.Error("empty string should not have marker")
	}
}

func TestDecryptRoundtrip(t *testing.T) {
	key1, key2 := testKey(1), testKey(2)
	kr := &Keyring{keys: map[byte][keyLen]byte{1: key1, 2: key2}}

	// multiple keyIDs coexist; envelope keyID selects the key
	for _, tc := range []struct {
		keyID byte
		key   [keyLen]byte
	}{
		{1, key1},
		{2, key2},
	} {
		ct := encryptForTest(t, "sk-test-api-key", tc.key, tc.keyID)
		pt, err := Decrypt(ct, kr)
		if err != nil {
			t.Fatalf("Decrypt(keyID %d): %v", tc.keyID, err)
		}
		if pt != "sk-test-api-key" {
			t.Errorf("Decrypt(keyID %d) = %q, want %q", tc.keyID, pt, "sk-test-api-key")
		}
	}
}

func TestDecryptErrors(t *testing.T) {
	key1, key2 := testKey(1), testKey(2)
	kr := &Keyring{keys: map[byte][keyLen]byte{1: key1}}

	// nil keyring: marker value must fail, error must not leak material
	if _, err := Decrypt(Marker+"AAAA", nil); err != ErrNoKeyring {
		t.Errorf("Decrypt(nil keyring) = %v, want ErrNoKeyring", err)
	}

	// unknown keyID
	ct := encryptForTest(t, "secret", key2, 2)
	if _, err := Decrypt(ct, kr); err == nil || !strings.Contains(err.Error(), "unknown keyID 2") {
		t.Errorf("Decrypt(unknown keyID) = %v", err)
	}

	// wrong key material (GCM authentication failure)
	if _, err := Decrypt(ct, &Keyring{keys: map[byte][keyLen]byte{2: key1}}); err == nil ||
		!strings.Contains(err.Error(), "decrypt failed") {
		t.Errorf("Decrypt(wrong key) = %v", err)
	}

	// malformed envelopes
	for _, value := range []string{
		Marker + "!!!not-base64!!!",
		Marker + base64.StdEncoding.EncodeToString([]byte("short")),
	} {
		if _, err := Decrypt(value, kr); err == nil {
			t.Errorf("Decrypt(%q) should fail", value)
		}
	}

	// tampered ciphertext bit must fail GCM (disk bit-flip protection)
	good := encryptForTest(t, "sk-tamper-check", key1, 1)
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(good, Marker))
	raw[len(raw)-1] ^= 0x01
	if _, err := Decrypt(Marker+base64.StdEncoding.EncodeToString(raw), kr); err == nil {
		t.Error("Decrypt(tampered) should fail GCM authentication")
	}
}

func writeKeyring(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "export.keys")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLoadKeyringFile(t *testing.T) {
	key := testKey(9)
	keyB64 := base64.StdEncoding.EncodeToString(key[:])

	// valid: bare + quoted keys, comment, active key
	path := writeKeyring(t, fmt.Sprintf(`
# rotation: old key kept for legacy ciphertext
ActiveKeyID = 2
[Keys]
1 = "%s"
"2" = "%s"
`, keyB64, keyB64))

	kr, err := LoadKeyringFile(path)
	if err != nil {
		t.Fatalf("LoadKeyringFile: %v", err)
	}
	if _, ok := kr.Key(1); !ok {
		t.Error("keyID 1 missing")
	}
	if _, ok := kr.Key(2); !ok {
		t.Error("keyID 2 missing")
	}
	if kr.ActiveKeyID() != 2 {
		t.Errorf("ActiveKeyID = %d, want 2", kr.ActiveKeyID())
	}
	if ids := kr.KeyIDs(); len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Errorf("KeyIDs = %v", ids)
	}
}

func TestLoadKeyringFileErrors(t *testing.T) {
	key := testKey(9)
	keyB64 := base64.StdEncoding.EncodeToString(key[:])
	shortB64 := base64.StdEncoding.EncodeToString([]byte("short"))

	cases := []struct {
		name    string
		content string
	}{
		{"no keys section", fmt.Sprintf("1 = \"%s\"\n", keyB64)},
		{"empty keyring", "[Keys]\n"},
		{"duplicate keyID", fmt.Sprintf("[Keys]\n1 = \"%s\"\n1 = \"%s\"\n", keyB64, keyB64)},
		{"bad base64", "[Keys]\n1 = \"!!!\"\n"},
		{"bad key length", fmt.Sprintf("[Keys]\n1 = \"%s\"\n", shortB64)},
		{"unknown section", "[Secrets]\n"},
		{"garbage line", "[Keys]\nnot-an-entry\n"},
		{"unquoted value", "[Keys]\n1 = abc\n"},
		{"bad keyID", fmt.Sprintf("[Keys]\nabc = \"%s\"\n", keyB64)},
		{"keyID out of range", fmt.Sprintf("[Keys]\n300 = \"%s\"\n", keyB64)},
		{"active not in keys", fmt.Sprintf("ActiveKeyID = 7\n[Keys]\n1 = \"%s\"\n", keyB64)},
	}
	for _, tc := range cases {
		path := writeKeyring(t, tc.content)
		if _, err := LoadKeyringFile(path); err == nil {
			t.Errorf("%s: LoadKeyringFile should fail", tc.name)
		}
	}

	if _, err := LoadKeyringFile(filepath.Join(t.TempDir(), "not-exist.keys")); err == nil {
		t.Error("LoadKeyringFile(missing file) should fail")
	}
}

func TestDecryptLoadedKeyring(t *testing.T) {
	// end-to-end: keyring file produced by the control plane decrypts an
	// envelope produced with the matching key material.
	key := testKey(5)
	keyBytes := key[:]
	path := writeKeyring(t, fmt.Sprintf("[Keys]\n1 = \"%s\"\n",
		base64.StdEncoding.EncodeToString(keyBytes)))

	kr, err := LoadKeyringFile(path)
	if err != nil {
		t.Fatalf("LoadKeyringFile: %v", err)
	}
	ct := encryptForTest(t, "sk-from-file", key, 1)
	pt, err := Decrypt(ct, kr)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if pt != "sk-from-file" {
		t.Errorf("Decrypt = %q", pt)
	}
}

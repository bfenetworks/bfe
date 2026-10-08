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

// Package crypto implements decrypt-only support for the field-level
// encrypted config values produced by the control plane at export time
// (see docs/zh_cn/sys_design/config_file_field_encryption.md). Key material
// is distributed via a shared keyring file; BFE never encrypts.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Marker is the envelope prefix marking a field value as ciphertext:
//
//	enc$v1$<base64( keyID(1B) | nonce(12B) | AES-256-GCM(plaintext)+tag(16B) )>
//
// Values without the marker are legacy plaintext and pass through unchanged.
const Marker = "enc$v1$"

const (
	keyIDLen = 1
	nonceLen = 12
	tagLen   = 16
)

// ErrNoKeyring is returned when a marker-prefixed value is decrypted
// without a keyring (Security.KeyFile not configured).
var ErrNoKeyring = errors.New("crypto: keyring not configured (Security.KeyFile)")

// HasMarker reports whether a config field value is an enc$v1$ ciphertext
// envelope rather than plaintext.
func HasMarker(value string) bool {
	return strings.HasPrefix(value, Marker)
}

// Decrypt decrypts a single enc$v1$ envelope with the keyring. The key is
// selected by the keyID carried in the envelope itself, so ciphertexts
// encrypted with different keyIDs coexist naturally during rotation.
//
// Error messages deliberately contain only the keyID and envelope length —
// never ciphertext or plaintext material.
func Decrypt(value string, kr *Keyring) (string, error) {
	if kr == nil {
		return "", ErrNoKeyring
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, Marker))
	if err != nil {
		return "", fmt.Errorf("crypto: envelope base64 decode failed: %s", err)
	}
	if len(raw) < keyIDLen+nonceLen+tagLen {
		return "", fmt.Errorf("crypto: envelope too short (%d bytes)", len(raw))
	}

	keyID := raw[0]
	key, ok := kr.Key(keyID)
	if !ok {
		return "", fmt.Errorf("crypto: unknown keyID %d", keyID)
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("crypto: new cipher for keyID %d: %s", keyID, err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: new GCM for keyID %d: %s", keyID, err)
	}

	plaintext, err := gcm.Open(nil, raw[keyIDLen:keyIDLen+nonceLen],
		raw[keyIDLen+nonceLen:], nil)
	if err != nil {
		return "", fmt.Errorf("crypto: decrypt failed (keyID %d, len %d): %s",
			keyID, len(raw), err)
	}

	return string(plaintext), nil
}

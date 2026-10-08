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
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// keyLen is the AES-256 key length in bytes.
const keyLen = 32

// Keyring holds all in-service file-encryption keys, keyed by keyID.
// Multiple keyIDs coexist during rotation: decryption selects the key by
// the keyID embedded in each envelope, so old and new ciphertexts are both
// readable until the old keyID is retired.
type Keyring struct {
	keys   map[byte][keyLen]byte
	active byte
}

// Key returns the key material for a keyID.
func (kr *Keyring) Key(id byte) ([keyLen]byte, bool) {
	key, ok := kr.keys[id]
	return key, ok
}

// ActiveKeyID returns the keyID marked active in the keyring file
// (informational on the BFE decrypt-only side).
func (kr *Keyring) ActiveKeyID() byte {
	return kr.active
}

// KeyIDs returns the sorted in-service keyIDs (for status/monitoring).
func (kr *Keyring) KeyIDs() []byte {
	ids := make([]byte, 0, len(kr.keys))
	for id := range kr.keys {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// LoadKeyringFile parses the shared TOML keyring file format (same file is
// used by the control plane to encrypt):
//
//	ActiveKeyID = 2
//	[Keys]
//	  1 = "<base64 of 32-byte key>"   # old key: decrypt legacy ciphertext
//	  2 = "<base64 of 32-byte key>"   # current key
//
// ActiveKeyID is optional on the BFE (decrypt-only) side. Parsing is strict
// and fail-closed: malformed lines, duplicate keyIDs and bad key material
// are all errors.
func LoadKeyringFile(path string) (*Keyring, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("crypto: open keyring file: %s", err)
	}
	defer file.Close()

	kr := &Keyring{keys: make(map[byte][keyLen]byte)}
	section := ""
	haveActive := false

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 64*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") {
			if line != "[Keys]" {
				return nil, fmt.Errorf("crypto: keyring line %d: unknown section %q", lineNo, line)
			}
			section = "Keys"
			continue
		}

		key, value, quoted, err := parseKeyringEntry(line, lineNo)
		if err != nil {
			return nil, err
		}

		if section == "" {
			if key != "ActiveKeyID" {
				return nil, fmt.Errorf("crypto: keyring line %d: unknown entry %q (only ActiveKeyID allowed here)", lineNo, key)
			}
			// TOML integer form (ActiveKeyID = 2) is the canonical one;
			// a quoted string is accepted as well
			id, err := parseKeyID(value, lineNo)
			if err != nil {
				return nil, err
			}
			kr.active = id
			haveActive = true
			continue
		}

		if !quoted {
			return nil, fmt.Errorf("crypto: keyring line %d: key material must be a quoted string", lineNo)
		}

		id, err := parseKeyID(key, lineNo)
		if err != nil {
			return nil, err
		}
		if _, dup := kr.keys[id]; dup {
			return nil, fmt.Errorf("crypto: keyring line %d: duplicate keyID %d", lineNo, id)
		}
		material, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, fmt.Errorf("crypto: keyring line %d: keyID %d not base64: %s", lineNo, id, err)
		}
		if len(material) != keyLen {
			return nil, fmt.Errorf("crypto: keyring line %d: keyID %d length %d, want %d bytes",
				lineNo, id, len(material), keyLen)
		}
		var fixed [keyLen]byte
		copy(fixed[:], material)
		kr.keys[id] = fixed
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("crypto: read keyring file: %s", err)
	}

	if len(kr.keys) == 0 {
		return nil, fmt.Errorf("crypto: keyring file has no [Keys] entries")
	}
	if haveActive {
		if _, ok := kr.keys[kr.active]; !ok {
			return nil, fmt.Errorf("crypto: ActiveKeyID %d not found in [Keys]", kr.active)
		}
	}

	return kr, nil
}

// parseKeyringEntry splits one `key = value` line. The key may be bare or
// double-quoted (TOML allows both, e.g. 1 = "..." or "1" = "..."). The
// value may be a double-quoted string (required for key material) or a bare
// token (allowed for the ActiveKeyID integer, e.g. ActiveKeyID = 2).
// Trailing whitespace or a # comment after the value is tolerated.
func parseKeyringEntry(line string, lineNo int) (string, string, bool, error) {
	eq := strings.IndexByte(line, '=')
	if eq < 0 {
		return "", "", false, fmt.Errorf("crypto: keyring line %d: no '=' found", lineNo)
	}

	key := strings.TrimSpace(line[:eq])
	key = strings.Trim(key, `"`)
	if key == "" {
		return "", "", false, fmt.Errorf("crypto: keyring line %d: empty key", lineNo)
	}

	rest := strings.TrimSpace(line[eq+1:])
	if rest == "" {
		return "", "", false, fmt.Errorf("crypto: keyring line %d: empty value", lineNo)
	}

	if rest[0] == '"' {
		end := strings.IndexByte(rest[1:], '"')
		if end < 0 {
			return "", "", false, fmt.Errorf("crypto: keyring line %d: unterminated string", lineNo)
		}
		// only whitespace or a comment may follow the closing quote
		tail := strings.TrimSpace(rest[1+end+1:])
		if tail != "" && !strings.HasPrefix(tail, "#") {
			return "", "", false, fmt.Errorf("crypto: keyring line %d: unexpected content after value", lineNo)
		}
		return key, rest[1 : 1+end], true, nil
	}

	// bare token: up to whitespace or comment
	end := strings.IndexAny(rest, " \t#")
	if end < 0 {
		end = len(rest)
	}
	if end == 0 {
		return "", "", false, fmt.Errorf("crypto: keyring line %d: empty value", lineNo)
	}

	return key, rest[:end], false, nil
}

// parseKeyID parses a decimal keyID in [0, 255] (envelope keyID is 1 byte).
func parseKeyID(s string, lineNo int) (byte, error) {
	id, err := strconv.Atoi(s)
	if err != nil || id < 0 || id > 255 {
		return 0, fmt.Errorf("crypto: keyring line %d: invalid keyID %q", lineNo, s)
	}
	return byte(id), nil
}

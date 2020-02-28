package jwk

import (
	"encoding/base64"
	"testing"
)

func TestNewBase64URL(t *testing.T) {
	value := base64.RawURLEncoding.EncodeToString([]byte("test"))
	b, err := NewBase64URL(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(b.Decoded) != "test" {
		t.Errorf("wrong decoded string: %s", b.Decoded)
	}
}

func TestNewBase64URLUint(t *testing.T) {
	b, err := NewBase64URLUint("AA")
	if err != nil {
		t.Fatal(err)
	}
	if !b.Decoded.IsInt64() || b.Decoded.Int64() != 0 {
		t.Errorf("wrong decoded value: %+v", b.Decoded)
	}
}

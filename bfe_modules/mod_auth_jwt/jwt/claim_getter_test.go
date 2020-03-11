package jwt

import (
	"testing"
	"time"
)

func TestNewClaims(t *testing.T) {
	header := map[string]interface{}{
		"iss": "issuer",
		"exp": time.Now().Unix(),
	}
	payload := map[string]interface{}{
		"aud": "audience",
	}
	claims, _ := NewClaims(header, payload, true)
	if _, _, ok := claims.Exp(); !ok {
		t.Error("failed to get claim from header")
	}
	if claim, exp, ok := claims.Exp(); !ok {
		t.Logf("%+v, %+v", claim, exp)
		t.Error("failed to convert claim type")
	}
}

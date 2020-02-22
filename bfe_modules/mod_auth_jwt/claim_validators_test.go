package mod_auth_jwt

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

var (
	token        []string
	config       *ProductConfigItem
	emptyJsonB64 = base64.RawURLEncoding.EncodeToString([]byte("{}"))
)

func init() {
	config = new(ProductConfigItem)
	config.EnabledPayloadClaims = true
	config.ValidateClaimExp = true
	config.ValidateClaimNbf = true
	config.ValidateClaimIss = "iss"
	config.ValidateClaimAud = "aud"
	config.ValidateClaimSub = "sub"
}

func TestValidateClaims_invalid(t *testing.T) {
	claims := map[string]interface{}{
		"exp": time.Unix(time.Now().Unix()-1000, 0).Unix(),
		"nbf": time.Unix(time.Now().Unix()+1000, 0).Unix(),
		"iss": "bad iss",
		"aud": "bad aud",
		"sub": "bad sub",
	}
	i := 0
	for claim, value := range claims {
		data, _ := json.Marshal(map[string]interface{}{
			claim: value,
		})
		if i < 3 {
			token = []string{
				base64.RawURLEncoding.EncodeToString(data), emptyJsonB64, "",
			}
		} else {
			// test for get claim from payload
			token = []string{
				emptyJsonB64, base64.RawURLEncoding.EncodeToString(data), "",
			}
		}
		if ValidateClaims(token, config) == nil {
			t.Errorf("something wrong validating invalid claim: %s", claim)
		}
		i++
	}
}

func TestValidateClaims_valid(t *testing.T) {
	claims := map[string]interface{}{
		"exp": time.Unix(time.Now().Unix()+1000, 0).Unix(),
		"nbf": time.Unix(time.Now().Unix()-1000, 0).Unix(),
		"iss": "iss",
		"aud": "aud",
		"sub": "sub",
	}
	i := 0
	for claim, value := range claims {
		data, _ := json.Marshal(map[string]interface{}{
			claim: value,
		})
		if i < 3 {
			token = []string{
				base64.RawURLEncoding.EncodeToString(data), emptyJsonB64, "",
			}
		} else {
			// test for get claim from payload
			token = []string{
				emptyJsonB64, base64.RawURLEncoding.EncodeToString(data), "",
			}
		}
		if err := ValidateClaims(token, config); err != nil {
			t.Errorf("something wrong validating valid claim: %s\n%s", claim, err)
		}
		i++
	}
}

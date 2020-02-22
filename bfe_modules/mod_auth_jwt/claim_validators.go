package mod_auth_jwt

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type claimValidator func(map[string]interface{}, map[string]interface{}, *ProductConfigItem) error

var validators = []claimValidator{
	validateExp, validateNbf, validateIss, validateSub, validateAud,
}

func ValidateClaims(token []string, config *ProductConfigItem) (err error) {
	var header, payload map[string]interface{}
	headerByte, _ := base64.RawURLEncoding.DecodeString(token[0]) // error ignored (checked)
	header = make(map[string]interface{})
	err = json.Unmarshal(headerByte, &header)
	if err != nil {
		return NewTypedError(JsonDecoderError, err)
	}
	if config.EnabledPayloadClaims && len(token) == 3 { // for JWS only
		payloadByte, _ := base64.RawURLEncoding.DecodeString(token[1])
		payload = make(map[string]interface{})
		// decoding payload might failed due to the JWT was a nested-JWT or encrypted-JWT
		// for more detail: https://tools.ietf.org/html/rfc7519#appendix-A
		// in this case, just ignore any error and let the payload keep empty
		_ = json.Unmarshal(payloadByte, &payload)
	}
	// apply claim validators
	for _, validator := range validators {
		if err = validator(header, payload, config); err != nil {
			return NewTypedError(TokenClaimValidationFailed, err)
		}
	}
	return nil
}

// try to get claim from header and payload(if available)
func getClaim(name string, header, payload map[string]interface{}) (claim interface{}, ok bool) {
	claim, ok = header[name]
	if !ok {
		if payload == nil {
			// no relative claim present
			return nil, false
		}
		if claim, ok = payload[name]; !ok {
			// no relative claim present
			return nil, false
		}
	}
	return claim, true
}

func validateExp(header, payload map[string]interface{}, config *ProductConfigItem) (err error) {
	if !config.ValidateClaimExp {
		// validation not enabled
		return nil
	}
	exp, ok := getClaim("exp", header, payload)
	if !ok {
		return nil
	}
	expSec, ok := exp.(float64)
	if !ok {
		return errors.New("invalid exp claim")
	}
	if time.Now().After(time.Unix(int64(expSec), 0)) {
		return errors.New("your access token has been expired")
	}
	return nil
}

// simply validate if the specific claim value equal to the target value
func simpleValidator(claim string, target interface{}, header, payload map[string]interface{}) (ok bool) {
	value, ok := getClaim(claim, header, payload)
	if !ok {
		// no relative claim present, no validation applied
		return true
	}
	return value == target
}

func validateNbf(header, payload map[string]interface{}, config *ProductConfigItem) (err error) {
	if !config.ValidateClaimNbf {
		return nil
	}
	nbf, ok := getClaim("nbf", header, payload)
	if !ok {
		return nil
	}
	nbfSec, ok := nbf.(float64)
	if !ok {
		return errors.New("invalid nbf claim")
	}
	nbfTime := time.Unix(int64(nbfSec), 0)
	if time.Now().Before(nbfTime) {
		return errors.New(fmt.Sprintf(
			"your access token coulid not be accepted now (could be accepted on %s)",
			nbfTime.String()))
	}
	return nil
}

func validateIss(header, payload map[string]interface{}, config *ProductConfigItem) (err error) {
	iss := config.ValidateClaimIss
	if len(iss) == 0 || simpleValidator("iss", iss, header, payload) {
		return nil
	}
	return errors.New("invalid token issuer")
}

func validateSub(header, payload map[string]interface{}, config *ProductConfigItem) (err error) {
	sub := config.ValidateClaimSub
	if len(sub) == 0 || simpleValidator("sub", sub, header, payload) {
		return nil
	}
	return errors.New("invalid token subject")
}

func validateAud(header, payload map[string]interface{}, config *ProductConfigItem) (err error) {
	aud := config.ValidateClaimAud
	if len(aud) == 0 || simpleValidator("aud", aud, header, payload) {
		return nil
	}
	return errors.New("invalid token audience")
}

// defined a set of key for different key type

package jwk

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type curveParams struct {
	Crv int        // Curve
	X   *Base64URL // X Coordinate
	Y   *Base64URL // Y Coordinate
	D   *Base64URL // ECC Private Key
}

type symmetricParams struct {
	K *Base64URL // Key Value
}

type oth struct {
	R *Base64URLUint //Prime Factor
	D *Base64URLUint //Factor CRT Exponent
	T *Base64URLUint //Factor CRT Coefficient
}

type rsaParams struct {
	D    *Base64URLUint // Private Exponent
	N    *Base64URLUint // Modules
	E    *Base64URLUint // Exponent
	P    *Base64URLUint // First Prime Factor
	Q    *Base64URLUint // Second Prime Factor
	DP   *Base64URLUint // First Factor CRT Exponent
	DQ   *Base64URLUint // Second Factor CRT Exponent
	QI   *Base64URLUint // First CRT Coefficient
	Oth  []*oth         // Other Primes Info
	Full bool           // tells if all parameters available (OTH excluded)
}

// build string to Base64url-encoded or Base64urlUInt-encoded
type paramsBuilder func(s string) (*reflect.Value, error)

// type check rule for keys
var (
	checkRuleSym = map[string]reflect.Kind{
		"k": reflect.String,
	}
	checkRuleCrvPub = map[string]reflect.Kind{
		"crv": reflect.String,
		"x":   reflect.String,
		"y":   reflect.String,
	}
	checkRuleCrvPriv = map[string]reflect.Kind{
		"d": reflect.String,
	}
	checkRuleOth = map[string]reflect.Kind{
		"r": reflect.String,
		"d": reflect.String,
		"t": reflect.String,
	}
	// RSA public key parameter n, e & required private key parameter d
	checkRuleRSAPub = map[string]reflect.Kind{
		"n": reflect.String,
		"e": reflect.String,
	}
	checkRuleRSAPriv = map[string]reflect.Kind{
		"d": reflect.String,
	}
	// RSA key parameters except listed above
	// all parameters should be present if any private key parameter except d present
	checkRuleRSAFull = map[string]reflect.Kind{
		"p":  reflect.String,
		"q":  reflect.String,
		"dp": reflect.String,
		"dq": reflect.String,
		"qi": reflect.String,
	}
	// optional OTH
	checkRuleRSAOTH = map[string]reflect.Kind{
		"oth": reflect.String,
	}
)

func base64URLBuilder(s string) (v *reflect.Value, err error) {
	ptr, err := NewBase64URL(s)
	if err != nil {
		return nil, err
	}
	v0 := reflect.Indirect(reflect.ValueOf(ptr))
	return &v0, nil
}

func base64URLUintBuilder(s string) (v *reflect.Value, err error) {
	ptr, err := NewBase64URLUint(s)
	if err != nil {
		return nil, err
	}
	v0 := reflect.Indirect(reflect.ValueOf(ptr))
	return &v0, nil
}

func buildSymmetricParams(keyMap map[string]interface{}) (params *symmetricParams, err error) {
	if err = KeyCheck(keyMap, checkRuleSym); err != nil {
		return nil, err
	}
	k, err := NewBase64URL(keyMap["k"].(string))
	if err != nil {
		return nil, err
	}
	return &symmetricParams{k}, nil
}

func buildCurveParams(keyMap map[string]interface{}, private bool) (params *curveParams, err error) {
	// key type check
	if err = KeyCheck(keyMap, checkRuleCrvPub); err != nil {
		return nil, err
	}
	if private {
		// check for private key parameters
		if err = KeyCheck(keyMap, checkRuleCrvPriv); err != nil {
			return nil, err
		}
	}
	crvCode, ok := GetCrvCode(keyMap["crv"].(string))
	if !ok {
		return nil, fmt.Errorf("invalid value for key: crv")
	}
	params = &curveParams{Crv: crvCode}
	refValue := reflect.Indirect(reflect.ValueOf(params))
	// set x and y for curve params
	if err = buildAndSetParams(base64URLBuilder, keyMap, &refValue); err != nil {
		return nil, err
	}
	return params, nil
}

func buildRSAParams(keyMap map[string]interface{}, private bool) (params *rsaParams, err error) {
	if err = KeyCheck(keyMap, checkRuleRSAPub); err != nil {
		return nil, err
	}
	if private {
		// check for private key parameter
		if err = KeyCheck(keyMap, checkRuleRSAPriv); err != nil {
			return nil, err
		}
	}
	params = &rsaParams{Full: true}
	if err = KeyCheck(keyMap, checkRuleRSAFull); err != nil {
		// only n, e, d available
		// other parameters ignored
		params.Full = false
		for k := range keyMap {
			switch k {
			case "n", "e", "d", "oth":
				// do nothing, just break the switch
			default:
				// delete ignored item
				delete(keyMap, k)
			}
		}
	}
	refV := reflect.Indirect(reflect.ValueOf(params))
	// build key parameters
	if err = buildAndSetParams(base64URLUintBuilder, keyMap, &refV); err != nil {
		return nil, err
	}
	// build oth
	if oth, ok := keyMap["oth"]; ok {
		if err = KeyCheck(keyMap, checkRuleRSAOTH); err != nil {
			return nil, fmt.Errorf("parameter value for oth: %s", err)
		}
		params.Oth, err = parseOth(oth.(string))
		if err != nil {
			return nil, fmt.Errorf("parsing oth: %s", err)
		}
	}
	return params, nil
}

// parse oth from json string
func parseOth(jsonStr string) (others []*oth, err error) {
	var o []map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &o)
	if err != nil {
		return nil, err
	}
	for _, keyMap := range o {
		if err = KeyCheck(keyMap, checkRuleOth); err != nil {
			return nil, err
		}
		other := new(oth)
		refO := reflect.Indirect(reflect.ValueOf(other))
		if err = buildAndSetParams(base64URLUintBuilder, keyMap, &refO); err != nil {
			return nil, err
		}
		others = append(others, other)
	}
	return others, nil
}

func buildAndSetParams(builder paramsBuilder, keyMap map[string]interface{}, dst *reflect.Value) (err error) {
	for k, v := range keyMap {
		field := dst.FieldByName(strings.ToUpper(k))
		if !field.IsValid() {
			// may be not exists
			continue
		}
		value, err := builder(v.(string))
		if err != nil {
			return err
		}
		field.Set(value.Addr())
	}
	return nil
}

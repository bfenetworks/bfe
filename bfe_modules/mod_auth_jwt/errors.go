package mod_auth_jwt

// custom error with type

const (
	TypeUndefined = iota
	ConfigItemRequired
	ConfigItemInvalid
	ConfigLoadFailed
	JsonDecoderError
	BuildConfigItemFailed
	BuildCondFailed
	InvalidConfigItem
	BadSecretConfig
	ModuleConfigLoadFailed
	MetricsInitFailed
	AuthServiceRegisterFailed
	MonitorServiceRegisterFailed
	HotDeploymentServiceRegisterFailed
	TokenClaimValidationFailed
)

// map error code with error name
var typeMapping = map[int]string{
	TypeUndefined:                      "TypeUndefined",
	ConfigItemRequired:                 "ConfigItemRequired",
	ConfigItemInvalid:                  "ConfigItemInvalid",
	ConfigLoadFailed:                   "ConfigLoadFailed",
	JsonDecoderError:                   "JsonDecoderError",
	BuildConfigItemFailed:              "BuildConfigItemFailed",
	BuildCondFailed:                    "BuildCondFailed",
	InvalidConfigItem:                  "InvalidConfigItem",
	BadSecretConfig:                    "BadSecretConfig",
	ModuleConfigLoadFailed:             "ModuleConfigLoadFailed",
	MetricsInitFailed:                  "MetricsInitFailed",
	AuthServiceRegisterFailed:          "AuthServiceRegisterFailed",
	MonitorServiceRegisterFailed:       "MonitorServiceRegisterFailed",
	HotDeploymentServiceRegisterFailed: "HotDeploymentServiceRegisterFailed",
	TokenClaimValidationFailed:         "TokenClaimValidationFailed",
}

type TypedError struct {
	code    int
	context error
}

func (e TypedError) Error() string {
	errorType, ok := typeMapping[e.code]
	if !ok {
		errorType = "TypedError"
	}
	return errorType + ": " + e.context.Error()
}

// Factory function to generate typed error
func NewTypedError(code int, context error) *TypedError {
	return &TypedError{code, context}
}

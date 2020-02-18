package mod_auth_jwt

// custom error with type

const (
	TypeUndefined = iota
	ConfigItemRequired
	ConfigItemInvalid
)

// map error code with error name
var typeMapping = map[int]string{
	TypeUndefined:      "TypeUndefined",
	ConfigItemRequired: "ConfigItemRequired",
	ConfigItemInvalid:  "ConfigItemInvalid",
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
func NewTypedError(code int, context error) TypedError {
	return TypedError{code, context}
}

package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

type Validator struct {
	apiKey string
}

type Failure struct {
	Code    string
	Message string
}

func NewValidator(apiKey string) Validator {
	return Validator{apiKey: apiKey}
}

func (v Validator) Validate(r *http.Request) *Failure {
	values := r.Header.Values("Authorization")
	if len(values) == 0 {
		return &Failure{Code: "missing_api_key", Message: "missing bearer token"}
	}
	if len(values) != 1 {
		return &Failure{Code: "invalid_api_key", Message: "ambiguous authorization header"}
	}
	parts := strings.SplitN(values[0], " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return &Failure{Code: "invalid_api_key", Message: "invalid authorization scheme"}
	}
	token := parts[1]
	if token == "" || strings.ContainsAny(token, "\r\n\t") {
		return &Failure{Code: "invalid_api_key", Message: "invalid bearer token"}
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(v.apiKey)) != 1 {
		return &Failure{Code: "invalid_api_key", Message: "invalid bearer token"}
	}
	return nil
}

package model

import "fmt"

// Error messages are authored locally; upstream response bodies must never be exposed.
type Error struct {
	Status  int    `json:"-"`
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
	Param   any    `json:"param"`
}

func (e *Error) Error() string { return e.Message }

func Failure(status int, code, message string) *Error {
	kind := "invalid_request_error"
	if status >= 500 {
		kind = "api_error"
	}
	if status == 401 {
		kind = "authentication_error"
	}
	if status == 429 {
		kind = "rate_limit_error"
	}
	return &Error{Status: status, Code: code, Type: kind, Message: message}
}

func Unsupported(cap Capability) *Error {
	return Failure(422, "unsupported_capability", fmt.Sprintf("model does not support verified capability %s", cap))
}

package inference

import (
	"errors"
	"fmt"
)

// ErrorCode identifies typed inference failure classes.
type ErrorCode string

const (
	ErrorCodeGatewayResolution   ErrorCode = "gateway_resolution"
	ErrorCodeSchemaLookup        ErrorCode = "schema_lookup"
	ErrorCodeGatewayFailure      ErrorCode = "gateway_failure"
	ErrorCodeOutputValidation    ErrorCode = "output_validation"
	ErrorCodeReasoningCapability ErrorCode = "reasoning_capability"
)

// Error is a typed inference error that preserves a machine-readable error code.
type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}

	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

// Unwrap returns the wrapped underlying error.
func (e *Error) Unwrap() error {
	return e.Cause
}

// WrapError wraps an error with a typed inference error code.
func WrapError(code ErrorCode, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// NewError creates a typed inference error without a wrapped cause.
func NewError(code ErrorCode, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// CodeOf extracts an inference error code from a wrapped error chain.
func CodeOf(err error) (ErrorCode, bool) {
	var typedErr *Error
	if errors.As(err, &typedErr) {
		return typedErr.Code, true
	}

	return "", false
}

// IsCode reports whether an error chain contains the provided inference error code.
func IsCode(err error, code ErrorCode) bool {
	errCode, ok := CodeOf(err)
	if !ok {
		return false
	}

	return errCode == code
}

// GatewayHTTPError captures non-success gateway HTTP status responses.
type GatewayHTTPError struct {
	StatusCode int
	Body       string
}

func (e *GatewayHTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("gateway returned HTTP %d", e.StatusCode)
	}

	return fmt.Sprintf("gateway returned HTTP %d: %s", e.StatusCode, e.Body)
}

// ReasoningCapabilityError indicates runtime reasoning capability mismatch.
type ReasoningCapabilityError struct {
	Message string
	Cause   error
}

func (e *ReasoningCapabilityError) Error() string {
	if e.Cause == nil {
		return e.Message
	}

	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

// Unwrap returns the wrapped cause.
func (e *ReasoningCapabilityError) Unwrap() error {
	return e.Cause
}

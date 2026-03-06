package harness

import (
	"errors"
	"fmt"
)

// ErrorCode identifies typed harness runner failure classes.
type ErrorCode string

const (
	ErrorCodeTemplateRender   ErrorCode = "harness_template_render"
	ErrorCodeInference        ErrorCode = "harness_inference"
	ErrorCodeOutputValidation ErrorCode = "harness_output_validation"
	ErrorCodeInfrastructure   ErrorCode = "harness_runtime_infrastructure"
	ErrorCodeLimitExceeded    ErrorCode = "harness_limit_exceeded"
)

// LimitMetadata contains deterministic limit breach metadata.
type LimitMetadata struct {
	LimitKey        string
	ConfiguredValue string
	ObservedValue   string
	NodeID          string
	StepID          string
}

// Error is a typed harness error for machine-readable failure handling.
type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
	Limit   *LimitMetadata
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}

	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

// Unwrap returns wrapped cause.
func (e *Error) Unwrap() error {
	return e.Cause
}

// NewError creates a typed harness error without wrapped cause.
func NewError(code ErrorCode, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WrapError creates a typed harness error with wrapped cause.
func WrapError(code ErrorCode, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

// NewLimitError creates a typed guardrail-limit harness error.
func NewLimitError(message string, limit LimitMetadata) *Error {
	return &Error{
		Code:    ErrorCodeLimitExceeded,
		Message: message,
		Limit:   &limit,
	}
}

// CodeOf extracts harness error code from wrapped chains.
func CodeOf(err error) (ErrorCode, bool) {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Code, true
	}

	return "", false
}

// LimitOf extracts guardrail limit metadata from wrapped chains when present.
func LimitOf(err error) (LimitMetadata, bool) {
	var typed *Error
	if !errors.As(err, &typed) || typed.Limit == nil {
		return LimitMetadata{}, false
	}
	return *typed.Limit, true
}

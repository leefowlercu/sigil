package repl

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidSessionOptions is returned when required session options are missing.
	ErrInvalidSessionOptions = errors.New("invalid session options")
	// ErrSessionClosed is returned when executing against a closed session.
	ErrSessionClosed = errors.New("session is closed")
)

// ErrorCode identifies typed REPL failure classes.
type ErrorCode string

const (
	ErrorCodeSessionInit         ErrorCode = "repl_session_init"
	ErrorCodeCodeSizeExceeded    ErrorCode = "repl_code_size_exceeded"
	ErrorCodeImportBlocked       ErrorCode = "repl_import_blocked"
	ErrorCodeExecutionTimeout    ErrorCode = "repl_execution_timeout"
	ErrorCodeExecutionCompile    ErrorCode = "repl_execution_compile"
	ErrorCodeExecutionRuntime    ErrorCode = "repl_execution_runtime"
	ErrorCodeChildDepthLimit     ErrorCode = "repl_child_depth_limit"
	ErrorCodeChildFailure        ErrorCode = "repl_child_failure"
	ErrorCodeSubcallInvalidInput ErrorCode = "repl_subcall_invalid_input"
	ErrorCodeSubcallInference    ErrorCode = "repl_subcall_inference"
	ErrorCodeSubcallEventPersist ErrorCode = "repl_subcall_event_persist"
)

// Error is a typed REPL runtime error that preserves machine-readable code.
type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

type fatalExecutionError struct {
	Cause error
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

func (e *fatalExecutionError) Error() string {
	if e == nil || e.Cause == nil {
		return ""
	}
	return e.Cause.Error()
}

func (e *fatalExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewError returns typed REPL error without cause.
func NewError(code ErrorCode, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// WrapError returns typed REPL error with cause.
func WrapError(code ErrorCode, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// CodeOf extracts typed REPL error code from wrapped chains.
func CodeOf(err error) (ErrorCode, bool) {
	var typedErr *Error
	if errors.As(err, &typedErr) {
		return typedErr.Code, true
	}

	return "", false
}

// IsCode checks for specific typed REPL error code.
func IsCode(err error, code ErrorCode) bool {
	errCode, ok := CodeOf(err)
	if !ok {
		return false
	}

	return errCode == code
}

// MarkFatalExecution marks an error as requiring immediate action termination.
func MarkFatalExecution(err error) error {
	if err == nil {
		return nil
	}
	return &fatalExecutionError{Cause: err}
}

// IsFatalExecution reports whether an error should abort the active REPL action.
func IsFatalExecution(err error) bool {
	var fatalErr *fatalExecutionError
	return errors.As(err, &fatalErr)
}

// UnwrapFatalExecution returns the underlying cause of a fatal execution error.
func UnwrapFatalExecution(err error) error {
	var fatalErr *fatalExecutionError
	if errors.As(err, &fatalErr) && fatalErr.Cause != nil {
		return fatalErr.Cause
	}
	return err
}

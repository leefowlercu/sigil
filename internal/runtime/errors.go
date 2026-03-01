package runtime

import "errors"

var (
	// ErrInvalidTransition is returned when a state transition is not allowed by the graph.
	ErrInvalidTransition = errors.New("invalid transition")
	// ErrTerminalState is returned when transitions are requested from terminal run states.
	ErrTerminalState = errors.New("terminal state")
	// ErrRunNotRunning is returned when an operation requires the run to be in running state.
	ErrRunNotRunning = errors.New("run is not running")
	// ErrParentNodeNotFound is returned when creating a child for a missing parent node.
	ErrParentNodeNotFound = errors.New("parent node not found")
	// ErrNodeNotFound is returned when node-scoped activity references a missing node.
	ErrNodeNotFound = errors.New("node not found")
	// ErrInvalidEvent is returned when an event envelope fails contract validation.
	ErrInvalidEvent = errors.New("invalid event")
	// ErrInvalidSchemaVersion is returned when schema_version is unsupported.
	ErrInvalidSchemaVersion = errors.New("invalid schema_version")
	// ErrUnknownEventType is returned when type is not a canonical v1 event.
	ErrUnknownEventType = errors.New("unknown event type")
	// ErrUnknownEnvelopeField is returned when envelope contains unknown fields.
	ErrUnknownEnvelopeField = errors.New("unknown envelope field")
	// ErrUnknownPayloadField is returned when payload contains unknown fields.
	ErrUnknownPayloadField = errors.New("unknown payload field")
	// ErrNonContiguousSequence is returned when seq is not the next contiguous value.
	ErrNonContiguousSequence = errors.New("non-contiguous sequence")
	// ErrImmutableEventLog is returned when attempting to rewrite existing event positions.
	ErrImmutableEventLog = errors.New("immutable event log")
	// ErrIntegrityFailure is returned when persisted event integrity checks fail.
	ErrIntegrityFailure = errors.New("integrity failure")
)

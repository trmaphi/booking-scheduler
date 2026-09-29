package application

import (
	"errors"
	"fmt"
)

var (
	ErrValidation                 = errors.New("validation error")
	ErrInvalidReference           = errors.New("invalid reference")
	ErrInvalidAvailabilityContext = errors.New("invalid availability context")
	ErrResourceConflict           = errors.New("booking resources are no longer available")
	ErrPersistence                = errors.New("booking persistence failed")
	ErrIdempotencyConflict        = errors.New("idempotency key was already used for different input")
	ErrNotFound                   = errors.New("resource not found")
)

// ValidationError identifies a caller-controlled field without coupling the
// application package to an HTTP error representation.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

func (e *ValidationError) Unwrap() error { return ErrValidation }

// InvalidReferenceError reports an absent or inactive booking reference.
type InvalidReferenceError struct {
	Reference string
}

func (e *InvalidReferenceError) Error() string {
	return fmt.Sprintf("%s is unknown or inactive", e.Reference)
}

func (e *InvalidReferenceError) Unwrap() error { return ErrInvalidReference }

type AvailabilityContextError struct {
	Reason string
}

func (e *AvailabilityContextError) Error() string {
	return "invalid availability context: " + e.Reason
}

func (e *AvailabilityContextError) Unwrap() error { return ErrInvalidAvailabilityContext }

package sessionx

import "errors"

var (
	// ErrNotFound is returned by a Store when no session carries the id.
	ErrNotFound = errors.New("sessionx: session not found")

	// ErrExpired is returned when a session exists but is past its TTL.
	ErrExpired = errors.New("sessionx: session expired")

	// ErrRevoked is returned when a session was deliberately ended.
	ErrRevoked = errors.New("sessionx: session revoked")

	// ErrUnsupported is returned by a Store asked for an operation its
	// design cannot honour, such as enumerating a stateless cookie store.
	ErrUnsupported = errors.New("sessionx: operation unsupported by this store")

	// ErrInvalidTransition is returned when a lifecycle move is refused.
	ErrInvalidTransition = errors.New("sessionx: invalid status transition")

	// ErrTampered is returned when a signed payload fails verification.
	ErrTampered = errors.New("sessionx: payload signature mismatch")
)

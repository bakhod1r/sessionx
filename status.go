package sessionx

import "github.com/bakhod1r/enumx"

// Status is where a session is in its life. The set is closed: a value
// outside these four is not a state this package will act on.
type Status string

const (
	// StatusActive is a session in use. Requests on it are served.
	StatusActive Status = "active"

	// StatusIdle is a session past its idle window but inside its absolute
	// TTL. It is dormant, not dead: the next request revives it.
	StatusIdle Status = "idle"

	// StatusExpired is a session past its absolute TTL. Terminal.
	StatusExpired Status = "expired"

	// StatusRevoked is a session ended deliberately — a sign-out, an
	// administrative kill. Terminal, and distinct from expired so an audit
	// log can tell the two apart.
	StatusRevoked Status = "revoked"
)

func init() {
	enumx.Register(StatusActive, StatusIdle, StatusExpired, StatusRevoked)
	enumx.RegisterTransitions(
		enumx.Transition[Status]{From: StatusActive, To: StatusIdle},
		enumx.Transition[Status]{From: StatusActive, To: StatusExpired},
		enumx.Transition[Status]{From: StatusActive, To: StatusRevoked},
		enumx.Transition[Status]{From: StatusIdle, To: StatusActive},
		enumx.Transition[Status]{From: StatusIdle, To: StatusExpired},
		enumx.Transition[Status]{From: StatusIdle, To: StatusRevoked},
	)
}

// Valid reports whether s is one of the four known states.
func (s Status) Valid() bool { return enumx.IsValid(s) }

// CanMoveTo reports whether the lifecycle permits s -> next. Terminal states
// permit nothing, which is what makes them terminal.
func (s Status) CanMoveTo(next Status) bool { return enumx.CanTransition(s, next) }

// Terminal reports whether s admits no further transition.
func (s Status) Terminal() bool { return s == StatusExpired || s == StatusRevoked }

// String returns the state as it is stored and serialised.
func (s Status) String() string { return string(s) }

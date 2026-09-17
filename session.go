package sessionx

import (
	"fmt"
	"maps"
	"time"
)

// Session is one authenticated or anonymous visit: who, from what, from
// where, and for how much longer.
type Session struct {
	// ID is the opaque identifier the client presents. See NewID.
	ID string

	// UserID is the caller's subject. Empty for an anonymous session.
	UserID string

	// Status is the lifecycle state. See Status.
	Status Status

	// Device is the hardware and software that opened the session.
	Device Device

	// Client is what kind of thing opened it — human, bot, tool — and how
	// well its claims held together. See Client.
	Client Client

	// Network is where it was opened from.
	Network Network

	// Locale is the language tag resolved for this session, for example
	// "uz". Empty when no resolver was configured.
	Locale string

	// LocaleSource records where the locale came from: "url", "query",
	// "cookie", "header" or "default". It is what makes a "why is the site
	// in the wrong language" report answerable.
	LocaleSource string

	// Data is the caller's own payload. sessionx neither reads nor
	// interprets it.
	Data map[string]any

	// CreatedAt is when the session began.
	CreatedAt time.Time

	// LastSeen is the last request served on it, which is what the idle
	// window is measured from.
	LastSeen time.Time

	// ExpiresAt is the absolute deadline. It is not extended by activity;
	// only Renew moves it.
	ExpiresAt time.Time

	// RenewedAt is when the id was last rotated, zero if never.
	RenewedAt time.Time
}

// Expired reports whether the absolute deadline has passed at now.
func (s *Session) Expired(now time.Time) bool {
	return !s.ExpiresAt.IsZero() && now.After(s.ExpiresAt)
}

// IdleSince reports whether the session has gone untouched for longer than
// idle. A zero idle disables the check: some applications want only an
// absolute lifetime.
func (s *Session) IdleSince(now time.Time, idle time.Duration) bool {
	if idle <= 0 {
		return false
	}
	return now.Sub(s.LastSeen) > idle
}

// MoveTo advances the lifecycle, refusing any move the FSM does not permit
// and leaving the session untouched when it refuses.
func (s *Session) MoveTo(next Status) error {
	if !s.Status.CanMoveTo(next) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, s.Status, next)
	}
	s.Status = next
	return nil
}

// Clone returns a copy that shares nothing mutable with the original. A
// Store that hands out its own memory must clone, or a caller's write to
// Data silently mutates the stored session.
func (s *Session) Clone() *Session {
	if s == nil {
		return nil
	}
	c := *s
	if s.Data != nil {
		c.Data = maps.Clone(s.Data)
	}
	return &c
}

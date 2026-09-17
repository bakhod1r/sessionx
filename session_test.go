package sessionx

import (
	"testing"
	"time"
)

func newTestSession(now time.Time) *Session {
	return &Session{
		ID:        "test-id",
		UserID:    "u1",
		Status:    StatusActive,
		Data:      map[string]any{"role": "admin"},
		CreatedAt: now,
		LastSeen:  now,
		ExpiresAt: now.Add(time.Hour),
	}
}

func TestSessionExpired(t *testing.T) {
	now := time.Now()
	s := newTestSession(now)

	if s.Expired(now.Add(59 * time.Minute)) {
		t.Fatal("not expired before ExpiresAt")
	}
	if !s.Expired(now.Add(time.Hour + time.Second)) {
		t.Fatal("expired after ExpiresAt")
	}
}

func TestSessionIdleSince(t *testing.T) {
	now := time.Now()
	s := newTestSession(now)

	if s.IdleSince(now.Add(5*time.Minute), 15*time.Minute) {
		t.Fatal("not idle inside the window")
	}
	if !s.IdleSince(now.Add(16*time.Minute), 15*time.Minute) {
		t.Fatal("idle outside the window")
	}
	if s.IdleSince(now.Add(99*time.Hour), 0) {
		t.Fatal("a zero idle window disables the check entirely")
	}
}

func TestSessionMoveToEnforcesTheFSM(t *testing.T) {
	s := newTestSession(time.Now())

	if err := s.MoveTo(StatusRevoked); err != nil {
		t.Fatalf("active -> revoked must be allowed: %v", err)
	}
	if s.Status != StatusRevoked {
		t.Fatalf("status = %q, want revoked", s.Status)
	}
	if err := s.MoveTo(StatusActive); err == nil {
		t.Fatal("revoked -> active must be refused")
	}
	if s.Status != StatusRevoked {
		t.Fatal("a refused transition must leave the status untouched")
	}
}

func TestSessionCloneIsDeep(t *testing.T) {
	s := newTestSession(time.Now())
	c := s.Clone()
	c.Data["role"] = "guest"

	if s.Data["role"] != "admin" {
		t.Fatal("Clone must not share the Data map")
	}
}

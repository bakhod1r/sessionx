package sessionx

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeStore is a minimal in-package Store so manager_test does not import
// the memory package and create an import cycle through storetest.
type fakeStore struct {
	byID map[string]*Session

	// failSaveOnCall, when non-zero, makes the Nth call to Save (1-indexed)
	// fail with errFake instead of writing.
	failSaveOnCall int
	saveCalls      int
	// failDelete makes every Delete fail with errFake.
	failDelete bool
}

var errFake = errors.New("fakeStore: injected failure")

func newFake() *fakeStore { return &fakeStore{byID: map[string]*Session{}} }

func (f *fakeStore) Save(_ context.Context, s *Session) error {
	f.saveCalls++
	if f.failSaveOnCall != 0 && f.saveCalls == f.failSaveOnCall {
		return errFake
	}
	f.byID[s.ID] = s.Clone()
	return nil
}

func (f *fakeStore) Load(_ context.Context, id string) (*Session, error) {
	s, ok := f.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	return s.Clone(), nil
}

func (f *fakeStore) Delete(_ context.Context, id string) error {
	if f.failDelete {
		return errFake
	}
	delete(f.byID, id)
	return nil
}

func (f *fakeStore) ListByUser(_ context.Context, userID string) ([]*Session, error) {
	var out []*Session
	for _, s := range f.byID {
		if s.UserID == userID {
			out = append(out, s.Clone())
		}
	}
	return out, nil
}

func (f *fakeStore) DeleteByUser(_ context.Context, userID string) (int, error) {
	n := 0
	for id, s := range f.byID {
		if s.UserID == userID {
			delete(f.byID, id)
			n++
		}
	}
	return n, nil
}

// androidUA is declared in device_test.go — same package, one definition.

func testManager(t *testing.T, now *time.Time, opts Options) (*Manager, *fakeStore) {
	t.Helper()
	store := newFake()
	opts.Now = func() time.Time { return *now }
	return NewManager(store, opts), store
}

func TestCollectFillsDeviceAndNetwork(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour})

	s, err := m.Collect(context.Background(), Input{
		UserID:     "u1",
		UserAgent:  androidUA,
		RemoteAddr: "203.0.113.7:51234",
		Data:       map[string]any{"role": "admin"},
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if s.ID == "" {
		t.Fatal("Collect must issue an id")
	}
	if s.Status != StatusActive {
		t.Fatalf("status = %q, want active", s.Status)
	}
	if s.Device.Brand != "Samsung" {
		t.Fatalf("device not collected: %+v", s.Device)
	}
	if s.Network.IP != "203.0.113.7" {
		t.Fatalf("network not collected: %+v", s.Network)
	}
	if !s.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("ExpiresAt = %v, want now+TTL", s.ExpiresAt)
	}
}

func TestGetRefusesAnExpiredSession(t *testing.T) {
	now := time.Now()
	m, store := testManager(t, &now, Options{TTL: time.Hour})
	s, _ := m.Collect(context.Background(), Input{UserID: "u1"})

	now = now.Add(2 * time.Hour)

	_, err := m.Get(context.Background(), s.ID)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want ErrExpired", err)
	}
	got, err := store.Load(context.Background(), s.ID)
	if err != nil {
		t.Fatalf("the expired session should still be stored for audit: %v", err)
	}
	if got.Status != StatusExpired {
		t.Fatalf("status = %q, want the store updated to expired", got.Status)
	}
}

func TestGetRefusesARevokedSession(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour})
	s, _ := m.Collect(context.Background(), Input{UserID: "u1"})

	if err := m.Revoke(context.Background(), s.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := m.Get(context.Background(), s.ID); !errors.Is(err, ErrRevoked) {
		t.Fatalf("err = %v, want ErrRevoked", err)
	}
}

func TestTouchRevivesAnIdleSession(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour, IdleTimeout: 15 * time.Minute})
	s, _ := m.Collect(context.Background(), Input{UserID: "u1"})

	now = now.Add(20 * time.Minute)

	got, err := m.Touch(context.Background(), s.ID)
	if err != nil {
		t.Fatalf("Touch on an idle-but-live session must succeed: %v", err)
	}
	if got.Status != StatusActive {
		t.Fatalf("status = %q, want active after Touch", got.Status)
	}
	if !got.LastSeen.Equal(now) {
		t.Fatalf("LastSeen = %v, want it moved to now", got.LastSeen)
	}
}

func TestRenewRotatesTheID(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour})
	s, _ := m.Collect(context.Background(), Input{UserID: "u1", Data: map[string]any{"role": "admin"}})
	oldID := s.ID

	now = now.Add(10 * time.Minute)

	got, err := m.Renew(context.Background(), oldID)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if got.ID == oldID {
		t.Fatal("Renew must rotate the id — that is the fixation defence")
	}
	if got.Data["role"] != "admin" {
		t.Fatal("Renew must carry the payload across")
	}
	if !got.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("ExpiresAt = %v, want now+TTL", got.ExpiresAt)
	}
	if _, err := m.Get(context.Background(), oldID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the old id must be gone, got %v", err)
	}
}

// TestRenewLeavesOldIDRevokedWhenDeleteFails proves the session-fixation
// property this round's Renew ordering exists to guarantee: even when the
// final Delete of the old row fails, the old id must not still load as a
// live session. Against the old "save next, then delete old" ordering, the
// old row is untouched on a Delete failure and stays StatusActive, so this
// test fails there.
func TestRenewLeavesOldIDRevokedWhenDeleteFails(t *testing.T) {
	now := time.Now()
	m, store := testManager(t, &now, Options{TTL: time.Hour})
	s, _ := m.Collect(context.Background(), Input{UserID: "u1"})
	oldID := s.ID

	store.failDelete = true

	_, err := m.Renew(context.Background(), oldID)
	if !errors.Is(err, errFake) {
		t.Fatalf("Renew: err = %v, want errFake", err)
	}

	store.failDelete = false
	if _, err := m.Get(context.Background(), oldID); !errors.Is(err, ErrRevoked) {
		t.Fatalf("old id must be revoked, not live, got %v", err)
	}
}

// TestRenewLeavesOldIDRevokedWhenSavingReplacementFails proves the same
// property for the other partial-failure point: if saving the replacement
// session fails, the old id must already be revoked rather than still live.
// Against the old ordering (save next before touching old), old is never
// modified when the save fails and this test fails.
func TestRenewLeavesOldIDRevokedWhenSavingReplacementFails(t *testing.T) {
	now := time.Now()
	m, store := testManager(t, &now, Options{TTL: time.Hour})
	s, _ := m.Collect(context.Background(), Input{UserID: "u1"})
	oldID := s.ID

	// Renew's fixed Save order is: revoke-old, then save-next. Reset the
	// call counter so it counts only Renew's own saves, then let the first
	// (the revocation) succeed and fail the second (the replacement).
	store.saveCalls = 0
	store.failSaveOnCall = 2

	_, err := m.Renew(context.Background(), oldID)
	if !errors.Is(err, errFake) {
		t.Fatalf("Renew: err = %v, want errFake", err)
	}

	if _, err := m.Get(context.Background(), oldID); !errors.Is(err, ErrRevoked) {
		t.Fatalf("old id must be revoked, not live, got %v", err)
	}
}

func TestMaxPerUserEvictsTheOldest(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour, MaxPerUser: 2})
	ctx := context.Background()

	first, _ := m.Collect(ctx, Input{UserID: "u1"})
	now = now.Add(time.Minute)
	_, _ = m.Collect(ctx, Input{UserID: "u1"})
	now = now.Add(time.Minute)
	_, _ = m.Collect(ctx, Input{UserID: "u1"})

	live, err := m.ListUser(ctx, "u1")
	if err != nil {
		t.Fatalf("ListUser: %v", err)
	}
	if len(live) != 2 {
		t.Fatalf("got %d live sessions, want MaxPerUser = 2", len(live))
	}
	if _, err := m.Get(ctx, first.ID); err == nil {
		t.Fatal("the oldest session must have been evicted")
	}
}

func TestRevokeUser(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour})
	ctx := context.Background()
	_, _ = m.Collect(ctx, Input{UserID: "u1"})
	_, _ = m.Collect(ctx, Input{UserID: "u1"})
	other, _ := m.Collect(ctx, Input{UserID: "u2"})

	n, err := m.RevokeUser(ctx, "u1")
	if err != nil {
		t.Fatalf("RevokeUser: %v", err)
	}
	if n != 2 {
		t.Fatalf("revoked %d, want 2", n)
	}
	if _, err := m.Get(ctx, other.ID); err != nil {
		t.Fatalf("another user's session was revoked: %v", err)
	}
}

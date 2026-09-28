package cookiestore_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bakhod1r/sessionx"
	"github.com/bakhod1r/sessionx/cookiestore"
)

var key = []byte("test-key-at-least-32-bytes-long!!")

func mustNew(t *testing.T, keys ...[]byte) *cookiestore.Store {
	t.Helper()
	st, err := cookiestore.New(keys...)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func sample() *sessionx.Session {
	now := time.Now().UTC().Truncate(time.Second)
	return &sessionx.Session{
		ID:        "s1",
		UserID:    "u1",
		Status:    sessionx.StatusActive,
		Device:    sessionx.Device{Raw: "ua", Brand: "Samsung", Name: "Galaxy S10"},
		Network:   sessionx.Network{IP: "203.0.113.7"},
		Data:      map[string]any{"role": "admin"},
		CreatedAt: now,
		LastSeen:  now,
		ExpiresAt: now.Add(time.Hour),
	}
}

func TestNewRejectsWeakKeys(t *testing.T) {
	if _, err := cookiestore.New(); err == nil {
		t.Fatal("New with no key succeeded")
	}
	if _, err := cookiestore.New([]byte("short")); !errors.Is(err, cookiestore.ErrWeakKey) {
		t.Fatalf("New(short) err = %v, want ErrWeakKey", err)
	}
}

func TestTokenRoundTrip(t *testing.T) {
	st := mustNew(t, key)
	token, err := st.Token(sample())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	got, err := st.Load(t.Context(), token)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.UserID != "u1" || got.Device.Brand != "Samsung" || got.Data["role"] != "admin" {
		t.Fatalf("round trip lost fields: %+v", got)
	}
}

func TestLoadRejectsATamperedToken(t *testing.T) {
	st := mustNew(t, key)
	token, _ := st.Token(sample())
	parts := strings.SplitN(token, ".", 2)
	tampered := parts[0][:len(parts[0])-1] + "A" + "." + parts[1]

	_, err := st.Load(t.Context(), tampered)
	if !errors.Is(err, sessionx.ErrTampered) || !errors.Is(err, sessionx.ErrNotFound) {
		t.Fatalf("err = %v, want ErrTampered wrapped as ErrNotFound", err)
	}
}

func TestLoadRejectsAnotherKeysToken(t *testing.T) {
	token, _ := mustNew(t, key).Token(sample())
	other := mustNew(t, []byte("a-completely-different-key-here!!"))
	if _, err := other.Load(t.Context(), token); !errors.Is(err, sessionx.ErrTampered) {
		t.Fatalf("err = %v, want ErrTampered", err)
	}
}

// Key rotation: the first key signs, every key verifies, so tokens issued
// before a rotation keep working until they expire.
func TestKeyRotation(t *testing.T) {
	oldKey := []byte("the-old-signing-key-32-bytes-long!")
	token, _ := mustNew(t, oldKey).Token(sample())

	rotated := mustNew(t, key, oldKey)
	if _, err := rotated.Load(t.Context(), token); err != nil {
		t.Fatalf("old token rejected after rotation: %v", err)
	}
	fresh, _ := rotated.Token(sample())
	if _, err := mustNew(t, key).Load(t.Context(), fresh); err != nil {
		t.Fatalf("new token not signed with the first key: %v", err)
	}
}

func TestTokenTooLargeForACookie(t *testing.T) {
	s := sample()
	s.Data = map[string]any{"blob": strings.Repeat("x", 5000)}
	if _, err := mustNew(t, key).Token(s); !errors.Is(err, cookiestore.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

// The store holds nothing: memory does not grow with sessions saved.
func TestSaveKeepsNoState(t *testing.T) {
	st := mustNew(t, key)
	for i := 0; i < 1000; i++ {
		s := sample()
		s.ID = strings.Repeat("x", i%7) + string(rune('a'+i%26))
		if err := st.Save(t.Context(), s); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.Load(t.Context(), "a"); !errors.Is(err, sessionx.ErrNotFound) {
		t.Fatalf("Load by bare id = %v, want ErrNotFound: nothing is kept server-side", err)
	}
}

// Revocation needs server state this store does not have; it says so.
func TestRevokeIsUnsupported(t *testing.T) {
	st := mustNew(t, key)
	s := sample()
	s.Status = sessionx.StatusRevoked
	if err := st.Save(t.Context(), s); !errors.Is(err, sessionx.ErrUnsupported) {
		t.Fatalf("Save(revoked) = %v, want ErrUnsupported", err)
	}
	if _, err := st.ListByUser(t.Context(), "u1"); !errors.Is(err, sessionx.ErrUnsupported) {
		t.Fatalf("ListByUser err = %v", err)
	}
	if _, err := st.DeleteByUser(t.Context(), "u1"); !errors.Is(err, sessionx.ErrUnsupported) {
		t.Fatalf("DeleteByUser err = %v", err)
	}
}

// End to end through the Manager: the cookie carries the session, a second
// process with the same key serves it, and every touch refreshes the cookie.
func TestManagerAcrossProcesses(t *testing.T) {
	a := sessionx.NewManager(mustNew(t, key), sessionx.Options{TTL: time.Hour})
	b := sessionx.NewManager(mustNew(t, key), sessionx.Options{TTL: time.Hour})

	s, err := a.Collect(t.Context(), sessionx.Input{UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if err := a.Issue(rec, s, sessionx.DefaultCookie()); err != nil {
		t.Fatal(err)
	}
	cookie := rec.Result().Cookies()[0]
	if cookie.Value == s.ID {
		t.Fatal("stateless store issued the bare id; the cookie must carry the signed session")
	}

	var seen *sessionx.Session
	h := b.Middleware(sessionx.DefaultCookie())(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen, _ = sessionx.FromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req)
	if seen == nil || seen.UserID != "u1" {
		t.Fatalf("second process did not recover the session: %+v", seen)
	}
	if len(rec2.Result().Cookies()) != 1 {
		t.Fatal("middleware did not refresh the cookie after a touch")
	}

	if err := b.Revoke(t.Context(), cookie.Value); !errors.Is(err, sessionx.ErrUnsupported) {
		t.Fatalf("Revoke = %v, want ErrUnsupported", err)
	}
}

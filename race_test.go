package sessionx_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bakhod1r/sessionx"
	"github.com/bakhod1r/sessionx/memory"
)

// hookStore runs beforeSave once, just before the next Save reaches the
// store. It stands in for another request that lands between a Load and a
// Save.
type hookStore struct {
	sessionx.Store
	mu         sync.Mutex
	beforeSave func()
}

func (h *hookStore) Save(ctx context.Context, s *sessionx.Session) error {
	h.mu.Lock()
	f := h.beforeSave
	h.beforeSave = nil
	h.mu.Unlock()
	if f != nil {
		f()
	}
	return h.Store.Save(ctx, s)
}

func TestTouchCannotUndoAConcurrentRevoke(t *testing.T) {
	ctx := context.Background()
	st := &hookStore{Store: memory.New()}
	m := sessionx.NewManager(st, sessionx.Options{TTL: time.Hour})
	s, err := m.Collect(ctx, sessionx.Input{UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}

	st.beforeSave = func() {
		if err := m.Revoke(ctx, s.ID); err != nil {
			t.Errorf("Revoke: %v", err)
		}
	}
	if _, err := m.Touch(ctx, s.ID); !errors.Is(err, sessionx.ErrRevoked) {
		t.Fatalf("Touch after a concurrent revoke: err = %v, want ErrRevoked", err)
	}
	if _, err := m.Get(ctx, s.ID); !errors.Is(err, sessionx.ErrRevoked) {
		t.Fatalf("the sign-out was undone: Get err = %v", err)
	}
}

func TestTouchCannotRestoreARenewedID(t *testing.T) {
	ctx := context.Background()
	st := &hookStore{Store: memory.New()}
	m := sessionx.NewManager(st, sessionx.Options{TTL: time.Hour})
	s, _ := m.Collect(ctx, sessionx.Input{UserID: "u1"})

	st.beforeSave = func() {
		if _, err := m.Renew(ctx, s.ID); err != nil {
			t.Errorf("Renew: %v", err)
		}
	}
	_, _ = m.Touch(ctx, s.ID)
	if _, err := m.Get(ctx, s.ID); err == nil {
		t.Fatal("the pre-rotation id is live again after a racing Touch")
	}
}

func TestConcurrentRenewDoesNotFork(t *testing.T) {
	ctx := context.Background()
	st := &hookStore{Store: memory.New()}
	m := sessionx.NewManager(st, sessionx.Options{TTL: time.Hour})
	s, _ := m.Collect(ctx, sessionx.Input{UserID: "u1"})

	st.beforeSave = func() {
		if _, err := m.Renew(ctx, s.ID); err != nil {
			t.Errorf("inner Renew: %v", err)
		}
	}
	if _, err := m.Renew(ctx, s.ID); !errors.Is(err, sessionx.ErrRevoked) {
		t.Fatalf("second Renew of one id: err = %v, want ErrRevoked", err)
	}
	live, err := m.ListUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 {
		t.Fatalf("%d live sessions after two racing renews, want 1", len(live))
	}
}

// flakyStore fails Load with a transient error.
type flakyStore struct{ sessionx.Store }

var errBlip = errors.New("connection reset")

func (flakyStore) Load(context.Context, string) (*sessionx.Session, error) { return nil, errBlip }

func TestMiddlewareKeepsCookieOnTransientStoreError(t *testing.T) {
	m := sessionx.NewManager(flakyStore{memory.New()}, sessionx.Options{TTL: time.Hour})
	h := m.Middleware(sessionx.DefaultCookie())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionx.DefaultCookieName, Value: "some-id"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if n := len(rec.Result().Cookies()); n != 0 {
		t.Fatalf("a store outage cleared the session cookie (%d cookies set)", n)
	}
}

func TestZeroCookieConfigIsSecure(t *testing.T) {
	m := sessionx.NewManager(memory.New(), sessionx.Options{TTL: time.Hour})
	s, _ := m.Collect(context.Background(), sessionx.Input{})
	rec := httptest.NewRecorder()
	m.Issue(rec, s, sessionx.CookieConfig{Name: "sid"})
	c := rec.Result().Cookies()[0]
	if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("zero CookieConfig gave Secure=%v HttpOnly=%v SameSite=%v, want true/true/Lax", c.Secure, c.HttpOnly, c.SameSite)
	}
}

// A row with a status this package does not know (a bad write, another
// writer) is not a live session.
func TestUnknownStatusIsNotLive(t *testing.T) {
	st := memory.New()
	m := sessionx.NewManager(st, sessionx.Options{TTL: time.Hour})
	now := time.Now()
	_ = st.Save(context.Background(), &sessionx.Session{ID: "x", Status: "", ExpiresAt: now.Add(time.Hour)})
	if _, err := m.Get(context.Background(), "x"); !errors.Is(err, sessionx.ErrNotFound) {
		t.Fatalf("Get = %v, want ErrNotFound for an unknown status", err)
	}
}

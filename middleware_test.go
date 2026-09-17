package sessionx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bakhod1r/alx"
)

func TestMiddlewareAttachesAnExistingSession(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour})
	s, _ := m.Collect(t.Context(), Input{UserID: "u1", UserAgent: androidUA, RemoteAddr: "203.0.113.7:1"})

	var seen *Session
	h := m.Middleware(DefaultCookie())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := FromContext(r.Context())
		if !ok {
			t.Fatal("middleware did not attach the session")
		}
		seen = got
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: DefaultCookie().Name, Value: s.ID})
	h.ServeHTTP(httptest.NewRecorder(), req)

	if seen.UserID != "u1" {
		t.Fatalf("attached the wrong session: %+v", seen)
	}
	if seen.Device.Brand != "Samsung" {
		t.Fatalf("device lost: %+v", seen.Device)
	}
}

func TestMiddlewareWithoutACookieAttachesNothing(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour})

	called := false
	h := m.Middleware(DefaultCookie())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if _, ok := FromContext(r.Context()); ok {
			t.Fatal("no cookie must mean no session, not a fabricated one")
		}
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !called {
		t.Fatal("the middleware must always call the next handler")
	}
}

func TestMiddlewareClearsAnExpiredCookie(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour})
	s, _ := m.Collect(t.Context(), Input{UserID: "u1"})

	now = now.Add(2 * time.Hour)

	h := m.Middleware(DefaultCookie())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: DefaultCookie().Name, Value: s.ID})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Fatalf("an expired session must clear its cookie, got %+v", cookies)
	}
}

func TestIssueSetsASecureCookie(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour})
	s, _ := m.Collect(t.Context(), Input{UserID: "u1"})

	rec := httptest.NewRecorder()
	m.Issue(rec, s, DefaultCookie())

	c := rec.Result().Cookies()[0]
	if c.Value != s.ID {
		t.Fatalf("cookie value = %q, want the session id", c.Value)
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("insecure cookie defaults: %+v", c)
	}
}

func TestIssueWithZeroExpiresAtWritesASessionCookie(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour})

	s := &Session{ID: "sess-1", UserID: "u1"}

	rec := httptest.NewRecorder()
	m.Issue(rec, s, DefaultCookie())

	c := rec.Result().Cookies()[0]
	if c.MaxAge != 0 {
		t.Fatalf("MaxAge = %d, want 0 (net/http's way of saying the attribute was omitted)", c.MaxAge)
	}
	if !c.Expires.IsZero() {
		t.Fatalf("Expires = %v, want zero — a session with no deadline must not get one", c.Expires)
	}
}

func TestIssueWithAFutureDeadlineWritesAPositiveMaxAge(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour})
	s, _ := m.Collect(t.Context(), Input{UserID: "u1"})

	rec := httptest.NewRecorder()
	m.Issue(rec, s, DefaultCookie())

	c := rec.Result().Cookies()[0]
	if c.MaxAge <= 0 {
		t.Fatalf("MaxAge = %d, want a positive number of seconds", c.MaxAge)
	}
}

func TestIssueWithAPastDeadlineWritesANegativeMaxAge(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour})
	s, _ := m.Collect(t.Context(), Input{UserID: "u1"})

	s.ExpiresAt = now.Add(-time.Minute)

	rec := httptest.NewRecorder()
	m.Issue(rec, s, DefaultCookie())

	c := rec.Result().Cookies()[0]
	if c.MaxAge >= 0 {
		t.Fatalf("MaxAge = %d, want negative — an already-expired session must tell the browser to delete the cookie", c.MaxAge)
	}
}

func TestInputFromReadsTheRequest(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", androidUA)
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	req.RemoteAddr = "10.0.0.5:4444"

	in := m.InputFrom(req, "u1")

	if in.UserAgent != androidUA || in.RemoteAddr != "10.0.0.5:4444" || in.Forwarded != "198.51.100.9" {
		t.Fatalf("InputFrom lost a field: %+v", in)
	}
	if in.UserID != "u1" {
		t.Fatalf("user = %q, want u1", in.UserID)
	}
	if in.Device.Brand != "Samsung" {
		t.Fatalf("InputFrom must collect the device from the whole request: %+v", in.Device)
	}
}

func TestInputFromResolvesTheLocale(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{
		TTL: time.Hour,
		Locale: alx.NewResolver(
			alx.WithSupported("en", "uz", "ru"),
			alx.WithCookie("lang"),
			alx.WithHeader(),
			alx.WithDefault("en"),
		),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "uz-UZ,uz;q=0.9,en;q=0.5")

	in := m.InputFrom(req, "u1")

	if in.Locale != "uz" {
		t.Fatalf("locale = %q, want uz", in.Locale)
	}
	if in.LocaleSource != string(alx.SourceHeader) {
		t.Fatalf("locale source = %q, want header", in.LocaleSource)
	}
}

func TestInputFromWithoutAResolverRecordsNoLocale(t *testing.T) {
	now := time.Now()
	m, _ := testManager(t, &now, Options{TTL: time.Hour})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "uz-UZ,uz;q=0.9")

	if in := m.InputFrom(req, "u1"); in.Locale != "" {
		t.Fatalf("locale = %q, want empty — no resolver was configured", in.Locale)
	}
}

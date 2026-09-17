package sessionx

import (
	"context"
	"net/http"
)

// ctxKey is unexported so no other package can collide with or forge the
// context value.
type ctxKey struct{}

// FromContext returns the session the middleware attached, if any. A false
// second return is the honest answer for an anonymous request: this package
// never fabricates a session to keep a handler's code shorter.
func FromContext(ctx context.Context) (*Session, bool) {
	s, ok := ctx.Value(ctxKey{}).(*Session)
	return s, ok
}

// NewContext returns a context carrying s. Framework adapters use it to
// attach a session to their own request context.
func NewContext(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// InputFrom reads the request facts a session is collected from.
//
// The device is resolved here rather than in Collect because only here is
// the whole request available: Client Hints live in headers Collect would
// never see, and they are where a modern Chrome actually states its
// platform and model.
func (m *Manager) InputFrom(r *http.Request, userID string) Input {
	device, client := DeviceFromRequest(r)

	in := Input{
		UserID:     userID,
		UserAgent:  r.UserAgent(),
		RemoteAddr: r.RemoteAddr,
		Forwarded:  r.Header.Get("X-Forwarded-For"),
		Device:     device,
		Client:     client,
	}

	if m.opts.Locale != nil {
		res := m.opts.Locale.ResolveRequest(r)
		in.Locale = res.Tag
		in.LocaleSource = string(res.Source)
	}
	return in
}

// Middleware reads the session cookie, touches the session, and attaches it
// to the request context.
//
// It never rejects a request. A missing, unknown, expired or revoked session
// means the handler simply sees no session — deciding what an anonymous
// request may do is the application's business, not this package's. An
// invalid cookie is cleared on the way out so the browser stops sending it.
//
// This covers net/http, chi, gorilla/mux and httprouter alike: all four
// speak http.Handler.
func (m *Manager) Middleware(c CookieConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(c.name())
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}

			s, err := m.Touch(r.Context(), cookie.Value)
			if err != nil {
				m.Clear(w, c)
				next.ServeHTTP(w, r)
				return
			}

			next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), s)))
		})
	}
}

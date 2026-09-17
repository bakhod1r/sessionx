package sessionx

import (
	"net/http"
	"time"
)

// DefaultCookieName is the cookie a session travels in unless configured
// otherwise. The __Host- prefix is not used by default because it forbids a
// Domain and requires HTTPS, which would break local development; an
// application serving only HTTPS should set it.
const DefaultCookieName = "sessionx_id"

// CookieConfig describes the cookie a session id travels in.
type CookieConfig struct {
	// Name is the cookie name.
	Name string

	// Path scopes the cookie. Defaults to "/".
	Path string

	// Domain scopes the cookie to a host. Empty means the origin host only,
	// which is the safer default.
	Domain string

	// Secure restricts the cookie to HTTPS. Default true.
	Secure bool

	// HTTPOnly hides the cookie from scripts. Default true, and there is no
	// good reason to turn it off: a session id readable by JavaScript is a
	// session id stealable by one XSS.
	HTTPOnly bool

	// SameSite controls cross-site sending. Default Lax.
	SameSite http.SameSite
}

// DefaultCookie returns the secure defaults: HttpOnly, Secure, SameSite=Lax,
// path "/".
func DefaultCookie() CookieConfig {
	return CookieConfig{
		Name:     DefaultCookieName,
		Path:     "/",
		Secure:   true,
		HTTPOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

// Issue writes the session id to the response as a cookie whose lifetime
// matches the session's own deadline. There are three cases: a zero
// ExpiresAt means the session has no absolute deadline (see session.go), so
// Issue omits both Expires and Max-Age and lets the browser treat it as a
// session cookie, kept until the browser closes — the honest transport-level
// reading of "no deadline", rather than the huge negative Max-Age a naive
// subtraction would produce, which a browser reads as "delete immediately".
// A deadline in the future gets Expires set and a Max-Age of at least one
// second, so a deadline a fraction of a second away does not truncate to
// zero and get misread as "no deadline" by net/http, which omits a zero
// Max-Age. A deadline already in the past gets Expires set and a negative
// Max-Age, which does mean "delete this" and is left alone.
func (m *Manager) Issue(w http.ResponseWriter, s *Session, c CookieConfig) {
	cookie := &http.Cookie{
		Name:     c.name(),
		Value:    s.ID,
		Path:     c.path(),
		Domain:   c.Domain,
		Secure:   c.Secure,
		HttpOnly: c.HTTPOnly,
		SameSite: c.SameSite,
	}

	if !s.ExpiresAt.IsZero() {
		cookie.Expires = s.ExpiresAt
		if remaining := time.Until(s.ExpiresAt); remaining > 0 {
			maxAge := int(remaining.Seconds())
			if maxAge < 1 {
				maxAge = 1
			}
			cookie.MaxAge = maxAge
		} else {
			maxAge := int(remaining.Seconds())
			if maxAge > -1 {
				maxAge = -1
			}
			cookie.MaxAge = maxAge
		}
	}

	http.SetCookie(w, cookie)
}

// Clear removes the session cookie from the client.
func (m *Manager) Clear(w http.ResponseWriter, c CookieConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     c.name(),
		Value:    "",
		Path:     c.path(),
		Domain:   c.Domain,
		MaxAge:   -1,
		Secure:   c.Secure,
		HttpOnly: c.HTTPOnly,
		SameSite: c.SameSite,
	})
}

func (c CookieConfig) name() string {
	if c.Name == "" {
		return DefaultCookieName
	}
	return c.Name
}

func (c CookieConfig) path() string {
	if c.Path == "" {
		return "/"
	}
	return c.Path
}

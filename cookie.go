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
// matches the session's own deadline.
func (m *Manager) Issue(w http.ResponseWriter, s *Session, c CookieConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     c.name(),
		Value:    s.ID,
		Path:     c.path(),
		Domain:   c.Domain,
		Expires:  s.ExpiresAt,
		MaxAge:   int(time.Until(s.ExpiresAt).Seconds()),
		Secure:   c.Secure,
		HttpOnly: c.HTTPOnly,
		SameSite: c.SameSite,
	})
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

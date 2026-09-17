package sessionx

import (
	"net/netip"
	"time"

	"github.com/bakhod1r/alx"
)

// DefaultTTL is the absolute lifetime applied when Options.TTL is zero.
const DefaultTTL = 24 * time.Hour

// Options configures a Manager. The zero value is usable: it yields a
// 24-hour absolute lifetime, no idle timeout, no per-user cap and no
// trusted proxies.
type Options struct {
	// TTL is the absolute lifetime of a session, measured from creation or
	// from the last Renew. Activity does not extend it. Zero means
	// DefaultTTL.
	TTL time.Duration

	// IdleTimeout is how long a session may go untouched before it is
	// treated as idle. Zero disables the idle check.
	IdleTimeout time.Duration

	// MaxPerUser caps how many live sessions one user may hold. When the cap
	// is reached, collecting a new session revokes the least recently seen.
	// Zero means no cap.
	MaxPerUser int

	// TrustedProxies are the networks whose X-Forwarded-For header this
	// Manager will believe. Empty means the header is recorded but never
	// acted on. See NetworkFrom.
	TrustedProxies []netip.Prefix

	// Locale resolves the session's language across URL, query, cookie and
	// Accept-Language. Nil means no locale is recorded — Session.Locale
	// stays empty rather than being guessed from the header alone.
	//
	// It is an *alx.Resolver because deciding the language is alx's whole
	// subject: quality values, wildcards, fallback chains and provenance
	// are already solved there and would be re-implemented badly here.
	Locale *alx.Resolver

	// Now is the clock. Nil means time.Now. It exists so tests can move
	// time without sleeping.
	Now func() time.Time
}

func (o Options) ttl() time.Duration {
	if o.TTL <= 0 {
		return DefaultTTL
	}
	return o.TTL
}

func (o Options) now() time.Time {
	if o.Now == nil {
		return time.Now()
	}
	return o.Now()
}

// Package cookiestore is a sessionx Store that keeps no server state: the
// whole session travels in the client's cookie, signed with HMAC-SHA256.
//
// The trade is explicit. Nothing to provision, nothing to scale, no lookup on
// the hot path, and any process holding the key serves any session — against
// no server-side revocation, because there is no row to revoke. Revoke,
// ListByUser and DeleteByUser return sessionx.ErrUnsupported rather than
// pretending otherwise; an application that needs "sign out everywhere", or
// needs Renew to invalidate the pre-rotation cookie, wants a different store.
// A stolen cookie stays valid until its ExpiresAt, so keep the TTL short.
//
// The payload is signed, not encrypted: the client can read the session
// contents. Do not put a secret in Session.Data with this store.
package cookiestore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/bakhod1r/sessionx"
)

// MinKeyLen is the shortest signing key New accepts.
const MinKeyLen = 32

// maxToken keeps the cookie under the 4096-byte limit browsers enforce on
// name, value and attributes together.
const maxToken = 3800

var (
	// ErrWeakKey is returned by New for a missing or short key.
	ErrWeakKey = fmt.Errorf("cookiestore: signing key must be at least %d bytes", MinKeyLen)
	// ErrTooLarge is returned when a session does not fit in a cookie.
	ErrTooLarge = errors.New("cookiestore: session too large for a cookie")
)

// Store signs and verifies session tokens. It holds only its keys and is
// safe for concurrent use.
type Store struct {
	keys [][]byte
}

// New returns a store. The first key signs; every key verifies, so a key can
// be rotated by putting the new one first and keeping the old one until the
// tokens it signed have expired. Each key must be at least MinKeyLen bytes
// from a secure random source.
func New(keys ...[]byte) (*Store, error) {
	if len(keys) == 0 {
		return nil, ErrWeakKey
	}
	st := &Store{}
	for _, k := range keys {
		if len(k) < MinKeyLen {
			return nil, ErrWeakKey
		}
		st.keys = append(st.keys, append([]byte(nil), k...))
	}
	return st, nil
}

// Token returns the signed token carrying the session: base64url payload, a
// dot, base64url signature. The Manager writes it as the cookie value.
func (s *Store) Token(sess *sessionx.Session) (string, error) {
	payload, err := json.Marshal(sess)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	tok := body + "." + sign(s.keys[0], body)
	if len(tok) > maxToken {
		return "", ErrTooLarge
	}
	return tok, nil
}

// decode verifies the signature against every key before the payload is
// parsed, so a forged token never reaches the JSON decoder.
func (s *Store) decode(token string) (*sessionx.Session, error) {
	body, sig, ok := strings.Cut(token, ".")
	if !ok || len(token) > maxToken {
		return nil, sessionx.ErrTampered
	}
	valid := false
	for _, k := range s.keys {
		if hmac.Equal([]byte(sig), []byte(sign(k, body))) {
			valid = true
			break
		}
	}
	if !valid {
		return nil, sessionx.ErrTampered
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, sessionx.ErrTampered
	}
	var sess sessionx.Session
	if err := json.Unmarshal(payload, &sess); err != nil {
		return nil, sessionx.ErrTampered
	}
	return &sess, nil
}

func sign(key []byte, body string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Save has nothing to persist: the authoritative copy is the cookie, written
// by Manager.Issue. It refuses a terminal session with ErrUnsupported, because
// recording "this session ended" needs state this store does not keep.
func (s *Store) Save(_ context.Context, sess *sessionx.Session) error {
	if sess.Status.Terminal() {
		return sessionx.ErrUnsupported
	}
	return nil
}

// Load decodes a token. A token that fails verification is reported as
// ErrNotFound (wrapping ErrTampered), so the middleware clears the cookie.
func (s *Store) Load(_ context.Context, token string) (*sessionx.Session, error) {
	sess, err := s.decode(token)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", sessionx.ErrNotFound, err)
	}
	return sess, nil
}

// Delete has nothing to remove server-side. Clearing the client's cookie is
// the caller's job, via Manager.Clear.
func (s *Store) Delete(context.Context, string) error { return nil }

// ListByUser always returns ErrUnsupported: a stateless store holds no set of
// a user's sessions to enumerate.
func (s *Store) ListByUser(context.Context, string) ([]*sessionx.Session, error) {
	return nil, sessionx.ErrUnsupported
}

// DeleteByUser always returns ErrUnsupported, for the same reason.
func (s *Store) DeleteByUser(context.Context, string) (int, error) {
	return 0, sessionx.ErrUnsupported
}

var _ sessionx.Tokenizer = (*Store)(nil)

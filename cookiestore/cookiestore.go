// Package cookiestore is a sessionx Store that keeps no server state: the
// session travels in the client's cookie, signed.
//
// The trade is explicit. Nothing to provision, nothing to scale, and no
// lookup on the hot path — against no server-side revocation, because there
// is no row to revoke. ListByUser and DeleteByUser return
// sessionx.ErrUnsupported rather than pretending otherwise, and an
// application that needs "sign out everywhere" wants a different store.
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
	"strings"
	"sync"

	"github.com/bakhod1r/sessionx"
)

// Store signs and verifies session tokens with HMAC-SHA256.
type Store struct {
	key []byte

	// live holds the token issued for each id within one request cycle, so
	// that Load after Save works for the Manager without a round trip
	// through the client. It is not a session store: entries are dropped on
	// Delete and the map never outlives the process.
	mu   sync.RWMutex
	live map[string]liveToken
}

type liveToken struct {
	token  string
	status sessionx.Status
}

// New returns a store signing with key. The key should be at least 32 bytes
// from a secure source; a short key weakens every token it signs.
func New(key []byte) *Store {
	return &Store{key: key, live: make(map[string]liveToken)}
}

// Encode returns the signed token carrying the session: base64url payload,
// a dot, base64url signature.
func (s *Store) Encode(sess *sessionx.Session) (string, error) {
	payload, err := json.Marshal(sess)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + s.sign(body), nil
}

// Decode verifies the signature and returns the session, or ErrTampered.
// The signature is checked before the payload is parsed, so a forged token
// never reaches the JSON decoder.
func (s *Store) Decode(token string) (*sessionx.Session, error) {
	body, sig, ok := strings.Cut(token, ".")
	if !ok {
		return nil, sessionx.ErrTampered
	}
	if !hmac.Equal([]byte(sig), []byte(s.sign(body))) {
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

func (s *Store) sign(body string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Save encodes the session and remembers the token for this process, so a
// Manager can Load what it just Saved. The authoritative copy is the one the
// client holds.
func (s *Store) Save(_ context.Context, sess *sessionx.Session) error {
	token, err := s.Encode(sess)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.live[sess.ID]; ok && cur.status.Terminal() {
		return cur.status.Err()
	}
	s.live[sess.ID] = liveToken{token: token, status: sess.Status}
	return nil
}

// Load returns the session for an id this process issued, or ErrNotFound.
// A client's token is decoded with Decode, not with Load.
func (s *Store) Load(_ context.Context, id string) (*sessionx.Session, error) {
	s.mu.RLock()
	lt, ok := s.live[id]
	s.mu.RUnlock()

	if !ok {
		return nil, sessionx.ErrNotFound
	}
	return s.Decode(lt.token)
}

// Delete forgets the process-local token. It does not reach the client's
// cookie: clearing that is the caller's job, via Manager.Clear.
func (s *Store) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	delete(s.live, id)
	s.mu.Unlock()
	return nil
}

// ListByUser always returns ErrUnsupported: a stateless store holds no set
// of a user's sessions to enumerate.
func (s *Store) ListByUser(context.Context, string) ([]*sessionx.Session, error) {
	return nil, sessionx.ErrUnsupported
}

// DeleteByUser always returns ErrUnsupported, for the same reason as
// ListByUser.
func (s *Store) DeleteByUser(context.Context, string) (int, error) {
	return 0, sessionx.ErrUnsupported
}

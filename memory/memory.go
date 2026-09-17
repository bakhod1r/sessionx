// Package memory is a sessionx Store that keeps sessions in process memory.
//
// It is the right store for a single-process application, for tests, and for
// development. It is the wrong store behind a load balancer, where a request
// routed to a second process will not find the session the first one made.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/bakhod1r/sessionx"
)

// Store keeps sessions in a map guarded by a read-write mutex.
//
// A sync.Map is not used: ListByUser is a full scan under a read lock, and a
// plain map with RWMutex makes that scan consistent, which sync.Map's
// per-key atomicity does not.
type Store struct {
	mu   sync.RWMutex
	byID map[string]*sessionx.Session
}

// New returns an empty in-memory store, ready for concurrent use.
func New() *Store {
	return &Store{byID: make(map[string]*sessionx.Session)}
}

// Save writes the session, replacing any session with the same ID. The
// session is cloned, so a later write by the caller does not reach into the
// store.
func (s *Store) Save(_ context.Context, sess *sessionx.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[sess.ID] = sess.Clone()
	return nil
}

// Load returns a clone of the stored session, or ErrNotFound.
func (s *Store) Load(_ context.Context, id string) (*sessionx.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.byID[id]
	if !ok {
		return nil, sessionx.ErrNotFound
	}
	return sess.Clone(), nil
}

// Delete removes the session. Removing an absent session is not an error.
func (s *Store) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, id)
	return nil
}

// ListByUser returns clones of every session belonging to userID.
func (s *Store) ListByUser(_ context.Context, userID string) ([]*sessionx.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*sessionx.Session
	for _, sess := range s.byID {
		if sess.UserID == userID {
			out = append(out, sess.Clone())
		}
	}
	return out, nil
}

// DeleteByUser removes every session belonging to userID.
func (s *Store) DeleteByUser(_ context.Context, userID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for id, sess := range s.byID {
		if sess.UserID == userID {
			delete(s.byID, id)
			n++
		}
	}
	return n, nil
}

// GC removes every session whose absolute deadline has passed and returns
// how many went. Nothing calls it automatically: a background goroutine
// started by a library is a goroutine the caller cannot stop, so the
// schedule is the caller's to choose.
func (s *Store) GC(now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for id, sess := range s.byID {
		if sess.Expired(now) {
			delete(s.byID, id)
			n++
		}
	}
	return n
}

// Len reports how many sessions are held.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byID)
}

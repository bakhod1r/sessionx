// Package redisstore is a sessionx Store backed by Redis.
//
// It is the store for an application behind a load balancer: every process
// sees the same session. Sessions are written as JSON under a key with the
// session's own TTL, so expiry is Redis's job rather than a sweeper's, and
// each user's session ids are held in a set so that "sign out everywhere"
// is one round trip plus a pipeline.
package redisstore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/bakhod1r/sessionx"
)

// DefaultPrefix namespaces every key this store writes.
const DefaultPrefix = "sessionx:"

// Options configures the store.
type Options struct {
	// Prefix namespaces the keys. Empty means DefaultPrefix.
	Prefix string
}

// Store persists sessions in Redis.
type Store struct {
	c      redis.UniversalClient
	prefix string
}

// New returns a store over the given client. A UniversalClient is taken so
// a single node, a sentinel setup and a cluster all work unchanged.
func New(client redis.UniversalClient, opts Options) *Store {
	prefix := opts.Prefix
	if prefix == "" {
		prefix = DefaultPrefix
	}
	return &Store{c: client, prefix: prefix}
}

func (s *Store) key(id string) string       { return s.prefix + "s:" + id }
func (s *Store) userKey(user string) string { return s.prefix + "u:" + user }

// Save writes the session with a TTL matching its own deadline and indexes
// it under its user.
func (s *Store) Save(ctx context.Context, sess *sessionx.Session) error {
	payload, err := json.Marshal(sess)
	if err != nil {
		return err
	}

	ttl := time.Until(sess.ExpiresAt)
	if ttl <= 0 {
		// An already-expired session still gets a short life so that a
		// Load immediately after Save can read it and report the state
		// honestly, rather than reporting "not found" for a session that
		// exists and is expired.
		ttl = time.Minute
	}

	pipe := s.c.TxPipeline()
	pipe.Set(ctx, s.key(sess.ID), payload, ttl)
	if sess.UserID != "" {
		pipe.SAdd(ctx, s.userKey(sess.UserID), sess.ID)
		pipe.Expire(ctx, s.userKey(sess.UserID), ttl)
	}
	_, err = pipe.Exec(ctx)
	return err
}

// Load reads one session, returning sessionx.ErrNotFound for a missing key.
func (s *Store) Load(ctx context.Context, id string) (*sessionx.Session, error) {
	payload, err := s.c.Get(ctx, s.key(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, sessionx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var sess sessionx.Session
	if err := json.Unmarshal(payload, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

// Delete removes the session and its index entry.
func (s *Store) Delete(ctx context.Context, id string) error {
	sess, err := s.Load(ctx, id)
	if err != nil && !errors.Is(err, sessionx.ErrNotFound) {
		return err
	}

	pipe := s.c.TxPipeline()
	pipe.Del(ctx, s.key(id))
	if sess != nil && sess.UserID != "" {
		pipe.SRem(ctx, s.userKey(sess.UserID), id)
	}
	_, err = pipe.Exec(ctx)
	return err
}

// ListByUser reads every session in the user's index, skipping ids whose
// session key has already expired and pruning them from the index as it goes.
func (s *Store) ListByUser(ctx context.Context, userID string) ([]*sessionx.Session, error) {
	ids, err := s.c.SMembers(ctx, s.userKey(userID)).Result()
	if err != nil {
		return nil, err
	}

	var out []*sessionx.Session
	var stale []string
	for _, id := range ids {
		sess, err := s.Load(ctx, id)
		if errors.Is(err, sessionx.ErrNotFound) {
			stale = append(stale, id)
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}

	if len(stale) > 0 {
		_ = s.c.SRem(ctx, s.userKey(userID), toAny(stale)...).Err()
	}
	return out, nil
}

// DeleteByUser removes every session the user holds.
func (s *Store) DeleteByUser(ctx context.Context, userID string) (int, error) {
	ids, err := s.c.SMembers(ctx, s.userKey(userID)).Result()
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	keys := make([]string, 0, len(ids)+1)
	for _, id := range ids {
		keys = append(keys, s.key(id))
	}
	keys = append(keys, s.userKey(userID))

	if err := s.c.Del(ctx, keys...).Err(); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func toAny(in []string) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

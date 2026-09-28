// Package redisstore is a sessionx Store backed by Redis.
//
// It is the store for an application behind a load balancer: every process
// sees the same session. Sessions are written as JSON under a key with the
// session's own TTL, so expiry is Redis's job rather than a sweeper's, and
// each user's session ids are held in a set so that "sign out everywhere"
// is one round trip plus a pipeline. No command spans two keys, so a cluster
// works as well as a single node.
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

// saveScript writes a session unless the stored one is terminal, in one
// atomic step on one key. It returns the stored terminal status, or "" when
// it wrote.
var saveScript = redis.NewScript(`
local cur = redis.call('GET', KEYS[1])
if cur then
  local ok, doc = pcall(cjson.decode, cur)
  if ok and (doc.Status == 'revoked' or doc.Status == 'expired') then
    return doc.Status
  end
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return ''
`)

// extendScript adds a member to the user index and pushes the index's
// expiry out to at least ARGV[2] ms, never pulling it in: the index has to
// outlive the longest-lived session it lists.
var extendScript = redis.NewScript(`
redis.call('SADD', KEYS[1], ARGV[1])
local want = tonumber(ARGV[2])
local cur = redis.call('PTTL', KEYS[1])
if cur < want then -- -1 (no expiry yet) included
  redis.call('PEXPIRE', KEYS[1], want)
end
return 1
`)

// Save writes the session with a TTL matching its own deadline and indexes
// it under its user. A stored terminal session is final: Save leaves it and
// returns its status's Err.
//
// Each command touches a single key, so the store works on Redis Cluster,
// where the session key and the user index hash to different slots.
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

	stored, err := saveScript.Run(ctx, s.c, []string{s.key(sess.ID)}, payload, ttl.Milliseconds()).Text()
	if err != nil {
		return err
	}
	if stored != "" {
		return sessionx.Status(stored).Err()
	}
	if sess.UserID != "" {
		return extendScript.Run(ctx, s.c, []string{s.userKey(sess.UserID)}, sess.ID, ttl.Milliseconds()).Err()
	}
	return nil
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

	if err := s.c.Del(ctx, s.key(id)).Err(); err != nil {
		return err
	}
	if sess != nil && sess.UserID != "" {
		return s.c.SRem(ctx, s.userKey(sess.UserID), id).Err()
	}
	return nil
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

	// One DEL per key rather than one multi-key DEL: on a cluster the keys
	// live in different slots and a multi-key DEL fails with CROSSSLOT. A
	// plain (non-transactional) pipeline keeps it to one round trip.
	pipe := s.c.Pipeline()
	for _, id := range ids {
		pipe.Del(ctx, s.key(id))
	}
	pipe.Del(ctx, s.userKey(userID))
	if _, err := pipe.Exec(ctx); err != nil {
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

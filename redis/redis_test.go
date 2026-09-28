package redisstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/bakhod1r/sessionx"
	redisstore "github.com/bakhod1r/sessionx/redis"
	"github.com/bakhod1r/sessionx/storetest"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, "redis", func(t *testing.T) sessionx.Store {
		srv := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
		t.Cleanup(func() { _ = client.Close() })
		return redisstore.New(client, redisstore.Options{})
	}, storetest.Capabilities{Enumerates: true})
}

// TestIndexOutlivesAnExpiredSibling: saving an already-expired session gives
// it a one-minute life. That must not cut the user's index down to a minute,
// or a live sibling drops out of ListByUser and "sign out everywhere" misses
// it.
func TestIndexOutlivesAnExpiredSibling(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	st := redisstore.New(client, redisstore.Options{})
	ctx := t.Context()

	now := time.Now()
	live := &sessionx.Session{ID: "live", UserID: "u1", Status: sessionx.StatusActive, CreatedAt: now, LastSeen: now, ExpiresAt: now.Add(24 * time.Hour)}
	dead := &sessionx.Session{ID: "dead", UserID: "u1", Status: sessionx.StatusExpired, CreatedAt: now, LastSeen: now, ExpiresAt: now.Add(-time.Hour)}
	if err := st.Save(ctx, live); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(ctx, dead); err != nil {
		t.Fatal(err)
	}

	srv.FastForward(2 * time.Minute)

	got, err := st.ListByUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "live" {
		t.Fatalf("ListByUser = %v, want the live session", ids(got))
	}
}

// TestSaveUsesNoCrossSlotCommand: every command touches one key, so the store
// works on Redis Cluster, where the session key and the user index hash to
// different slots.
func TestSaveUsesNoCrossSlotCommand(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	var multi []string
	client.AddHook(cmdHook{onPipe: func(cmds []redis.Cmder) {
		for _, c := range cmds {
			if c.Name() == "multi" {
				multi = append(multi, c.Name())
			}
		}
	}})
	st := redisstore.New(client, redisstore.Options{})
	ctx := t.Context()
	now := time.Now()
	s := &sessionx.Session{ID: "a", UserID: "u1", Status: sessionx.StatusActive, CreatedAt: now, LastSeen: now, ExpiresAt: now.Add(time.Hour)}
	if err := st.Save(ctx, s); err != nil {
		t.Fatal(err)
	}
	if err := st.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	_ = st.Save(ctx, &sessionx.Session{ID: "b", UserID: "u1", Status: sessionx.StatusActive, ExpiresAt: now.Add(time.Hour)})
	if _, err := st.DeleteByUser(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if len(multi) > 0 {
		t.Fatalf("store used MULTI across keys %d times; that fails with CROSSSLOT on a cluster", len(multi))
	}
}

type cmdHook struct{ onPipe func([]redis.Cmder) }

func (cmdHook) DialHook(next redis.DialHook) redis.DialHook          { return next }
func (cmdHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook { return next }
func (h cmdHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		h.onPipe(cmds)
		return next(ctx, cmds)
	}
}

func ids(ss []*sessionx.Session) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.ID
	}
	return out
}

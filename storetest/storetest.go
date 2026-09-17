// Package storetest holds the conformance suite every sessionx Store must
// pass. It exists so that "pluggable storage" is a tested claim rather than
// an aspiration: a backend is correct when it passes Run, and not before.
package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bakhod1r/sessionx"
)

// Capabilities describes what a backend can honestly do, so the suite can
// hold a stateless store to a different — but still exact — standard.
type Capabilities struct {
	// Enumerates is true when the backend can list and bulk-delete by user.
	// A stateless cookie store cannot, and must return ErrUnsupported.
	Enumerates bool
}

// Run executes the full conformance suite against the store the factory
// builds. The factory is called once per subtest so no test sees another's
// state.
func Run(t *testing.T, name string, factory func(t *testing.T) sessionx.Store, caps Capabilities) {
	t.Helper()

	t.Run(name+"/SaveThenLoad", func(t *testing.T) {
		ctx := context.Background()
		st := factory(t)
		s := sample("s1", "u1")

		if err := st.Save(ctx, s); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, err := st.Load(ctx, "s1")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.UserID != "u1" || got.Status != sessionx.StatusActive {
			t.Fatalf("round trip lost fields: %+v", got)
		}
		if got.Data["role"] != "admin" {
			t.Fatalf("Data did not survive the round trip: %+v", got.Data)
		}
		if got.Device.Brand != "Samsung" {
			t.Fatalf("Device did not survive the round trip: %+v", got.Device)
		}
		if got.Client.Kind != "Human" || got.Client.SpoofScore != 0.25 {
			t.Fatalf("Client did not survive the round trip: %+v", got.Client)
		}
		if got.Locale != "uz" || got.LocaleSource != "header" {
			t.Fatalf("Locale did not survive the round trip: %q/%q", got.Locale, got.LocaleSource)
		}
	})

	t.Run(name+"/LoadMissing", func(t *testing.T) {
		ctx := context.Background()
		st := factory(t)

		_, err := st.Load(ctx, "nope")
		if !errors.Is(err, sessionx.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run(name+"/Delete", func(t *testing.T) {
		ctx := context.Background()
		st := factory(t)
		_ = st.Save(ctx, sample("s1", "u1"))

		if err := st.Delete(ctx, "s1"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := st.Load(ctx, "s1"); !errors.Is(err, sessionx.ErrNotFound) {
			t.Fatalf("session survived Delete: %v", err)
		}
		if err := st.Delete(ctx, "s1"); err != nil {
			t.Fatalf("deleting an absent session must not error: %v", err)
		}
	})

	t.Run(name+"/NoAliasing", func(t *testing.T) {
		ctx := context.Background()
		st := factory(t)
		s := sample("s1", "u1")
		_ = st.Save(ctx, s)

		s.Data["role"] = "mutated-after-save"

		got, err := st.Load(ctx, "s1")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.Data["role"] != "admin" {
			t.Fatal("the store shares memory with the caller; it must clone")
		}
	})

	t.Run(name+"/ListByUser", func(t *testing.T) {
		ctx := context.Background()
		st := factory(t)
		_ = st.Save(ctx, sample("s1", "u1"))
		_ = st.Save(ctx, sample("s2", "u1"))
		_ = st.Save(ctx, sample("s3", "u2"))

		got, err := st.ListByUser(ctx, "u1")
		if !caps.Enumerates {
			if !errors.Is(err, sessionx.ErrUnsupported) {
				t.Fatalf("a non-enumerating store must return ErrUnsupported, got %v", err)
			}
			return
		}
		if err != nil {
			t.Fatalf("ListByUser: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d sessions for u1, want 2", len(got))
		}
	})

	t.Run(name+"/DeleteByUser", func(t *testing.T) {
		ctx := context.Background()
		st := factory(t)
		_ = st.Save(ctx, sample("s1", "u1"))
		_ = st.Save(ctx, sample("s2", "u1"))
		_ = st.Save(ctx, sample("s3", "u2"))

		n, err := st.DeleteByUser(ctx, "u1")
		if !caps.Enumerates {
			if !errors.Is(err, sessionx.ErrUnsupported) {
				t.Fatalf("a non-enumerating store must return ErrUnsupported, got %v", err)
			}
			return
		}
		if err != nil {
			t.Fatalf("DeleteByUser: %v", err)
		}
		if n != 2 {
			t.Fatalf("deleted %d, want 2", n)
		}
		if _, err := st.Load(ctx, "s3"); err != nil {
			t.Fatalf("another user's session was deleted: %v", err)
		}
	})

	t.Run(name+"/ConcurrentAccess", func(t *testing.T) {
		ctx := context.Background()
		st := factory(t)
		done := make(chan struct{})

		for i := 0; i < 8; i++ {
			go func(i int) {
				defer func() { done <- struct{}{} }()
				id := "concurrent"
				for j := 0; j < 50; j++ {
					_ = st.Save(ctx, sample(id, "u1"))
					_, _ = st.Load(ctx, id)
				}
			}(i)
		}
		for i := 0; i < 8; i++ {
			<-done
		}
	})
}

func sample(id, userID string) *sessionx.Session {
	now := time.Now().UTC().Truncate(time.Second)
	return &sessionx.Session{
		ID:           id,
		UserID:       userID,
		Status:       sessionx.StatusActive,
		Device:       sessionx.Device{Raw: "ua", Brand: "Samsung", Name: "Galaxy S10", Type: "Mobile", OS: "Android"},
		Client:       sessionx.Client{Kind: "Human", SpoofScore: 0.25},
		Network:      sessionx.Network{IP: "203.0.113.7"},
		Locale:       "uz",
		LocaleSource: "header",
		Data:         map[string]any{"role": "admin"},
		CreatedAt:    now,
		LastSeen:     now,
		ExpiresAt:    now.Add(time.Hour),
	}
}

package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/bakhod1r/sessionx"
	"github.com/bakhod1r/sessionx/memory"
	"github.com/bakhod1r/sessionx/storetest"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, "memory", func(t *testing.T) sessionx.Store {
		return memory.New()
	}, storetest.Capabilities{Enumerates: true})
}

func TestGCRemovesOnlyExpired(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Now()

	_ = st.Save(ctx, &sessionx.Session{ID: "live", UserID: "u", Status: sessionx.StatusActive, ExpiresAt: now.Add(time.Hour)})
	_ = st.Save(ctx, &sessionx.Session{ID: "dead", UserID: "u", Status: sessionx.StatusActive, ExpiresAt: now.Add(-time.Minute)})

	if n := st.GC(now); n != 1 {
		t.Fatalf("GC removed %d, want 1", n)
	}
	if st.Len() != 1 {
		t.Fatalf("Len = %d, want 1", st.Len())
	}
	if _, err := st.Load(ctx, "live"); err != nil {
		t.Fatalf("GC removed a live session: %v", err)
	}
}

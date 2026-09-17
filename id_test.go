package sessionx

import (
	"encoding/base64"
	"testing"
)

func TestNewIDShapeAndUniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id, err := NewID()
		if err != nil {
			t.Fatalf("NewID: %v", err)
		}
		raw, err := base64.RawURLEncoding.DecodeString(id)
		if err != nil {
			t.Fatalf("id is not base64url: %v", err)
		}
		if len(raw) != 32 {
			t.Fatalf("want 32 bytes of entropy, got %d", len(raw))
		}
		if _, dup := seen[id]; dup {
			t.Fatal("NewID produced a duplicate")
		}
		seen[id] = struct{}{}
	}
}

func TestEqualID(t *testing.T) {
	if !EqualID("abc", "abc") {
		t.Fatal("identical ids must compare equal")
	}
	if EqualID("abc", "abd") || EqualID("abc", "abcd") {
		t.Fatal("different ids must not compare equal")
	}
}

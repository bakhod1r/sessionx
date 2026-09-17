package cookiestore_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bakhod1r/sessionx"
	"github.com/bakhod1r/sessionx/cookiestore"
	"github.com/bakhod1r/sessionx/storetest"
)

var key = []byte("test-key-at-least-32-bytes-long!!")

func sample() *sessionx.Session {
	now := time.Now().UTC().Truncate(time.Second)
	return &sessionx.Session{
		ID:        "s1",
		UserID:    "u1",
		Status:    sessionx.StatusActive,
		Device:    sessionx.Device{Raw: "ua", Brand: "Samsung", Name: "Galaxy S10"},
		Network:   sessionx.Network{IP: "203.0.113.7"},
		Data:      map[string]any{"role": "admin"},
		CreatedAt: now,
		LastSeen:  now,
		ExpiresAt: now.Add(time.Hour),
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	st := cookiestore.New(key)

	token, err := st.Encode(sample())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	got, err := st.Decode(token)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.UserID != "u1" || got.Device.Brand != "Samsung" || got.Data["role"] != "admin" {
		t.Fatalf("round trip lost fields: %+v", got)
	}
}

func TestDecodeRejectsATamperedToken(t *testing.T) {
	st := cookiestore.New(key)
	token, _ := st.Encode(sample())

	parts := strings.SplitN(token, ".", 2)
	tampered := parts[0][:len(parts[0])-1] + "A" + "." + parts[1]

	if _, err := st.Decode(tampered); !errors.Is(err, sessionx.ErrTampered) {
		t.Fatalf("err = %v, want ErrTampered", err)
	}
}

func TestDecodeRejectsAnotherKeysToken(t *testing.T) {
	token, _ := cookiestore.New(key).Encode(sample())

	other := cookiestore.New([]byte("a-completely-different-key-here!!"))
	if _, err := other.Decode(token); !errors.Is(err, sessionx.ErrTampered) {
		t.Fatalf("err = %v, want ErrTampered", err)
	}
}

func TestEnumerationIsRefusedHonestly(t *testing.T) {
	st := cookiestore.New(key)

	if _, err := st.ListByUser(t.Context(), "u1"); !errors.Is(err, sessionx.ErrUnsupported) {
		t.Fatalf("ListByUser err = %v, want ErrUnsupported", err)
	}
	if _, err := st.DeleteByUser(t.Context(), "u1"); !errors.Is(err, sessionx.ErrUnsupported) {
		t.Fatalf("DeleteByUser err = %v, want ErrUnsupported", err)
	}
}

func TestConformance(t *testing.T) {
	storetest.Run(t, "cookiestore", func(t *testing.T) sessionx.Store {
		return cookiestore.New(key)
	}, storetest.Capabilities{Enumerates: false})
}

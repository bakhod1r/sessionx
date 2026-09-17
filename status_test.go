package sessionx

import "testing"

func TestStatusValid(t *testing.T) {
	for _, s := range []Status{StatusActive, StatusIdle, StatusExpired, StatusRevoked} {
		if !s.Valid() {
			t.Fatalf("%q should be valid", s)
		}
	}
	if Status("banana").Valid() {
		t.Fatal("unregistered status must not validate")
	}
}

func TestStatusTransitions(t *testing.T) {
	legal := []struct{ from, to Status }{
		{StatusActive, StatusIdle},
		{StatusActive, StatusExpired},
		{StatusActive, StatusRevoked},
		{StatusIdle, StatusActive},
		{StatusIdle, StatusExpired},
		{StatusIdle, StatusRevoked},
	}
	for _, c := range legal {
		if !c.from.CanMoveTo(c.to) {
			t.Fatalf("%q -> %q must be legal", c.from, c.to)
		}
	}
	illegal := []struct{ from, to Status }{
		{StatusExpired, StatusActive},
		{StatusRevoked, StatusActive},
		{StatusRevoked, StatusIdle},
		{StatusExpired, StatusRevoked},
	}
	for _, c := range illegal {
		if c.from.CanMoveTo(c.to) {
			t.Fatalf("%q -> %q must be refused", c.from, c.to)
		}
	}
}

func TestTerminalStatesAreTerminal(t *testing.T) {
	if !StatusExpired.Terminal() || !StatusRevoked.Terminal() {
		t.Fatal("expired and revoked are terminal")
	}
	if StatusActive.Terminal() || StatusIdle.Terminal() {
		t.Fatal("active and idle are not terminal")
	}
}

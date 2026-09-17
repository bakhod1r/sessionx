package sessionx

import (
	"net/netip"
	"testing"
)

func mustPrefixes(t *testing.T, cidrs ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		p, err := netip.ParsePrefix(c)
		if err != nil {
			t.Fatalf("bad test cidr %q: %v", c, err)
		}
		out = append(out, p)
	}
	return out
}

func TestNetworkIgnoresForwardedWithoutTrustedProxies(t *testing.T) {
	n := NetworkFrom("203.0.113.7:51234", "1.2.3.4", nil)

	if n.IP != "203.0.113.7" {
		t.Fatalf("ip = %q, want the socket peer 203.0.113.7", n.IP)
	}
	if n.Forwarded != "1.2.3.4" {
		t.Fatalf("the header must still be recorded for audit, got %q", n.Forwarded)
	}
}

func TestNetworkTrustsForwardedFromATrustedProxy(t *testing.T) {
	n := NetworkFrom("10.0.0.5:4444", "198.51.100.9, 10.0.0.5", mustPrefixes(t, "10.0.0.0/8"))

	if n.IP != "198.51.100.9" {
		t.Fatalf("ip = %q, want the leftmost client address", n.IP)
	}
}

func TestNetworkRejectsGarbageForwarded(t *testing.T) {
	n := NetworkFrom("10.0.0.5:4444", "not-an-ip", mustPrefixes(t, "10.0.0.0/8"))

	if n.IP != "10.0.0.5" {
		t.Fatalf("an unparsable header must fall back to the peer, got %q", n.IP)
	}
}

func TestNetworkBareAddressWithoutPort(t *testing.T) {
	n := NetworkFrom("203.0.113.7", "", nil)

	if n.IP != "203.0.113.7" {
		t.Fatalf("ip = %q, want 203.0.113.7", n.IP)
	}
}

package sessionx

import (
	"net"
	"net/netip"
	"strings"
)

// Network is where a session was opened from.
type Network struct {
	// IP is the client address this package is willing to attribute the
	// session to.
	IP string

	// Forwarded is the X-Forwarded-For chain exactly as it arrived, kept
	// whether or not it was trusted, because an audit log wants to see a
	// spoof attempt as much as a legitimate hop.
	Forwarded string
}

// NetworkFrom resolves the client address.
//
// The socket peer is the default answer, and the only answer when trusted is
// empty: X-Forwarded-For is attacker-controlled on a direct connection, so
// honouring it unconditionally would let any client claim any address. The
// header is consulted only when the peer itself falls inside one of the
// trusted prefixes, in which case the leftmost parsable address in the chain
// is taken as the client.
func NetworkFrom(remoteAddr, forwarded string, trusted []netip.Prefix) Network {
	n := Network{IP: hostOf(remoteAddr), Forwarded: strings.TrimSpace(forwarded)}

	if n.Forwarded == "" || len(trusted) == 0 {
		return n
	}

	peer, err := netip.ParseAddr(n.IP)
	if err != nil || !anyContains(trusted, peer) {
		return n
	}

	for _, part := range strings.Split(n.Forwarded, ",") {
		if addr, err := netip.ParseAddr(strings.TrimSpace(part)); err == nil {
			n.IP = addr.String()
			break
		}
	}
	return n
}

// hostOf strips the port from an address, tolerating an address that carries
// none.
func hostOf(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

func anyContains(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

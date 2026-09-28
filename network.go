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
// trusted prefixes. It is then walked right to left — proxies append, so the
// left end is whatever the client sent — and the first hop that is not a
// trusted proxy is the client. An unparsable hop stops the walk: nothing to
// its left can be vouched for.
func NetworkFrom(remoteAddr, forwarded string, trusted []netip.Prefix) Network {
	n := Network{IP: hostOf(remoteAddr), Forwarded: strings.TrimSpace(forwarded)}
	if a, err := netip.ParseAddr(n.IP); err == nil {
		n.IP = a.Unmap().String()
	}

	if n.Forwarded == "" || len(trusted) == 0 {
		return n
	}

	peer, err := netip.ParseAddr(n.IP)
	if err != nil || !anyContains(trusted, peer) {
		return n
	}

	hops := strings.Split(n.Forwarded, ",")
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		addr = addr.Unmap()
		n.IP = addr.String()
		if !anyContains(trusted, addr) {
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

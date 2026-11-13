package httpserver

import (
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

// ClientIP finds the client's address for rate limits and anti-spam checks (the captcha
// verification wants it). It is the only place that reads X-Forwarded-For: the header is
// trusted only when the direct peer is one of the proxies in front of us (nginx), and then the
// client is the rightmost address that isn't a trusted proxy. Anything else could be forged.
type ClientIP struct{ trusted []netip.Prefix }

// NewClientIP trusts X-Forwarded-For from peers in trusted, e.g. 10.0.0.0/8 for the proxies.
func NewClientIP(trusted []netip.Prefix) ClientIP { return ClientIP{trusted: trusted} }

// Of returns the client address of r, or the zero Addr when RemoteAddr isn't an address.
func (c ClientIP) Of(r *http.Request) netip.Addr {
	peer := remoteAddr(r)
	if !c.isTrusted(peer) {
		return peer
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for _, hop := range slices.Backward(hops) {
		a, err := netip.ParseAddr(strings.TrimSpace(hop))
		if err != nil {
			return peer // a malformed hop: stop at the last address we can vouch for
		}
		if a = a.Unmap(); !c.isTrusted(a) {
			return a
		}
		peer = a
	}
	return peer
}

func (c ClientIP) isTrusted(a netip.Addr) bool {
	for _, p := range c.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func remoteAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return a.Unmap()
}

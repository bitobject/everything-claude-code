package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"example.com/app/internal/platform/httpserver"
)

func TestClientIPTrustsForwardedForOnlyFromProxies(t *testing.T) {
	ip := httpserver.NewClientIP([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	tests := []struct {
		name   string
		remote string
		xff    []string // one entry per header line
		want   string
	}{
		{"a direct client's header is ignored", "203.0.113.7:4321", []string{"198.51.100.1"}, "203.0.113.7"},
		{"through the proxy", "10.0.0.2:4321", []string{"203.0.113.7"}, "203.0.113.7"},
		{"a hop the client wrote itself is skipped", "10.0.0.2:4321", []string{"198.51.100.1, 203.0.113.7"}, "203.0.113.7"},
		{"a chain of trusted proxies", "10.0.0.2:4321", []string{"203.0.113.7, 10.0.0.1"}, "203.0.113.7"},
		{"several header lines are one list", "10.0.0.2:4321", []string{"198.51.100.1", "203.0.113.7"}, "203.0.113.7"},
		{"the proxy itself, without the header", "10.0.0.2:4321", nil, "10.0.0.2"},
		{"a malformed hop stops at the last trusted address", "10.0.0.2:4321", []string{"203.0.113.7, not-an-ip"}, "10.0.0.2"},
		{"an IPv4-mapped IPv6 peer", "[::ffff:10.0.0.2]:4321", []string{"203.0.113.7"}, "203.0.113.7"},
		{"an IPv6 client", "10.0.0.2:4321", []string{"2001:db8::1"}, "2001:db8::1"},
		{"RemoteAddr that is not an address", "@", nil, "invalid IP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remote
			for _, v := range tt.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := ip.Of(r).String(); got != tt.want {
				t.Errorf("Of() = %s, want %s", got, tt.want)
			}
		})
	}
}

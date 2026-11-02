package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/econumo/econumo/internal/shared/reqctx"
)

// ClientIP resolves the caller's IP into reqctx for the rate limiter; it is held
// in memory only and never logged. A header named by the operator wins, since
// only they know their proxy sets it (a client could otherwise forge it, which
// is why it is opt-in). Without one, X-Forwarded-For is read only when the peer
// is on a private network, where a reverse proxy would be; a request directly
// from the internet cannot forge its way past its own address.
func ClientIP(header string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := reqctx.WithClientIP(r.Context(), resolveClientIP(r, header))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func resolveClientIP(r *http.Request, header string) string {
	peer := parseIP(r.RemoteAddr)
	if header != "" {
		// A list keeps the entry the nearest proxy appended: earlier ones are
		// whatever the client sent.
		if list := forwardedEntries(r, header); len(list) > 0 {
			if ip := parseIP(list[len(list)-1]); ip.IsValid() {
				return bucket(ip)
			}
		}
		return bucket(peer)
	}
	if !peer.IsValid() || !isInternal(peer) {
		return bucket(peer)
	}
	// Walk back through the proxies to the first address outside them. All
	// internal means a client on the same network, the leftmost one.
	found := peer
	list := forwardedEntries(r, "X-Forwarded-For")
	for i := len(list) - 1; i >= 0; i-- {
		ip := parseIP(list[i])
		if !ip.IsValid() {
			break
		}
		found = ip
		if !isInternal(ip) {
			break
		}
	}
	return bucket(found)
}

func forwardedEntries(r *http.Request, header string) []string {
	var out []string
	for _, line := range r.Header.Values(header) {
		for _, entry := range strings.Split(line, ",") {
			out = append(out, strings.TrimSpace(entry))
		}
	}
	return out
}

func parseIP(value string) netip.Addr {
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	ip, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}
	}
	return ip.Unmap().WithZone("")
}

func isInternal(ip netip.Addr) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}

// bucket groups an IPv6 address by its /64: one home or VPS is handed the whole
// network, so counting single addresses would let it walk past any limit.
func bucket(ip netip.Addr) string {
	if !ip.IsValid() {
		return ""
	}
	if ip.Is6() {
		if p, err := ip.Prefix(64); err == nil {
			return p.Addr().String() + "/64"
		}
	}
	return ip.String()
}

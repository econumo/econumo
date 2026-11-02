package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"

	"github.com/econumo/econumo/internal/shared/reqctx"
)

// ClientIP resolves the caller's IP into reqctx for the rate limiter; it is held
// in memory only and never logged. With a proxies list the named header counts
// only on a request from one of them: from anyone else it could be forged to
// dodge the limits or spend another caller's. With no list it counts from anyone,
// for a server a firewall keeps reachable only through its proxy.
func ClientIP(header string, trusted []netip.Prefix) func(http.Handler) http.Handler {
	// Unconfigured behind a proxy, all its users share one rate-limit bucket and
	// nothing else would point at the cause. A forged header costs one log line.
	var warnOnce sync.Once
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if header == "" && looksProxied(r) {
				warnOnce.Do(func() {
					slog.Warn("request carries a forwarding header but ECONUMO_CLIENT_IP_HEADER is not set; " +
						"behind a reverse proxy all its users count as one IP for rate limits")
				})
			}
			ctx := reqctx.WithClientIP(r.Context(), resolveClientIP(r, header, trusted))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// resolveClientIP walks a forwarded list from the nearest hop back to the first
// address that is not a trusted proxy. Anything further left is whatever that
// caller sent. All trusted means the caller is one of those networks itself.
func resolveClientIP(r *http.Request, header string, trusted []netip.Prefix) string {
	peer := parseIP(r.RemoteAddr)
	if header == "" || !peer.IsValid() || (len(trusted) > 0 && !isTrusted(peer, trusted)) {
		return bucket(peer)
	}
	found := peer
	list := forwardedEntries(r, header)
	for i := len(list) - 1; i >= 0; i-- {
		ip := parseIP(list[i])
		if !ip.IsValid() {
			break
		}
		found = ip
		if !isTrusted(ip, trusted) {
			break
		}
	}
	return bucket(found)
}

func looksProxied(r *http.Request) bool {
	for _, h := range []string{"X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP", "Forwarded"} {
		if r.Header.Get(h) != "" {
			return true
		}
	}
	return false
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

func isTrusted(ip netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(ip) {
			return true
		}
	}
	return false
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

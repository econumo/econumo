package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestClientIP_WarnsOnceWhenProxiedWithoutAHeader(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	const msg = "ECONUMO_CLIENT_IP_HEADER is not set"

	serve := func(header string, forwarded bool, times int) {
		h := ClientIP(header, nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		for range times {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if forwarded {
				r.Header.Set("X-Forwarded-For", "198.51.100.1")
			}
			h.ServeHTTP(httptest.NewRecorder(), r)
		}
	}

	serve("", false, 2)
	if strings.Contains(logs.String(), msg) {
		t.Fatalf("direct traffic warned: %s", logs.String())
	}
	serve("CF-Connecting-IP", true, 2)
	if strings.Contains(logs.String(), msg) {
		t.Fatalf("configured header warned: %s", logs.String())
	}
	serve("", true, 3)
	if got := strings.Count(logs.String(), msg); got != 1 {
		t.Fatalf("warned %d times, want once:\n%s", got, logs.String())
	}
}

func TestResolveClientIP(t *testing.T) {
	const cf, xff = "Cf-Connecting-Ip", "X-Forwarded-For"
	proxies := []string{"173.245.48.0/20", "172.18.0.0/16"}
	cases := []struct {
		name, peer, header string
		headers            map[string][]string
		noProxies          bool
		want               string
	}{
		{name: "direct IPv4", peer: "203.0.113.9:4000", want: "203.0.113.9"},
		{name: "IPv4-mapped peer", peer: "[::ffff:203.0.113.9]:4000", want: "203.0.113.9"},
		{name: "IPv6 is grouped by /64", peer: "[2001:db8:1:2:3:4:5:6]:4000", want: "2001:db8:1:2::/64"},
		{name: "unparsable peer resolves to nothing", peer: "@", want: ""},

		{name: "Cloudflare traffic uses the header", peer: "173.245.48.1:4000", header: "CF-Connecting-IP",
			headers: map[string][]string{cf: {"198.51.100.1"}}, want: "198.51.100.1"},
		{name: "Cloudflare traffic over IPv6", peer: "173.245.48.1:4000", header: "CF-Connecting-IP",
			headers: map[string][]string{cf: {"2001:db8:1:2:3:4:5:6"}}, want: "2001:db8:1:2::/64"},
		{name: "Cloudflare Tunnel from a trusted private peer", peer: "172.18.0.5:4000", header: "CF-Connecting-IP",
			headers: map[string][]string{cf: {"198.51.100.1"}, xff: {"1.1.1.1"}}, want: "198.51.100.1"},
		{name: "direct traffic without the header uses the peer", peer: "203.0.113.9:4000", header: "CF-Connecting-IP", want: "203.0.113.9"},
		{name: "direct traffic forging the header uses the peer", peer: "203.0.113.9:4000", header: "CF-Connecting-IP",
			headers: map[string][]string{cf: {"198.51.100.1"}}, want: "203.0.113.9"},
		{name: "private peer that is not trusted cannot forge", peer: "192.168.1.5:4000", header: xff,
			headers: map[string][]string{xff: {"198.51.100.1"}}, want: "192.168.1.5"},
		{name: "no header configured ignores forwarding", peer: "172.18.0.5:4000", header: "", noProxies: true,
			headers: map[string][]string{xff: {"198.51.100.1"}}, want: "172.18.0.5"},
		{name: "header alone counts from any peer", peer: "173.245.48.1:4000", header: "CF-Connecting-IP", noProxies: true,
			headers: map[string][]string{cf: {"198.51.100.1"}}, want: "198.51.100.1"},
		{name: "header alone, absent, uses the peer", peer: "203.0.113.9:4000", header: "CF-Connecting-IP", noProxies: true, want: "203.0.113.9"},
		{name: "header alone keeps a list's nearest entry", peer: "172.18.0.5:4000", header: xff, noProxies: true,
			headers: map[string][]string{xff: {"1.1.1.1, 198.51.100.1"}}, want: "198.51.100.1"},
		{name: "header alone with a bad value uses the peer", peer: "203.0.113.9:4000", header: "CF-Connecting-IP", noProxies: true,
			headers: map[string][]string{cf: {"nonsense"}}, want: "203.0.113.9"},

		{name: "a caller's own entry cannot replace the one its proxy appended", peer: "172.18.0.2:4000", header: xff,
			headers: map[string][]string{xff: {"1.1.1.1, 198.51.100.1"}}, want: "198.51.100.1"},
		{name: "a private caller behind the proxy cannot forge either", peer: "172.18.0.2:4000", header: xff,
			headers: map[string][]string{xff: {"1.1.1.1, 192.168.1.5"}}, want: "192.168.1.5"},
		{name: "CDN then own proxy walks to the visitor", peer: "172.18.0.2:4000", header: xff,
			headers: map[string][]string{xff: {"198.51.100.1, 173.245.48.1", "172.18.0.9"}}, want: "198.51.100.1"},
		{name: "every entry trusted is the leftmost", peer: "172.18.0.2:4000", header: xff,
			headers: map[string][]string{xff: {"172.18.0.7"}}, want: "172.18.0.7"},
		{name: "garbage stops the walk", peer: "172.18.0.2:4000", header: xff,
			headers: map[string][]string{xff: {"nonsense"}}, want: "172.18.0.2"},
		{name: "trusted proxy sending no header is the peer", peer: "172.18.0.2:4000", header: xff, want: "172.18.0.2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: tc.peer, Header: http.Header(tc.headers)}
			if r.Header == nil {
				r.Header = http.Header{}
			}
			var trusted []netip.Prefix
			if !tc.noProxies {
				for _, p := range proxies {
					trusted = append(trusted, netip.MustParsePrefix(p))
				}
			}
			if got := resolveClientIP(r, tc.header, trusted); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

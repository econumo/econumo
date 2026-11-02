package middleware

import (
	"net/http"
	"testing"
)

func TestResolveClientIP(t *testing.T) {
	cases := []struct {
		name, peer, header string
		headers            map[string][]string
		want               string
	}{
		{name: "direct IPv4", peer: "203.0.113.9:4000", want: "203.0.113.9"},
		{name: "IPv4-mapped peer", peer: "[::ffff:203.0.113.9]:4000", want: "203.0.113.9"},
		{name: "IPv6 is grouped by /64", peer: "[2001:db8:1:2:3:4:5:6]:4000", want: "2001:db8:1:2::/64"},
		{name: "unparsable peer resolves to nothing", peer: "@", want: ""},
		{name: "public peer's forwarded header is forged",
			peer: "203.0.113.9:4000", headers: map[string][]string{"X-Forwarded-For": {"198.51.100.1"}}, want: "203.0.113.9"},
		{name: "proxy on a private network",
			peer: "172.18.0.2:4000", headers: map[string][]string{"X-Forwarded-For": {"198.51.100.1"}}, want: "198.51.100.1"},
		{name: "a client's own entry cannot replace the one its proxy appended",
			peer: "127.0.0.1:4000", headers: map[string][]string{"X-Forwarded-For": {"1.1.1.1, 198.51.100.1"}}, want: "198.51.100.1"},
		{name: "chain of internal proxies is skipped",
			peer: "10.0.0.2:4000", headers: map[string][]string{"X-Forwarded-For": {"198.51.100.1, 10.0.0.9", "10.0.0.8"}}, want: "198.51.100.1"},
		{name: "client on the proxy's own network",
			peer: "10.0.0.2:4000", headers: map[string][]string{"X-Forwarded-For": {"192.168.1.5"}}, want: "192.168.1.5"},
		{name: "garbage entry stops the walk",
			peer: "10.0.0.2:4000", headers: map[string][]string{"X-Forwarded-For": {"not-an-ip"}}, want: "10.0.0.2"},
		{name: "private peer with no header is the proxy itself", peer: "10.0.0.2:4000", want: "10.0.0.2"},
		{name: "configured header wins, public peer included",
			peer: "173.245.48.1:4000", header: "CF-Connecting-IP", headers: map[string][]string{"Cf-Connecting-Ip": {"198.51.100.1"}}, want: "198.51.100.1"},
		{name: "configured header ignores X-Forwarded-For",
			peer: "10.0.0.2:4000", header: "CF-Connecting-IP", headers: map[string][]string{"X-Forwarded-For": {"1.1.1.1"}}, want: "10.0.0.2"},
		{name: "configured list header keeps the nearest entry",
			peer: "10.0.0.2:4000", header: "X-Forwarded-For", headers: map[string][]string{"X-Forwarded-For": {"1.1.1.1, 198.51.100.1"}}, want: "198.51.100.1"},
		{name: "configured header with a bad value falls back to the peer",
			peer: "10.0.0.2:4000", header: "X-Real-IP", headers: map[string][]string{"X-Real-Ip": {"nonsense"}}, want: "10.0.0.2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: tc.peer, Header: http.Header(tc.headers)}
			if r.Header == nil {
				r.Header = http.Header{}
			}
			if got := resolveClientIP(r, tc.header); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

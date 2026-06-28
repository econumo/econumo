package pgsql

import (
	"net/url"
	"testing"
)

func TestSanitizeDSN(t *testing.T) {
	cases := []struct {
		name string
		in   string
		// wantParams is the expected remaining query (order-independent); nil means
		// "expect the DSN returned unchanged".
		wantParams map[string]string
		unchanged  bool
	}{
		{
			name:       "strips serverVersion and charset, keeps sslmode",
			in:         "postgresql://u:p@host:6432/db?serverVersion=17&charset=utf8&sslmode=disable",
			wantParams: map[string]string{"sslmode": "disable"},
		},
		{
			name:       "case-insensitive keys",
			in:         "postgres://u:p@host/db?ServerVersion=16&CharSet=utf8",
			wantParams: map[string]string{},
		},
		{
			name:      "no doctrine params -> unchanged",
			in:        "postgres://u:p@host/db?sslmode=require&application_name=econumo",
			unchanged: true,
		},
		{
			name:      "no query -> unchanged",
			in:        "postgres://u:p@host:5432/db",
			unchanged: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeDSN(tc.in)
			if tc.unchanged {
				if got != tc.in {
					t.Errorf("expected unchanged, got %q", got)
				}
				return
			}
			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("result not a URL: %v", err)
			}
			q := u.Query()
			if len(q) != len(tc.wantParams) {
				t.Errorf("param count = %d, want %d (%q)", len(q), len(tc.wantParams), got)
			}
			for k, v := range tc.wantParams {
				if q.Get(k) != v {
					t.Errorf("param %s = %q, want %q", k, q.Get(k), v)
				}
			}
			// The doctrine-only params must be gone.
			for _, bad := range doctrineOnlyParams {
				if q.Has(bad) {
					t.Errorf("%s should have been stripped", bad)
				}
			}
			// User/host/path preserved.
			if u.User == nil || u.Host == "" {
				t.Errorf("userinfo/host not preserved: %q", got)
			}
		})
	}
}

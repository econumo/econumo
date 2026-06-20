package config

import "testing"

func TestDriverFromURL(t *testing.T) {
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"sqlite:///var/db/db.sqlite", "sqlite", false},
		{"sqlite://relative.sqlite", "sqlite", false},
		{"postgresql://u:p@localhost:5432/econumo?sslmode=disable", "postgresql", false},
		{"postgres://u:p@localhost/econumo", "postgresql", false},
		{"SQLITE:///x.sqlite", "sqlite", false}, // scheme is case-insensitive
		{"mysql://localhost/db", "", true},      // unsupported engine
		{"/var/db/db.sqlite", "", true},         // no scheme
		{"", "", true},
	}
	for _, tc := range cases {
		got, err := driverFromURL(tc.url)
		if tc.wantErr {
			if err == nil {
				t.Errorf("driverFromURL(%q) = %q, want error", tc.url, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("driverFromURL(%q) unexpected error: %v", tc.url, err)
			continue
		}
		if got != tc.want {
			t.Errorf("driverFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

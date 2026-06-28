package config

import (
	"os"
	"testing"
)

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

func TestResolveProjectDir(t *testing.T) {
	wd, _ := os.Getwd()
	cases := []struct {
		in, want string
	}{
		{"%kernel.project_dir%/config/jwt/.secret.public.pem", wd + "/config/jwt/.secret.public.pem"},
		{"/var/www/config/jwt/public.pem", "/var/www/config/jwt/public.pem"}, // no placeholder -> unchanged
		{"", ""},
	}
	for _, c := range cases {
		if got := ResolveProjectDir(c.in); got != c.want {
			t.Errorf("ResolveProjectDir(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGetStringList(t *testing.T) {
	const key = "ECONUMO_TEST_STRING_LIST"
	def := []string{"d"}

	cases := []struct {
		name string
		set  bool
		val  string
		want []string
	}{
		{"unset returns default", false, "", def},
		{"empty returns default", true, "", def},
		{"all-empty returns default", true, " , , ", def},
		{"simple list", true, "a,b", []string{"a", "b"}},
		{"trims and drops empties", true, " a , ,b ", []string{"a", "b"}},
		{"single value", true, "https://app.example.com", []string{"https://app.example.com"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(key, tc.val)
			} else {
				os.Unsetenv(key)
			}
			got := getStringList(key, def)
			if len(got) != len(tc.want) {
				t.Fatalf("getStringList(%q) = %v, want %v", tc.val, got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("getStringList(%q) = %v, want %v", tc.val, got, tc.want)
				}
			}
		})
	}
}

func TestLoadLogLevel(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")

	// Default when unset.
	t.Setenv("ECONUMO_LOG_LEVEL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("default LogLevel = %q, want %q", cfg.LogLevel, "info")
	}

	// Honored when set.
	t.Setenv("ECONUMO_LOG_LEVEL", "debug")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}

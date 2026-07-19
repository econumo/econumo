package config

import (
	"os"
	"testing"
	"time"
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

func TestParseMailerDSN(t *testing.T) {
	cases := []struct {
		name                            string
		dsn                             string
		provider, apiKey, from, replyTo string
		wantErr                         bool
	}{
		{name: "empty defaults to console", dsn: "", provider: "console"},
		{name: "blank defaults to console", dsn: "   ", provider: "console"},
		{name: "console scheme", dsn: "console://", provider: "console"},
		{name: "log scheme", dsn: "log://", provider: "console"},
		{name: "console with envelope", dsn: "console://?from=a@x.test&reply_to=b@x.test", provider: "console", from: "a@x.test", replyTo: "b@x.test"},
		{name: "resend with key", dsn: "resend://re_Abc_123", provider: "resend", apiKey: "re_Abc_123"},
		{name: "resend with envelope", dsn: "resend://re_Abc_123?from=a@x.test&reply_to=b@x.test", provider: "resend", apiKey: "re_Abc_123", from: "a@x.test", replyTo: "b@x.test"},
		{name: "scheme is case-insensitive", dsn: "RESEND://re_Abc_123", provider: "resend", apiKey: "re_Abc_123"},
		{name: "resend without key errors", dsn: "resend://", wantErr: true},
		{name: "unsupported scheme errors", dsn: "smtp://localhost", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider, apiKey, from, replyTo, err := parseMailerDSN(tc.dsn)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseMailerDSN(%q) = (%q,%q,%q,%q), want error", tc.dsn, provider, apiKey, from, replyTo)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMailerDSN(%q) unexpected error: %v", tc.dsn, err)
			}
			if provider != tc.provider || apiKey != tc.apiKey || from != tc.from || replyTo != tc.replyTo {
				t.Errorf("parseMailerDSN(%q) = (%q,%q,%q,%q), want (%q,%q,%q,%q)",
					tc.dsn, provider, apiKey, from, replyTo, tc.provider, tc.apiKey, tc.from, tc.replyTo)
			}
		})
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

func TestLoad_RateLimitDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.RateLimitLogin != 5 || c.RateLimitReset != 5 || c.RateLimitRemind != 3 || c.RateLimitRegister != 5 {
		t.Fatalf("per-endpoint defaults = %d/%d/%d/%d, want 5/5/3/5",
			c.RateLimitLogin, c.RateLimitReset, c.RateLimitRemind, c.RateLimitRegister)
	}
	if c.RateLimitWindow != 15*time.Minute {
		t.Fatalf("window = %v, want 15m", c.RateLimitWindow)
	}
	if c.RateLimitGlobal != 60 {
		t.Fatalf("global = %d, want 60", c.RateLimitGlobal)
	}
}

func TestLoad_RateLimitOverridesAndDisable(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	t.Setenv("ECONUMO_RATE_LIMIT_LOGIN", "10")
	t.Setenv("ECONUMO_RATE_LIMIT_RESET", "0") // 0 = disabled
	t.Setenv("ECONUMO_RATE_LIMIT_REMIND", "7")
	t.Setenv("ECONUMO_RATE_LIMIT_REGISTER", "8")
	t.Setenv("ECONUMO_RATE_LIMIT_WINDOW", "1h30m")
	t.Setenv("ECONUMO_RATE_LIMIT_GLOBAL", "0")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.RateLimitLogin != 10 || c.RateLimitReset != 0 || c.RateLimitRemind != 7 || c.RateLimitRegister != 8 {
		t.Fatalf("overrides not applied: %+v", c)
	}
	if c.RateLimitWindow != 90*time.Minute || c.RateLimitGlobal != 0 {
		t.Fatalf("window/global overrides not applied: %v / %d", c.RateLimitWindow, c.RateLimitGlobal)
	}
}

func TestLoad_RateLimitBadValuesFailBoot(t *testing.T) {
	cases := []struct {
		name string
		key  string
		bad  string
	}{
		{"LOGIN_non-numeric", "ECONUMO_RATE_LIMIT_LOGIN", "five"},
		{"GLOBAL_negative", "ECONUMO_RATE_LIMIT_GLOBAL", "-1"},
		{"WINDOW_unparseable", "ECONUMO_RATE_LIMIT_WINDOW", "15minutes"},
		{"WINDOW_zero", "ECONUMO_RATE_LIMIT_WINDOW", "0"},
		{"WINDOW_negative", "ECONUMO_RATE_LIMIT_WINDOW", "-5m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
			t.Setenv(tc.key, tc.bad)
			if _, err := Load(); err == nil {
				t.Fatalf("Load() with %s=%q succeeded, want boot error", tc.key, tc.bad)
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

func TestLoad_CheckUpdates(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.CheckUpdates {
		t.Fatal("CheckUpdates default = false, want true")
	}
	t.Setenv("ECONUMO_CHECK_UPDATES", "false")
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.CheckUpdates {
		t.Fatal("CheckUpdates with ECONUMO_CHECK_UPDATES=false = true, want false")
	}
}

func TestLoad_Analytics(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Analytics {
		t.Fatal("Analytics default = false, want true")
	}
	t.Setenv("ECONUMO_ANALYTICS", "false")
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Analytics {
		t.Fatal("Analytics with ECONUMO_ANALYTICS=false = true, want false")
	}
	// Strict parse: a typo while trying to disable analytics fails at boot
	// rather than silently leaving it enabled.
	t.Setenv("ECONUMO_ANALYTICS", "flase")
	if _, err = Load(); err == nil {
		t.Fatal("Load with ECONUMO_ANALYTICS=flase: err = nil, want boot error")
	}
}

func TestLoad_SPAOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.APIURL != "" {
		t.Fatalf("APIURL default = %q, want empty (leave the dist value)", c.APIURL)
	}
	if c.AllowCustomAPI != nil {
		t.Fatal("AllowCustomAPI default != nil, want nil (leave the dist value)")
	}
	t.Setenv("ECONUMO_API_URL", "https://api.example.test")
	t.Setenv("ECONUMO_ALLOW_CUSTOM_API", "false")
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.APIURL != "https://api.example.test" {
		t.Fatalf("APIURL = %q, want %q", c.APIURL, "https://api.example.test")
	}
	if c.AllowCustomAPI == nil || *c.AllowCustomAPI {
		t.Fatal("AllowCustomAPI = nil or true, want false")
	}
	t.Setenv("ECONUMO_ALLOW_CUSTOM_API", "maybe")
	if _, err = Load(); err == nil {
		t.Fatal("Load with ECONUMO_ALLOW_CUSTOM_API=maybe: err = nil, want boot error")
	}
	t.Setenv("ECONUMO_ALLOW_CUSTOM_API", "true")
	t.Setenv("ECONUMO_API_URL", "not a url")
	if _, err = Load(); err == nil {
		t.Fatal("Load with malformed ECONUMO_API_URL: err = nil, want boot error")
	}
}

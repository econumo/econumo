package authserver

import "testing"

func TestNewServiceEnabledOnlyForSecureIssuer(t *testing.T) {
	cases := []struct {
		appURL  string
		enabled bool
	}{
		{"https://econumo.example.test", true},
		{"https://econumo.example.test/", true},
		{"http://localhost:8181", true},
		{"http://127.0.0.1", true},
		{"http://[::1]:8181", true},
		{"http://econumo.lan", false},
		{"http://192.168.1.5", false},
		{"http://econumo.example.test", false},
		{"garbage", false},
		{"//econumo.example.test", false},
		{"https://", false},
		{"ftp://localhost", false},
		{"", false},
	}
	for _, c := range cases {
		s := NewService(nil, nil, nil, nil, nil, c.appURL)
		if s.Enabled() != c.enabled {
			t.Errorf("NewService(%q).Enabled() = %v, want %v", c.appURL, s.Enabled(), c.enabled)
		}
		if !c.enabled && s.Issuer() != "" {
			t.Errorf("NewService(%q).Issuer() = %q, want empty", c.appURL, s.Issuer())
		}
	}
}

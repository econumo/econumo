package authserver

import "testing"

func TestNewServiceEnabledOnlyForSecureIssuer(t *testing.T) {
	cases := []struct {
		appURL  string
		enabled bool
		issuer  string
	}{
		{"https://econumo.example.test", true, ""},
		{"https://econumo.example.test/", true, ""},
		{"http://localhost:8181", true, ""},
		{"http://127.0.0.1", true, ""},
		{"http://[::1]:8181", true, ""},
		{"http://econumo.lan", false, ""},
		{"http://192.168.1.5", false, ""},
		{"http://econumo.example.test", false, ""},
		{"garbage", false, ""},
		{"//econumo.example.test", false, ""},
		{"https://", false, ""},
		{"ftp://localhost", false, ""},
		{"", false, ""},
		{"https://user:pw@econumo.example.test", false, ""},
		{"https://econumo.example.test?x=1", false, ""},
		{"https://econumo.example.test/#frag", false, ""},
		{"https://econumo.example.test/?", false, ""},
		{"HTTPS://Econumo.Example.TEST/", true, "https://econumo.example.test"},
		{"https://econumo.example.test/app/", true, "https://econumo.example.test/app"},
		{"  https://econumo.example.test  ", true, "https://econumo.example.test"},
	}
	for _, c := range cases {
		s := NewService(nil, nil, nil, nil, nil, c.appURL)
		if s.Enabled() != c.enabled {
			t.Errorf("NewService(%q).Enabled() = %v, want %v", c.appURL, s.Enabled(), c.enabled)
		}
		if c.enabled && s.Issuer() != c.issuer && c.issuer != "" {
			t.Errorf("NewService(%q).Issuer() = %q, want %q", c.appURL, s.Issuer(), c.issuer)
		}
		if !c.enabled && s.Issuer() != "" {
			t.Errorf("NewService(%q).Issuer() = %q, want empty", c.appURL, s.Issuer())
		}
	}
}

package authserver

import "testing"

func TestValidateRedirectURI(t *testing.T) {
	ok := []string{"https://claude.ai/api/mcp/auth_callback", "http://127.0.0.1:33418/callback", "http://localhost/cb", "http://[::1]:9/cb"}
	bad := []string{"", "/relative", "http://example.com/cb", "https://a.test/cb#frag", "javascript:alert(1)", "https://user:pw@a.test/cb", "ftp://a.test/cb", "cursor://anysphere/cb"}
	for _, u := range ok {
		if err := ValidateRedirectURI(u); err != nil {
			t.Errorf("%q rejected: %v", u, err)
		}
	}
	for _, u := range bad {
		if err := ValidateRedirectURI(u); err == nil {
			t.Errorf("%q accepted", u)
		}
	}
}

func TestMatchRedirectURI(t *testing.T) {
	reg := []string{"https://claude.ai/api/mcp/auth_callback", "http://localhost:4000/callback"}
	cases := map[string]bool{
		"https://claude.ai/api/mcp/auth_callback":     true,
		"https://claude.ai/api/mcp/auth_callback/":    false,
		"https://claude.ai/api/mcp/auth_callback?x=1": false,
		"http://localhost:5555/callback":              true,
		"http://localhost/callback":                   true,
		"http://127.0.0.1:4000/callback":              false,
		"http://localhost:4000/other":                 false,
	}
	for in, want := range cases {
		if got := MatchRedirectURI(reg, in); got != want {
			t.Errorf("%q = %v, want %v", in, got, want)
		}
	}
}

func TestRedirectHost(t *testing.T) {
	if h, lb := RedirectHost("https://claude.ai/api/mcp/auth_callback"); h != "claude.ai" || lb {
		t.Fatal(h, lb)
	}
	if _, lb := RedirectHost("http://127.0.0.1:1/cb"); !lb {
		t.Fatal("loopback")
	}
}

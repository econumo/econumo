package authserver

import (
	"errors"
	"net/url"
	"unicode/utf8"
)

func isLoopbackHost(h string) bool { return h == "127.0.0.1" || h == "::1" || h == "localhost" }

func ValidateRedirectURI(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" || u.Fragment != "" || u.User != nil {
		return errors.New("redirect URI must be absolute, without fragment or userinfo")
	}
	// A non-ASCII host can spell a lookalike of a trusted one; the consent
	// page shows the host, so only its ASCII (punycode) form is acceptable.
	for i := 0; i < len(u.Hostname()); i++ {
		if u.Hostname()[i] >= utf8.RuneSelf {
			return errors.New("redirect URI host must be ASCII")
		}
	}
	switch {
	case u.Scheme == "https":
		return nil
	case u.Scheme == "http" && isLoopbackHost(u.Hostname()):
		return nil
	}
	return errors.New("redirect URI must be https, or http on a loopback host")
}

// Loopback redirects match ignoring the port: native clients (Claude Code,
// Codex) bind an ephemeral port per sign-in (RFC 8252 section 7.3).
func MatchRedirectURI(registered []string, presented string) bool {
	p, err := url.Parse(presented)
	if err != nil || ValidateRedirectURI(presented) != nil {
		return false
	}
	for _, r := range registered {
		if r == presented {
			return true
		}
		ru, err := url.Parse(r)
		if err != nil || ru.Scheme != "http" || !isLoopbackHost(ru.Hostname()) {
			continue
		}
		if p.Scheme == "http" && p.Hostname() == ru.Hostname() && p.EscapedPath() == ru.EscapedPath() && p.RawQuery == ru.RawQuery {
			return true
		}
	}
	return false
}

func RedirectHost(uri string) (string, bool) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", false
	}
	return u.Hostname(), u.Scheme == "http" && isLoopbackHost(u.Hostname())
}

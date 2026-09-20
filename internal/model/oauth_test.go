package model

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestNewPasswordlessUser(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	u := NewPasswordlessUser(vo.NewId(), "a@example.test", "Alice", "face:sky", now)
	if u.Algorithm != AlgorithmNone || u.Password != "" || u.Salt != "" || !u.EmailVerified || !u.IsActive {
		t.Fatalf("unexpected passwordless user: %+v", u)
	}
	if u.HasPassword() {
		t.Fatal("passwordless user must report no password")
	}
	u.UpdatePassword("hash", AlgorithmArgon2id, now)
	if !u.HasPassword() {
		t.Fatal("after UpdatePassword the user has a password")
	}
}

func TestOAuthStateAndHandoffExpiry(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	s := OAuthState{ExpiresAt: now.Add(OAuthStateTTL)}
	if s.IsExpired(now) || !s.IsExpired(now.Add(OAuthStateTTL)) {
		t.Fatal("state expiry is exclusive of ExpiresAt")
	}
	h := OAuthHandoff{ExpiresAt: now.Add(OAuthHandoffTTL)}
	if h.IsExpired(now) || !h.IsExpired(now.Add(OAuthHandoffTTL)) {
		t.Fatal("handoff expiry is exclusive of ExpiresAt")
	}
}

func TestStartOAuthRequestValidate(t *testing.T) {
	cases := []struct {
		req  StartOAuthRequest
		want string // field key of the first error, "" when valid
	}{
		{StartOAuthRequest{Provider: "google", Client: "web"}, ""},
		{StartOAuthRequest{Provider: "oidc", Client: "app"}, ""},
		{StartOAuthRequest{Provider: "github", Client: "web"}, "provider"},
		{StartOAuthRequest{Provider: "", Client: "web"}, "provider"},
		{StartOAuthRequest{Provider: "google", Client: "desktop"}, "client"},
	}
	for _, c := range cases {
		err := c.req.Validate()
		if c.want == "" {
			if err != nil {
				t.Fatalf("%+v: unexpected %v", c.req, err)
			}
			continue
		}
		v, ok := errs.AsValidation(err)
		if !ok || len(v.Fields) == 0 || v.Fields[0].Key != c.want {
			t.Fatalf("%+v: want field %q error, got %v", c.req, c.want, err)
		}
	}
	if err := (ExchangeHandoffRequest{}).Validate(); err == nil {
		t.Fatal("blank code must fail")
	}
	if err := (UnlinkIdentityRequest{Provider: "apple"}).Validate(); err != nil {
		t.Fatal(err)
	}
}

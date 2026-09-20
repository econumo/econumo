package oauth_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestListIdentityEmails(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "owner@example.test", "argon2id")
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "google", h.fake.IssuerURL(), "g1", "alice@gmail.test", h.clock.Now()))
	// A provider that reported no address: the column defaults to "", and an
	// empty recipient must never reach the Cc list.
	saveIdentity(t, h, model.NewIdentity(vo.NewId(), u.ID, "oidc", h.fake.IssuerURL(), "o1", "", h.clock.Now()))

	got, err := h.svc.ListIdentityEmails(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("ListIdentityEmails: %v", err)
	}
	if len(got) != 1 || got[0] != "alice@gmail.test" {
		t.Fatalf("emails = %v, want only the address a provider actually reported", got)
	}
}

func TestListIdentityEmails_NoIdentities(t *testing.T) {
	h := newHarness(t, false, true)
	u := h.users.seed(t, "lonely@example.test", "argon2id")

	got, err := h.svc.ListIdentityEmails(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("ListIdentityEmails: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("emails = %v, want none", got)
	}
}

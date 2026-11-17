package user_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func TestServerOnlyOptionHidden(t *testing.T) {
	ctx := context.Background()
	h := newRegisterHarness(t)
	u := h.registerUser(t, "aff@example.test")
	u.SetAffiliate("partner_1", vo.NewId(), time.Now().UTC())
	if err := h.db.TX.WithTx(ctx, func(ctx context.Context) error { return h.repo.Save(ctx, u) }); err != nil {
		t.Fatalf("Save: %v", err)
	}
	stored, err := h.repo.GetByEmail(ctx, "aff@example.test")
	if err != nil || stored.Affiliate() != "partner_1" {
		t.Fatalf("affiliate not persisted: %v %q", err, stored.Affiliate())
	}

	read := appuser.NewReadService(userrepo.NewReadRepo(h.db.Engine, h.db.TX), auth.NewEncodeService(""), trialClock{})
	list, err := read.GetOptionList(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetOptionList: %v", err)
	}
	assertNoAffiliate(t, "GetOptionList", list.Items)
	data, err := read.GetUserData(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserData: %v", err)
	}
	assertNoAffiliate(t, "GetUserData", data.User.Options)

	login, err := h.svc.Login(ctx, model.LoginRequest{Username: "aff@example.test", Password: "secretpass"}, "test", time.Now().UTC())
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	assertNoAffiliate(t, "Login (toCurrentUser)", login.User.Options)
}

func assertNoAffiliate(t *testing.T, where string, opts []model.OptionResult) {
	t.Helper()
	if len(opts) == 0 {
		t.Fatalf("%s: no options at all", where)
	}
	for _, o := range opts {
		if o.Name == model.OptionAffiliate {
			t.Errorf("%s exposes the affiliate option", where)
		}
	}
}

func TestRegisterStoresAffiliate(t *testing.T) {
	ctx := reqctx.WithAffiliate(context.Background(), "abc123")
	h := newRegisterHarness(t)
	res, err := h.svc.Register(ctx, model.RegisterRequest{Name: "Aff", Email: "aff-reg@example.test", Password: "secretpass"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	for _, o := range res.User.Options {
		if o.Name == model.OptionAffiliate {
			t.Fatal("the Register response exposes the affiliate option")
		}
	}
	stored, err := h.repo.GetByEmail(context.Background(), "aff-reg@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Affiliate() != "abc123" {
		t.Fatalf("Affiliate = %q, want abc123", stored.Affiliate())
	}
	if stored.Option(model.OptionAnalytics) == nil {
		t.Fatal("default options must survive alongside the affiliate")
	}
}

func TestProvisionExternalUserStoresAffiliate(t *testing.T) {
	ctx := reqctx.WithAffiliate(context.Background(), "abc123")
	h := newRegisterHarness(t)
	u, err := h.svc.ProvisionExternalUser(ctx, "Ext", "ext-aff@example.test")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := h.repo.GetByID(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Affiliate() != "abc123" {
		t.Fatalf("Affiliate = %q, want abc123", stored.Affiliate())
	}
}

func TestAdminCreateUserIgnoresAffiliate(t *testing.T) {
	ctx := reqctx.WithAffiliate(context.Background(), "abc123")
	h := newRegisterHarness(t)
	id, err := h.svc.AdminCreateUser(ctx, "Cli", "cli-aff@example.test", "secretpass")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := h.repo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Option(model.OptionAffiliate) != nil {
		t.Fatal("an operator-created account must carry no affiliate")
	}
}

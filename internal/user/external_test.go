package user_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestProvisionExternalUser_PasswordlessVerifiedWithDefaults(t *testing.T) {
	db := dbtest.New(t)
	s, _, _ := newTrialSvc(t, db, 0)
	ctx := context.Background()

	u, err := s.ProvisionExternalUser(ctx, "Alice", "Alice@Example.test")
	if err != nil {
		t.Fatal(err)
	}
	if u.Algorithm != model.AlgorithmNone || u.HasPassword() || !u.EmailVerified {
		t.Fatalf("want passwordless verified user, got %+v", u)
	}
	if u.Option(model.OptionCurrency) == nil || u.Option(model.OptionAnalytics) == nil {
		t.Fatal("default options must be seeded like registration")
	}
	if _, err := s.ProvisionExternalUser(ctx, "Alice", "alice@example.test"); err == nil {
		t.Fatal("duplicate email must fail")
	}
	// Password login on a passwordless user is the frozen 401.
	_, lerr := s.Login(ctx, model.LoginRequest{Username: "alice@example.test", Password: "anything"}, "ua", time.Now())
	if _, ok := errs.AsUnauthorized(lerr); !ok {
		t.Fatalf("want 401, got %v", lerr)
	}
}

func TestCreateExternalSession_StampsProviderAndReturnsLogin(t *testing.T) {
	db := dbtest.New(t)
	s, repo, _ := newTrialSvc(t, db, 0)
	ctx := context.Background()

	u, err := s.ProvisionExternalUser(ctx, "Bob", "bob@example.test")
	if err != nil {
		t.Fatal(err)
	}
	idTok := "raw.id.token"
	res, err := s.CreateExternalSession(ctx, u.ID, "Mozilla/5.0", model.OAuthProviderOIDC, &idTok)
	if err != nil {
		t.Fatal(err)
	}
	if res.Token == "" || res.User.Id != u.ID.String() || res.User.HasPassword {
		t.Fatalf("unexpected login result %+v", res)
	}
	sessions, err := s.ListSessions(ctx, u.ID, vo.Id{})
	if err != nil || len(sessions) != 1 || sessions[0].Provider != "oidc" {
		t.Fatalf("session must carry provider: %+v %v", sessions, err)
	}

	u.Deactivate(time.Now())
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateExternalSession(ctx, u.ID, "ua", model.OAuthProviderOIDC, nil); err == nil {
		t.Fatal("inactive user must not get a session")
	}
}

func TestReplaceVerifiedEmail(t *testing.T) {
	db := dbtest.New(t)
	s, repo, _ := newTrialSvc(t, db, 0)
	ctx := context.Background()

	u, err := s.ProvisionExternalUser(ctx, "Carol", "carol@old.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceVerifiedEmail(ctx, u.ID, "carol@new.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByEmail(ctx, "carol@new.test"); err != nil {
		t.Fatalf("new email must resolve: %v", err)
	}
}

type fakeLogoutURLs struct{ url string }

func (f fakeLogoutURLs) EndSessionURL(_ context.Context, provider, idToken string) (string, error) {
	if provider == "oidc" && idToken != "" {
		return f.url, nil
	}
	return "", nil
}

func TestLogout_ReturnsEndSessionURLForOIDCSessions(t *testing.T) {
	db := dbtest.New(t)
	s, _, _ := newTrialSvc(t, db, 0)
	ctx := context.Background()

	s.SetLogoutURLBuilder(fakeLogoutURLs{url: "https://idp.example.test/end?x=1"})
	u, err := s.ProvisionExternalUser(ctx, "Dan", "dan@example.test")
	if err != nil {
		t.Fatal(err)
	}
	idTok := "t"
	res, err := s.CreateExternalSession(ctx, u.ID, "ua", "oidc", &idTok)
	if err != nil {
		t.Fatal(err)
	}
	_, tid, _, err := s.Authenticate(ctx, res.Token)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Logout(ctx, tid)
	if err != nil || out.LogoutUrl != "https://idp.example.test/end?x=1" || out.Provider != "oidc" || out.Result != "test" {
		t.Fatalf("logout result %+v %v", out, err)
	}

	// A Google session (no id token) logs out locally but still names the provider.
	res2, err := s.CreateExternalSession(ctx, u.ID, "ua", "google", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, tid2, _, err := s.Authenticate(ctx, res2.Token)
	if err != nil {
		t.Fatal(err)
	}
	out2, err := s.Logout(ctx, tid2)
	if err != nil {
		t.Fatal(err)
	}
	if out2.LogoutUrl != "" || out2.Provider != "google" {
		t.Fatalf("google logout %+v", out2)
	}
}

func TestRevokeAllSessionsAndMarkEmailVerified(t *testing.T) {
	db := dbtest.New(t)
	s, repo, _ := newTrialSvc(t, db, 0)
	ctx := context.Background()

	u, err := s.Register(ctx, model.RegisterRequest{Name: "Eve", Email: "eve@example.test", Password: "secret123"})
	if err != nil {
		t.Fatal(err)
	}
	uid := vo.MustParseId(u.User.Id)
	first, err := s.Login(ctx, model.LoginRequest{Username: "eve@example.test", Password: "secret123"}, "ua", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	pat, err := s.CreatePersonalToken(ctx, uid, model.CreatePersonalTokenRequest{Name: "ci"})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.RevokeAllSessions(ctx, uid); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Authenticate(ctx, first.Token); err == nil {
		t.Fatal("every session must be revoked")
	}
	if _, _, _, err := s.Authenticate(ctx, pat.Token); err != nil {
		t.Fatalf("personal tokens survive: %v", err)
	}

	if err := s.MarkEmailVerified(ctx, uid); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetByID(ctx, uid)
	if err != nil || !stored.EmailVerified {
		t.Fatalf("email must be verified: %+v %v", stored, err)
	}
}

package user_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// registerHarness wraps newTrialSvc (trial_integration_test.go) with a
// registerUser convenience so analytics-seeding tests don't re-derive the
// service wiring.
type registerHarness struct {
	svc  *appuser.Service
	repo *userrepo.Repo
	db   *dbtest.DB
}

func newHarnessWithAnalyticsDefault(t *testing.T, analyticsDefault bool) *registerHarness {
	t.Helper()
	db := dbtest.New(t)
	svc, repo, _ := newTrialSvc(t, db, 0, analyticsDefault)
	return &registerHarness{svc: svc, repo: repo, db: db}
}

func (h *registerHarness) registerUser(t *testing.T, email string) *model.User {
	t.Helper()
	ctx := context.Background()
	if _, err := h.svc.Register(ctx, model.RegisterRequest{
		Name: "Seed User", Email: email, Password: "secretpass",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	u, err := h.repo.GetByEmail(ctx, email)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	return u
}

// insertUserWithoutOptions writes a user row with no users_options rows at
// all, via a raw repo Save rather than Register (which always seeds
// analytics). This is the pre-migration shape: an existing user predating the
// per-user preference.
func (h *registerHarness) insertUserWithoutOptions(t *testing.T, email string) *model.User {
	t.Helper()
	ctx := context.Background()
	id := h.repo.NextIdentity()
	now := trialNow
	u := model.NewUser(id, email, "Seed User", "face:fuchsia", "hash", "salt", now)
	if err := h.db.TX.WithTx(ctx, func(ctx context.Context) error { return h.repo.Save(ctx, u) }); err != nil {
		t.Fatalf("insertUserWithoutOptions Save: %v", err)
	}
	return u
}

func TestRegisterSeedsAnalyticsOptionFromConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		def  bool
		want string
	}{
		{"default on", true, "1"},
		{"default off", false, "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarnessWithAnalyticsDefault(t, tc.def)
			u := h.registerUser(t, "seed@example.test")

			o := u.Option(model.OptionAnalytics)
			if o == nil || o.Value == nil {
				t.Fatal("analytics option not seeded")
			}
			if *o.Value != tc.want {
				t.Fatalf("value = %q, want %q", *o.Value, tc.want)
			}
		})
	}
}

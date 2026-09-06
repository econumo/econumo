package user_test

import (
	"context"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// readHarness wires both the write Service (to register a user) and the
// ReadService (to serve get-user-data) over the same sqlite db, mirroring the
// wiring in server.BuildAPI.
type readHarness struct {
	svc  *appuser.Service
	read *appuser.ReadService
}

type fixedNowClock struct{ now time.Time }

func (c fixedNowClock) Now() time.Time { return c.now }

func newHarness(t *testing.T) *readHarness {
	t.Helper()
	db := dbtest.New(t)
	enc := auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()
	repo := userrepo.NewRepo(db.Engine, db.TX)
	tokens := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	lookup := currencyrepo.New(db.Engine, db.TX)
	budgets := server.NewUserBudgetAccess(db.Engine, db.TX)
	clk := fixedNowClock{now: time.Now().UTC()}
	svc := appuser.NewService(repo, db.TX, enc, hasher, tokens, server.NewUserCurrencyLookup(lookup), budgets, nil, nil,
		userrepo.NewEmailVerificationRepo(db.Engine, db.TX), nil,
		userrepo.NewEmailChangeRequestRepo(db.Engine, db.TX), nil,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clk, nil, true, 0, false)
	readRepo := userrepo.NewReadRepo(db.Engine, db.TX)
	readSvc := appuser.NewReadService(readRepo, enc, clk)
	return &readHarness{svc: svc, read: readSvc}
}

func (h *readHarness) registerUser(t *testing.T, email string) vo.Id {
	t.Helper()
	uid, err := h.svc.AdminCreateUser(context.Background(), "Created At Tester", email, "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	return uid
}

func TestCurrentUserCarriesCreatedAt(t *testing.T) {
	h := newHarness(t)
	uid := h.registerUser(t, "created@example.test")

	got, err := h.read.GetUserData(context.Background(), uid)
	if err != nil {
		t.Fatalf("GetUserData: %v", err)
	}
	if got.User.CreatedAt == "" {
		t.Fatal("createdAt is empty")
	}
	if _, perr := time.Parse("2006-01-02 15:04:05", got.User.CreatedAt); perr != nil {
		t.Fatalf("createdAt %q is not the frozen layout: %v", got.User.CreatedAt, perr)
	}
}

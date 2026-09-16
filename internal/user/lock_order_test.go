package user_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// callLog records the persistence calls a use case makes, in order, so a test
// can assert the user row is locked BEFORE the row is read and written. The
// race itself only shows up across two connections (see
// mutate_lock_pgsql_test.go); this is the cheap engine-independent guard that
// every one of those call sites still takes the lock first.
type callLog struct{ calls []string }

func (l *callLog) add(name string) { l.calls = append(l.calls, name) }
func (l *callLog) reset()          { l.calls = nil }

type orderRepo struct {
	appuser.Repository
	log *callLog
}

func (r *orderRepo) LockRow(ctx context.Context, userID vo.Id) error {
	r.log.add("LockRow")
	return r.Repository.LockRow(ctx, userID)
}

func (r *orderRepo) GetByID(ctx context.Context, id vo.Id) (*model.User, error) {
	r.log.add("GetByID")
	return r.Repository.GetByID(ctx, id)
}

func (r *orderRepo) Save(ctx context.Context, u *model.User) error {
	r.log.add("Save")
	return r.Repository.Save(ctx, u)
}

// orderRequests records the reset-code store's writes, so the remind/reset
// pair can be asserted to issue and consume a code under the same row lock.
type orderRequests struct {
	appuser.PasswordRequests
	log *callLog
}

func (r *orderRequests) DeleteByUser(ctx context.Context, userID vo.Id) error {
	r.log.add("DeleteByUser")
	return r.PasswordRequests.DeleteByUser(ctx, userID)
}

func (r *orderRequests) Save(ctx context.Context, pr *model.PasswordRequest) error {
	r.log.add("SaveRequest")
	return r.PasswordRequests.Save(ctx, pr)
}

func (r *orderRequests) Consume(ctx context.Context, id, userID vo.Id) (int64, error) {
	r.log.add("Consume")
	return r.PasswordRequests.Consume(ctx, id, userID)
}

type orderTokens struct {
	appuser.AccessTokens
	log *callLog
}

func (t *orderTokens) InsertIfGeneration(ctx context.Context, tok *model.AccessToken, generation int64) (int64, error) {
	t.log.add("InsertIfGeneration")
	return t.AccessTokens.InsertIfGeneration(ctx, tok, generation)
}

func (t *orderTokens) InsertIfPresenterLive(ctx context.Context, tok *model.AccessToken, presentingTokenID vo.Id) (int64, error) {
	t.log.add("InsertIfPresenterLive")
	return t.AccessTokens.InsertIfPresenterLive(ctx, tok, presentingTokenID)
}

// assertLockedFirst checks that want occurs, in order, after the first LockRow.
func assertLockedFirst(t *testing.T, got []string, want ...string) {
	t.Helper()
	lock := -1
	for i, c := range got {
		if c == "LockRow" {
			lock = i
			break
		}
	}
	if lock < 0 {
		t.Fatalf("no LockRow: calls = %v", got)
	}
	rest := got[lock+1:]
	for _, w := range want {
		found := -1
		for i, c := range rest {
			if c == w {
				found = i
				break
			}
		}
		if found < 0 {
			t.Fatalf("%q must come after LockRow: calls = %v", w, got)
		}
		rest = rest[found+1:]
	}
}

func TestEveryExistingUserWriteTakesTheRowLockFirst(t *testing.T) {
	const email = "lock-order@econumo.test"
	const password = "secretpass"

	// The whole-aggregate writers: the lock, then the read the write is built
	// from. The credential mints follow below — they lock, then insert.
	// want defaults to the whole-aggregate pair; a case that writes something
	// else under the lock (the reset's code consumption, remind's issuance)
	// names its own calls.
	for _, tc := range []struct {
		name string
		run  func(t *testing.T, svc *appuser.Service, db *dbtest.DB, uid vo.Id)
		want []string
	}{
		{name: "update-name (mutate)", run: func(t *testing.T, svc *appuser.Service, _ *dbtest.DB, uid vo.Id) {
			if _, err := svc.UpdateName(context.Background(), uid, model.UpdateNameRequest{Name: "Renamed"}); err != nil {
				t.Fatalf("UpdateName: %v", err)
			}
		}},
		{name: "confirm-email", run: func(t *testing.T, svc *appuser.Service, db *dbtest.DB, uid vo.Id) {
			ctx := context.Background()
			ev := model.NewEmailVerification(vo.NewId(), uid, appuser.HashResetCode("123456"), time.Now().UTC())
			if err := userrepo.NewEmailVerificationRepo(db.Engine, db.TX).Save(ctx, ev); err != nil {
				t.Fatalf("seed verification: %v", err)
			}
			if _, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: email, Code: "123456"}); err != nil {
				t.Fatalf("ConfirmEmail: %v", err)
			}
		}},
		{name: "reset-password", run: func(t *testing.T, svc *appuser.Service, db *dbtest.DB, uid vo.Id) {
			ctx := context.Background()
			pr := model.NewPasswordRequest(vo.NewId(), uid, appuser.HashResetCode("482913"), time.Now().UTC())
			if err := userrepo.NewPasswordRequestRepo(db.Engine, db.TX).Save(ctx, pr); err != nil {
				t.Fatalf("seed password request: %v", err)
			}
			if _, err := svc.ResetPassword(ctx, model.ResetPasswordRequest{
				Username: email, Code: "482913", Password: "reset-password",
			}); err != nil {
				t.Fatalf("ResetPassword: %v", err)
			}
		}, want: []string{"GetByID", "Consume", "Save"}},
		{name: "remind-password", run: func(t *testing.T, svc *appuser.Service, _ *dbtest.DB, _ vo.Id) {
			if _, err := svc.RemindPassword(context.Background(), model.RemindPasswordRequest{Username: email}); err != nil {
				t.Fatalf("RemindPassword: %v", err)
			}
		}, want: []string{"DeleteByUser", "SaveRequest"}},
		{name: "admin change-email", run: func(t *testing.T, svc *appuser.Service, _ *dbtest.DB, _ vo.Id) {
			if err := svc.AdminChangeEmail(context.Background(), email, "lock-order-new@econumo.test"); err != nil {
				t.Fatalf("AdminChangeEmail: %v", err)
			}
		}},
		{name: "admin change-password", run: func(t *testing.T, svc *appuser.Service, _ *dbtest.DB, _ vo.Id) {
			if err := svc.AdminChangePassword(context.Background(), email, "operator-set-pw"); err != nil {
				t.Fatalf("AdminChangePassword: %v", err)
			}
		}},
		{name: "admin activate", run: func(t *testing.T, svc *appuser.Service, _ *dbtest.DB, _ vo.Id) {
			if err := svc.AdminActivate(context.Background(), email); err != nil {
				t.Fatalf("AdminActivate: %v", err)
			}
		}},
		{name: "admin deactivate", run: func(t *testing.T, svc *appuser.Service, _ *dbtest.DB, _ vo.Id) {
			if err := svc.AdminDeactivate(context.Background(), email); err != nil {
				t.Fatalf("AdminDeactivate: %v", err)
			}
		}},
		{name: "admin verify-email", run: func(t *testing.T, svc *appuser.Service, _ *dbtest.DB, _ vo.Id) {
			if err := svc.AdminVerifyEmail(context.Background(), email); err != nil {
				t.Fatalf("AdminVerifyEmail: %v", err)
			}
		}},
		{name: "admin set-access", run: func(t *testing.T, svc *appuser.Service, _ *dbtest.DB, _ vo.Id) {
			if _, err := svc.AdminSetAccess(context.Background(), email, model.AccessLevelReadonly, nil); err != nil {
				t.Fatalf("AdminSetAccess: %v", err)
			}
		}},
		{name: "admin set-access by id", run: func(t *testing.T, svc *appuser.Service, _ *dbtest.DB, uid vo.Id) {
			if _, _, err := svc.AdminSetAccessByID(context.Background(), uid, model.AccessLevelReadonly, nil); err != nil {
				t.Fatalf("AdminSetAccessByID: %v", err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := dbtest.New(t)
			log := &callLog{}
			svc, uid := newOrderEnv(t, db, log, email, password)
			log.reset()
			tc.run(t, svc, db, uid)
			want := tc.want
			if want == nil {
				want = []string{"GetByID", "Save"}
			}
			assertLockedFirst(t, log.calls, want...)
		})
	}

	t.Run("login (session mint)", func(t *testing.T) {
		db := dbtest.New(t)
		log := &callLog{}
		svc, _ := newOrderEnv(t, db, log, email, password)
		log.reset()
		if _, err := svc.Login(context.Background(), model.LoginRequest{Username: email, Password: password}, "test-agent", time.Now()); err != nil {
			t.Fatalf("Login: %v", err)
		}
		assertLockedFirst(t, log.calls, "InsertIfGeneration")
	})

	t.Run("create-personal-token", func(t *testing.T) {
		db := dbtest.New(t)
		log := &callLog{}
		svc, uid := newOrderEnv(t, db, log, email, password)
		tokens := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
		exp := time.Now().Add(24 * time.Hour)
		presenting := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_lock-order", &exp)
		log.reset()
		if _, err := svc.CreatePersonalToken(context.Background(), uid, presenting, model.CreatePersonalTokenRequest{Name: "ci"}); err != nil {
			t.Fatalf("CreatePersonalToken: %v", err)
		}
		assertLockedFirst(t, log.calls, "InsertIfPresenterLive")
	})
}

// newOrderEnv builds a Service whose repo and token store record their calls,
// plus one active user to drive them against.
func newOrderEnv(t *testing.T, db *dbtest.DB, log *callLog, email, password string) (*appuser.Service, vo.Id) {
	t.Helper()
	repo := &orderRepo{Repository: userrepo.NewRepo(db.Engine, db.TX), log: log}
	svc, _, _ := newUserSvcWithPorts(t, db, repo, func(tok appuser.AccessTokens) appuser.AccessTokens {
		return &orderTokens{AccessTokens: tok, log: log}
	}, func(reqs appuser.PasswordRequests) appuser.PasswordRequests {
		return &orderRequests{PasswordRequests: reqs, log: log}
	})
	uid, err := svc.AdminCreateUser(context.Background(), "Lock Order", email, password)
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	return svc, uid
}

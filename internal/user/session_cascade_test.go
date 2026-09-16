package user_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// cascadeEnv: a user with two live sessions and one live PAT.
type cascadeEnv struct {
	db       *dbtest.DB
	svc      *appuser.Service
	tokens   *userrepo.AccessTokenRepo
	uid      vo.Id
	sessionA vo.Id
	sessionB vo.Id
	pat      vo.Id
}

func newCascadeEnv(t *testing.T) *cascadeEnv {
	t.Helper()
	db := dbtest.New(t)
	svc, tokens, _, uid := newAuthEnvOn(t, db)
	exp := authT0.Add(appuser.SessionTTL)
	patExp := authT0.Add(90 * 24 * time.Hour)
	return &cascadeEnv{
		db: db, svc: svc, tokens: tokens, uid: uid,
		sessionA: seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_cascade-a", &exp),
		sessionB: seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_cascade-b", &exp),
		pat:      seedToken(t, tokens, uid, model.TokenKindPersonal, "eco_pat_cascade", &patExp),
	}
}

// liveness returns whether each of the three seeded tokens is currently live.
func (e *cascadeEnv) liveness(t *testing.T, now time.Time) (a, b, pat bool) {
	t.Helper()
	ctx := context.Background()
	check := func(id vo.Id) bool {
		tok, err := e.tokens.GetByID(ctx, id)
		if err != nil {
			t.Fatalf("GetByID(%s): %v", id, err)
		}
		return tok.IsLive(now)
	}
	return check(e.sessionA), check(e.sessionB), check(e.pat)
}

func TestUpdatePassword_RevokesOtherSessionsKeepsCurrentAndPATs(t *testing.T) {
	e := newCascadeEnv(t)
	ctx := context.Background()

	_, err := e.svc.UpdatePassword(ctx, e.uid, e.sessionA, model.UpdatePasswordRequest{
		OldPassword: "secretpass", NewPassword: "next-secret",
	})
	if err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	a, b, pat := e.liveness(t, authT0.Add(time.Minute))
	if !a {
		t.Error("current session must survive a password change")
	}
	if b {
		t.Error("other session must be revoked on password change")
	}
	if !pat {
		t.Error("PATs must survive a password change")
	}
}

// A rotation is a credential change, so everything issued under the old
// password goes with it: the pending grants that let a holder sign in or move
// the login key without knowing the new one, and every other session. What the
// owner is still holding stays — this session and the personal tokens.
func TestUpdatePassword_SweepsPendingGrantsAndBumpsTheGeneration(t *testing.T) {
	db := dbtest.New(t)
	svc, tokens, _, uid, pwreqs := newAuthEnvFullOn(t, db)
	users := userrepo.NewRepo(db.Engine, db.TX)
	changes := userrepo.NewEmailChangeRequestRepo(db.Engine, db.TX)
	ctx := context.Background()

	exp := authT0.Add(appuser.SessionTTL)
	patExp := authT0.Add(90 * 24 * time.Hour)
	current := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_rotate-current", &exp)
	other := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_rotate-other", &exp)
	pat := seedToken(t, tokens, uid, model.TokenKindPersonal, "eco_pat_rotate", &patExp)

	resetCode := appuser.HashResetCode("482913")
	if err := pwreqs.Save(ctx, model.NewPasswordRequest(vo.NewId(), uid, resetCode, authT0)); err != nil {
		t.Fatalf("seed password request: %v", err)
	}
	cr := model.NewEmailChangeRequest(vo.NewId(), uid, "rotated-new@econumo.test", appuser.HashResetCode("135790"), authT0)
	if n, err := changes.Save(ctx, cr, 0); err != nil || n != 1 {
		t.Fatalf("seed email change request: %d %v", n, err)
	}

	before, err := users.GetByID(ctx, uid)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if _, err := svc.UpdatePassword(ctx, uid, current, model.UpdatePasswordRequest{
		OldPassword: "secretpass", NewPassword: "next-secret",
	}); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}

	after, err := users.GetByID(ctx, uid)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if after.CredentialsGeneration != before.CredentialsGeneration+1 {
		t.Errorf("credentials generation %d -> %d, want +1", before.CredentialsGeneration, after.CredentialsGeneration)
	}
	if _, err := pwreqs.GetByUserAndCode(ctx, uid, resetCode); err == nil {
		t.Error("an outstanding reset code must not outlive a password change")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("GetByUserAndCode: %v", err)
	}
	if _, err := changes.GetByUser(ctx, uid); err == nil {
		t.Error("a pending email change must not outlive a password change")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("GetByUser: %v", err)
	}

	now := authT0.Add(time.Minute)
	for _, tc := range []struct {
		id       vo.Id
		name     string
		wantLive bool
	}{
		{current, "the presenting session", true},
		{other, "the other session", false},
		{pat, "the personal token", true},
	} {
		tok, gerr := tokens.GetByID(ctx, tc.id)
		if gerr != nil {
			t.Fatalf("GetByID(%s): %v", tc.name, gerr)
		}
		if tok.IsLive(now) != tc.wantLive {
			t.Errorf("%s live = %v, want %v", tc.name, !tc.wantLive, tc.wantLive)
		}
	}
}

const (
	rotationOldPassword = "secretpass"
	rotationNewPassword = "rotated-secret"
)

// rotatingRepo lands a completed update-password in the window between Login's
// evidence read and its session insert, the same race resettingRepo exercises
// for a reset. Armed once, like a single rotation committing.
type rotatingRepo struct {
	appuser.Repository
	svc   *appuser.Service
	armed bool
}

func (r *rotatingRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	u, err := r.Repository.GetByEmail(ctx, email)
	if err != nil || !r.armed {
		return u, err
	}
	r.armed = false
	_, uerr := r.svc.UpdatePassword(ctx, u.ID, vo.Id{}, model.UpdatePasswordRequest{
		OldPassword: rotationOldPassword, NewPassword: rotationNewPassword,
	})
	return u, uerr
}

// Both orders of the same pair end the same way: a sign-in presenting the OLD
// password never becomes a live session. The sequential order is settled by the
// stored hash; the interleaved one — the login read its row before the rotation
// committed — is why the rotation bumps the generation inside its transaction.
func TestLogin_WithThePasswordAnUpdateReplacesMintsNothing(t *testing.T) {
	const email = "rotated-login@econumo.test"
	for _, tc := range []struct {
		name        string
		interleaved bool
	}{
		{name: "the rotation commits after the login's evidence read", interleaved: true},
		{name: "the rotation commits before the login starts", interleaved: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := dbtest.New(t)
			repo := &rotatingRepo{Repository: userrepo.NewRepo(db.Engine, db.TX)}
			svc, _, _ := newUserSvcWithRepo(t, db, repo)
			repo.svc = svc
			ctx := context.Background()

			id, err := svc.AdminCreateUser(ctx, "Rotated", email, rotationOldPassword)
			if err != nil {
				t.Fatalf("AdminCreateUser: %v", err)
			}
			if tc.interleaved {
				repo.armed = true
			} else if _, err := svc.UpdatePassword(ctx, id, vo.Id{}, model.UpdatePasswordRequest{
				OldPassword: rotationOldPassword, NewPassword: rotationNewPassword,
			}); err != nil {
				t.Fatalf("UpdatePassword: %v", err)
			}

			_, err = svc.Login(ctx, model.LoginRequest{Username: email, Password: rotationOldPassword}, "test-agent", time.Now())
			var unauthorized *errs.UnauthorizedError
			if !errors.As(err, &unauthorized) || unauthorized.Msg != "Invalid credentials." {
				t.Fatalf("Login err = %v, want *errs.UnauthorizedError %q", err, "Invalid credentials.")
			}
			var n int
			if err := db.Raw.QueryRowContext(ctx, db.Rebind(
				"SELECT COUNT(*) FROM access_tokens WHERE user_id = ? AND kind = ? AND revoked_at IS NULL"),
				id.String(), model.TokenKindSession).Scan(&n); err != nil {
				t.Fatalf("count live sessions: %v", err)
			}
			if n != 0 {
				t.Fatalf("the rotation was outrun: %d live session rows", n)
			}
		})
	}
}

// The operator's user:change-password is the account reclaim too: it is what an
// admin runs to evict whoever holds the account, so nothing that never proved
// the mailbox may outlive it — not a personal token, not an in-flight sign-in,
// and not a linked identity vouching for someone else's address.
func TestAdminChangePassword_IsAFullReclaim(t *testing.T) {
	db := dbtest.New(t)
	svc, tokens, _, uid := newAuthEnvOn(t, db)
	users := userrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()
	exp := authT0.Add(appuser.SessionTTL)
	patExp := authT0.Add(90 * 24 * time.Hour)
	ses := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_admin-reclaim", &exp)
	pat := seedToken(t, tokens, uid, model.TokenKindPersonal, "eco_pat_admin-reclaim", &patExp)
	reclaimer := &fakeReclaimer{}
	svc.SetOAuthReclaimer(reclaimer)

	before, err := users.GetByID(ctx, uid)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if err := svc.AdminChangePassword(ctx, "auth@econumo.test", "operator-set-pw-1"); err != nil {
		t.Fatalf("AdminChangePassword: %v", err)
	}

	now := authT0.Add(time.Minute)
	for _, tc := range []struct {
		id   vo.Id
		name string
	}{{ses, "session"}, {pat, "personal token"}} {
		tok, gerr := tokens.GetByID(ctx, tc.id)
		if gerr != nil {
			t.Fatalf("GetByID(%s): %v", tc.name, gerr)
		}
		if tok.IsLive(now) {
			t.Errorf("%s must be revoked by the operator's password change", tc.name)
		}
	}
	after, err := users.GetByID(ctx, uid)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if after.CredentialsGeneration != before.CredentialsGeneration+1 {
		t.Errorf("credentials generation %d -> %d, want +1", before.CredentialsGeneration, after.CredentialsGeneration)
	}
	if len(reclaimer.calls) != 1 || reclaimer.calls[0].userID != uid.String() || reclaimer.calls[0].email != "auth@econumo.test" {
		t.Fatalf("the account's own address must drive the identity reclaim: %+v", reclaimer.calls)
	}
}

func TestAdminDeactivate_RevokesEverything(t *testing.T) {
	e := newCascadeEnv(t)
	ctx := context.Background()
	users := userrepo.NewRepo(e.db.Engine, e.db.TX)
	before, err := users.GetByID(ctx, e.uid)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if err := e.svc.AdminDeactivate(ctx, "auth@econumo.test"); err != nil {
		t.Fatalf("AdminDeactivate: %v", err)
	}
	a, b, pat := e.liveness(t, authT0.Add(time.Minute))
	if a || b || pat {
		t.Errorf("deactivate must revoke everything (a=%v b=%v pat=%v)", a, b, pat)
	}
	after, err := users.GetByID(ctx, e.uid)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if after.CredentialsGeneration != before.CredentialsGeneration+1 {
		t.Errorf("credentials generation %d -> %d, want +1", before.CredentialsGeneration, after.CredentialsGeneration)
	}
}

// deactivatingRepo lands a deactivation in the window between Login's evidence
// read and its session insert, the same race resettingRepo exercises for a
// password reset: AdminDeactivate is the last credential-eviction path that
// used to leave that window unfenced.
type deactivatingRepo struct {
	appuser.Repository
	svc   *appuser.Service
	armed bool
}

func (r *deactivatingRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	u, err := r.Repository.GetByEmail(ctx, email)
	if err != nil || !r.armed {
		return u, err
	}
	r.armed = false
	return u, r.svc.AdminDeactivate(ctx, email)
}

// Login must present the generation that came out of the SAME row read as the
// password hash it verified. A deactivation landing after that read, inside
// its own transaction, must bump the generation so the guarded session insert
// still fails closed — a deactivated user must never end up with a live
// session, even one that raced the deactivation.
func TestLogin_DeactivateBetweenTheEvidenceReadAndTheSessionInsertMintsNothing(t *testing.T) {
	db := dbtest.New(t)
	repo := &deactivatingRepo{Repository: userrepo.NewRepo(db.Engine, db.TX)}
	svc, _, _ := newUserSvcWithRepo(t, db, repo)
	repo.svc = svc
	ctx := context.Background()

	id, err := svc.AdminCreateUser(ctx, "Raced Deactivation", "raced-deactivate@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	repo.armed = true

	_, err = svc.Login(ctx, model.LoginRequest{Username: "raced-deactivate@econumo.test", Password: "secretpass"}, "test-agent", time.Now())
	var unauthorized *errs.UnauthorizedError
	if !errors.As(err, &unauthorized) || unauthorized.Msg != "Invalid credentials." {
		t.Fatalf("Login err = %v, want *errs.UnauthorizedError %q", err, "Invalid credentials.")
	}
	var n int
	if err := db.Raw.QueryRowContext(ctx, db.Rebind(
		"SELECT COUNT(*) FROM access_tokens WHERE user_id = ? AND kind = ? AND revoked_at IS NULL"), id.String(), model.TokenKindSession).Scan(&n); err != nil {
		t.Fatalf("count live sessions: %v", err)
	}
	if n != 0 {
		t.Fatalf("the deactivation was outrun: %d live session rows", n)
	}
}

// A reset is the account's ownership proof, so it is also its reclaim: unlike
// update-password, it takes the personal tokens too. An attacker who had
// registered someone else's address would otherwise keep API access after the
// rightful owner reset the password.
func TestResetPassword_RevokesEverySessionAndToken(t *testing.T) {
	svc, tokens, _, uid, pwreqs := newAuthEnvFull(t)
	ctx := context.Background()
	exp := authT0.Add(appuser.SessionTTL)
	patExp := authT0.Add(90 * 24 * time.Hour)
	sesA := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_reset-a", &exp)
	sesB := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_reset-b", &exp)
	pat := seedToken(t, tokens, uid, model.TokenKindPersonal, "eco_pat_reset", &patExp)

	// Seed a valid reset code directly (the remind flow's persistence shape): the
	// stored value is the hash, the plaintext is what the reset request submits.
	pr := &model.PasswordRequest{
		ID: vo.NewId(), UserID: uid, Code: appuser.HashResetCode("482913"),
		CreatedAt: authT0, UpdatedAt: authT0, ExpiredAt: authT0.Add(10 * time.Minute),
	}
	if err := pwreqs.Save(ctx, pr); err != nil {
		t.Fatalf("seed password request: %v", err)
	}

	reclaimer := &fakeReclaimer{}
	svc.SetOAuthReclaimer(reclaimer)

	_, err := svc.ResetPassword(ctx, model.ResetPasswordRequest{
		Username: "auth@econumo.test", Code: "482913", Password: "next-secret",
	})
	if err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	now := authT0.Add(time.Minute)
	for _, tc := range []struct {
		id   vo.Id
		name string
	}{{sesA, "session a"}, {sesB, "session b"}, {pat, "pat"}} {
		tok, gerr := tokens.GetByID(ctx, tc.id)
		if gerr != nil {
			t.Fatalf("GetByID(%s): %v", tc.name, gerr)
		}
		if tok.IsLive(now) {
			t.Errorf("%s must be revoked by a reset", tc.name)
		}
	}
	if len(reclaimer.calls) != 1 || reclaimer.calls[0].userID != uid.String() || reclaimer.calls[0].email != "auth@econumo.test" {
		t.Fatalf("the proven address must drive the identity reclaim: %+v", reclaimer.calls)
	}
}

// fakeReclaimer records the oauth-side unlink the reset cascade delegates.
type fakeReclaimer struct {
	calls []struct{ userID, email string }
	fail  error
}

func (f *fakeReclaimer) ReclaimAccount(_ context.Context, userID vo.Id, provenEmail string) (int64, int64, error) {
	f.calls = append(f.calls, struct{ userID, email string }{userID.String(), provenEmail})
	return 0, 0, f.fail
}

// The reclaim shares the password write's transaction: a failure to drop a
// foreign sign-in method must not leave a reset that only changed the password.
func TestResetPassword_RollsBackWhenTheIdentityReclaimFails(t *testing.T) {
	svc, _, _, uid, pwreqs := newAuthEnvFull(t)
	ctx := context.Background()
	pr := &model.PasswordRequest{
		ID: vo.NewId(), UserID: uid, Code: appuser.HashResetCode("482913"),
		CreatedAt: authT0, UpdatedAt: authT0, ExpiredAt: authT0.Add(10 * time.Minute),
	}
	if err := pwreqs.Save(ctx, pr); err != nil {
		t.Fatalf("seed password request: %v", err)
	}
	svc.SetOAuthReclaimer(&fakeReclaimer{fail: errors.New("boom")})

	if _, err := svc.ResetPassword(ctx, model.ResetPasswordRequest{
		Username: "auth@econumo.test", Code: "482913", Password: "next-secret",
	}); err == nil {
		t.Fatal("a failed reclaim must fail the reset")
	}
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "auth@econumo.test", Password: "next-secret"}, "ua", authT0); err == nil {
		t.Fatal("the password write must have rolled back with it")
	}
}

func TestPurgeDeadTokens(t *testing.T) {
	svc, tokens, clk, uid := newAuthEnv(t)
	ctx := context.Background()

	liveExp := authT0.Add(appuser.SessionTTL)
	oldExp := authT0.Add(-40 * 24 * time.Hour)
	liveID := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_purge-live", &liveExp)
	deadID := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_purge-dead", &oldExp)

	clk.now = authT0
	n, err := svc.PurgeDeadTokens(ctx, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("PurgeDeadTokens: %v", err)
	}
	if n != 1 {
		t.Errorf("purged = %d, want 1", n)
	}
	if _, err := tokens.GetByID(ctx, liveID); err != nil {
		t.Error("live session must survive the purge")
	}
	if _, err := tokens.GetByID(ctx, deadID); err == nil {
		t.Error("dead session must be purged")
	}

	if _, err := svc.PurgeDeadTokens(ctx, -time.Hour); err == nil {
		t.Error("negative retention must be rejected")
	}
}

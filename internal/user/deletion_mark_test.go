package user_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

type deletionEnv struct {
	db       *dbtest.DB
	svc      *appuser.Service
	tokens   *userrepo.AccessTokenRepo
	pwreqs   *userrepo.PasswordRequestRepo
	clk      *testClock
	uid      vo.Id
	users    *userrepo.Repo
	store    *userrepo.DeletionRepo
	audit    *userrepo.AuditRepo
	requests *userrepo.DeletionRequestRepo
}

func newDeletionEnv(t *testing.T) *deletionEnv {
	t.Helper()
	db := dbtest.New(t)
	svc, tokens, clk, uid, pwreqs := newAuthEnvFullOn(t, db)
	e := &deletionEnv{
		db: db, svc: svc, tokens: tokens, pwreqs: pwreqs, clk: clk, uid: uid,
		users:    userrepo.NewRepo(db.Engine, db.TX),
		store:    userrepo.NewDeletionRepo(db.Engine, db.TX),
		audit:    userrepo.NewAuditRepo(db.Engine, db.TX),
		requests: userrepo.NewDeletionRequestRepo(db.Engine, db.TX),
	}
	svc.SetDeletion(e.store, e.audit, e.requests, appuser.DeletionConfig{InactiveDays: 365, DeletionDays: 30})
	return e
}

func (e *deletionEnv) state(t *testing.T) model.DeletionState {
	t.Helper()
	st, err := e.store.GetState(context.Background(), e.uid)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	return st
}

func (e *deletionEnv) generation(t *testing.T) int64 {
	t.Helper()
	u, err := e.users.GetByID(context.Background(), e.uid)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	return u.CredentialsGeneration
}

func (e *deletionEnv) auditRows(t *testing.T, event string) []model.UserAuditEvent {
	t.Helper()
	rows, err := e.audit.ListByEvent(context.Background(), event, time.Time{}, "", 100)
	if err != nil {
		t.Fatalf("ListByEvent: %v", err)
	}
	return rows
}

func (e *deletionEnv) setLastActive(t *testing.T, at time.Time) {
	t.Helper()
	e.db.Exec(t, e.db.Rebind(`UPDATE users SET last_active_at = ? WHERE id = ?`), at, e.uid.String())
}

func (e *deletionEnv) seedDeletionRequest(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	u, err := e.users.GetByID(ctx, e.uid)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	r := model.NewDeletionRequest(vo.NewId(), e.uid, u.Email, "code-hash", e.clk.now)
	if n, err := e.requests.Save(ctx, r, u.CredentialsGeneration); err != nil || n != 1 {
		t.Fatalf("seed deletion request: %d %v", n, err)
	}
}

func (e *deletionEnv) hasDeletionRequest(t *testing.T) bool {
	t.Helper()
	_, err := e.requests.GetByUser(context.Background(), e.uid)
	if err == nil {
		return true
	}
	if _, ok := errs.AsNotFound(err); ok {
		return false
	}
	t.Fatalf("GetByUser: %v", err)
	return false
}

func TestMark_RevokesEverything(t *testing.T) {
	e := newDeletionEnv(t)
	ctx := context.Background()
	rev := &fakeGrantRevoker{}
	e.svc.SetMCPGrantRevoker(rev)
	exp := authT0.Add(appuser.SessionTTL)
	seedToken(t, e.tokens, e.uid, model.TokenKindSession, "eco_ses_mark-session", &exp)
	seedToken(t, e.tokens, e.uid, model.TokenKindPersonal, "eco_pat_mark-pat", nil)
	oauthRaw := issueOAuth(t, e.svc, e.uid, vo.NewId())
	raws := []string{"eco_ses_mark-session", "eco_pat_mark-pat", oauthRaw}
	for _, raw := range raws {
		if _, err := e.svc.Authenticate(ctx, raw); err != nil {
			t.Fatalf("before the mark %q must authenticate: %v", raw[:8], err)
		}
	}
	e.seedDeletionRequest(t)
	gen := e.generation(t)

	if err := e.svc.AdminMarkDeleted(ctx, oauthTestEmail); err != nil {
		t.Fatalf("AdminMarkDeleted: %v", err)
	}

	for _, raw := range raws {
		_, err := e.svc.Authenticate(ctx, raw)
		if !isUnauthorized(err) || !strings.Contains(err.Error(), "Invalid access token") {
			t.Errorf("after the mark %q: want Invalid access token, got %v", raw[:8], err)
		}
	}
	if got := e.generation(t); got != gen+1 {
		t.Errorf("credentials generation = %d, want %d", got, gen+1)
	}
	st := e.state(t)
	if st.MarkedAt == nil || st.Reason != model.DeletionReasonCLI {
		t.Fatalf("state = %+v, want marked by cli", st)
	}
	if len(rev.users) != 1 || !rev.users[0].Equal(e.uid) {
		t.Errorf("MCP grants revoked for %v, want the user", rev.users)
	}
	if e.hasDeletionRequest(t) {
		t.Error("the pending deletion code must go with the mark")
	}
	rows := e.auditRows(t, model.AuditDeletionMarked)
	if len(rows) != 1 {
		t.Fatalf("deletion_marked rows = %d, want 1", len(rows))
	}
	if !rows[0].UserID.Equal(e.uid) || rows[0].Details != `{"reason":"cli"}` || rows[0].EmailHash != nil {
		t.Errorf("audit row = %+v", rows[0])
	}
}

func TestMark_Twice(t *testing.T) {
	e := newDeletionEnv(t)
	ctx := context.Background()
	if err := e.svc.AdminMarkDeleted(ctx, oauthTestEmail); err != nil {
		t.Fatalf("first mark: %v", err)
	}
	err := e.svc.AdminMarkDeleted(ctx, oauthTestEmail)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.MsgCode != errs.CodeUserDeletionAlreadyMarked {
		t.Fatalf("second mark = %v, want %s", err, errs.CodeUserDeletionAlreadyMarked)
	}
	if n := len(e.auditRows(t, model.AuditDeletionMarked)); n != 1 {
		t.Errorf("deletion_marked rows = %d, want 1", n)
	}
}

func TestLogin_ClearsTheMark(t *testing.T) {
	e := newDeletionEnv(t)
	ctx := context.Background()
	if err := e.svc.AdminMarkDeleted(ctx, oauthTestEmail); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if _, err := e.svc.Login(ctx, model.LoginRequest{Username: oauthTestEmail, Password: "secretpass"}, "ua", e.clk.now); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if st := e.state(t); st.MarkedAt != nil {
		t.Fatalf("state after login = %+v, want unmarked", st)
	}
	rows := e.auditRows(t, model.AuditDeletionCancelled)
	if len(rows) != 1 {
		t.Fatalf("deletion_cancelled rows = %d, want 1", len(rows))
	}
	if !rows[0].UserID.Equal(e.uid) || rows[0].Details != `{"reason":"cli","cause":"sign_in"}` || rows[0].EmailHash != nil {
		t.Errorf("audit row = %+v", rows[0])
	}

	if _, err := e.svc.Login(ctx, model.LoginRequest{Username: oauthTestEmail, Password: "secretpass"}, "ua", e.clk.now); err != nil {
		t.Fatalf("second Login: %v", err)
	}
	if n := len(e.auditRows(t, model.AuditDeletionCancelled)); n != 1 {
		t.Errorf("a login of an unmarked user wrote a cancel row: %d rows", n)
	}
}

func TestLogin_StartedBeforeTheMarkIsRefused(t *testing.T) {
	e := newDeletionEnv(t)
	ctx := context.Background()
	gen := e.generation(t)
	if err := e.svc.AdminMarkDeleted(ctx, oauthTestEmail); err != nil {
		t.Fatalf("mark: %v", err)
	}
	_, err := e.svc.CreateSessionForTest(ctx, e.uid, "ua", "", nil, e.clk.now, gen)
	if !isUnauthorized(err) {
		t.Fatalf("createSession with the pre-mark generation = %v, want unauthorized", err)
	}
	if st := e.state(t); st.MarkedAt == nil {
		t.Fatal("a refused session must leave the mark")
	}
	if n := len(e.auditRows(t, model.AuditDeletionCancelled)); n != 0 {
		t.Errorf("deletion_cancelled rows = %d, want 0", n)
	}
}

func TestActivity_SessionAndRefreshStamp(t *testing.T) {
	e := newDeletionEnv(t)
	ctx := context.Background()

	e.setLastActive(t, e.clk.now.Add(-72*time.Hour))
	if _, err := e.svc.Login(ctx, model.LoginRequest{Username: oauthTestEmail, Password: "secretpass"}, "ua", e.clk.now); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if st := e.state(t); st.LastActiveAt == nil || !st.LastActiveAt.Equal(e.clk.now) {
		t.Fatalf("after login last_active_at = %v, want %v", st.LastActiveAt, e.clk.now)
	}

	e.setLastActive(t, e.clk.now.Add(-72*time.Hour))
	issueOAuth(t, e.svc, e.uid, vo.NewId())
	if st := e.state(t); st.LastActiveAt == nil || !st.LastActiveAt.Equal(e.clk.now) {
		t.Fatalf("after oauth issue last_active_at = %v, want %v", st.LastActiveAt, e.clk.now)
	}
}

func TestActivity_TouchIsDaily(t *testing.T) {
	e := newDeletionEnv(t)
	ctx := context.Background()
	exp := authT0.Add(appuser.SessionTTL)
	seedToken(t, e.tokens, e.uid, model.TokenKindSession, "eco_ses_daily", &exp)

	e.clk.now = authT0.Add(10 * time.Minute)
	recent := e.clk.now.Add(-2 * time.Hour)
	e.setLastActive(t, recent)
	if _, err := e.svc.Authenticate(ctx, "eco_ses_daily"); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if st := e.state(t); st.LastActiveAt == nil || !st.LastActiveAt.Equal(recent) {
		t.Fatalf("a 2h-old stamp moved to %v", st.LastActiveAt)
	}

	// The first call touched the token; 10 more minutes make it due again.
	e.clk.now = e.clk.now.Add(10 * time.Minute)
	e.setLastActive(t, e.clk.now.Add(-25*time.Hour))
	if _, err := e.svc.Authenticate(ctx, "eco_ses_daily"); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if st := e.state(t); st.LastActiveAt == nil || !st.LastActiveAt.Equal(e.clk.now) {
		t.Fatalf("a 25h-old stamp = %v, want %v", st.LastActiveAt, e.clk.now)
	}
}

func TestRegister_StampsNewUser(t *testing.T) {
	db := dbtest.New(t)
	svc, repo, _ := newTrialSvc(t, db, 0)
	store := userrepo.NewDeletionRepo(db.Engine, db.TX)
	svc.SetDeletion(store, userrepo.NewAuditRepo(db.Engine, db.TX), userrepo.NewDeletionRequestRepo(db.Engine, db.TX), appuser.DeletionConfig{})
	ctx := context.Background()
	if _, err := svc.Register(ctx, model.RegisterRequest{Name: "Stamped", Email: "stamped@econumo.test", Password: "secretpass"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	u, err := repo.GetByEmail(ctx, "stamped@econumo.test")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	st, err := store.GetState(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.LastActiveAt == nil {
		t.Fatal("a new user must start with an activity stamp")
	}
}

func TestDeletionCode_DiesWithTheCredential(t *testing.T) {
	e := newDeletionEnv(t)
	ctx := context.Background()
	exp := authT0.Add(appuser.SessionTTL)
	sess := seedToken(t, e.tokens, e.uid, model.TokenKindSession, "eco_ses_code-dies", &exp)

	e.seedDeletionRequest(t)
	if _, err := e.svc.UpdatePassword(ctx, e.uid, sess, model.UpdatePasswordRequest{
		OldPassword: "secretpass", NewPassword: "second-secret",
	}); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	if e.hasDeletionRequest(t) {
		t.Error("update-password must drop the pending deletion code")
	}

	e.seedDeletionRequest(t)
	resetChangeEmailPassword(t, e.svc, e.pwreqs, e.uid, oauthTestEmail, "third-secret")
	if e.hasDeletionRequest(t) {
		t.Error("reset-password must drop the pending deletion code")
	}

	e.seedDeletionRequest(t)
	if err := e.svc.AdminChangePassword(ctx, oauthTestEmail, "fourth-secret"); err != nil {
		t.Fatalf("AdminChangePassword: %v", err)
	}
	if e.hasDeletionRequest(t) {
		t.Error("user:change-password must drop the pending deletion code")
	}
}

package user_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/mailer"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

const regrantEmail = "anna@example.test"

type regrantEnv struct {
	svc    *appuser.Service
	users  *userrepo.Repo
	audit  *userrepo.AuditRepo
	pwreqs *userrepo.PasswordRequestRepo
	cap    *captureMailer
	clk    *testClock
}

func newRegrantEnv(t *testing.T, emailVerification bool) *regrantEnv {
	t.Helper()
	db := dbtest.New(t)
	clk := &testClock{now: authT0}
	cap := &captureMailer{}
	users := userrepo.NewRepo(db.Engine, db.TX)
	pwreqs := userrepo.NewPasswordRequestRepo(db.Engine, db.TX)
	svc := appuser.NewService(users, db.TX, auth.NewEncodeService(""), auth.NewPasswordHasher(),
		userrepo.NewAccessTokenRepo(db.Engine, db.TX),
		server.NewUserCurrencyLookup(currencyrepo.New(db.Engine, db.TX)),
		server.NewUserBudgetAccess(db.Engine, db.TX),
		pwreqs, mailer.NewResetSender(cap, "noreply@econumo.test", ""),
		userrepo.NewEmailVerificationRepo(db.Engine, db.TX), mailer.NewVerifySender(cap, "noreply@econumo.test", ""),
		userrepo.NewEmailChangeRequestRepo(db.Engine, db.TX), mailer.NewChangeEmailSender(cap, "noreply@econumo.test", ""),
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clk, nil, true, 14, emailVerification)
	audit := userrepo.NewAuditRepo(db.Engine, db.TX)
	svc.SetDeletion(userrepo.NewDeletionRepo(db.Engine, db.TX), audit, userrepo.NewDeletionRequestRepo(db.Engine, db.TX),
		appuser.DeletionConfig{InactiveDays: 365, DeletionDays: 30})
	return &regrantEnv{svc: svc, users: users, audit: audit, pwreqs: pwreqs, cap: cap, clk: clk}
}

// seedDeleted writes the deleted row a wipe leaves behind. The hash is set
// only for a lifetime account, as the wipe does.
func (e *regrantEnv) seedDeleted(t *testing.T, email string, lifetime bool) vo.Id {
	t.Helper()
	details, _ := json.Marshal(map[string]any{"reason": "self_initiated", "lifetimeAccess": lifetime})
	ev := model.UserAuditEvent{
		ID: vo.NewId(), UserID: vo.NewId(), Event: model.AuditDeleted,
		Details: string(details), CreatedAt: e.clk.now,
	}
	if lifetime {
		h := model.EmailHash(email)
		ev.EmailHash = &h
	}
	if ok, err := e.audit.Insert(context.Background(), ev); err != nil || !ok {
		t.Fatalf("seed deleted row: %v %v", ok, err)
	}
	return ev.ID
}

func (e *regrantEnv) regrants(t *testing.T) []model.UserAuditEvent {
	t.Helper()
	rows, err := e.audit.ListByEvent(context.Background(), model.AuditLifetimeRegranted, time.Time{}, "", 100)
	if err != nil {
		t.Fatalf("ListByEvent: %v", err)
	}
	return rows
}

func (e *regrantEnv) user(t *testing.T, id vo.Id) *model.User {
	t.Helper()
	u, err := e.users.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	return u
}

func (e *regrantEnv) userByEmail(t *testing.T, email string) *model.User {
	t.Helper()
	u, err := e.users.GetByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	return u
}

func (e *regrantEnv) moveAway(t *testing.T, id vo.Id, email string) {
	t.Helper()
	if err := e.users.SetEmail(context.Background(), id, email); err != nil {
		t.Fatalf("SetEmail: %v", err)
	}
}

func (e *regrantEnv) register(t *testing.T, email string) *model.User {
	t.Helper()
	if _, err := e.svc.Register(context.Background(), model.RegisterRequest{Name: "Anna", Email: email, Password: "secretpass1"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return e.userByEmail(t, email)
}

func (e *regrantEnv) resetPassword(t *testing.T, uid vo.Id, email string) {
	t.Helper()
	pr := &model.PasswordRequest{
		ID: vo.NewId(), UserID: uid, Code: appuser.HashResetCode("482913"),
		CreatedAt: e.clk.now, UpdatedAt: e.clk.now, ExpiredAt: e.clk.now.Add(10 * time.Minute),
	}
	if err := e.pwreqs.Save(context.Background(), pr); err != nil {
		t.Fatalf("seed password request: %v", err)
	}
	if _, err := e.svc.ResetPassword(context.Background(), model.ResetPasswordRequest{
		Username: email, Code: "482913", Password: "newsecret1",
	}); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
}

func (e *regrantEnv) trialEnd() time.Time { return model.TrialEnd(e.clk.now, 14) }

func assertLifetime(t *testing.T, u *model.User) {
	t.Helper()
	if u.AccessLevel != model.AccessLevelFull || u.AccessUntil != nil {
		t.Fatalf("access = %s until %v, want full with no end", u.AccessLevel, u.AccessUntil)
	}
}

func (e *regrantEnv) assertTrial(t *testing.T, u *model.User) {
	t.Helper()
	if u.AccessLevel != model.AccessLevelFull || u.AccessUntil == nil || !u.AccessUntil.Equal(e.trialEnd()) {
		t.Fatalf("access = %s until %v, want the trial ending %v", u.AccessLevel, u.AccessUntil, e.trialEnd())
	}
}

func assertRegrantRow(t *testing.T, row model.UserAuditEvent, userID, deletedID vo.Id) {
	t.Helper()
	if !row.UserID.Equal(userID) {
		t.Errorf("regrant user = %s, want %s", row.UserID, userID)
	}
	if row.RefID == nil || !row.RefID.Equal(deletedID) {
		t.Errorf("regrant ref = %v, want %s", row.RefID, deletedID)
	}
	if row.EmailHash == nil || *row.EmailHash != model.EmailHash(regrantEmail) {
		t.Errorf("regrant email hash = %v", row.EmailHash)
	}
	var d struct {
		FromUserID     string `json:"fromUserId"`
		DeletedEventID string `json:"deletedEventId"`
	}
	if err := json.Unmarshal([]byte(row.Details), &d); err != nil {
		t.Fatalf("details %q: %v", row.Details, err)
	}
	if d.DeletedEventID != deletedID.String() || d.FromUserID == "" {
		t.Errorf("details = %s, want deletedEventId %s and a fromUserId", row.Details, deletedID)
	}
}

func TestRegrant_ProviderSignUp(t *testing.T) {
	e := newRegrantEnv(t, true)
	deletedID := e.seedDeleted(t, regrantEmail, true)
	u, err := e.svc.ProvisionExternalUser(context.Background(), "Anna", regrantEmail)
	if err != nil {
		t.Fatalf("ProvisionExternalUser: %v", err)
	}
	assertLifetime(t, e.user(t, u.ID))
	rows := e.regrants(t)
	if len(rows) != 1 {
		t.Fatalf("regrant rows = %d, want 1", len(rows))
	}
	assertRegrantRow(t, rows[0], u.ID, deletedID)
}

func TestRegrant_CLI(t *testing.T) {
	e := newRegrantEnv(t, true)
	deletedID := e.seedDeleted(t, regrantEmail, true)
	uid, err := e.svc.AdminCreateUser(context.Background(), "Anna", regrantEmail, "secretpass1")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	assertLifetime(t, e.user(t, uid))
	rows := e.regrants(t)
	if len(rows) != 1 {
		t.Fatalf("regrant rows = %d, want 1", len(rows))
	}
	assertRegrantRow(t, rows[0], uid, deletedID)
}

func TestRegrant_PasswordWithGate(t *testing.T) {
	e := newRegrantEnv(t, true)
	deletedID := e.seedDeleted(t, regrantEmail, true)
	u := e.register(t, regrantEmail)
	e.assertTrial(t, u)
	if len(e.regrants(t)) != 0 {
		t.Fatal("an unproven registration must not regrant")
	}
	if _, _, err := e.svc.ResendVerificationCode(context.Background(), model.ResendVerificationCodeRequest{Username: regrantEmail}); err != nil {
		t.Fatalf("ResendVerificationCode: %v", err)
	}
	if len(e.cap.msgs) != 1 {
		t.Fatalf("want 1 email, got %d", len(e.cap.msgs))
	}
	code := codeFrom(t, e.cap.msgs[0].Text)
	if _, err := e.svc.ConfirmEmail(context.Background(), model.ConfirmEmailRequest{Username: regrantEmail, Code: code}); err != nil {
		t.Fatalf("ConfirmEmail: %v", err)
	}
	assertLifetime(t, e.user(t, u.ID))
	rows := e.regrants(t)
	if len(rows) != 1 {
		t.Fatalf("regrant rows = %d, want 1", len(rows))
	}
	assertRegrantRow(t, rows[0], u.ID, deletedID)
}

func TestRegrant_PasswordWithoutGate(t *testing.T) {
	e := newRegrantEnv(t, false)
	deletedID := e.seedDeleted(t, regrantEmail, true)
	u := e.register(t, regrantEmail)
	e.assertTrial(t, u)
	if err := e.svc.AdminVerifyEmail(context.Background(), regrantEmail); err != nil {
		t.Fatalf("AdminVerifyEmail: %v", err)
	}
	assertLifetime(t, e.user(t, u.ID))
	rows := e.regrants(t)
	if len(rows) != 1 {
		t.Fatalf("regrant rows = %d, want 1", len(rows))
	}
	assertRegrantRow(t, rows[0], u.ID, deletedID)
}

func TestRegrant_OnlyOnce(t *testing.T) {
	e := newRegrantEnv(t, true)
	e.seedDeleted(t, regrantEmail, true)
	first, err := e.svc.ProvisionExternalUser(context.Background(), "Anna", regrantEmail)
	if err != nil {
		t.Fatalf("ProvisionExternalUser: %v", err)
	}
	assertLifetime(t, e.user(t, first.ID))
	e.moveAway(t, first.ID, "anna.moved@example.test")

	second, err := e.svc.ProvisionExternalUser(context.Background(), "Anna Again", regrantEmail)
	if err != nil {
		t.Fatalf("second ProvisionExternalUser: %v", err)
	}
	e.assertTrial(t, e.user(t, second.ID))
	if n := len(e.regrants(t)); n != 1 {
		t.Fatalf("regrant rows = %d, want 1", n)
	}
}

func TestRegrant_SameSecond(t *testing.T) {
	e := newRegrantEnv(t, true)
	e.seedDeleted(t, regrantEmail, true)
	first, err := e.svc.ProvisionExternalUser(context.Background(), "Anna", regrantEmail)
	if err != nil {
		t.Fatalf("ProvisionExternalUser: %v", err)
	}
	assertLifetime(t, e.user(t, first.ID))
	rows := e.regrants(t)
	if len(rows) != 1 || !rows[0].CreatedAt.Equal(e.clk.now) {
		t.Fatalf("regrant rows = %+v, want one at %v", rows, e.clk.now)
	}

	// The regranted account is deleted again in the same second.
	e.moveAway(t, first.ID, "anna.gone@example.test")
	secondDeleted := e.seedDeleted(t, regrantEmail, true)

	second, err := e.svc.ProvisionExternalUser(context.Background(), "Anna Again", regrantEmail)
	if err != nil {
		t.Fatalf("second ProvisionExternalUser: %v", err)
	}
	assertLifetime(t, e.user(t, second.ID))
	rows = e.regrants(t)
	if len(rows) != 2 {
		t.Fatalf("regrant rows = %d, want 2", len(rows))
	}
	var got *model.UserAuditEvent
	for i := range rows {
		if rows[i].UserID.Equal(second.ID) {
			got = &rows[i]
		}
	}
	if got == nil {
		t.Fatal("no regrant row for the second account")
	}
	assertRegrantRow(t, *got, second.ID, secondDeleted)

	e.moveAway(t, second.ID, "anna.third@example.test")
	third, err := e.svc.ProvisionExternalUser(context.Background(), "Anna Third", regrantEmail)
	if err != nil {
		t.Fatalf("third ProvisionExternalUser: %v", err)
	}
	e.assertTrial(t, e.user(t, third.ID))
	if n := len(e.regrants(t)); n != 2 {
		t.Fatalf("regrant rows = %d, want 2", n)
	}
}

func TestRegrant_PasswordReset(t *testing.T) {
	e := newRegrantEnv(t, true)
	deletedID := e.seedDeleted(t, regrantEmail, true)
	u := e.register(t, regrantEmail)
	e.assertTrial(t, u)
	e.resetPassword(t, u.ID, regrantEmail)
	got := e.user(t, u.ID)
	assertLifetime(t, got)
	if !got.EmailVerified {
		t.Error("the reset must mark the address verified")
	}
	rows := e.regrants(t)
	if len(rows) != 1 {
		t.Fatalf("regrant rows = %d, want 1", len(rows))
	}
	assertRegrantRow(t, rows[0], u.ID, deletedID)
}

func TestRegrant_NotOnEmailChange(t *testing.T) {
	e := newRegrantEnv(t, false)
	e.seedDeleted(t, regrantEmail, true)
	u := e.register(t, "bob@example.test")
	e.assertTrial(t, u)
	ctx := context.Background()
	if _, err := e.svc.RequestEmailChange(ctx, u.ID, model.RequestEmailChangeRequest{NewEmail: regrantEmail, Password: "secretpass1"}); err != nil {
		t.Fatalf("RequestEmailChange: %v", err)
	}
	if len(e.cap.msgs) == 0 {
		t.Fatal("no change-email code was sent")
	}
	code := changeCodeFrom(t, e.cap.msgs[0].Text)
	if _, err := e.svc.ConfirmEmailChange(ctx, u.ID, vo.Id{}, model.ConfirmEmailChangeRequest{Code: code}); err != nil {
		t.Fatalf("ConfirmEmailChange: %v", err)
	}
	e.assertTrial(t, e.user(t, u.ID))
	if n := len(e.regrants(t)); n != 0 {
		t.Fatalf("regrant rows = %d, want 0", n)
	}

	e.resetPassword(t, u.ID, regrantEmail)
	e.assertTrial(t, e.user(t, u.ID))
	if n := len(e.regrants(t)); n != 0 {
		t.Fatalf("regrant rows after the reset = %d, want 0", n)
	}
}

func TestRegrant_LostRace(t *testing.T) {
	e := newRegrantEnv(t, true)
	deletedID := e.seedDeleted(t, regrantEmail, true)
	// The winner's row carries no hash, so the loser's match still sees the
	// deletion as open and reaches the insert, where ref_id refuses it.
	ref := deletedID
	if ok, err := e.audit.Insert(context.Background(), model.UserAuditEvent{
		ID: vo.NewId(), UserID: vo.NewId(), Event: model.AuditLifetimeRegranted,
		RefID: &ref, Details: "{}", CreatedAt: e.clk.now,
	}); err != nil || !ok {
		t.Fatalf("seed winner: %v %v", ok, err)
	}
	u, err := e.svc.ProvisionExternalUser(context.Background(), "Anna", regrantEmail)
	if err != nil {
		t.Fatalf("ProvisionExternalUser: %v", err)
	}
	e.assertTrial(t, e.user(t, u.ID))
	if n := len(e.regrants(t)); n != 1 {
		t.Fatalf("regrant rows = %d, want only the winner's", n)
	}
}

func TestRegrant_NotLifetime(t *testing.T) {
	e := newRegrantEnv(t, true)
	// A non-lifetime deletion with the hash present anyway, so only the
	// lifetimeAccess flag decides.
	h := model.EmailHash(regrantEmail)
	if ok, err := e.audit.Insert(context.Background(), model.UserAuditEvent{
		ID: vo.NewId(), UserID: vo.NewId(), Event: model.AuditDeleted, EmailHash: &h,
		Details: `{"reason":"inactive","lifetimeAccess":false}`, CreatedAt: e.clk.now,
	}); err != nil || !ok {
		t.Fatalf("seed: %v %v", ok, err)
	}
	u, err := e.svc.ProvisionExternalUser(context.Background(), "Anna", regrantEmail)
	if err != nil {
		t.Fatalf("ProvisionExternalUser: %v", err)
	}
	e.assertTrial(t, e.user(t, u.ID))
	if n := len(e.regrants(t)); n != 0 {
		t.Fatalf("regrant rows = %d, want 0", n)
	}
}

func TestRegrant_OtherAddress(t *testing.T) {
	e := newRegrantEnv(t, true)
	e.seedDeleted(t, "someone.else@example.test", true)
	u, err := e.svc.ProvisionExternalUser(context.Background(), "Anna", regrantEmail)
	if err != nil {
		t.Fatalf("ProvisionExternalUser: %v", err)
	}
	e.assertTrial(t, e.user(t, u.ID))
	if n := len(e.regrants(t)); n != 0 {
		t.Fatalf("regrant rows = %d, want 0", n)
	}
}

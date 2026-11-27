package user_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/mailer"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

const passPassword = "secretpass"

type passMailer struct {
	msgs   []mailer.Message
	failTo map[string]bool
	hook   func(m mailer.Message)
}

func (p *passMailer) Send(_ context.Context, m mailer.Message) error {
	if p.hook != nil {
		p.hook(m)
	}
	if p.failTo[m.To] {
		return errors.New("smtp: refused")
	}
	p.msgs = append(p.msgs, m)
	return nil
}

type noopWiper struct{}

func (noopWiper) LockBudgets(context.Context, vo.Id) error { return nil }
func (noopWiper) Erase(context.Context, vo.Id) error       { return nil }

type failingFirstWiper struct{ fail vo.Id }

type wipeRefused struct{}

func (wipeRefused) Error() string { return "constraint user_secret@example.test" }

func (failingFirstWiper) LockBudgets(context.Context, vo.Id) error { return nil }
func (w failingFirstWiper) Erase(_ context.Context, id vo.Id) error {
	if id.Equal(w.fail) {
		return wipeRefused{}
	}
	return nil
}

type hookedDeletionStore struct {
	appuser.DeletionStore
	afterListMarked func()
	listErr         error
}

func (h *hookedDeletionStore) ListMarked(ctx context.Context) ([]model.MarkedUser, error) {
	if h.listErr != nil {
		return nil, h.listErr
	}
	out, err := h.DeletionStore.ListMarked(ctx)
	if h.afterListMarked != nil {
		h.afterListMarked()
	}
	return out, err
}

type passEnv struct {
	db    *dbtest.DB
	svc   *appuser.Service
	store *hookedDeletionStore
	audit *userrepo.AuditRepo
	users *userrepo.Repo
	mail  *passMailer
	clk   *testClock
}

func newPassEnv(t *testing.T, cfg appuser.DeletionConfig) *passEnv {
	t.Helper()
	db := dbtest.New(t)
	clk := &testClock{now: authT0}
	users := userrepo.NewRepo(db.Engine, db.TX)
	svc := appuser.NewService(users, db.TX, auth.NewEncodeService(""), auth.NewPasswordHasher(),
		userrepo.NewAccessTokenRepo(db.Engine, db.TX),
		server.NewUserCurrencyLookup(currencyrepo.New(db.Engine, db.TX)),
		server.NewUserBudgetAccess(db.Engine, db.TX),
		userrepo.NewPasswordRequestRepo(db.Engine, db.TX), nil,
		userrepo.NewEmailVerificationRepo(db.Engine, db.TX), nil,
		userrepo.NewEmailChangeRequestRepo(db.Engine, db.TX), nil,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clk, nil, false, 0, false)
	e := &passEnv{
		db: db, svc: svc, users: users, clk: clk,
		store: &hookedDeletionStore{DeletionStore: userrepo.NewDeletionRepo(db.Engine, db.TX)},
		audit: userrepo.NewAuditRepo(db.Engine, db.TX),
		mail:  &passMailer{failTo: map[string]bool{}},
	}
	svc.SetDeletion(e.store, e.audit, userrepo.NewDeletionRequestRepo(db.Engine, db.TX), cfg)
	svc.SetDeletionSender(mailer.NewDeletionSender(e.mail, "noreply@econumo.test", ""))
	svc.SetDataWiper(noopWiper{})
	return e
}

// user creates a user whose last activity was lastActive ago.
func (e *passEnv) user(t *testing.T, email string, lastActive time.Duration) vo.Id {
	t.Helper()
	id, err := e.svc.AdminCreateUser(context.Background(), "Name "+email[:3], email, passPassword)
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	e.setLastActive(t, id, e.clk.now.Add(-lastActive))
	return id
}

func (e *passEnv) setLastActive(t *testing.T, id vo.Id, at time.Time) {
	t.Helper()
	e.db.Exec(t, e.db.Rebind(`UPDATE users SET last_active_at = ? WHERE id = ?`), at, id.String())
}

func (e *passEnv) connect(t *testing.T, a, b vo.Id) {
	t.Helper()
	fixture.New(t, e.db).Connect(a.String(), b.String())
}

func (e *passEnv) mark(t *testing.T, id vo.Id, reason model.DeletionReason, at time.Time) {
	t.Helper()
	if n, err := e.store.Mark(context.Background(), id, reason, at); err != nil || n != 1 {
		t.Fatalf("Mark: %d %v", n, err)
	}
}

func (e *passEnv) state(t *testing.T, id vo.Id) model.DeletionState {
	t.Helper()
	st, err := e.store.GetState(context.Background(), id)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	return st
}

func (e *passEnv) exists(t *testing.T, id vo.Id) bool {
	t.Helper()
	_, err := e.store.GetState(context.Background(), id)
	if err == nil {
		return true
	}
	if _, ok := errs.AsNotFound(err); ok {
		return false
	}
	t.Fatalf("GetState: %v", err)
	return false
}

func (e *passEnv) auditRows(t *testing.T, event string) []model.UserAuditEvent {
	t.Helper()
	rows, err := e.audit.ListByEvent(context.Background(), event, time.Time{}, "", 100)
	if err != nil {
		t.Fatalf("ListByEvent: %v", err)
	}
	return rows
}

func (e *passEnv) login(t *testing.T, email string) string {
	t.Helper()
	res, err := e.svc.Login(context.Background(), model.LoginRequest{Username: email, Password: passPassword}, "ua", e.clk.now)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	return res.Token
}

func (e *passEnv) markInactive(t *testing.T) int {
	t.Helper()
	n, err := e.svc.MarkInactive(context.Background())
	if err != nil {
		t.Fatalf("MarkInactive: %v", err)
	}
	return n
}

func (e *passEnv) release(t *testing.T) int {
	t.Helper()
	n, err := e.svc.ReleaseInactiveMarks(context.Background())
	if err != nil {
		t.Fatalf("ReleaseInactiveMarks: %v", err)
	}
	return n
}

const day = 24 * time.Hour

var inactive30 = appuser.DeletionConfig{InactiveDays: 30, DeletionDays: 30}

func TestMarkInactive_CutoffRule(t *testing.T) {
	e := newPassEnv(t, inactive30)
	recent := e.user(t, "recent@pass.test", 30*day+23*time.Hour)
	idle := e.user(t, "idle@pass.test", 31*day+time.Hour)

	if n := e.markInactive(t); n != 1 {
		t.Fatalf("marked = %d, want 1", n)
	}
	if st := e.state(t, recent); st.MarkedAt != nil {
		t.Errorf("a stamp 30 days 23 h old is not inactive: %+v", st)
	}
	if st := e.state(t, idle); st.MarkedAt == nil {
		t.Errorf("a stamp 31 days 1 h old is inactive: %+v", st)
	}
}

func TestMarkInactive_MarksAndEmailsOnce(t *testing.T) {
	e := newPassEnv(t, inactive30)
	u := e.user(t, "Idle.User@Pass.test", 40*day)
	e.db.Exec(t, e.db.Rebind(`UPDATE users SET language = ? WHERE id = ?`), "de", u.String())

	if n := e.markInactive(t); n != 1 {
		t.Fatalf("marked = %d, want 1", n)
	}
	st := e.state(t, u)
	if st.MarkedAt == nil || st.Reason != model.DeletionReasonInactive {
		t.Fatalf("state = %+v, want marked inactive", st)
	}
	if len(e.mail.msgs) != 1 {
		t.Fatalf("emails = %d, want 1", len(e.mail.msgs))
	}
	m := e.mail.msgs[0]
	if m.To != "idle.user@pass.test" {
		t.Errorf("to = %q, want the decoded address", m.To)
	}
	if m.Subject != "Ihre Econumo-Daten sind zur Löschung vorgemerkt" {
		t.Errorf("subject = %q, want the stored language's", m.Subject)
	}
	if !strings.Contains(m.Text, "30") {
		t.Errorf("body does not carry the 30 days:\n%s", m.Text)
	}

	if n := e.markInactive(t); n != 0 {
		t.Errorf("second pass marked %d", n)
	}
	if len(e.mail.msgs) != 1 {
		t.Errorf("second pass sent mail: %d", len(e.mail.msgs))
	}
}

func TestMarkInactive_SkipsExcludedAndConnected(t *testing.T) {
	cfg := inactive30
	cfg.Exclude = map[string]bool{"keep@pass.test": true}
	e := newPassEnv(t, cfg)
	excluded := e.user(t, "Keep@Pass.test", 40*day)
	connected := e.user(t, "household@pass.test", 40*day)
	partner := e.user(t, "partner@pass.test", time.Hour)
	e.connect(t, connected, partner)

	if n := e.markInactive(t); n != 0 {
		t.Fatalf("marked = %d, want 0", n)
	}
	for _, id := range []vo.Id{excluded, connected, partner} {
		if st := e.state(t, id); st.MarkedAt != nil {
			t.Errorf("%s marked: %+v", id, st)
		}
	}
	if len(e.mail.msgs) != 0 {
		t.Errorf("emails = %d, want none", len(e.mail.msgs))
	}
}

func TestMarkInactive_WholeHousehold(t *testing.T) {
	e := newPassEnv(t, inactive30)
	a := e.user(t, "a@pass.test", 40*day)
	b := e.user(t, "b@pass.test", 45*day)
	e.connect(t, a, b)

	if n := e.markInactive(t); n != 2 {
		t.Fatalf("marked = %d, want 2", n)
	}
	for _, id := range []vo.Id{a, b} {
		if st := e.state(t, id); st.MarkedAt == nil {
			t.Errorf("%s not marked", id)
		}
	}
}

func TestMarkInactive_MailerFailureIsIsolated(t *testing.T) {
	e := newPassEnv(t, inactive30)
	a := e.user(t, "first@pass.test", 40*day)
	b := e.user(t, "second@pass.test", 40*day)
	var first string
	e.mail.hook = func(m mailer.Message) {
		if first == "" {
			first = m.To
			e.mail.failTo[m.To] = true
		}
	}

	n, err := e.svc.MarkInactive(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("MarkInactive = %d, %v; want 1, nil", n, err)
	}
	failed, sent := a, b
	if first == "second@pass.test" {
		failed, sent = b, a
	}
	if st := e.state(t, failed); st.MarkedAt != nil {
		t.Errorf("the user whose notice failed was marked: %+v", st)
	}
	if st := e.state(t, sent); st.MarkedAt == nil {
		t.Error("the user whose notice went out was not marked")
	}
}

func TestMarkInactive_SignedInWhileTheMailWasSent(t *testing.T) {
	e := newPassEnv(t, inactive30)
	u := e.user(t, "busy@pass.test", 40*day)
	var token string
	e.mail.hook = func(mailer.Message) { token = e.login(t, "busy@pass.test") }

	if n := e.markInactive(t); n != 0 {
		t.Fatalf("marked = %d, want 0", n)
	}
	if st := e.state(t, u); st.MarkedAt != nil {
		t.Fatalf("state = %+v, want unmarked", st)
	}
	if _, err := e.svc.Authenticate(context.Background(), token); err != nil {
		t.Errorf("the new session must still authenticate: %v", err)
	}
}

func TestMarkInactive_EmailChangedWhileTheMailWasSent(t *testing.T) {
	e := newPassEnv(t, inactive30)
	u := e.user(t, "moving@pass.test", 40*day)
	e.mail.hook = func(mailer.Message) {
		if err := e.users.SetEmail(context.Background(), u, "moved@pass.test"); err != nil {
			t.Fatalf("SetEmail: %v", err)
		}
	}

	if n := e.markInactive(t); n != 0 {
		t.Fatalf("marked = %d, want 0", n)
	}
	if st := e.state(t, u); st.MarkedAt != nil {
		t.Fatalf("state = %+v, want unmarked", st)
	}
}

func TestMarkInactive_Off(t *testing.T) {
	e := newPassEnv(t, appuser.DeletionConfig{InactiveDays: 0, DeletionDays: 30})
	e.user(t, "ancient@pass.test", 4000*day)
	e.svc.SetDeletionSender(nil)

	if n := e.markInactive(t); n != 0 {
		t.Fatalf("marked = %d, want 0", n)
	}
	if len(e.mail.msgs) != 0 {
		t.Errorf("emails = %d, want none", len(e.mail.msgs))
	}
}

func TestMarkInactive_SenderUnwired(t *testing.T) {
	e := newPassEnv(t, inactive30)
	e.svc.SetDeletionSender(nil)
	if _, err := e.svc.MarkInactive(context.Background()); err == nil {
		t.Fatal("MarkInactive without a sender must fail")
	}
}

func TestDeletionPasses_Unwired(t *testing.T) {
	e := newPassEnv(t, inactive30)
	e.svc.SetDeletion(nil, nil, nil, inactive30)
	ctx := context.Background()
	if _, err := e.svc.MarkInactive(ctx); err == nil {
		t.Error("MarkInactive must fail unwired")
	}
	if _, err := e.svc.ReleaseInactiveMarks(ctx); err == nil {
		t.Error("ReleaseInactiveMarks must fail unwired")
	}
	if _, _, err := e.svc.WipeExpired(ctx); err == nil {
		t.Error("WipeExpired must fail unwired")
	}
}

func assertCancelled(t *testing.T, e *passEnv, id vo.Id, details string) {
	t.Helper()
	for _, r := range e.auditRows(t, model.AuditDeletionCancelled) {
		if r.UserID.Equal(id) {
			if r.Details != details {
				t.Errorf("cancel details = %s, want %s", r.Details, details)
			}
			return
		}
	}
	t.Errorf("no deletion_cancelled row for %s", id)
}

func TestRelease_PartnerCameBack(t *testing.T) {
	e := newPassEnv(t, inactive30)
	a := e.user(t, "a@pass.test", 40*day)
	b := e.user(t, "b@pass.test", 40*day)
	e.connect(t, a, b)
	if n := e.markInactive(t); n != 2 {
		t.Fatalf("marked = %d, want 2", n)
	}
	e.clk.now = e.clk.now.Add(time.Hour)
	e.login(t, "b@pass.test")

	if n := e.release(t); n != 1 {
		t.Fatalf("released = %d, want 1", n)
	}
	if st := e.state(t, a); st.MarkedAt != nil {
		t.Fatalf("A still marked: %+v", st)
	}
	assertCancelled(t, e, a, `{"reason":"inactive","cause":"active_connection"}`)
}

func TestRelease_MarkedPartnerDoesNotCount(t *testing.T) {
	e := newPassEnv(t, inactive30)
	a := e.user(t, "a@pass.test", 40*day)
	b := e.user(t, "b@pass.test", time.Hour)
	e.connect(t, a, b)
	e.mark(t, a, model.DeletionReasonInactive, e.clk.now)
	e.mark(t, b, model.DeletionReasonSelf, e.clk.now)

	if n := e.release(t); n != 0 {
		t.Fatalf("released = %d, want 0", n)
	}
	if st := e.state(t, a); st.MarkedAt == nil {
		t.Fatal("A must stay marked: a marked partner does not count")
	}
}

func TestRelease_InactivityOff(t *testing.T) {
	e := newPassEnv(t, appuser.DeletionConfig{InactiveDays: 0, DeletionDays: 30})
	idle := e.user(t, "idle@pass.test", 40*day)
	cli := e.user(t, "cli@pass.test", 40*day)
	self := e.user(t, "self@pass.test", 40*day)
	e.mark(t, idle, model.DeletionReasonInactive, e.clk.now)
	e.mark(t, cli, model.DeletionReasonCLI, e.clk.now)
	e.mark(t, self, model.DeletionReasonSelf, e.clk.now)

	if n := e.release(t); n != 1 {
		t.Fatalf("released = %d, want 1", n)
	}
	if st := e.state(t, idle); st.MarkedAt != nil {
		t.Errorf("inactive mark kept: %+v", st)
	}
	assertCancelled(t, e, idle, `{"reason":"inactive","cause":"inactivity_off"}`)
	if st := e.state(t, cli); st.Reason != model.DeletionReasonCLI {
		t.Errorf("cli mark touched: %+v", st)
	}
	if st := e.state(t, self); st.Reason != model.DeletionReasonSelf {
		t.Errorf("self_initiated mark touched: %+v", st)
	}
}

func TestRelease_RunsWhenDeletionDaysIsZero(t *testing.T) {
	e := newPassEnv(t, appuser.DeletionConfig{InactiveDays: 30, DeletionDays: 0})
	a := e.user(t, "a@pass.test", 40*day)
	b := e.user(t, "b@pass.test", time.Hour)
	e.connect(t, a, b)
	e.mark(t, a, model.DeletionReasonInactive, e.clk.now)

	res := e.svc.PurgeDeleted(context.Background())
	if res.Err != nil || res.Released != 1 || res.Wiped != 0 {
		t.Fatalf("PurgeDeleted = %+v, want one release and no wipe", res)
	}
	if st := e.state(t, a); st.MarkedAt != nil {
		t.Fatalf("A still marked: %+v", st)
	}
}

func TestRelease_DoesNotCancelAReplacementMark(t *testing.T) {
	e := newPassEnv(t, inactive30)
	a := e.user(t, "a@pass.test", 40*day)
	b := e.user(t, "b@pass.test", time.Hour)
	e.connect(t, a, b)
	e.mark(t, a, model.DeletionReasonInactive, e.clk.now.Add(-time.Hour))
	e.store.afterListMarked = func() {
		e.store.afterListMarked = nil
		e.login(t, "a@pass.test")
		if err := e.svc.AdminMarkDeleted(context.Background(), "a@pass.test"); err != nil {
			t.Fatalf("AdminMarkDeleted: %v", err)
		}
	}

	if n := e.release(t); n != 0 {
		t.Fatalf("released = %d, want 0", n)
	}
	if st := e.state(t, a); st.MarkedAt == nil || st.Reason != model.DeletionReasonCLI {
		t.Fatalf("state = %+v, want the cli mark", st)
	}
}

func TestPurgeDeleted_ListingFails(t *testing.T) {
	e := newPassEnv(t, inactive30)
	boom := errors.New("listing failed")
	e.store.listErr = boom
	if res := e.svc.PurgeDeleted(context.Background()); !errors.Is(res.Err, boom) {
		t.Fatalf("PurgeDeleted().Err = %v, want the listing error", res.Err)
	}
}

func TestWipeExpired_AgeRule(t *testing.T) {
	e := newPassEnv(t, inactive30)
	young := e.user(t, "young@pass.test", 40*day)
	old := e.user(t, "old@pass.test", 40*day)
	e.mark(t, young, model.DeletionReasonCLI, e.clk.now.Add(-29*day))
	e.mark(t, old, model.DeletionReasonCLI, e.clk.now.Add(-31*day))

	wiped, failed, err := e.svc.WipeExpired(context.Background())
	if err != nil || wiped != 1 || len(failed) != 0 {
		t.Fatalf("WipeExpired = %d, %v, %v; want 1 wiped", wiped, failed, err)
	}
	if !e.exists(t, young) {
		t.Error("a mark 29 days old was wiped")
	}
	if e.exists(t, old) {
		t.Error("a mark 31 days old survived")
	}
}

func TestWipeExpired_DeletionDaysZero(t *testing.T) {
	e := newPassEnv(t, appuser.DeletionConfig{InactiveDays: 30, DeletionDays: 0})
	old := e.user(t, "old@pass.test", 40*day)
	e.mark(t, old, model.DeletionReasonCLI, e.clk.now.Add(-400*day))

	wiped, failed, err := e.svc.WipeExpired(context.Background())
	if err != nil || wiped != 0 || failed != nil {
		t.Fatalf("WipeExpired = %d, %v, %v; want nothing", wiped, failed, err)
	}
	if !e.exists(t, old) {
		t.Error("DeletionDays 0 must wipe nobody")
	}
}

func TestWipeExpired_ContinuesPastAFailure(t *testing.T) {
	e := newPassEnv(t, inactive30)
	first := e.user(t, "first@pass.test", 40*day)
	second := e.user(t, "second@pass.test", 40*day)
	e.mark(t, first, model.DeletionReasonCLI, e.clk.now.Add(-32*day))
	e.mark(t, second, model.DeletionReasonCLI, e.clk.now.Add(-31*day))
	e.svc.SetDataWiper(failingFirstWiper{fail: first})

	wiped, failed, err := e.svc.WipeExpired(context.Background())
	if err != nil || wiped != 1 {
		t.Fatalf("WipeExpired = %d, %v; want 1 wiped", wiped, err)
	}
	if len(failed) != 1 || !failed[0].UserID.Equal(first) {
		t.Fatalf("failed = %+v, want the first user", failed)
	}
	if failed[0].ErrType != "user_test.wipeRefused" {
		t.Errorf("ErrType = %q, want the Go type name", failed[0].ErrType)
	}
	if _, ok := failed[0].Err.(wipeRefused); !ok {
		t.Errorf("Err = %v, want the wipe's error", failed[0].Err)
	}
	if !e.exists(t, first) || e.exists(t, second) {
		t.Error("want the first user kept and the second wiped")
	}
}

func TestWipe_ReleasesInsteadOfWiping(t *testing.T) {
	e := newPassEnv(t, inactive30)
	u := e.user(t, "idle@pass.test", 40*day)
	p := e.user(t, "partner@pass.test", time.Hour)
	e.connect(t, u, p)
	e.mark(t, u, model.DeletionReasonInactive, e.clk.now)

	out, err := e.svc.Wipe(context.Background(), u, nil)
	if err != nil || out != appuser.WipeReleased {
		t.Fatalf("Wipe = %q, %v; want released", out, err)
	}
	if !e.exists(t, u) {
		t.Fatal("the user row must stay")
	}
	assertCancelled(t, e, u, `{"reason":"inactive","cause":"active_connection"}`)
}

func TestWipe_InactivityOffReleases(t *testing.T) {
	e := newPassEnv(t, appuser.DeletionConfig{InactiveDays: 0, DeletionDays: 30})
	u := e.user(t, "idle@pass.test", 40*day)
	e.mark(t, u, model.DeletionReasonInactive, e.clk.now)

	out, err := e.svc.Wipe(context.Background(), u, nil)
	if err != nil || out != appuser.WipeReleased {
		t.Fatalf("Wipe = %q, %v; want released", out, err)
	}
	if !e.exists(t, u) {
		t.Fatal("the user row must stay")
	}
	assertCancelled(t, e, u, `{"reason":"inactive","cause":"inactivity_off"}`)
}

func TestWipe_InactiveWithNoActiveConnectionWipes(t *testing.T) {
	e := newPassEnv(t, inactive30)
	u := e.user(t, "idle@pass.test", 40*day)
	p := e.user(t, "partner@pass.test", 40*day)
	e.connect(t, u, p)
	e.mark(t, u, model.DeletionReasonInactive, e.clk.now)

	out, err := e.svc.Wipe(context.Background(), u, nil)
	if err != nil || out != appuser.WipeDone {
		t.Fatalf("Wipe = %q, %v; want deleted", out, err)
	}
}

func TestDeletionJob_RunOnceOrder(t *testing.T) {
	e := newPassEnv(t, inactive30)
	a := e.user(t, "a@pass.test", 40*day)
	b := e.user(t, "b@pass.test", time.Hour)
	e.connect(t, a, b)
	e.mark(t, a, model.DeletionReasonInactive, e.clk.now.Add(-31*day))
	old := e.user(t, "old@pass.test", 40*day)
	e.mark(t, old, model.DeletionReasonCLI, e.clk.now.Add(-31*day))
	idle := e.user(t, "idle@pass.test", 40*day)

	res := appuser.NewDeletionJob(e.svc, e.clk).RunOnce(context.Background())
	if res.Err != nil || res.Released != 1 || res.Wiped != 1 || res.Marked != 1 || len(res.Failed) != 0 {
		t.Fatalf("RunOnce = %+v, want one release, one wipe, one mark", res)
	}
	if st := e.state(t, a); st.MarkedAt != nil {
		t.Errorf("A must be released before the wipe step: %+v", st)
	}
	if e.exists(t, old) {
		t.Error("the expired cli mark survived")
	}
	if st := e.state(t, idle); st.Reason != model.DeletionReasonInactive {
		t.Errorf("the idle user was not marked: %+v", st)
	}
}

type panickingWiper struct{}

func (panickingWiper) LockBudgets(context.Context, vo.Id) error { return nil }
func (panickingWiper) Erase(context.Context, vo.Id) error {
	panic("wipe exploded for user_secret@example.test")
}

// captureLog sends the default logger to a buffer for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// A wipe that panics is that user's failure: the pass goes on, the log names
// the user and the panic's type, never its value.
func TestDeletionJob_RunOnceRecoversAPanic(t *testing.T) {
	e := newPassEnv(t, inactive30)
	u := e.user(t, "doomed@pass.test", 40*day)
	e.mark(t, u, model.DeletionReasonCLI, e.clk.now.Add(-31*day))
	e.svc.SetDataWiper(panickingWiper{})
	logs := captureLog(t)

	res := appuser.NewDeletionJob(e.svc, e.clk).RunOnce(context.Background())
	if res.Err != nil || len(res.Failed) != 1 || !res.Failed[0].UserID.Equal(u) || res.Failed[0].ErrType != "panic:string" {
		t.Fatalf("RunOnce = %+v, want one failure for the user, reason panic:string", res)
	}
	if !e.exists(t, u) {
		t.Error("the user is gone after a panicking wipe")
	}
	if strings.Contains(logs.String(), "user_secret@example.test") {
		t.Errorf("the log carries the panic value: %s", logs.String())
	}
	if !strings.Contains(logs.String(), u.String()) {
		t.Errorf("the log does not name the user: %s", logs.String())
	}

	// the next tick runs and can write: the panicking wipe left no transaction open
	e.svc.SetDataWiper(noopWiper{})
	if res := appuser.NewDeletionJob(e.svc, e.clk).RunOnce(context.Background()); res.Err != nil || res.Wiped != 1 {
		t.Fatalf("the next RunOnce = %+v, want the user wiped", res)
	}
	if e.exists(t, u) {
		t.Error("the user survived the next tick")
	}
}

type panickingFirstWiper struct{ fail vo.Id }

func (panickingFirstWiper) LockBudgets(context.Context, vo.Id) error { return nil }
func (w panickingFirstWiper) Erase(_ context.Context, id vo.Id) error {
	if id.Equal(w.fail) {
		panic("wipe exploded for user_secret@example.test")
	}
	return nil
}

func TestWipeExpired_ContinuesPastAPanic(t *testing.T) {
	e := newPassEnv(t, inactive30)
	first := e.user(t, "first@pass.test", 40*day)
	second := e.user(t, "second@pass.test", 40*day)
	e.mark(t, first, model.DeletionReasonCLI, e.clk.now.Add(-32*day))
	e.mark(t, second, model.DeletionReasonCLI, e.clk.now.Add(-31*day))
	e.svc.SetDataWiper(panickingFirstWiper{fail: first})

	wiped, failed, err := e.svc.WipeExpired(context.Background())
	if err != nil || wiped != 1 {
		t.Fatalf("WipeExpired = %d, %v; want 1 wiped", wiped, err)
	}
	if len(failed) != 1 || !failed[0].UserID.Equal(first) || failed[0].ErrType != "panic:string" {
		t.Fatalf("failed = %+v, want the first user with reason panic:string", failed)
	}
	if !e.exists(t, first) || e.exists(t, second) {
		t.Error("want the first user kept and the second wiped")
	}
}

// The outer recover stays as the last line of defence for a panic outside a wipe.
func TestDeletionJob_RunOnceRecoversAPanicOutsideAWipe(t *testing.T) {
	e := newPassEnv(t, inactive30)
	e.store.afterListMarked = func() { panic("listing exploded for user_secret@example.test") }
	logs := captureLog(t)

	res := appuser.NewDeletionJob(e.svc, e.clk).RunOnce(context.Background())
	if res.Err == nil {
		t.Error("a pass that panicked must report an error")
	}
	if strings.Contains(logs.String(), "user_secret@example.test") {
		t.Errorf("the log carries the panic value: %s", logs.String())
	}
}

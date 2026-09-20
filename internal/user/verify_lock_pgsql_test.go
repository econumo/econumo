//go:build enginecompare

package user_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/pgsql"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// hookedVerifications fires a hook once, right after GetByUser handed the
// confirmation the row it treats as its evidence — the window a resend has to
// replace that code in.
type hookedVerifications struct {
	appuser.EmailVerifications
	hook func()
}

func (h *hookedVerifications) GetByUser(ctx context.Context, userID vo.Id) (*model.EmailVerification, error) {
	ev, err := h.EmailVerifications.GetByUser(ctx, userID)
	if err != nil || h.hook == nil {
		return ev, err
	}
	fire := h.hook
	h.hook = nil
	fire()
	return ev, nil
}

// A resend that lands while a confirmation is in flight must not lose its code:
// the confirmation sweeping every row of the user would delete a code the user
// has just been emailed. Only observable across two connections — every other
// suite pins its test database to one, where the resend nests inside the
// confirmation's own transaction and neither the lock nor its absence changes
// the interleaving. So this opens a SECOND pool against the same Postgres
// schema and runs the real ResendVerificationCode on it while the confirmation
// holds its row read: with the user row lock the resend waits and re-issues
// after the confirmation committed; without it, it commits inside the window
// and the confirmation's sweep takes the fresh code with it.
func TestConfirmEmail_ResendDuringTheConfirmKeepsTheFreshCode(t *testing.T) {
	db := dbtest.New(t)
	if db.Engine != "postgresql" {
		t.Skipf("the race needs two real connections; engine is %q (run with DBTEST_ENGINE=pgsql)", db.Engine)
	}
	ctx := context.Background()
	const email = "verify-race@econumo.test"
	const password = "secretpass1"

	confirmMail := &captureMailer{}
	evs := &hookedVerifications{EmailVerifications: userrepo.NewEmailVerificationRepo(db.Engine, db.TX)}
	svc, _ := newVerifySvcStore(t, db, confirmMail, evs, true)

	if _, err := svc.Register(ctx, model.RegisterRequest{Name: "Racer", Email: email, Password: password}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// The blocked login is what issues the first code.
	if _, err := svc.Login(ctx, model.LoginRequest{Username: email, Password: password}, "ua", time.Now()); err == nil {
		t.Fatal("login before confirmation must be denied")
	}
	if len(confirmMail.msgs) != 1 {
		t.Fatalf("want the first verification email, got %d", len(confirmMail.msgs))
	}
	firstCode := codeFrom(t, confirmMail.msgs[0].Text)

	plain := userrepo.NewRepo(db.Engine, db.TX)
	u, err := plain.GetByEmail(ctx, email)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}

	// The second pool: same schema, its own connection, so its transaction can
	// be in flight while the confirmation holds its read.
	var schema string
	if err := db.Raw.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatalf("read current_schema: %v", err)
	}
	raw2, err := pgsql.OpenDB(os.Getenv(dbtest.PgsqlURLEnv))
	if err != nil {
		t.Fatalf("open second pool: %v", err)
	}
	t.Cleanup(func() { _ = raw2.Close() })
	raw2.SetMaxOpenConns(1)
	if _, err := raw2.ExecContext(ctx, fmt.Sprintf(`SET search_path TO %q`, schema)); err != nil {
		t.Fatalf("set search_path on the second pool: %v", err)
	}
	db2 := &dbtest.DB{Raw: raw2, TX: backend.NewTxManager(raw2), Engine: db.Engine}
	resendMail := &captureMailer{}
	svc2, clk2 := newVerifySvcStore(t, db2, resendMail, userrepo.NewEmailVerificationRepo(db2.Engine, db2.TX), true)
	// Past the resend gap, so the second connection really issues a code.
	clk2.now = clk2.now.Add(2 * model.EmailVerificationResendGap)

	resend := make(chan error, 1)
	evs.hook = func() {
		go func() {
			_, _, rerr := svc2.ResendVerificationCode(ctx, model.ResendVerificationCodeRequest{Username: email})
			resend <- rerr
		}()
		// Give the resend its chance to commit inside the window. Under the row
		// lock it blocks here instead and finishes after the confirmation
		// commits; the verdict is the stored code either way, never the timing.
		select {
		case rerr := <-resend:
			resend <- rerr // put it back for the wait below
		case <-time.After(2 * time.Second):
		}
	}

	if _, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: email, Code: firstCode}); err != nil {
		t.Fatalf("ConfirmEmail: %v", err)
	}
	select {
	case rerr := <-resend:
		if rerr != nil {
			t.Fatalf("ResendVerificationCode on the second connection: %v", rerr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the resend never completed")
	}
	if len(resendMail.msgs) != 1 {
		t.Fatalf("the resend must have emailed a fresh code, got %d emails", len(resendMail.msgs))
	}
	freshCode := codeFrom(t, resendMail.msgs[0].Text)

	ev, gerr := userrepo.NewEmailVerificationRepo(db.Engine, db.TX).GetByUser(ctx, u.ID)
	if gerr != nil {
		t.Fatalf("the freshly issued code was swept by the confirmation: %v", gerr)
	}
	if ev.Code != appuser.HashResetCode(freshCode) {
		t.Fatal("the outstanding code is not the one the resend emailed")
	}
	after, err := plain.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID after the race: %v", err)
	}
	if !after.EmailVerified {
		t.Error("the confirmation's own write must have landed")
	}
}

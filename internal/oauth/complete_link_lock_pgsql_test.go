//go:build enginecompare

package oauth_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/pgsql"
	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	oauthrepo "github.com/econumo/econumo/internal/oauth/repo"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// hookedHandoffs fires a hook once, right after the redemption consumed its
// code — the point where CompleteLink holds the fence value it will write
// under and has taken no lock yet.
type hookedHandoffs struct {
	appoauth.Handoffs
	afterDelete func()
}

func (h *hookedHandoffs) Delete(ctx context.Context, codeHash string) (int64, error) {
	n, err := h.Handoffs.Delete(ctx, codeHash)
	if err == nil && h.afterDelete != nil {
		fire := h.afterDelete
		h.afterDelete = nil
		fire()
	}
	return n, err
}

// hookedReclaimer runs the REAL reclaim and then pauses inside the reset's
// still-open transaction, so the other connection gets its chance to write an
// identity behind the sweep that just ran.
type hookedReclaimer struct {
	inner      *appoauth.Service
	afterSweep func()
}

func (r *hookedReclaimer) ReclaimAccount(ctx context.Context, userID vo.Id, provenEmail string) (int64, int64, error) {
	ids, grants, err := r.inner.ReclaimAccount(ctx, userID, provenEmail)
	if err == nil && r.afterSweep != nil {
		fire := r.afterSweep
		r.afterSweep = nil
		fire()
	}
	return ids, grants, err
}

// An identity INSERT is a credential mint, and its generation fence is a plain
// SELECT: under PostgreSQL's READ COMMITTED it cannot see a reclaim whose
// transaction is still open, so an unlocked insert passes the fence, lands
// behind the reclaim's identity sweep, and leaves the recovered account with a
// sign-in method its owner just revoked. The user row lock is what orders the
// two. Two real connections are required — one schema, two pools — because a
// single pool serializes the transactions regardless.
func TestCompleteLink_InsertCannotLandInsideAnOpenReclaim(t *testing.T) {
	db := dbtest.New(t)
	if db.Engine != "postgresql" {
		t.Skipf("the race needs two real connections; engine is %q (run with DBTEST_ENGINE=pgsql)", db.Engine)
	}
	ctx := context.Background()
	const email = "reclaim-race@econumo.test"

	// The second pool: same schema, its own connection, so its reset can be in
	// flight while the redemption runs on the first.
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

	// Pool 2 runs the owner's password reset, with the real oauth reclaim
	// wired behind it exactly as the composition root does.
	usvc := appuser.NewService(userrepo.NewRepo(db2.Engine, db2.TX), db2.TX, auth.NewEncodeService(""), auth.NewPasswordHasher(),
		userrepo.NewAccessTokenRepo(db2.Engine, db2.TX),
		server.NewUserCurrencyLookup(currencyrepo.New(db2.Engine, db2.TX)),
		server.NewUserBudgetAccess(db2.Engine, db2.TX),
		userrepo.NewPasswordRequestRepo(db2.Engine, db2.TX), nil,
		userrepo.NewEmailVerificationRepo(db2.Engine, db2.TX), nil,
		userrepo.NewEmailChangeRequestRepo(db2.Engine, db2.TX), nil,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clock.New(), nil, false, 0, false)
	osvc2 := appoauth.NewService(nil, nil, oauthrepo.NewIdentityRepo(db2.Engine, db2.TX), oauthrepo.NewStateRepo(db2.Engine, db2.TX),
		oauthrepo.NewHandoffRepo(db2.Engine, db2.TX), db2.TX, clock.New(), nil, "https://app.example.test", true, false)
	reclaimer := &hookedReclaimer{inner: osvc2}
	usvc.SetOAuthReclaimer(reclaimer)

	uid, err := usvc.AdminCreateUser(ctx, "Owner", email, "owner-old-password")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	pr := model.NewPasswordRequest(vo.NewId(), uid, appuser.HashResetCode("482913"), time.Now().UTC())
	if err := userrepo.NewPasswordRequestRepo(db2.Engine, db2.TX).Save(ctx, pr); err != nil {
		t.Fatalf("seed password request: %v", err)
	}

	// Pool 1 redeems a link handoff the squatter started before the reset.
	ids := oauthrepo.NewIdentityRepo(db.Engine, db.TX)
	hands := &hookedHandoffs{Handoffs: oauthrepo.NewHandoffRepo(db.Engine, db.TX)}
	clk := &fixedClock{t: time.Now().UTC().Truncate(time.Second)}
	svc1 := appoauth.NewService(nil, newFakeUsers(t, db), ids, oauthrepo.NewStateRepo(db.Engine, db.TX), hands,
		db.TX, clk, nil, "https://app.example.test", true, false)

	owner, err := userrepo.NewRepo(db.Engine, db.TX).GetByID(ctx, uid)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	code, _ := oidc.RandomToken()
	flow, _ := oidc.RandomToken()
	if err := oauthrepo.NewHandoffRepo(db.Engine, db.TX).Insert(ctx, &model.OAuthHandoff{
		CodeHash: oidc.Sha256Hex(code), Kind: model.OAuthHandoffKindLink, UserID: uid,
		Provider: "google", Issuer: "https://issuer.example.test", Subject: "squatter-sub", Email: "squatter@example.test",
		FlowHash: oidc.Sha256Hex(flow), Generation: owner.CredentialsGeneration,
		CreatedAt: clk.t, ExpiresAt: clk.t.Add(model.OAuthHandoffTTL),
	}); err != nil {
		t.Fatalf("seed link handoff: %v", err)
	}

	swept := make(chan struct{})
	resume := make(chan struct{})
	reset := make(chan error, 1)
	reclaimer.afterSweep = func() {
		close(swept)
		// Hold the reset's transaction open across the redemption's write. It
		// blocks on the row lock instead and finishes after this commits; the
		// verdict is the stored state either way, never the timing.
		select {
		case <-resume:
		case <-time.After(2 * time.Second):
		}
	}
	hands.afterDelete = func() {
		go func() {
			_, rerr := usvc.ResetPassword(ctx, model.ResetPasswordRequest{
				Username: email, Code: "482913", Password: "owner-new-password",
			})
			reset <- rerr
		}()
		select {
		case <-swept:
		case <-time.After(10 * time.Second):
			t.Error("the reclaim never reached its identity sweep")
		}
	}

	_, cerr := svc1.CompleteLink(ctx, uid, model.CompleteLinkRequest{Code: code, Flow: flow})
	close(resume)
	select {
	case rerr := <-reset:
		if rerr != nil {
			t.Fatalf("ResetPassword on the second connection: %v", rerr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the reset never completed")
	}

	if _, err := ids.GetByProviderSubject(ctx, "google", "https://issuer.example.test", "squatter-sub"); err == nil {
		t.Fatal("the link landed behind the reclaim: the squatter keeps a sign-in method")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("GetByProviderSubject: %v", err)
	}
	if v, ok := errs.AsValidation(cerr); !ok || v.MsgCode != errs.CodeOAuthLinkInvalid {
		t.Fatalf("want link_invalid from the fence, got %v", cerr)
	}
}

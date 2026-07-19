package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	accessUsdID = "dffc2a06-6f29-4704-8575-31709adee926"
	accessUserA = "11111111-1111-1111-1111-111111111111"
	accessUserB = "22222222-2222-2222-2222-222222222222"
	accessAcctA = "aaaa1111-0000-0000-0000-0000000000a1"
	accessAcctB = "bbbb1111-0000-0000-0000-0000000000b1"
	accessAcctC = "00000000-0000-0000-0000-0000000000c1"
)

var accessFixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

func newAccessRepo(t *testing.T) (*accountrepo.AccessRepo, *dbtest.DB, *fixture.Builder) {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: accessUserA, Name: "u"})
	f.User(fixture.User{ID: accessUserB, Name: "u"})
	f.Account(fixture.Account{ID: accessAcctA, UserID: accessUserA, CurrencyID: accessUsdID, Name: "A", Type: 2, Icon: "x"})
	f.Account(fixture.Account{ID: accessAcctB, UserID: accessUserB, CurrencyID: accessUsdID, Name: "B", Type: 2, Icon: "x"})
	return accountrepo.NewAccessRepo(db.Engine, db.TX), db, f
}

func TestAccessRepo_SaveGetRoundTrip_Pending(t *testing.T) {
	repo, _, _ := newAccessRepo(t)
	ctx := context.Background()
	grant := model.NewAccountAccess(vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB), model.RoleUser, accessFixedTime)
	if err := repo.Save(ctx, grant); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.Get(ctx, vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Role != model.RoleUser {
		t.Errorf("role mismatch: %d", got.Role)
	}
	if got.IsAccepted {
		t.Fatal("stored grant must round-trip as pending")
	}
	if !got.CreatedAt.Equal(accessFixedTime) {
		t.Errorf("createdAt mismatch: %v", got.CreatedAt)
	}
}

func TestAccessRepo_AcceptSaveGet(t *testing.T) {
	repo, _, _ := newAccessRepo(t)
	ctx := context.Background()
	grant := model.NewAccountAccess(vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB), model.RoleUser, accessFixedTime)
	if err := repo.Save(ctx, grant); err != nil {
		t.Fatalf("Save pending: %v", err)
	}
	got, err := repo.Get(ctx, vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got.Accept(accessFixedTime.Add(time.Hour))
	if err := repo.Save(ctx, got); err != nil {
		t.Fatalf("Save accepted: %v", err)
	}
	got2, err := repo.Get(ctx, vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB))
	if err != nil {
		t.Fatalf("Get after accept: %v", err)
	}
	if !got2.IsAccepted {
		t.Fatal("accept must persist")
	}
}

func TestAccessRepo_DeleteThenGetNotFound(t *testing.T) {
	repo, _, _ := newAccessRepo(t)
	ctx := context.Background()
	grant := model.NewAccountAccess(vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB), model.RoleUser, accessFixedTime)
	if err := repo.Save(ctx, grant); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Delete(ctx, vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := repo.Get(ctx, vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB))
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestAccessRepo_Get_NotFound(t *testing.T) {
	repo, _, _ := newAccessRepo(t)
	_, err := repo.Get(context.Background(), vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB))
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestAccessRepo_ListByAccount(t *testing.T) {
	repo, _, _ := newAccessRepo(t)
	ctx := context.Background()
	if err := repo.Save(ctx, model.NewAccountAccess(vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB), model.RoleUser, accessFixedTime)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	byAcct, err := repo.ListByAccount(ctx, vo.MustParseId(accessAcctA))
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(byAcct) != 1 || byAcct[0].UserID.String() != accessUserB {
		t.Fatalf("want 1 grant to userB on acctA, got %+v", byAcct)
	}
}

func TestAccessRepo_ReceivedVsPendingReceived(t *testing.T) {
	repo, _, _ := newAccessRepo(t)
	ctx := context.Background()

	// userA grants access on acctA to userB (still pending).
	pending := model.NewAccountAccess(vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB), model.RoleUser, accessFixedTime)
	if err := repo.Save(ctx, pending); err != nil {
		t.Fatalf("Save pending: %v", err)
	}
	// userB grants access on acctB to... itself isn't valid, so grant acctB access
	// to userA and accept it immediately, giving userB two received rows total
	// only if both target the same user. Instead: userB grants access on acctB to
	// userA (accepted), and we check received/pending for userA.
	accepted := model.NewAccountAccess(vo.MustParseId(accessAcctB), vo.MustParseId(accessUserA), model.RoleGuest, accessFixedTime.Add(time.Minute))
	accepted.Accept(accessFixedTime.Add(2 * time.Minute))
	if err := repo.Save(ctx, accepted); err != nil {
		t.Fatalf("Save accepted: %v", err)
	}
	// Give userA a second, pending grant too (on acctA, granted by acctA's
	// owner... acctA is owned by userA already, so instead reuse acctB owner
	// granting a second pending row to userA is impossible without a third
	// account). Simplify: userA now has exactly one accepted received grant
	// (from acctB) and zero pending; userB has exactly one pending received
	// grant (from acctA) and zero accepted.

	recvA, err := repo.ListReceived(ctx, vo.MustParseId(accessUserA))
	if err != nil {
		t.Fatalf("ListReceived userA: %v", err)
	}
	if len(recvA) != 1 || !recvA[0].IsAccepted {
		t.Fatalf("want userA to have 1 accepted received grant, got %+v", recvA)
	}
	pendA, err := repo.ListPendingReceived(ctx, vo.MustParseId(accessUserA))
	if err != nil {
		t.Fatalf("ListPendingReceived userA: %v", err)
	}
	if len(pendA) != 0 {
		t.Fatalf("want userA to have 0 pending received grants, got %+v", pendA)
	}

	recvB, err := repo.ListReceived(ctx, vo.MustParseId(accessUserB))
	if err != nil {
		t.Fatalf("ListReceived userB: %v", err)
	}
	if len(recvB) != 1 || recvB[0].IsAccepted {
		t.Fatalf("want userB to have 1 pending received grant, got %+v", recvB)
	}
	pendB, err := repo.ListPendingReceived(ctx, vo.MustParseId(accessUserB))
	if err != nil {
		t.Fatalf("ListPendingReceived userB: %v", err)
	}
	if len(pendB) != 1 || pendB[0].AccountID.String() != accessAcctA {
		t.Fatalf("want userB's 1 pending grant to be on acctA, got %+v", pendB)
	}
}

func TestAccessRepo_ListPendingReceived_Ordering(t *testing.T) {
	repo, _, f := newAccessRepo(t)
	ctx := context.Background()

	f.Account(fixture.Account{ID: accessAcctC, UserID: accessUserA, CurrencyID: accessUsdID, Name: "C", Type: 2, Icon: "x"})

	// Grant on acctA (lexicographically LARGER id) created first.
	first := model.NewAccountAccess(vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB), model.RoleUser, accessFixedTime)
	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	// Grant on acctC (lexicographically SMALLER id) created an hour later. If
	// ListPendingReceived sorted by account_id alone, acctC would sort first;
	// pinning created_at as the primary sort key catches that regression.
	second := model.NewAccountAccess(vo.MustParseId(accessAcctC), vo.MustParseId(accessUserB), model.RoleUser, accessFixedTime.Add(time.Hour))
	if err := repo.Save(ctx, second); err != nil {
		t.Fatalf("Save second: %v", err)
	}

	got, err := repo.ListPendingReceived(ctx, vo.MustParseId(accessUserB))
	if err != nil {
		t.Fatalf("ListPendingReceived: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 pending grants, got %d: %+v", len(got), got)
	}
	if got[0].AccountID.String() != accessAcctA || got[1].AccountID.String() != accessAcctC {
		t.Fatalf("want order [acctA, acctC] (created_at asc), got [%s, %s]", got[0].AccountID.String(), got[1].AccountID.String())
	}
	if !got[0].CreatedAt.Equal(accessFixedTime) || !got[1].CreatedAt.Equal(accessFixedTime.Add(time.Hour)) {
		t.Fatalf("createdAt mismatch: got[0]=%v got[1]=%v", got[0].CreatedAt, got[1].CreatedAt)
	}
}

func TestAccessRepo_ListIssued(t *testing.T) {
	repo, _, _ := newAccessRepo(t)
	ctx := context.Background()
	if err := repo.Save(ctx, model.NewAccountAccess(vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB), model.RoleUser, accessFixedTime)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	iss, err := repo.ListIssued(ctx, vo.MustParseId(accessUserA))
	if err != nil {
		t.Fatalf("ListIssued: %v", err)
	}
	if len(iss) != 1 || iss[0].UserID.String() != accessUserB {
		t.Fatalf("want 1 issued grant to userB, got %+v", iss)
	}
	none, err := repo.ListIssued(ctx, vo.MustParseId(accessUserB))
	if err != nil {
		t.Fatalf("ListIssued userB: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("userB owns no accounts, want 0 issued grants, got %d", len(none))
	}
}

func TestAccessRepo_DeleteOption(t *testing.T) {
	repo, db, f := newAccessRepo(t)
	ctx := context.Background()
	f.AccountOption(accessAcctA, accessUserB, 0)
	if err := repo.DeleteOption(ctx, vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB)); err != nil {
		t.Fatalf("DeleteOption: %v", err)
	}
	var n int
	if err := db.Raw.QueryRowContext(ctx, db.Rebind(`SELECT COUNT(*) FROM accounts_options WHERE account_id = ? AND user_id = ?`), accessAcctA, accessUserB).Scan(&n); err != nil {
		t.Fatalf("count options: %v", err)
	}
	if n != 0 {
		t.Errorf("want option row deleted, still %d", n)
	}
	// Second call is a no-op (idempotent delete).
	if err := repo.DeleteOption(ctx, vo.MustParseId(accessAcctA), vo.MustParseId(accessUserB)); err != nil {
		t.Fatalf("DeleteOption idempotent: %v", err)
	}
}

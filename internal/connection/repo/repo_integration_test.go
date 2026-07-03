package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	domconnection "github.com/econumo/econumo/internal/connection"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	usdID = "dffc2a06-6f29-4704-8575-31709adee926"
	userA = "11111111-1111-1111-1111-111111111111"
	userB = "22222222-2222-2222-2222-222222222222"
	acctA = "aaaa1111-0000-0000-0000-0000000000a1"
	acctB = "bbbb1111-0000-0000-0000-0000000000b1"
)

var fixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

func seedUser(t *testing.T, f *fixture.Builder, id string) {
	t.Helper()
	f.User(fixture.User{ID: id, Name: "u"})
}

func seedAccount(t *testing.T, f *fixture.Builder, id, userID string) {
	t.Helper()
	f.Account(fixture.Account{ID: id, UserID: userID, CurrencyID: usdID, Name: "A", Type: 2, Icon: "x"})
}

func newRepo(t *testing.T) (*connectionrepo.Repo, *dbtest.DB, *fixture.Builder) {
	t.Helper()
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	seedUser(t, f, userA)
	seedUser(t, f, userB)
	seedAccount(t, f, acctA, userA)
	seedAccount(t, f, acctB, userB)
	return connectionrepo.NewRepo("sqlite", db.TX), db, f
}

func TestConnectionRepo_GrantCRUD(t *testing.T) {
	repo, _, _ := newRepo(t)
	ctx := context.Background()
	// userA grants access on acctA to userB.
	access := domconnection.FromState(vo.MustParseId(acctA), vo.MustParseId(userB), domconnection.RoleUser, fixedTime, fixedTime)
	if err := repo.Save(ctx, access); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.Get(ctx, vo.MustParseId(acctA), vo.MustParseId(userB))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Role() != domconnection.RoleUser {
		t.Errorf("role mismatch: %d", got.Role())
	}
	if !got.CreatedAt().Equal(fixedTime) {
		t.Errorf("createdAt mismatch: %v", got.CreatedAt())
	}

	if err := repo.Delete(ctx, vo.MustParseId(acctA), vo.MustParseId(userB)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = repo.Get(ctx, vo.MustParseId(acctA), vo.MustParseId(userB))
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound after delete, got %v", err)
	}
}

func TestConnectionRepo_Get_NotFound(t *testing.T) {
	repo, _, _ := newRepo(t)
	_, err := repo.Get(context.Background(), vo.MustParseId(acctA), vo.MustParseId(userB))
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestConnectionRepo_ReceivedIssuedByAccount(t *testing.T) {
	repo, _, _ := newRepo(t)
	ctx := context.Background()
	// userA grants access on acctA to userB.
	_ = repo.Save(ctx, domconnection.FromState(vo.MustParseId(acctA), vo.MustParseId(userB), domconnection.RoleUser, fixedTime, fixedTime))

	// userB RECEIVED the grant.
	recv, err := repo.ListReceived(ctx, vo.MustParseId(userB))
	if err != nil {
		t.Fatalf("ListReceived: %v", err)
	}
	if len(recv) != 1 || recv[0].AccountId().String() != acctA {
		t.Fatalf("want 1 received grant on acctA, got %+v", recv)
	}

	// userA ISSUED the grant (owns acctA).
	iss, err := repo.ListIssued(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("ListIssued: %v", err)
	}
	if len(iss) != 1 || iss[0].UserId().String() != userB {
		t.Fatalf("want 1 issued grant to userB, got %+v", iss)
	}

	// acctA has 1 grant.
	byAcct, err := repo.ListByAccount(ctx, vo.MustParseId(acctA))
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(byAcct) != 1 {
		t.Errorf("want 1 grant on acctA, got %d", len(byAcct))
	}

	// userA received none.
	none, err := repo.ListReceived(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("ListReceived userA: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("userA should have received no grants, got %d", len(none))
	}
}

func TestConnectionRepo_AccountOwner(t *testing.T) {
	repo, _, _ := newRepo(t)
	ctx := context.Background()
	owner, err := repo.AccountOwner(ctx, vo.MustParseId(acctA))
	if err != nil {
		t.Fatalf("AccountOwner: %v", err)
	}
	if owner.String() != userA {
		t.Errorf("want owner %s, got %s", userA, owner)
	}
	_, err = repo.AccountOwner(ctx, vo.NewId())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound for missing account, got %v", err)
	}
}

func TestConnectionRepo_ConnectUsersAndLinks(t *testing.T) {
	repo, _, _ := newRepo(t)
	ctx := context.Background()
	if err := repo.ConnectUsers(ctx, vo.MustParseId(userA), vo.MustParseId(userB)); err != nil {
		t.Fatalf("ConnectUsers: %v", err)
	}
	// Idempotent.
	if err := repo.ConnectUsers(ctx, vo.MustParseId(userA), vo.MustParseId(userB)); err != nil {
		t.Fatalf("ConnectUsers idempotent: %v", err)
	}

	connected, err := repo.ConnectedUserIDs(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("ConnectedUserIDs: %v", err)
	}
	if len(connected) != 1 || connected[0].String() != userB {
		t.Fatalf("want userA connected to userB, got %+v", connected)
	}
	// Symmetric.
	connectedB, _ := repo.ConnectedUserIDs(ctx, vo.MustParseId(userB))
	if len(connectedB) != 1 || connectedB[0].String() != userA {
		t.Fatalf("want symmetric link from userB, got %+v", connectedB)
	}

	if err := repo.DeleteConnection(ctx, vo.MustParseId(userA), vo.MustParseId(userB)); err != nil {
		t.Fatalf("DeleteConnection: %v", err)
	}
	after, _ := repo.ConnectedUserIDs(ctx, vo.MustParseId(userA))
	if len(after) != 0 {
		t.Errorf("want no links after delete, got %d", len(after))
	}
}

func TestConnectionRepo_DeleteOption(t *testing.T) {
	repo, db, f := newRepo(t)
	ctx := context.Background()
	f.AccountOption(acctA, userB, 0)
	if err := repo.DeleteOption(ctx, vo.MustParseId(acctA), vo.MustParseId(userB)); err != nil {
		t.Fatalf("DeleteOption: %v", err)
	}
	var n int
	if err := db.Raw.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts_options WHERE account_id = ? AND user_id = ?`, acctA, userB).Scan(&n); err != nil {
		t.Fatalf("count options: %v", err)
	}
	if n != 0 {
		t.Errorf("want option row deleted, still %d", n)
	}
}

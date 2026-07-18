// Integration tests for the account-access use cases (grant/accept/decline/
// revoke): real sqlite repos + fixtures, exercising the full tx-bound flow
// rather than stubbing AccessStore.
package account_test

import (
	"context"
	"errors"
	"testing"

	appaccount "github.com/econumo/econumo/internal/account"
	accountrepo "github.com/econumo/econumo/internal/account/repo"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// newAccessSvc wires a real account.Service over sqlite repos for the given db.
func newAccessSvc(t *testing.T, db *dbtest.DB) *appaccount.Service {
	t.Helper()
	txm := db.TX
	repo := accountrepo.NewRepo(db.Engine, txm)
	folderRepo := accountrepo.NewFolderRepo(db.Engine, txm)
	accessRepo := accountrepo.NewAccessRepo(db.Engine, txm)
	curLookup := currencyrepo.New(db.Engine, txm)
	accCur := server.NewAccountCurrencyLookup(curLookup)
	accUser := server.NewUserOwnerLookup(userrepo.NewRepo(db.Engine, txm))
	opGuard := operationrepo.NewGuard(db.Engine, txm)
	return appaccount.NewService(repo, folderRepo, accessRepo, accCur, accUser, txm, opGuard, clock.New())
}

const (
	accessOwnerID = "11111111-1111-1111-1111-111111111111"
	accessUserBID = "22222222-2222-2222-2222-222222222222"
	accessUserCID = "33333333-3333-3333-3333-333333333333"
)

// accessFixture seeds an owner + two other users; returns the builder plus the
// account id owned by accessOwnerID.
func accessFixture(t *testing.T, db *dbtest.DB) (*fixture.Builder, string) {
	t.Helper()
	f := fixture.New(t, db)
	f.User(fixture.User{ID: accessOwnerID, Name: "Owner"})
	f.User(fixture.User{ID: accessUserBID, Name: "UserB"})
	f.User(fixture.User{ID: accessUserCID, Name: "UserC"})
	acctID := f.Account(fixture.Account{UserID: accessOwnerID, Name: "Shared"})
	return f, acctID
}

func mustParseID(t *testing.T, s string) vo.Id {
	t.Helper()
	id, err := vo.ParseId(s)
	if err != nil {
		t.Fatalf("parse id %q: %v", s, err)
	}
	return id
}

func TestGrantAccess_CreatesPendingWithoutPlacement(t *testing.T) {
	db := dbtest.New(t)
	_, acctID := accessFixture(t, db)
	svc := newAccessSvc(t, db)
	ctx := context.Background()

	_, err := svc.GrantAccess(ctx, mustParseID(t, accessOwnerID), model.GrantAccountAccessRequest{
		AccountId: acctID, UserId: accessUserBID, Role: "user",
	})
	if err != nil {
		t.Fatalf("GrantAccess: %v", err)
	}

	accessRepo := accountrepo.NewAccessRepo(db.Engine, db.TX)
	grant, gerr := accessRepo.Get(ctx, mustParseID(t, acctID), mustParseID(t, accessUserBID))
	if gerr != nil {
		t.Fatalf("Get grant: %v", gerr)
	}
	if grant.IsAccepted {
		t.Errorf("new grant IsAccepted = true, want false (pending)")
	}
	if grant.Role != model.RoleUser {
		t.Errorf("role = %v, want RoleUser", grant.Role)
	}

	var n int
	if err := db.Raw.QueryRow(db.Rebind(`SELECT COUNT(*) FROM accounts_options WHERE account_id = ? AND user_id = ?`), acctID, accessUserBID).Scan(&n); err != nil {
		t.Fatalf("count options: %v", err)
	}
	if n != 0 {
		t.Errorf("accounts_options rows for pending grantee = %d, want 0", n)
	}
	memberships, merr := accountrepo.NewFolderRepo(db.Engine, db.TX).MembershipsByUser(ctx, mustParseID(t, accessUserBID))
	if merr != nil {
		t.Fatalf("MembershipsByUser: %v", merr)
	}
	for _, ids := range memberships {
		for _, id := range ids {
			if id == acctID {
				t.Fatalf("pending grantee has a folder membership for the account, want none")
			}
		}
	}
}

func TestGrantAccess_UpdateRoleKeepsAcceptance(t *testing.T) {
	db := dbtest.New(t)
	f, acctID := accessFixture(t, db)
	f.AccountAccess(acctID, accessUserBID, int(model.RoleUser))
	svc := newAccessSvc(t, db)
	ctx := context.Background()

	_, err := svc.GrantAccess(ctx, mustParseID(t, accessOwnerID), model.GrantAccountAccessRequest{
		AccountId: acctID, UserId: accessUserBID, Role: "admin",
	})
	if err != nil {
		t.Fatalf("GrantAccess (update): %v", err)
	}

	accessRepo := accountrepo.NewAccessRepo(db.Engine, db.TX)
	grant, gerr := accessRepo.Get(ctx, mustParseID(t, acctID), mustParseID(t, accessUserBID))
	if gerr != nil {
		t.Fatalf("Get grant: %v", gerr)
	}
	if grant.Role != model.RoleAdmin {
		t.Errorf("role = %v, want RoleAdmin", grant.Role)
	}
	if !grant.IsAccepted {
		t.Errorf("IsAccepted = false, want true (untouched by a role update)")
	}
}

func TestGrantAccess_RequiresOwnerOrAcceptedAdmin(t *testing.T) {
	db := dbtest.New(t)
	f, acctID := accessFixture(t, db)
	f.AccountAccessPending(acctID, accessUserBID, int(model.RoleAdmin))
	svc := newAccessSvc(t, db)
	ctx := context.Background()

	_, err := svc.GrantAccess(ctx, mustParseID(t, accessUserBID), model.GrantAccountAccessRequest{
		AccountId: acctID, UserId: accessUserCID, Role: "user",
	})
	if _, ok := errs.AsAccessDenied(err); !ok {
		t.Fatalf("pending admin grant: GrantAccess err = %v, want AccessDeniedError", err)
	}

	// Accept userB's own grant directly against the repo, then retry.
	accessRepo := accountrepo.NewAccessRepo(db.Engine, db.TX)
	grant, gerr := accessRepo.Get(ctx, mustParseID(t, acctID), mustParseID(t, accessUserBID))
	if gerr != nil {
		t.Fatalf("Get grant: %v", gerr)
	}
	grant.Accept(clock.New().Now())
	if serr := accessRepo.Save(ctx, grant); serr != nil {
		t.Fatalf("Save accepted grant: %v", serr)
	}

	if _, err := svc.GrantAccess(ctx, mustParseID(t, accessUserBID), model.GrantAccountAccessRequest{
		AccountId: acctID, UserId: accessUserCID, Role: "user",
	}); err != nil {
		t.Fatalf("GrantAccess after accept: %v", err)
	}
}

func TestAcceptAccess_PlacesIntoChosenFolder(t *testing.T) {
	db := dbtest.New(t)
	f, acctID := accessFixture(t, db)
	f.AccountAccessPending(acctID, accessUserBID, int(model.RoleUser))
	folderID := f.Folder(fixture.Folder{UserID: accessUserBID, Name: "Mine"})
	svc := newAccessSvc(t, db)
	ctx := context.Background()

	_, err := svc.AcceptAccess(ctx, mustParseID(t, accessUserBID), model.AcceptAccountAccessRequest{
		AccountId: acctID, FolderId: folderID,
	})
	if err != nil {
		t.Fatalf("AcceptAccess: %v", err)
	}

	accessRepo := accountrepo.NewAccessRepo(db.Engine, db.TX)
	grant, gerr := accessRepo.Get(ctx, mustParseID(t, acctID), mustParseID(t, accessUserBID))
	if gerr != nil {
		t.Fatalf("Get grant: %v", gerr)
	}
	if !grant.IsAccepted {
		t.Errorf("IsAccepted = false, want true after accept")
	}

	var pos int
	if err := db.Raw.QueryRow(db.Rebind(`SELECT position FROM accounts_options WHERE account_id = ? AND user_id = ?`), acctID, accessUserBID).Scan(&pos); err != nil {
		t.Fatalf("read position: %v", err)
	}
	if pos != 1 {
		t.Errorf("position = %d, want 1 (max 0 + 1)", pos)
	}

	folderRepo := accountrepo.NewFolderRepo(db.Engine, db.TX)
	memberships, merr := folderRepo.MembershipsByUser(ctx, mustParseID(t, accessUserBID))
	if merr != nil {
		t.Fatalf("MembershipsByUser: %v", merr)
	}
	found := false
	for _, id := range memberships[folderID] {
		if id == acctID {
			found = true
		}
	}
	if !found {
		t.Errorf("account not in chosen folder %s: memberships=%v", folderID, memberships)
	}

	list, lerr := svc.GetAccountList(ctx, mustParseID(t, accessUserBID))
	if lerr != nil {
		t.Fatalf("GetAccountList: %v", lerr)
	}
	if len(list.Items) != 1 {
		t.Fatalf("items=%d want 1", len(list.Items))
	}
	if list.Items[0].FolderId == nil || *list.Items[0].FolderId != folderID {
		t.Errorf("folderId=%v want %q (a normal, non-pending entry)", list.Items[0].FolderId, folderID)
	}
}

func TestAcceptAccess_ForeignFolderDenied(t *testing.T) {
	db := dbtest.New(t)
	f, acctID := accessFixture(t, db)
	f.AccountAccessPending(acctID, accessUserBID, int(model.RoleUser))
	ownerFolderID := f.Folder(fixture.Folder{UserID: accessOwnerID, Name: "Owner Folder"})
	svc := newAccessSvc(t, db)
	ctx := context.Background()

	_, err := svc.AcceptAccess(ctx, mustParseID(t, accessUserBID), model.AcceptAccountAccessRequest{
		AccountId: acctID, FolderId: ownerFolderID,
	})
	if _, ok := errs.AsAccessDenied(err); !ok {
		t.Fatalf("AcceptAccess with foreign folder err = %v, want AccessDeniedError", err)
	}

	accessRepo := accountrepo.NewAccessRepo(db.Engine, db.TX)
	grant, gerr := accessRepo.Get(ctx, mustParseID(t, acctID), mustParseID(t, accessUserBID))
	if gerr != nil {
		t.Fatalf("Get grant: %v", gerr)
	}
	if grant.IsAccepted {
		t.Errorf("grant IsAccepted = true, want still pending after denied accept")
	}
}

func TestAcceptAccess_NoFoldersCreatesGeneral(t *testing.T) {
	db := dbtest.New(t)
	f, acctID := accessFixture(t, db)
	f.AccountAccessPending(acctID, accessUserBID, int(model.RoleUser))
	svc := newAccessSvc(t, db)
	ctx := context.Background()

	_, err := svc.AcceptAccess(ctx, mustParseID(t, accessUserBID), model.AcceptAccountAccessRequest{
		AccountId: acctID, FolderId: "",
	})
	if err != nil {
		t.Fatalf("AcceptAccess (no folders, blank folderId): %v", err)
	}

	var name string
	if err := db.Raw.QueryRow(db.Rebind(`SELECT name FROM folders WHERE user_id = ?`), accessUserBID).Scan(&name); err != nil {
		t.Fatalf("read created folder: %v", err)
	}
	if name != "General" {
		t.Errorf("created folder name = %q, want %q", name, "General")
	}

	folderRepo := accountrepo.NewFolderRepo(db.Engine, db.TX)
	memberships, merr := folderRepo.MembershipsByUser(ctx, mustParseID(t, accessUserBID))
	if merr != nil {
		t.Fatalf("MembershipsByUser: %v", merr)
	}
	found := false
	for _, ids := range memberships {
		for _, id := range ids {
			if id == acctID {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("account not placed in the auto-created General folder: memberships=%v", memberships)
	}
}

func TestAcceptAccess_NoPendingDenied(t *testing.T) {
	db := dbtest.New(t)
	f, acctID := accessFixture(t, db)
	svc := newAccessSvc(t, db)
	ctx := context.Background()

	// No row at all.
	_, err := svc.AcceptAccess(ctx, mustParseID(t, accessUserBID), model.AcceptAccountAccessRequest{AccountId: acctID})
	if _, ok := errs.AsAccessDenied(err); !ok {
		t.Fatalf("AcceptAccess with no grant err = %v, want AccessDeniedError", err)
	}

	// Already-accepted row.
	f.AccountAccess(acctID, accessUserCID, int(model.RoleUser))
	_, err = svc.AcceptAccess(ctx, mustParseID(t, accessUserCID), model.AcceptAccountAccessRequest{AccountId: acctID})
	if _, ok := errs.AsAccessDenied(err); !ok {
		t.Fatalf("AcceptAccess with already-accepted grant err = %v, want AccessDeniedError", err)
	}
}

func TestDeclineAccess_RemovesOwnRow(t *testing.T) {
	db := dbtest.New(t)
	f, acctID := accessFixture(t, db)
	f.AccountAccessPending(acctID, accessUserBID, int(model.RoleUser))
	svc := newAccessSvc(t, db)
	ctx := context.Background()

	if _, err := svc.DeclineAccess(ctx, mustParseID(t, accessUserBID), model.DeclineAccountAccessRequest{AccountId: acctID}); err != nil {
		t.Fatalf("DeclineAccess (pending): %v", err)
	}
	accessRepo := accountrepo.NewAccessRepo(db.Engine, db.TX)
	if _, gerr := accessRepo.Get(ctx, mustParseID(t, acctID), mustParseID(t, accessUserBID)); gerr == nil {
		t.Fatalf("grant still present after declining a pending row")
	} else if _, ok := errs.AsNotFound(gerr); !ok {
		t.Fatalf("Get after decline err = %v, want NotFoundError", gerr)
	}

	// Accepted row, with folder/options placement, declined the same way.
	f.AccountAccess(acctID, accessUserCID, int(model.RoleUser))
	folderID := f.Folder(fixture.Folder{UserID: accessUserCID, Name: "Mine"})
	f.AccountInFolder(folderID, acctID)
	f.AccountOption(acctID, accessUserCID, 1)

	if _, err := svc.DeclineAccess(ctx, mustParseID(t, accessUserCID), model.DeclineAccountAccessRequest{AccountId: acctID}); err != nil {
		t.Fatalf("DeclineAccess (accepted): %v", err)
	}
	if _, gerr := accessRepo.Get(ctx, mustParseID(t, acctID), mustParseID(t, accessUserCID)); gerr == nil {
		t.Fatalf("grant still present after declining an accepted row")
	}
	var n int
	if err := db.Raw.QueryRow(db.Rebind(`SELECT COUNT(*) FROM accounts_options WHERE account_id = ? AND user_id = ?`), acctID, accessUserCID).Scan(&n); err != nil {
		t.Fatalf("count options: %v", err)
	}
	if n != 0 {
		t.Errorf("accounts_options rows after decline = %d, want 0", n)
	}
	folderRepo := accountrepo.NewFolderRepo(db.Engine, db.TX)
	memberships, merr := folderRepo.MembershipsByUser(ctx, mustParseID(t, accessUserCID))
	if merr != nil {
		t.Fatalf("MembershipsByUser: %v", merr)
	}
	for _, ids := range memberships {
		for _, id := range ids {
			if id == acctID {
				t.Fatalf("account still a folder member after decline: memberships=%v", memberships)
			}
		}
	}
}

func TestListPendingReceived_ExcludesSoftDeletedAccount(t *testing.T) {
	db := dbtest.New(t)
	f, acctID := accessFixture(t, db)
	f.AccountAccessPending(acctID, accessUserBID, int(model.RoleUser))
	svc := newAccessSvc(t, db)
	ctx := context.Background()

	if _, err := svc.DeleteAccount(ctx, mustParseID(t, accessOwnerID), model.DeleteAccountRequest{Id: acctID}); err != nil {
		t.Fatalf("DeleteAccount (owner soft-delete): %v", err)
	}

	list, err := svc.GetAccountList(ctx, mustParseID(t, accessUserBID))
	if err != nil {
		t.Fatalf("GetAccountList: %v", err)
	}
	for _, item := range list.Items {
		if item.Id == acctID {
			t.Fatalf("pending grant for a soft-deleted account still rides GetAccountList: items=%v", list.Items)
		}
	}
}

func TestAcceptAccess_DeletedAccountNotFound(t *testing.T) {
	db := dbtest.New(t)
	f, acctID := accessFixture(t, db)
	f.AccountAccessPending(acctID, accessUserBID, int(model.RoleUser))
	svc := newAccessSvc(t, db)
	ctx := context.Background()

	if _, err := svc.DeleteAccount(ctx, mustParseID(t, accessOwnerID), model.DeleteAccountRequest{Id: acctID}); err != nil {
		t.Fatalf("DeleteAccount (owner soft-delete): %v", err)
	}

	_, err := svc.AcceptAccess(ctx, mustParseID(t, accessUserBID), model.AcceptAccountAccessRequest{AccountId: acctID})
	if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("AcceptAccess on deleted account err = %v, want NotFoundError", err)
	}

	var n int
	if err := db.Raw.QueryRow(db.Rebind(`SELECT COUNT(*) FROM accounts_options WHERE account_id = ? AND user_id = ?`), acctID, accessUserBID).Scan(&n); err != nil {
		t.Fatalf("count options: %v", err)
	}
	if n != 0 {
		t.Errorf("accounts_options rows after denied accept = %d, want 0", n)
	}
	folderRepo := accountrepo.NewFolderRepo(db.Engine, db.TX)
	memberships, merr := folderRepo.MembershipsByUser(ctx, mustParseID(t, accessUserBID))
	if merr != nil {
		t.Fatalf("MembershipsByUser: %v", merr)
	}
	for _, ids := range memberships {
		for _, id := range ids {
			if id == acctID {
				t.Fatalf("account placed into a folder despite denied accept: memberships=%v", memberships)
			}
		}
	}
}

func TestGrantAccess_DeletedAccountNotFound(t *testing.T) {
	db := dbtest.New(t)
	_, acctID := accessFixture(t, db)
	svc := newAccessSvc(t, db)
	ctx := context.Background()

	if _, err := svc.DeleteAccount(ctx, mustParseID(t, accessOwnerID), model.DeleteAccountRequest{Id: acctID}); err != nil {
		t.Fatalf("DeleteAccount (owner soft-delete): %v", err)
	}

	_, err := svc.GrantAccess(ctx, mustParseID(t, accessOwnerID), model.GrantAccountAccessRequest{
		AccountId: acctID, UserId: accessUserBID, Role: "user",
	})
	if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("GrantAccess on deleted account err = %v, want NotFoundError", err)
	}
}

func TestDeclineAccess_DeletedAccountStillSucceeds(t *testing.T) {
	db := dbtest.New(t)
	f, acctID := accessFixture(t, db)
	f.AccountAccessPending(acctID, accessUserBID, int(model.RoleUser))
	svc := newAccessSvc(t, db)
	ctx := context.Background()

	if _, err := svc.DeleteAccount(ctx, mustParseID(t, accessOwnerID), model.DeleteAccountRequest{Id: acctID}); err != nil {
		t.Fatalf("DeleteAccount (owner soft-delete): %v", err)
	}

	if _, err := svc.DeclineAccess(ctx, mustParseID(t, accessUserBID), model.DeclineAccountAccessRequest{AccountId: acctID}); err != nil {
		t.Fatalf("DeclineAccess on a deleted account's pending row: %v", err)
	}
	accessRepo := accountrepo.NewAccessRepo(db.Engine, db.TX)
	if _, gerr := accessRepo.Get(ctx, mustParseID(t, acctID), mustParseID(t, accessUserBID)); gerr == nil {
		t.Fatalf("grant still present after declining a deleted account's pending row")
	} else if _, ok := errs.AsNotFound(gerr); !ok {
		t.Fatalf("Get after decline err = %v, want NotFoundError", gerr)
	}
}

func TestRevokeAccess_OwnerRemovesGrant(t *testing.T) {
	db := dbtest.New(t)
	f, acctID := accessFixture(t, db)
	f.AccountAccessPending(acctID, accessUserBID, int(model.RoleUser))
	svc := newAccessSvc(t, db)
	ctx := context.Background()

	if _, err := svc.RevokeAccess(ctx, mustParseID(t, accessOwnerID), model.RevokeAccountAccessRequest{
		AccountId: acctID, UserId: accessUserBID,
	}); err != nil {
		t.Fatalf("RevokeAccess (pending): %v", err)
	}
	accessRepo := accountrepo.NewAccessRepo(db.Engine, db.TX)
	if _, gerr := accessRepo.Get(ctx, mustParseID(t, acctID), mustParseID(t, accessUserBID)); gerr == nil {
		t.Fatalf("grant still present after revoking a pending row")
	}

	// Accepted grant with placement, unwound the same way.
	f.AccountAccess(acctID, accessUserCID, int(model.RoleUser))
	folderID := f.Folder(fixture.Folder{UserID: accessUserCID, Name: "Mine"})
	f.AccountInFolder(folderID, acctID)
	f.AccountOption(acctID, accessUserCID, 1)

	if _, err := svc.RevokeAccess(ctx, mustParseID(t, accessOwnerID), model.RevokeAccountAccessRequest{
		AccountId: acctID, UserId: accessUserCID,
	}); err != nil {
		t.Fatalf("RevokeAccess (accepted): %v", err)
	}
	if _, gerr := accessRepo.Get(ctx, mustParseID(t, acctID), mustParseID(t, accessUserCID)); gerr == nil {
		t.Fatalf("grant still present after revoking an accepted row")
	}
	var n int
	if err := db.Raw.QueryRow(db.Rebind(`SELECT COUNT(*) FROM accounts_options WHERE account_id = ? AND user_id = ?`), acctID, accessUserCID).Scan(&n); err != nil {
		t.Fatalf("count options: %v", err)
	}
	if n != 0 {
		t.Errorf("accounts_options rows after revoke = %d, want 0", n)
	}

	// Revoking a missing grant is NotFound.
	_, err := svc.RevokeAccess(ctx, mustParseID(t, accessOwnerID), model.RevokeAccountAccessRequest{
		AccountId: acctID, UserId: accessUserBID,
	})
	if _, ok := errs.AsNotFound(err); !ok {
		var nf *errs.NotFoundError
		if !errors.As(err, &nf) {
			t.Fatalf("RevokeAccess on missing grant err = %v, want NotFoundError", err)
		}
	}
}

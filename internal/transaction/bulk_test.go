package transaction_test

import (
	"context"
	"testing"

	appaccount "github.com/econumo/econumo/internal/account"
	accountrepo "github.com/econumo/econumo/internal/account/repo"
	appcategory "github.com/econumo/econumo/internal/category"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	apppayee "github.com/econumo/econumo/internal/payee"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	apptag "github.com/econumo/econumo/internal/tag"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	apptransaction "github.com/econumo/econumo/internal/transaction"
	transactionrepo "github.com/econumo/econumo/internal/transaction/repo"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// newWriteService wires a full transaction Service (matching
// internal/transaction/mcp/mcp_test.go's newTransactionService) so bulk
// update tests exercise the real checkWriteAccess/checkReferences path,
// including cross-feature owned-entity checks.
func newWriteService(t *testing.T, db *dbtest.DB) *apptransaction.Service {
	t.Helper()
	txm := db.TX
	curLookup := currencyrepo.New(db.Engine, txm)
	accessResolver := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(db.Engine, txm))
	accSvc := appaccount.NewService(
		accountrepo.NewRepo(db.Engine, txm), accountrepo.NewFolderRepo(db.Engine, txm), accountrepo.NewAccessRepo(db.Engine, txm),
		server.NewAccountCurrencyLookup(curLookup), server.NewUserOwnerLookup(userrepo.NewRepo(db.Engine, txm)),
		accessResolver, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(),
	)
	txRepo := transactionrepo.NewRepo(db.Engine, txm)
	catRepo := categoryrepo.NewRepo(db.Engine, txm)
	tgRepo := tagrepo.NewRepo(db.Engine, txm)
	pyRepo := payeerepo.NewRepo(db.Engine, txm)
	txExport := transactionrepo.NewExportLookup(txRepo, server.NewTransactionCategoryNameLookup(catRepo), server.NewTransactionTagNameLookup(tgRepo), server.NewTransactionPayeeNameLookup(pyRepo))
	catSvc := appcategory.NewService(catRepo, txm, catRepo, clock.New(), categoryrepo.NewReadRepo(db.Engine, txm), accessResolver)
	tgSvc := apptag.NewService(tgRepo, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(), tagrepo.NewReadRepo(db.Engine, txm), accessResolver)
	pySvc := apppayee.NewService(pyRepo, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(), payeerepo.NewReadRepo(db.Engine, txm), accessResolver)
	txImportAccounts := server.NewTransactionImportAccounts(accSvc, accountrepo.NewRepo(db.Engine, txm), accountrepo.NewFolderRepo(db.Engine, txm), curLookup, "USD")
	txImportCategories := server.NewTransactionImportCategories(catSvc, catRepo)
	txImportTags := server.NewTransactionImportTags(tgSvc, tgRepo)
	txImportPayees := server.NewTransactionImportPayees(pySvc, pyRepo)
	txImport := transactionrepo.NewImportLookup(txImportAccounts, accessResolver, txImportCategories, txImportPayees, txImportTags, txRepo)
	return apptransaction.NewService(
		txRepo, accSvc, accessResolver, accSvc,
		server.NewUserOwnerLookup(userrepo.NewRepo(db.Engine, txm)),
		txExport, txImport, txm, operationrepo.NewGuard(db.Engine, txm), clock.New(),
	)
}

func createExpense(t *testing.T, svc *apptransaction.Service, userID vo.Id, accountID, categoryID, date string) string {
	t.Helper()
	if len(date) == len("2024-04-01") {
		date += " 00:00:00"
	}
	res, err := svc.CreateTransaction(context.Background(), userID, model.CreateTransactionRequest{
		Id: vo.NewId().String(), Type: "expense", Amount: vo.NewFlexString("10.00"),
		AccountId: accountID, CategoryId: &categoryID, Date: date,
	})
	if err != nil {
		t.Fatalf("createExpense: %v", err)
	}
	return res.Item.Id
}

func TestBulkUpdateTransactions_HappyPath_SetsCategoryPayeeTag(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	accountID := f.Account(fixture.Account{UserID: userID})
	categoryID := f.Category(fixture.Category{UserID: userID, Type: 0})
	newCategoryID := f.Category(fixture.Category{UserID: userID, Type: 0})
	payeeID := f.Payee(fixture.Payee{UserID: userID})
	tagID := f.Tag(fixture.Tag{UserID: userID})

	svc := newWriteService(t, db)
	uid := mustID(t, userID)
	id1 := createExpense(t, svc, uid, accountID, categoryID, "2024-04-01")
	id2 := createExpense(t, svc, uid, accountID, categoryID, "2024-04-02")

	res, err := svc.BulkUpdateTransactions(context.Background(), uid, model.BulkUpdateTransactionsRequest{
		Ids: []string{id1, id2}, CategoryId: &newCategoryID, PayeeId: &payeeID, TagId: &tagID,
	})
	if err != nil {
		t.Fatalf("BulkUpdateTransactions: %v", err)
	}
	if res.Updated != 2 {
		t.Fatalf("Updated = %d, want 2", res.Updated)
	}

	list, err := svc.GetTransactionList(context.Background(), uid, model.TransactionListRequest{CategoryId: newCategoryID})
	if err != nil {
		t.Fatalf("GetTransactionList: %v", err)
	}
	if len(list.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(list.Items))
	}
	for _, it := range list.Items {
		if it.PayeeId == nil || *it.PayeeId != payeeID {
			t.Errorf("item %s payeeId = %v, want %q", it.Id, it.PayeeId, payeeID)
		}
		if it.TagId == nil || *it.TagId != tagID {
			t.Errorf("item %s tagId = %v, want %q", it.Id, it.TagId, tagID)
		}
	}
}

func TestBulkUpdateTransactions_ClearFields(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	accountID := f.Account(fixture.Account{UserID: userID})
	categoryID := f.Category(fixture.Category{UserID: userID, Type: 0})
	payeeID := f.Payee(fixture.Payee{UserID: userID})
	tagID := f.Tag(fixture.Tag{UserID: userID})

	svc := newWriteService(t, db)
	uid := mustID(t, userID)
	id1 := createExpense(t, svc, uid, accountID, categoryID, "2024-04-01")

	if _, err := svc.BulkUpdateTransactions(context.Background(), uid, model.BulkUpdateTransactionsRequest{
		Ids: []string{id1}, PayeeId: &payeeID, TagId: &tagID,
	}); err != nil {
		t.Fatalf("set payee/tag: %v", err)
	}

	res, err := svc.BulkUpdateTransactions(context.Background(), uid, model.BulkUpdateTransactionsRequest{
		Ids: []string{id1}, ClearPayee: true, ClearTag: true,
	})
	if err != nil {
		t.Fatalf("clear payee/tag: %v", err)
	}
	if res.Updated != 1 {
		t.Fatalf("Updated = %d, want 1", res.Updated)
	}

	list, err := svc.GetTransactionList(context.Background(), uid, model.TransactionListRequest{AccountId: accountID})
	if err != nil {
		t.Fatalf("GetTransactionList: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(list.Items))
	}
	if list.Items[0].PayeeId != nil || list.Items[0].TagId != nil {
		t.Fatalf("payee/tag not cleared: %#v", list.Items[0])
	}
	if list.Items[0].CategoryId == nil || *list.Items[0].CategoryId != categoryID {
		t.Fatalf("category unexpectedly changed: %#v", list.Items[0].CategoryId)
	}
}

func TestBulkUpdateTransactions_EmptyIds_Rejected(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	svc := newWriteService(t, db)

	_, err := svc.BulkUpdateTransactions(context.Background(), mustID(t, userID), model.BulkUpdateTransactionsRequest{
		Ids: nil, ClearPayee: true,
	})
	if _, ok := errs.AsValidation(err); !ok {
		t.Fatalf("want ValidationError, got %v (%T)", err, err)
	}
}

func TestBulkUpdateTransactions_TooManyIds_Rejected(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	svc := newWriteService(t, db)

	ids := make([]string, 101)
	for i := range ids {
		ids[i] = vo.NewId().String()
	}
	_, err := svc.BulkUpdateTransactions(context.Background(), mustID(t, userID), model.BulkUpdateTransactionsRequest{
		Ids: ids, ClearPayee: true,
	})
	ve, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("want ValidationError, got %v (%T)", err, err)
	}
	if ve.Msg != "at most 100 transactions per bulk update; batch the rest" {
		t.Fatalf("message = %q", ve.Msg)
	}
}

func TestBulkUpdateTransactions_NoChangeRequested_Rejected(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	svc := newWriteService(t, db)

	_, err := svc.BulkUpdateTransactions(context.Background(), mustID(t, userID), model.BulkUpdateTransactionsRequest{
		Ids: []string{vo.NewId().String()},
	})
	if _, ok := errs.AsValidation(err); !ok {
		t.Fatalf("want ValidationError, got %v (%T)", err, err)
	}
}

func TestBulkUpdateTransactions_SetAndClearSameField_Rejected(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	catID := vo.NewId().String()
	svc := newWriteService(t, db)

	_, err := svc.BulkUpdateTransactions(context.Background(), mustID(t, userID), model.BulkUpdateTransactionsRequest{
		Ids: []string{vo.NewId().String()}, CategoryId: &catID, ClearCategory: true,
	})
	if _, ok := errs.AsValidation(err); !ok {
		t.Fatalf("want ValidationError, got %v (%T)", err, err)
	}
}

func TestBulkUpdateTransactions_AccessDeniedID_RollsBackWholeBatch(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	ownerID := f.User(fixture.User{ID: "10000001-0000-0000-0000-000000000001"})
	strangerID := f.User(fixture.User{ID: "20000002-0000-0000-0000-000000000002"})
	accountID := f.Account(fixture.Account{UserID: ownerID})
	strangerAccountID := f.Account(fixture.Account{UserID: strangerID})
	categoryID := f.Category(fixture.Category{UserID: ownerID, Type: 0})
	strangerCategoryID := f.Category(fixture.Category{UserID: strangerID, Type: 0})
	newCategoryID := f.Category(fixture.Category{UserID: ownerID, Type: 0})

	svc := newWriteService(t, db)
	ownerUID := mustID(t, ownerID)
	strangerUID := mustID(t, strangerID)
	ownID := createExpense(t, svc, ownerUID, accountID, categoryID, "2024-04-01")
	strangerTxID := createExpense(t, svc, strangerUID, strangerAccountID, strangerCategoryID, "2024-04-01")

	_, err := svc.BulkUpdateTransactions(context.Background(), ownerUID, model.BulkUpdateTransactionsRequest{
		Ids: []string{ownID, strangerTxID}, CategoryId: &newCategoryID,
	})
	if _, ok := errs.AsValidation(err); !ok {
		t.Fatalf("want ValidationError (not_available), got %v (%T)", err, err)
	}

	// All-or-nothing: the owner's own transaction must be untouched too.
	list, lerr := svc.GetTransactionList(context.Background(), ownerUID, model.TransactionListRequest{AccountId: accountID})
	if lerr != nil {
		t.Fatalf("GetTransactionList: %v", lerr)
	}
	if len(list.Items) != 1 || list.Items[0].CategoryId == nil || *list.Items[0].CategoryId != categoryID {
		t.Fatalf("owner transaction changed despite rollback: %#v", list.Items)
	}
}

func TestBulkUpdateTransactions_ForeignCategoryID_RollsBackWholeBatch(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	ownerID := f.User(fixture.User{ID: "10000001-0000-0000-0000-000000000001"})
	strangerID := f.User(fixture.User{ID: "20000002-0000-0000-0000-000000000002"})
	accountID := f.Account(fixture.Account{UserID: ownerID})
	categoryID := f.Category(fixture.Category{UserID: ownerID, Type: 0})
	strangerCategoryID := f.Category(fixture.Category{UserID: strangerID, Type: 0})

	svc := newWriteService(t, db)
	ownerUID := mustID(t, ownerID)
	id1 := createExpense(t, svc, ownerUID, accountID, categoryID, "2024-04-01")

	_, err := svc.BulkUpdateTransactions(context.Background(), ownerUID, model.BulkUpdateTransactionsRequest{
		Ids: []string{id1}, CategoryId: &strangerCategoryID,
	})
	if _, ok := errs.AsValidation(err); !ok {
		t.Fatalf("want ValidationError (foreign category), got %v (%T)", err, err)
	}

	list, lerr := svc.GetTransactionList(context.Background(), ownerUID, model.TransactionListRequest{AccountId: accountID})
	if lerr != nil {
		t.Fatalf("GetTransactionList: %v", lerr)
	}
	if len(list.Items) != 1 || list.Items[0].CategoryId == nil || *list.Items[0].CategoryId != categoryID {
		t.Fatalf("category changed despite rejection: %#v", list.Items)
	}
}

func TestBulkUpdateTransactions_CategoryOnTransfer_Rejected(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	accountID := f.Account(fixture.Account{UserID: userID})
	accountID2 := f.Account(fixture.Account{UserID: userID})
	categoryID := f.Category(fixture.Category{UserID: userID, Type: 0})

	svc := newWriteService(t, db)
	uid := mustID(t, userID)
	transferRes, err := svc.CreateTransaction(context.Background(), uid, model.CreateTransactionRequest{
		Id: vo.NewId().String(), Type: "transfer", Amount: vo.NewFlexString("5.00"),
		AccountId: accountID, AccountRecipientId: &accountID2, Date: "2024-04-01 00:00:00",
	})
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}

	_, err = svc.BulkUpdateTransactions(context.Background(), uid, model.BulkUpdateTransactionsRequest{
		Ids: []string{transferRes.Item.Id}, CategoryId: &categoryID,
	})
	if _, ok := errs.AsValidation(err); !ok {
		t.Fatalf("want ValidationError (category on transfer), got %v (%T)", err, err)
	}

	list, lerr := svc.GetTransactionList(context.Background(), uid, model.TransactionListRequest{AccountId: accountID})
	if lerr != nil {
		t.Fatalf("GetTransactionList: %v", lerr)
	}
	if len(list.Items) != 1 || list.Items[0].CategoryId != nil {
		t.Fatalf("transfer unexpectedly gained a category: %#v", list.Items)
	}
}

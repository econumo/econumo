package repo_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	transactionrepo "github.com/econumo/econumo/internal/transaction/repo"
)

const (
	usdID  = "dffc2a06-6f29-4704-8575-31709adee926"
	userA  = "11111111-1111-1111-1111-111111111111"
	userB  = "22222222-2222-2222-2222-222222222222"
	acct1  = "aaaa1111-0000-0000-0000-0000000000a1"
	acct2  = "aaaa1111-0000-0000-0000-0000000000a2"
	acctB  = "bbbb1111-0000-0000-0000-0000000000b1"
	label1 = "1ab00000-0000-0000-0000-0000000000a1"
	label2 = "1ab00000-0000-0000-0000-0000000000a2"
	label3 = "1ab00000-0000-0000-0000-0000000000a3"
)

var fixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

func setup(t *testing.T) (*transactionrepo.Repo, *dbtest.DB) {
	t.Helper()
	db := dbtest.New(t)
	seedUser(t, db, userA)
	seedAccount(t, db, acct1, userA)
	seedAccount(t, db, acct2, userA)
	return transactionrepo.NewRepo(db.Engine, db.TX), db
}

func seedUser(t *testing.T, db *dbtest.DB, id string) {
	t.Helper()
	fixture.New(t, db).User(fixture.User{ID: id, Name: "u"})
}

func seedAccount(t *testing.T, db *dbtest.DB, id, userID string) {
	t.Helper()
	fixture.New(t, db).Account(fixture.Account{ID: id, CurrencyID: usdID, UserID: userID, Name: "A", Icon: "x"})
}

func seedLabel(t *testing.T, db *dbtest.DB, id, userID string) {
	t.Helper()
	fixture.New(t, db).Label(fixture.Label{ID: id, UserID: userID})
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func expense(id, account, amount string, spentAt time.Time) *model.Transaction {
	return model.FromState(model.NewState{
		ID: vo.MustParseId(id), UserID: vo.MustParseId(userA), Type: model.TransactionTypeExpense,
		AccountID: vo.MustParseId(account), Amount: amount, Description: "exp",
		SpentAt: spentAt, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	})
}

func TestTransactionRepo_SaveGetRoundTrip_Expense(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	id := "7c000000-0000-0000-0000-000000000001"
	spent := time.Date(2024, 3, 15, 9, 30, 0, 0, time.UTC)
	// SQLite's NUMERIC affinity stores the value exactly; trailing zeros on a
	// fractional literal are normalized off ("123.45000000" -> "123.45"), so use a
	// value with no trailing-zero ambiguity and assert it survives byte-exact.
	if err := repo.Save(ctx, expense(id, acct1, "123.45", spent)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(id))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if vo.NewDecimal(got.Amount).String() != vo.NewDecimal("123.45").String() {
		t.Errorf("amount mismatch: %q", got.Amount)
	}
	if got.Type != model.TransactionTypeExpense || got.AccountID.String() != acct1 {
		t.Errorf("fields mismatch: type=%d account=%s", got.Type, got.AccountID)
	}
	if !got.SpentAt.Equal(spent) {
		t.Errorf("spentAt mismatch: %v", got.SpentAt)
	}
	if got.AccountRecipID != nil {
		t.Error("expense should have no recipient")
	}
}

func TestTransactionRepo_SaveGetRoundTrip_Transfer(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	id := "7c000000-0000-0000-0000-000000000002"
	recip := vo.MustParseId(acct2)
	amtRecip := "90.5"
	tx := model.FromState(model.NewState{
		ID: vo.MustParseId(id), UserID: vo.MustParseId(userA), Type: model.TransactionTypeTransfer,
		AccountID: vo.MustParseId(acct1), AccountRecipID: &recip,
		Amount: "100.25", AmountRecipient: &amtRecip, Description: "xfer",
		SpentAt: fixedTime, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	})
	if err := repo.Save(ctx, tx); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(id))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if vo.NewDecimal(got.Amount).String() != vo.NewDecimal("100.25").String() {
		t.Errorf("amount mismatch: %q", got.Amount)
	}
	if got.AccountRecipID == nil || got.AccountRecipID.String() != acct2 {
		t.Errorf("recipient mismatch: %v", got.AccountRecipID)
	}
	if got.AmountRecipient == nil || vo.NewDecimal(*got.AmountRecipient).String() != vo.NewDecimal("90.5").String() {
		t.Errorf("amount_recipient mismatch: %v", deref(got.AmountRecipient))
	}
}

// seedRecurringTemplate inserts a template row directly: recurring_id on
// transactions is a real FK, so LinkRecurring needs a target to point at.
func seedRecurringTemplate(t *testing.T, db *dbtest.DB, id, userID, accountID string) {
	t.Helper()
	q := db.Rebind(`INSERT INTO recurring_transactions
		(id, user_id, account_id, type, amount, description, schedule, next_payment_at, scheduled_day, created_at, updated_at)
		VALUES (?, ?, ?, 0, '10', 'rent', 'monthly', ?, 1, ?, ?)`)
	if _, err := db.Raw.Exec(q, id, userID, accountID, fixedTime, fixedTime, fixedTime); err != nil {
		t.Fatalf("seed recurring template: %v", err)
	}
}

func TestTransactionRepo_LinkRecurring(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	const txID = "7c000000-0000-0000-0000-000000000009"
	const rtID = "7c000000-0000-0000-0000-00000000000a"
	seedRecurringTemplate(t, db, rtID, userA, acct1)
	if err := repo.Save(ctx, expense(txID, acct1, "10.5", fixedTime)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	linkedAt := fixedTime.Add(time.Hour)
	if err := repo.LinkRecurring(ctx, vo.MustParseId(txID), vo.MustParseId(rtID), linkedAt); err != nil {
		t.Fatalf("LinkRecurring: %v", err)
	}

	got, err := repo.GetByID(ctx, vo.MustParseId(txID))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.RecurringID == nil || got.RecurringID.String() != rtID {
		t.Fatalf("RecurringID = %v, want %s", got.RecurringID, rtID)
	}
	if !got.UpdatedAt.Equal(linkedAt) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, linkedAt)
	}

	// An ordinary Save must not clear the link (upsert skips recurring_id).
	got.Update(model.NewState{
		ID: got.ID, UserID: got.UserID, Type: got.Type, AccountID: got.AccountID,
		Amount: "11.5", Description: "edited", SpentAt: got.SpentAt,
		CreatedAt: got.CreatedAt, UpdatedAt: got.UpdatedAt,
	}, linkedAt.Add(time.Hour))
	if err := repo.Save(ctx, got); err != nil {
		t.Fatalf("Save after link: %v", err)
	}
	again, err := repo.GetByID(ctx, vo.MustParseId(txID))
	if err != nil {
		t.Fatalf("GetByID after edit: %v", err)
	}
	if again.RecurringID == nil || again.RecurringID.String() != rtID {
		t.Fatalf("edit cleared the link: RecurringID = %v", again.RecurringID)
	}
}

func TestTransactionRepo_GetByID_NotFound(t *testing.T) {
	repo, _ := setup(t)
	_, err := repo.GetByID(context.Background(), vo.NewId())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestTransactionRepo_Delete(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	id := "7c000000-0000-0000-0000-000000000003"
	if err := repo.Save(ctx, expense(id, acct1, "1.00000000", fixedTime)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Delete(ctx, vo.MustParseId(id)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := repo.GetByID(ctx, vo.MustParseId(id))
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound after delete, got %v", err)
	}
}

func TestTransactionRepo_ListByAccount_SourceOrRecipient(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	// One expense on acct1, one transfer acct2 -> acct1 (acct1 as recipient).
	_ = repo.Save(ctx, expense("7c000000-0000-0000-0000-000000000004", acct1, "5.00000000", fixedTime))
	recip := vo.MustParseId(acct1)
	amtR := "7.00000000"
	_ = repo.Save(ctx, model.FromState(model.NewState{
		ID: vo.MustParseId("7c000000-0000-0000-0000-000000000005"), UserID: vo.MustParseId(userA),
		Type: model.TransactionTypeTransfer, AccountID: vo.MustParseId(acct2), AccountRecipID: &recip,
		Amount: "7.00000000", AmountRecipient: &amtR, Description: "x",
		SpentAt: fixedTime, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}))

	list, err := repo.ListByAccount(ctx, vo.MustParseId(acct1))
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("want 2 (source + recipient), got %d", len(list))
	}
}

func TestTransactionRepo_ListByAccountIDs_PeriodBoundary(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	// Three transactions: one on the first of the month (boundary), one mid-month,
	// one the previous month. The period [Mar 1, Apr 1) must INCLUDE the Mar 1
	// boundary row and EXCLUDE the Feb row.
	_ = repo.Save(ctx, expense("7c000000-0000-0000-0000-000000000006", acct1, "1.00000000", time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)))
	_ = repo.Save(ctx, expense("7c000000-0000-0000-0000-000000000007", acct1, "2.00000000", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)))
	_ = repo.Save(ctx, expense("7c000000-0000-0000-0000-000000000008", acct1, "3.00000000", time.Date(2024, 2, 28, 0, 0, 0, 0, time.UTC)))

	start := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	list, err := repo.ListByAccountIDs(ctx, []vo.Id{vo.MustParseId(acct1)}, model.TransactionFilter{PeriodStart: start, PeriodEnd: end})
	if err != nil {
		t.Fatalf("ListByAccountIDs: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 in-period (incl. Mar 1 boundary), got %d", len(list))
	}

	// No period -> all three.
	all, err := repo.ListByAccountIDs(ctx, []vo.Id{vo.MustParseId(acct1)}, model.TransactionFilter{})
	if err != nil {
		t.Fatalf("ListByAccountIDs no period: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("want 3 without period, got %d", len(all))
	}

	// Empty id set -> nil.
	none, err := repo.ListByAccountIDs(ctx, nil, model.TransactionFilter{PeriodStart: start, PeriodEnd: end})
	if err != nil || none != nil {
		t.Errorf("empty ids should yield nil,nil; got %v, %v", none, err)
	}
}

func TestTransactionRepo_ListByAccountIDs_ClassificationFilters(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	catA := vo.MustParseId(f.Category(fixture.Category{UserID: userA, Type: 0}))
	catB := vo.MustParseId(f.Category(fixture.Category{UserID: userA, Type: 0}))
	payeeA := vo.MustParseId(f.Payee(fixture.Payee{UserID: userA}))
	tagA := vo.MustParseId(f.Tag(fixture.Tag{UserID: userA}))

	withCat := func(id string, cat *vo.Id, payee *vo.Id, tag *vo.Id) *model.Transaction {
		return model.FromState(model.NewState{
			ID: vo.MustParseId(id), UserID: vo.MustParseId(userA), Type: model.TransactionTypeExpense,
			AccountID: vo.MustParseId(acct1), Amount: "1.00", Description: "x",
			CategoryID: cat, PayeeID: payee, TagID: tag,
			SpentAt: fixedTime, CreatedAt: fixedTime, UpdatedAt: fixedTime,
		})
	}
	mustSave := func(tx *model.Transaction) {
		t.Helper()
		if err := repo.Save(ctx, tx); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	mustSave(withCat("7c000000-0000-0000-0000-000000000010", &catA, &payeeA, &tagA))
	mustSave(withCat("7c000000-0000-0000-0000-000000000011", &catB, nil, nil))
	mustSave(withCat("7c000000-0000-0000-0000-000000000012", nil, nil, nil))

	ids := []vo.Id{vo.MustParseId(acct1)}

	uncategorized, err := repo.ListByAccountIDs(ctx, ids, model.TransactionFilter{Uncategorized: true})
	if err != nil {
		t.Fatalf("uncategorized filter: %v", err)
	}
	if len(uncategorized) != 1 || uncategorized[0].ID.String() != "7c000000-0000-0000-0000-000000000012" {
		t.Fatalf("uncategorized filter = %#v, want just tx 12", uncategorized)
	}

	byCategory, err := repo.ListByAccountIDs(ctx, ids, model.TransactionFilter{CategoryID: &catA})
	if err != nil {
		t.Fatalf("category filter: %v", err)
	}
	if len(byCategory) != 1 || byCategory[0].ID.String() != "7c000000-0000-0000-0000-000000000010" {
		t.Fatalf("category filter = %#v, want just tx 10", byCategory)
	}

	byPayee, err := repo.ListByAccountIDs(ctx, ids, model.TransactionFilter{PayeeID: &payeeA})
	if err != nil {
		t.Fatalf("payee filter: %v", err)
	}
	if len(byPayee) != 1 || byPayee[0].ID.String() != "7c000000-0000-0000-0000-000000000010" {
		t.Fatalf("payee filter = %#v, want just tx 10", byPayee)
	}

	byTag, err := repo.ListByAccountIDs(ctx, ids, model.TransactionFilter{TagID: &tagA})
	if err != nil {
		t.Fatalf("tag filter: %v", err)
	}
	if len(byTag) != 1 || byTag[0].ID.String() != "7c000000-0000-0000-0000-000000000010" {
		t.Fatalf("tag filter = %#v, want just tx 10", byTag)
	}

	// Combined with a period window that excludes everything.
	empty, err := repo.ListByAccountIDs(ctx, ids, model.TransactionFilter{PeriodStart: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2099, 2, 1, 0, 0, 0, 0, time.UTC), Uncategorized: true})
	if err != nil {
		t.Fatalf("uncategorized+period filter: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("uncategorized+out-of-period filter = %#v, want empty", empty)
	}
}

func TestTransactionRepo_ListExportAccountsForUser_OwnPlusShared(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	// userB owns acctB and grants access to userA.
	seedUser(t, db, userB)
	seedAccount(t, db, acctB, userB)
	fixture.New(t, db).AccountAccess(acctB, userA, 1)

	rows, err := repo.ListExportAccountsForUser(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("ListExportAccountsForUser: %v", err)
	}
	// userA owns acct1 + acct2, plus shared acctB = 3.
	if len(rows) != 3 {
		t.Fatalf("want 3 accessible accounts (2 own + 1 shared), got %d", len(rows))
	}
	for _, r := range rows {
		if r.CurrencyCode != "USD" {
			t.Errorf("currency code mismatch: %q", r.CurrencyCode)
		}
	}
}

// equalOrdered compares two string slices element-by-element, INCLUDING
// order: LabelsByTransactionIDs's query carries an explicit
// "ORDER BY transaction_id, label_id" precisely so the label order per
// transaction is byte-identical across engines (SQLite's PK-index scan vs.
// PostgreSQL's insertion-order seq-scan on a small table would otherwise
// diverge -- see ListByAccountIDs above for the same hazard). Tests that only
// check set membership would not catch a dropped ORDER BY, so this helper
// (unlike a set-comparison) is order-sensitive by construction.
func equalOrdered(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestTransactionRepo_ReplaceLabels_RoundTrip(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	seedLabel(t, db, label1, userA)
	seedLabel(t, db, label2, userA)
	txID := "7c000000-0000-0000-0000-000000000020"
	if err := repo.Save(ctx, expense(txID, acct1, "1.00", fixedTime)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Inserted in DESCENDING label id order: if LabelsByTransactionIDs merely
	// reflected insertion/scan order instead of "ORDER BY ... label_id", this
	// would come back [label2, label1] and the ordered assertion below would
	// catch it.
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(txID), []vo.Id{vo.MustParseId(label2), vo.MustParseId(label1)}); err != nil {
		t.Fatalf("ReplaceLabels: %v", err)
	}

	got, err := repo.LabelsByTransactionIDs(ctx, []vo.Id{vo.MustParseId(txID)})
	if err != nil {
		t.Fatalf("LabelsByTransactionIDs: %v", err)
	}
	if !equalOrdered(got[txID], []string{label1, label2}) {
		t.Fatalf("labels = %v, want [%s %s] in ascending label_id order", got[txID], label1, label2)
	}
}

// TestTransactionRepo_ReplaceLabels_Idempotent exercises the delete-then-insert
// contract directly: calling ReplaceLabels twice with the SAME set must not
// error (would fail on a naive INSERT without delete-first) and must not
// duplicate the pair (would inflate the label count on a second call).
func TestTransactionRepo_ReplaceLabels_Idempotent(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	seedLabel(t, db, label1, userA)
	txID := "7c000000-0000-0000-0000-000000000021"
	if err := repo.Save(ctx, expense(txID, acct1, "1.00", fixedTime)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ids := []vo.Id{vo.MustParseId(label1)}
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(txID), ids); err != nil {
		t.Fatalf("ReplaceLabels (1st): %v", err)
	}
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(txID), ids); err != nil {
		t.Fatalf("ReplaceLabels (2nd, re-save): %v", err)
	}

	got, err := repo.LabelsByTransactionIDs(ctx, []vo.Id{vo.MustParseId(txID)})
	if err != nil {
		t.Fatalf("LabelsByTransactionIDs: %v", err)
	}
	if !equalOrdered(got[txID], []string{label1}) {
		t.Fatalf("labels = %v, want exactly one [%s] (no duplication)", got[txID], label1)
	}
}

func TestTransactionRepo_ReplaceLabels_ClearsOnEmpty(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	seedLabel(t, db, label1, userA)
	txID := "7c000000-0000-0000-0000-000000000022"
	if err := repo.Save(ctx, expense(txID, acct1, "1.00", fixedTime)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(txID), []vo.Id{vo.MustParseId(label1)}); err != nil {
		t.Fatalf("ReplaceLabels (set): %v", err)
	}
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(txID), nil); err != nil {
		t.Fatalf("ReplaceLabels (clear): %v", err)
	}

	got, err := repo.LabelsByTransactionIDs(ctx, []vo.Id{vo.MustParseId(txID)})
	if err != nil {
		t.Fatalf("LabelsByTransactionIDs: %v", err)
	}
	if len(got[txID]) != 0 {
		t.Fatalf("labels after clear = %v, want none", got[txID])
	}
}

func TestTransactionRepo_LabelsByTransactionIDs_MultipleTransactions(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	seedLabel(t, db, label1, userA)
	seedLabel(t, db, label2, userA)
	seedLabel(t, db, label3, userA)
	tx1 := "7c000000-0000-0000-0000-000000000023"
	tx2 := "7c000000-0000-0000-0000-000000000024"
	tx3 := "7c000000-0000-0000-0000-000000000025"
	for _, id := range []string{tx1, tx2, tx3} {
		if err := repo.Save(ctx, expense(id, acct1, "1.00", fixedTime)); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}
	// tx1 inserted in DESCENDING label id order, same rationale as
	// TestTransactionRepo_ReplaceLabels_RoundTrip: the ordered assertion below
	// only passes if the query orders by label_id rather than reflecting
	// insertion/scan order.
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(tx1), []vo.Id{vo.MustParseId(label2), vo.MustParseId(label1)}); err != nil {
		t.Fatalf("ReplaceLabels tx1: %v", err)
	}
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(tx2), []vo.Id{vo.MustParseId(label3)}); err != nil {
		t.Fatalf("ReplaceLabels tx2: %v", err)
	}
	// tx3 gets no labels at all -- must be absent from the result map, not a
	// present-but-empty entry (the caller ranges over the map).

	got, err := repo.LabelsByTransactionIDs(ctx, []vo.Id{vo.MustParseId(tx1), vo.MustParseId(tx2), vo.MustParseId(tx3)})
	if err != nil {
		t.Fatalf("LabelsByTransactionIDs: %v", err)
	}
	if !equalOrdered(got[tx1], []string{label1, label2}) {
		t.Fatalf("tx1 labels = %v, want [%s %s] in ascending label_id order", got[tx1], label1, label2)
	}
	if !equalOrdered(got[tx2], []string{label3}) {
		t.Fatalf("tx2 labels = %v, want [%s]", got[tx2], label3)
	}
	if _, ok := got[tx3]; ok {
		t.Fatalf("tx3 should have no entry, got %v", got[tx3])
	}
}

func TestTransactionRepo_LabelsByTransactionIDs_EmptyIDs(t *testing.T) {
	repo, _ := setup(t)
	got, err := repo.LabelsByTransactionIDs(context.Background(), nil)
	if err != nil || got != nil {
		t.Fatalf("empty ids should yield nil,nil (guards the IN() syntax error on PostgreSQL); got %v, %v", got, err)
	}
}

// chunkTestTxID deterministically derives a valid-UUID-shaped id from an
// index, so a large id set can be built without 500+ literals.
func chunkTestTxID(i int) string {
	return fmt.Sprintf("7c0c0000-0000-0000-0000-%012d", i)
}

// TestTransactionRepo_LabelsByTransactionIDs_ChunksAcrossBoundary exercises
// the batch loader with MORE ids than labelsByTransactionIDsChunkSize (500):
// a naive one-shot IN(...) would still work in SQLite for this count, but the
// point is proving the multi-round-trip merge itself is correct (every id's
// row present exactly once, none dropped or duplicated at the chunk seam) —
// the real motivation is PostgreSQL's bind-param ceiling, which this count
// stays safely under while still crossing the chunk boundary at least once.
func TestTransactionRepo_LabelsByTransactionIDs_ChunksAcrossBoundary(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	seedLabel(t, db, label1, userA)

	const n = 501
	ids := make([]vo.Id, n)
	for i := 0; i < n; i++ {
		txID := chunkTestTxID(i)
		if err := repo.Save(ctx, expense(txID, acct1, "1.00", fixedTime)); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
		if err := repo.ReplaceLabels(ctx, vo.MustParseId(txID), []vo.Id{vo.MustParseId(label1)}); err != nil {
			t.Fatalf("ReplaceLabels %d: %v", i, err)
		}
		ids[i] = vo.MustParseId(txID)
	}

	got, err := repo.LabelsByTransactionIDs(ctx, ids)
	if err != nil {
		t.Fatalf("LabelsByTransactionIDs: %v", err)
	}
	if len(got) != n {
		t.Fatalf("got %d transactions with labels, want %d (a dropped or duplicated chunk would miscount)", len(got), n)
	}
	for i := 0; i < n; i++ {
		if !equalOrdered(got[chunkTestTxID(i)], []string{label1}) {
			t.Fatalf("tx %d labels = %v, want [%s]", i, got[chunkTestTxID(i)], label1)
		}
	}
}

func TestTransactionRepo_ImportedTransactionIDs(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	const tx1, tx2, tx3 = "d0000000-0000-0000-0000-0000000000d1", "d0000000-0000-0000-0000-0000000000d2", "d0000000-0000-0000-0000-0000000000d3"
	for _, id := range []string{tx1, tx2, tx3} {
		if err := repo.Save(ctx, expense(id, acct1, "1.00000000", fixedTime)); err != nil {
			t.Fatal(err)
		}
	}
	f := fixture.New(t, db)
	srcID := f.ImportSource(fixture.ImportSource{UserID: userA, Name: "Bank"})
	// tx1 has two links (two sources would each link it once in real life; here
	// two external keys from one source is enough to prove no duplication).
	f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: srcID, ExternalAccountID: "acc", ExternalTransactionID: "e1", TransactionID: tx1, ExternalAmount: "1.00000000"})
	f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: srcID, ExternalAccountID: "acc", ExternalTransactionID: "e2", TransactionID: tx1, ExternalAmount: "1.00000000"})
	f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: srcID, ExternalAccountID: "acc", ExternalTransactionID: "e3", TransactionID: tx2, ExternalAmount: "1.00000000"})
	// A tombstone (transaction_id NULL) must not mark anything.
	f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: srcID, ExternalAccountID: "acc", ExternalTransactionID: "e4", ExternalAmount: "1.00000000"})

	got, err := repo.ImportedTransactionIDs(ctx, []vo.Id{vo.MustParseId(tx1), vo.MustParseId(tx2), vo.MustParseId(tx3)})
	if err != nil {
		t.Fatalf("ImportedTransactionIDs: %v", err)
	}
	if !got[tx1] || !got[tx2] || got[tx3] || len(got) != 2 {
		t.Fatalf("got %v, want {tx1, tx2}", got)
	}
	empty, err := repo.ImportedTransactionIDs(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty ids: %v, %v", empty, err)
	}
}

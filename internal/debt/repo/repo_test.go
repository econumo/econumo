package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/debt"
	debtrepo "github.com/econumo/econumo/internal/debt/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

var fixedTime = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

type env struct {
	repo           *debtrepo.Repo
	db             *dbtest.DB
	f              *fixture.Builder
	user, category string
	account        string
	budget         string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	user := f.User(fixture.User{Name: "u"})
	return &env{
		repo:     debtrepo.NewRepo(db.Engine, db.TX),
		db:       db,
		f:        f,
		user:     user,
		category: f.Category(fixture.Category{UserID: user, Name: "Loan"}),
		account:  f.Account(fixture.Account{UserID: user}),
		budget:   f.Budget(fixture.Budget{UserID: user}),
	}
}

func (e *env) debt(id string) *model.Debt {
	d := &model.Debt{
		ID: vo.MustParseId(id), BudgetID: vo.MustParseId(e.budget), CurrencyID: vo.MustParseId(fixture.USD),
		ElementID: vo.NewId(), ElementType: model.DebtElementCategory, Status: model.DebtActive,
		CreatedDay: time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
	d.AmountBorrowed = vo.NewDecimal("12000.25")
	d.SetTerms(model.DebtTerms{
		AsOf: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), Balance: vo.NewDecimal("10000.5"), Payment: vo.NewDecimal("250"),
		Schedule: model.RecurringScheduleMonthly, LastPaymentDate: time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC),
	})
	return d
}

func isNotFound(err error) bool {
	_, ok := errs.AsNotFound(err)
	return ok
}

func TestRepo_SaveGetDebtRoundTrip(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d := e.debt(fixture.NewID())
	start := time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC)
	d.StartDate = &start
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := e.repo.GetByID(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.BudgetID != d.BudgetID || got.CurrencyID != d.CurrencyID || !got.ElementID.Equal(d.ElementID) ||
		got.ElementType != model.DebtElementCategory || got.Status != model.DebtActive || got.EndedAt != nil {
		t.Errorf("identity mismatch: %+v", got)
	}
	if got.StartDate == nil || !got.StartDate.Equal(start) {
		t.Errorf("start date = %v, want %v", got.StartDate, start)
	}
	c := got.Current
	if !c.AsOf.Equal(d.Current.AsOf) || !c.LastPaymentDate.Equal(d.Current.LastPaymentDate) {
		t.Errorf("dates = %v / %v", c.AsOf, c.LastPaymentDate)
	}
	if c.Balance.String() != "10000.5" || c.Payment.String() != "250" || c.Schedule != model.RecurringScheduleMonthly {
		t.Errorf("terms = %+v", c)
	}
	if got.Payment.String() != "250" || got.Schedule != model.RecurringScheduleMonthly || got.AmountBorrowed.String() != "12000.25" {
		t.Errorf("debt payment/schedule/borrowed = %s / %s / %s", got.Payment, got.Schedule, got.AmountBorrowed)
	}
	if !got.CreatedAt.Equal(fixedTime) || !got.UpdatedAt.Equal(fixedTime) {
		t.Errorf("timestamps = %v / %v", got.CreatedAt, got.UpdatedAt)
	}
	if want := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC); !got.CreatedDay.Equal(want) {
		t.Errorf("created day = %v, want %v", got.CreatedDay, want)
	}

	// Updating clears the start, ends the debt and rewrites the terms.
	d.StartDate = nil
	end := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	d.Status, d.EndedAt = model.DebtPaidOff, &end
	d.Current.Balance = vo.NewDecimal("9000")
	d.CreatedDay = time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatalf("Save update: %v", err)
	}
	got, err = e.repo.GetByID(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.DebtPaidOff || got.EndedAt == nil || !got.EndedAt.Equal(end) || got.StartDate != nil || got.Current.Balance.String() != "9000" {
		t.Errorf("after update: %+v", got)
	}
	// the creation day is written once, at insert
	if want := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC); !got.CreatedDay.Equal(want) {
		t.Errorf("created day after update = %v, want %v", got.CreatedDay, want)
	}

	byBudget, err := e.repo.ListByBudget(ctx, d.BudgetID)
	if err != nil || len(byBudget) != 1 || byBudget[0].ID != d.ID {
		t.Errorf("ListByBudget = %v, %v", byBudget, err)
	}
	byBudgets, err := e.repo.ListByBudgets(ctx, []vo.Id{d.BudgetID, vo.NewId()})
	if err != nil || len(byBudgets) != 1 || byBudgets[0].ID != d.ID {
		t.Errorf("ListByBudgets = %v, %v", byBudgets, err)
	}

	if err := e.repo.Delete(ctx, d.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := e.repo.GetByID(ctx, d.ID); !isNotFound(err) {
		t.Errorf("GetByID after delete = %v, want not found", err)
	}
}

func TestRepo_SnapshotsOrdered(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d := e.debt(fixture.NewID())
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	other := e.debt(fixture.NewID())
	if err := e.repo.Save(ctx, other); err != nil {
		t.Fatal(err)
	}
	snap := func(debtID vo.Id, asOf string, created time.Time, balance string) *model.DebtSnapshot {
		at, err := time.Parse("2006-01-02", asOf)
		if err != nil {
			t.Fatal(err)
		}
		return &model.DebtSnapshot{ID: e.repo.NextIdentity(), DebtID: debtID, AsOf: at, Balance: vo.NewDecimal(balance),
			Payment: vo.NewDecimal("100"), Schedule: model.RecurringScheduleMonthly, LastPaymentDate: at, CreatedAt: created}
	}
	// Inserted out of order: as_of first, then created_at breaks a tie.
	for _, s := range []*model.DebtSnapshot{
		snap(d.ID, "2026-03-01", fixedTime.Add(2*time.Second), "3"),
		snap(d.ID, "2026-01-01", fixedTime.Add(3*time.Second), "1"),
		snap(d.ID, "2026-03-01", fixedTime.Add(time.Second), "2"),
		snap(other.ID, "2025-01-01", fixedTime, "9"),
	} {
		if err := e.repo.AddSnapshot(ctx, s); err != nil {
			t.Fatalf("AddSnapshot: %v", err)
		}
	}
	got, err := e.repo.Snapshots(ctx, []vo.Id{d.ID, other.ID})
	if err != nil {
		t.Fatalf("Snapshots: %v", err)
	}
	var balances []string
	for _, s := range got {
		balances = append(balances, s.Balance.String())
	}
	first := d.ID.String() < other.ID.String()
	want := []string{"1", "2", "3", "9"}
	if !first {
		want = []string{"9", "1", "2", "3"}
	}
	if len(balances) != len(want) {
		t.Fatalf("balances = %v, want %v", balances, want)
	}
	for i := range want {
		if balances[i] != want[i] {
			t.Fatalf("balances = %v, want %v", balances, want)
		}
	}
	if s := got[0]; s.Schedule != model.RecurringScheduleMonthly || s.Payment.String() != "100" || s.AsOf.IsZero() {
		t.Errorf("snapshot fields = %+v", s)
	}
}

func TestRepo_UpdateSnapshotRewritesTheOrigin(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d := e.debt(fixture.NewID())
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	origin := &model.DebtSnapshot{ID: e.repo.NextIdentity(), DebtID: d.ID, AsOf: at, Balance: vo.NewDecimal("12000"),
		Payment: vo.NewDecimal("600"), Schedule: model.RecurringScheduleMonthly, LastPaymentDate: at.AddDate(2, 0, 0), CreatedAt: fixedTime}
	if err := e.repo.AddSnapshot(ctx, origin); err != nil {
		t.Fatal(err)
	}
	origin.AsOf, origin.Balance, origin.Payment = at.AddDate(0, 0, 5), vo.NewDecimal("11000.5"), vo.NewDecimal("700")
	origin.Schedule, origin.LastPaymentDate = model.RecurringScheduleWeekly, at.AddDate(3, 0, 0)
	if err := e.repo.UpdateSnapshot(ctx, origin); err != nil {
		t.Fatalf("UpdateSnapshot: %v", err)
	}
	got, err := e.repo.Snapshots(ctx, []vo.Id{d.ID})
	if err != nil || len(got) != 1 {
		t.Fatalf("Snapshots = %v, %v", got, err)
	}
	if s := got[0]; s.ID != origin.ID || !s.AsOf.Equal(at.AddDate(0, 0, 5)) || s.Balance.String() != "11000.5" || s.Payment.String() != "700" ||
		s.Schedule != model.RecurringScheduleWeekly || !s.LastPaymentDate.Equal(at.AddDate(3, 0, 0)) || !s.CreatedAt.Equal(fixedTime) {
		t.Errorf("snapshot = %+v", s)
	}
	if err := e.repo.DeleteSnapshot(ctx, origin.ID); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}
	if got, err := e.repo.Snapshots(ctx, []vo.Id{d.ID}); err != nil || len(got) != 0 {
		t.Fatalf("Snapshots after delete = %v, %v", got, err)
	}
}

func (e *env) expense(t *testing.T, typ int, amount string) string {
	t.Helper()
	return e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: e.account, CategoryID: e.category, Type: typ, Amount: amount})
}

func TestRepo_InsertSplitIsIdempotent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d := e.debt(fixture.NewID())
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	txID := vo.MustParseId(e.expense(t, 0, "50"))
	split := model.DebtSplit{DebtID: d.ID, TransactionID: txID, Interest: vo.NewDecimal("12.5")}
	for range 2 {
		if err := e.repo.InsertSplit(ctx, split); err != nil {
			t.Fatalf("InsertSplit: %v", err)
		}
	}
	got, err := e.repo.Splits(ctx, []vo.Id{d.ID})
	if err != nil || len(got) != 1 || got[0].TransactionID != txID || got[0].Interest.String() != "12.5" {
		t.Fatalf("Splits = %v, %v", got, err)
	}
	if err := e.repo.DeleteSplits(ctx, d.ID, []vo.Id{txID}); err != nil {
		t.Fatalf("DeleteSplits: %v", err)
	}
	if got, _ := e.repo.Splits(ctx, []vo.Id{d.ID}); len(got) != 0 {
		t.Errorf("Splits after delete = %v", got)
	}
}

// A transaction can count for one debt in each of two budgets: its splits
// are keyed by (debt, transaction).
func TestRepo_SplitsOfOneTransactionForTwoDebts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a, b := e.debt(fixture.NewID()), e.debt(fixture.NewID())
	b.BudgetID = vo.MustParseId(e.f.Budget(fixture.Budget{UserID: e.user}))
	for _, d := range []*model.Debt{a, b} {
		if err := e.repo.Save(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	txID := vo.MustParseId(e.expense(t, 0, "50"))
	if err := e.repo.InsertSplit(ctx, model.DebtSplit{DebtID: a.ID, TransactionID: txID, Interest: vo.NewDecimal("12.5")}); err != nil {
		t.Fatalf("InsertSplit a: %v", err)
	}
	if err := e.repo.InsertSplit(ctx, model.DebtSplit{DebtID: b.ID, TransactionID: txID, Interest: vo.NewDecimal("7")}); err != nil {
		t.Fatalf("InsertSplit b: %v", err)
	}
	got, err := e.repo.Splits(ctx, []vo.Id{a.ID, b.ID})
	if err != nil || len(got) != 2 {
		t.Fatalf("Splits = %v, %v; want one per debt", got, err)
	}
	interest := map[vo.Id]string{}
	for _, sp := range got {
		interest[sp.DebtID] = sp.Interest.String()
	}
	if interest[a.ID] != "12.5" || interest[b.ID] != "7" {
		t.Errorf("interests = %v", interest)
	}
	if err := e.repo.DeleteSplits(ctx, a.ID, []vo.Id{txID}); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.repo.Splits(ctx, []vo.Id{a.ID, b.ID}); len(got) != 1 || got[0].DebtID != b.ID {
		t.Errorf("Splits after deleting a's = %v, want b's alone", got)
	}
}

func TestRepo_SplitDeletedWithTransaction(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d := e.debt(fixture.NewID())
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	txID := e.expense(t, 0, "50")
	if err := e.repo.InsertSplit(ctx, model.DebtSplit{DebtID: d.ID, TransactionID: vo.MustParseId(txID), Interest: vo.NewDecimal("1")}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Raw.ExecContext(ctx, e.db.Rebind("DELETE FROM transactions WHERE id = ?"), txID); err != nil {
		t.Fatalf("delete transaction: %v", err)
	}
	if got, err := e.repo.Splits(ctx, []vo.Id{d.ID}); err != nil || len(got) != 0 {
		t.Errorf("Splits = %v, %v; the transaction delete should cascade", got, err)
	}
}

func TestRepo_CandidateTransactionsExpensesOnly(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	eur := e.f.Currency(fixture.Currency{Code: "EUR", Symbol: "E"})
	eurAcc := e.f.Account(fixture.Account{UserID: e.user, CurrencyID: eur})
	expense := e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: eurAcc, CategoryID: e.category, Type: 0, Amount: "40"})
	e.expense(t, 1, "10")
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: e.account, AccountRecipientID: eurAcc, CategoryID: e.category, Type: 2, Amount: "5", AmountRecipient: "5"})
	elsewhere := e.f.Category(fixture.Category{UserID: e.user, Name: "Other"})
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: e.account, CategoryID: elsewhere, Type: 0, Amount: "7"})
	// a tagged expense counts on its tag's budget row, never on a debt's
	tag := e.f.Tag(fixture.Tag{UserID: e.user, Name: "Car"})
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: e.account, CategoryID: e.category, TagID: tag, Type: 0, Amount: "9"})

	got, err := e.repo.CandidateTransactions(ctx, []vo.Id{vo.MustParseId(e.category)}, []vo.Id{vo.MustParseId(e.account), vo.MustParseId(eurAcc)})
	if err != nil {
		t.Fatalf("CandidateTransactions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("candidates = %+v, want the one expense", got)
	}
	c := got[0]
	if c.ID.String() != expense || c.CurrencyID.String() != eur || c.CategoryID.String() != e.category || c.Amount.String() != "40" {
		t.Errorf("candidate = %+v", c)
	}
}

func TestRepo_LockBudget(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	budget := vo.MustParseId(e.budget)
	err := e.db.TX.WithTx(ctx, func(ctx context.Context) error {
		if e.repo.HoldsBudgetLock(ctx, budget) {
			t.Error("held before LockBudget")
		}
		if err := e.repo.LockBudget(ctx, budget); err != nil {
			return err
		}
		// a savepoint shares the transaction's locks
		if err := e.db.TX.WithTx(ctx, func(ctx context.Context) error {
			if !e.repo.HoldsBudgetLock(ctx, budget) {
				t.Error("not held after LockBudget")
			}
			return nil
		}); err != nil {
			return err
		}
		if err := e.repo.LockBudget(ctx, vo.NewId()); err != nil {
			return err
		}
		return e.repo.LockCategories(ctx, []vo.Id{vo.MustParseId(e.category), vo.MustParseId(e.category)})
	})
	if err != nil {
		t.Fatalf("locks: %v", err)
	}
	// a lock ends with its transaction
	if e.repo.HoldsBudgetLock(ctx, budget) {
		t.Error("held outside the transaction")
	}
	if err := e.db.TX.WithTx(ctx, func(ctx context.Context) error {
		if e.repo.HoldsBudgetLock(ctx, budget) {
			t.Error("held by the next transaction")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.LockCategories(ctx, nil); err != nil {
		t.Fatalf("no categories: %v", err)
	}
}

// A category deleted before the debt write is not found.
func TestRepo_LockCategoriesMissingNotFound(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	err := e.db.TX.WithTx(ctx, func(ctx context.Context) error {
		return e.repo.LockCategories(ctx, []vo.Id{vo.MustParseId(e.category), vo.NewId()})
	})
	if !isNotFound(err) {
		t.Fatalf("err = %v, want not found", err)
	}
}

// Deleting a budget deletes its debts with their snapshots and splits.
func TestRepo_BudgetDeleteCascades(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d := e.debt(fixture.NewID())
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.AddSnapshot(ctx, &model.DebtSnapshot{ID: vo.NewId(), DebtID: d.ID, AsOf: fixedTime, Balance: vo.NewDecimal("1"),
		Payment: vo.NewDecimal("1"), Schedule: model.RecurringScheduleMonthly, LastPaymentDate: fixedTime, CreatedAt: fixedTime}); err != nil {
		t.Fatal(err)
	}
	txID := vo.MustParseId(e.expense(t, 0, "50"))
	if err := e.repo.InsertSplit(ctx, model.DebtSplit{DebtID: d.ID, TransactionID: txID, Interest: vo.NewDecimal("1")}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Raw.ExecContext(ctx, e.db.Rebind("DELETE FROM budgets WHERE id = ?"), e.budget); err != nil {
		t.Fatalf("delete budget: %v", err)
	}
	if _, err := e.repo.GetByID(ctx, d.ID); !isNotFound(err) {
		t.Errorf("the debt survived its budget: %v", err)
	}
	for _, table := range []string{"debts_snapshots", "debts_payments"} {
		var n int
		if err := e.db.Raw.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s: %d rows, %v; want none", table, n, err)
		}
	}
}

// A payment deleted between the sync's read and its split insert writes
// nothing instead of failing the foreign key (and the read with it).
func TestRepo_InsertSplitForAGoneTransactionIsANoOp(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d := e.debt(fixture.NewID())
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.InsertSplit(ctx, model.DebtSplit{DebtID: d.ID, TransactionID: vo.NewId(), Interest: vo.NewDecimal("1")}); err != nil {
		t.Fatalf("InsertSplit for a gone transaction: %v", err)
	}
	if got, err := e.repo.Splits(ctx, []vo.Id{d.ID}); err != nil || len(got) != 0 {
		t.Errorf("Splits = %v, %v; want none", got, err)
	}
}

func TestRepo_SaveDebtWithoutTerms(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d := e.debt(fixture.NewID())
	d.Current = nil
	d.Payment, d.Schedule = vo.NewDecimal("300"), model.RecurringScheduleWeekly
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := e.repo.GetByID(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != nil || got.IsSetUp() {
		t.Errorf("Current = %+v, want nil", got.Current)
	}
	if got.Payment.String() != "300" || got.Schedule != model.RecurringScheduleWeekly {
		t.Errorf("payment/schedule = %s / %s", got.Payment, got.Schedule)
	}
	byBudget, err := e.repo.ListByBudget(ctx, d.BudgetID)
	if err != nil || len(byBudget) != 1 || byBudget[0].Current != nil {
		t.Errorf("ListByBudget = %v, %v", byBudget, err)
	}

	d.SetTerms(model.DebtTerms{
		AsOf: time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC), Balance: vo.NewDecimal("900"), Payment: vo.NewDecimal("300"),
		Schedule: model.RecurringScheduleWeekly, LastPaymentDate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
	})
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatalf("Save with terms: %v", err)
	}
	if got, err = e.repo.GetByID(ctx, d.ID); err != nil || got.Current == nil || got.Current.Balance.String() != "900" {
		t.Errorf("after setup: %+v, %v", got, err)
	}
	d.Current = nil
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatalf("Save back without terms: %v", err)
	}
	if got, err = e.repo.GetByID(ctx, d.ID); err != nil || got.Current != nil {
		t.Errorf("after clearing: %+v, %v", got, err)
	}
}

// Only the given accounts and categories count: an expense in a rule's
// category on an account outside the budget is not a candidate.
func TestRepo_CandidateTransactionsOnTheGivenAccounts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	other := e.f.Category(fixture.Category{UserID: e.user, Name: "Other"})
	ignored := e.f.Category(fixture.Category{UserID: e.user, Name: "Ignored"})
	outside := e.f.Account(fixture.Account{UserID: e.user})
	inFirst := e.expense(t, 0, "10")
	inOther := e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: e.account, CategoryID: other, Type: 0, Amount: "20"})
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: outside, CategoryID: e.category, Type: 0, Amount: "30"})
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: e.account, CategoryID: ignored, Type: 0, Amount: "40"})
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: e.account, CategoryID: other, Type: 1, Amount: "50"})
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: outside, CategoryID: other, Type: 0, Amount: "60"})

	got, err := e.repo.CandidateTransactions(ctx, []vo.Id{vo.MustParseId(e.category), vo.MustParseId(other)}, []vo.Id{vo.MustParseId(e.account)})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]debt.CandidateTx{}
	for _, c := range got {
		byID[c.ID.String()] = c
	}
	if len(got) != 2 || byID[inFirst].CategoryID.String() != e.category || byID[inOther].CategoryID.String() != other {
		t.Fatalf("candidates = %+v, want the two expenses on the listed account", got)
	}
	for _, empty := range [][2][]vo.Id{{nil, {vo.MustParseId(e.account)}}, {{vo.MustParseId(e.category)}, nil}} {
		if got, err := e.repo.CandidateTransactions(ctx, empty[0], empty[1]); err != nil || len(got) != 0 {
			t.Errorf("no categories or no accounts: %v, %v", got, err)
		}
	}
}

func TestRateChanges_SaveListReplaceDelete(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d := e.debt(fixture.NewID())
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	other := e.debt(fixture.NewID())
	if err := e.repo.Save(ctx, other); err != nil {
		t.Fatal(err)
	}
	change := func(debtID vo.Id, date, rate, payment string) *model.DebtRateChange {
		at, err := time.Parse("2006-01-02", date)
		if err != nil {
			t.Fatal(err)
		}
		return &model.DebtRateChange{ID: e.repo.NextIdentity(), DebtID: debtID, EffectiveDate: at,
			AnnualRate: vo.NewDecimal(rate), Payment: vo.NewDecimal(payment), CreatedAt: fixedTime}
	}
	for _, c := range []*model.DebtRateChange{
		change(d.ID, "2026-12-01", "4.1", "3100"),
		change(d.ID, "2026-09-01", "3.69", "3000.5"),
	} {
		if err := e.repo.SaveRateChange(ctx, c); err != nil {
			t.Fatalf("SaveRateChange: %v", err)
		}
	}
	got, err := e.repo.RateChanges(ctx, []vo.Id{d.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].EffectiveDate.Format("2006-01-02") != "2026-09-01" || got[0].AnnualRate.String() != "3.69" ||
		got[0].Payment.String() != "3000.5" || got[1].AnnualRate.String() != "4.1" {
		t.Fatalf("rate changes = %+v", got)
	}

	replaced := change(d.ID, "2026-09-01", "3.25", "2900")
	if err := e.repo.SaveRateChange(ctx, replaced); err != nil {
		t.Fatal(err)
	}
	got, _ = e.repo.RateChanges(ctx, []vo.Id{d.ID})
	if len(got) != 2 || got[0].AnnualRate.String() != "3.25" || got[0].Payment.String() != "2900" {
		t.Fatalf("after replace = %+v", got)
	}

	ok, err := e.repo.DeleteRateChange(ctx, other.ID, got[0].ID)
	if err != nil || ok {
		t.Fatalf("delete under another debt = %v, %v", ok, err)
	}
	ok, err = e.repo.DeleteRateChange(ctx, d.ID, got[0].ID)
	if err != nil || !ok {
		t.Fatalf("delete own = %v, %v", ok, err)
	}
	got, _ = e.repo.RateChanges(ctx, []vo.Id{d.ID})
	if len(got) != 1 {
		t.Fatalf("after delete = %+v", got)
	}

	if err := e.repo.Delete(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ = e.repo.RateChanges(ctx, []vo.Id{d.ID}); len(got) != 0 {
		t.Fatalf("rows survived the debt: %+v", got)
	}
}

// MoveToElement re-attaches one budget's debts of an element; ListByElement
// finds an element's debts in every budget.
func TestRepo_MoveToElementAndListByElement(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d := e.debt(fixture.NewID())
	other := e.f.Budget(fixture.Budget{UserID: e.user})
	elsewhere := e.debt(fixture.NewID())
	elsewhere.BudgetID = vo.MustParseId(other)
	d.ElementID, elsewhere.ElementID = vo.MustParseId(e.category), vo.MustParseId(e.category)
	for _, x := range []*model.Debt{d, elsewhere} {
		if err := e.repo.Save(ctx, x); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.repo.ListByElement(ctx, vo.MustParseId(e.category))
	if err != nil || len(got) != 2 {
		t.Fatalf("ListByElement = %v, %v", got, err)
	}
	envelope := vo.NewId()
	movedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if err := e.repo.MoveToElement(ctx, d.BudgetID, vo.MustParseId(e.category), envelope, model.DebtElementEnvelope, movedAt); err != nil {
		t.Fatal(err)
	}
	moved, err := e.repo.GetByID(ctx, d.ID)
	if err != nil || !moved.ElementID.Equal(envelope) || moved.ElementType != model.DebtElementEnvelope {
		t.Fatalf("moved = %+v, %v", moved, err)
	}
	if !moved.UpdatedAt.Equal(movedAt) {
		t.Errorf("moved updated_at = %v, want %v", moved.UpdatedAt, movedAt)
	}
	if kept, err := e.repo.GetByID(ctx, elsewhere.ID); err != nil || kept.ElementID.String() != e.category {
		t.Fatalf("the other budget's debt moved: %+v, %v", kept, err)
	}
}

// An element carries at most one active debt; paid-off ones may share it.
func TestRepo_OneActiveDebtPerElement(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	end := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	paid := e.debt(fixture.NewID())
	paid.Status, paid.EndedAt = model.DebtPaidOff, &end
	active := e.debt(fixture.NewID())
	paid.ElementID = active.ElementID
	for _, x := range []*model.Debt{paid, active} {
		if err := e.repo.Save(ctx, x); err != nil {
			t.Fatal(err)
		}
	}
	second := e.debt(fixture.NewID())
	second.ElementID = active.ElementID
	if err := e.repo.Save(ctx, second); err == nil {
		t.Fatal("a second active debt on the element was saved")
	}
}

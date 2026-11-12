package repo_test

import (
	"context"
	"slices"
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

func (e *env) rule(categoryID string, created time.Time) model.DebtRule {
	return model.DebtRule{ID: vo.NewId(), CategoryID: vo.MustParseId(categoryID), CreatedAt: created}
}

func (e *env) debt(id string) *model.Debt {
	d := &model.Debt{
		ID: vo.MustParseId(id), BudgetID: vo.MustParseId(e.budget), CurrencyID: vo.MustParseId(fixture.USD),
		Rules: []model.DebtRule{e.rule(e.category, fixedTime)}, Name: "Car loan",
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
	payee := e.f.Payee(fixture.Payee{UserID: e.user})
	tag := e.f.Tag(fixture.Tag{UserID: e.user})
	label := e.f.Label(fixture.Label{UserID: e.user})
	d := e.debt(fixture.NewID())
	pid, tid, lid := vo.MustParseId(payee), vo.MustParseId(tag), vo.MustParseId(label)
	start := time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC)
	d.Rules[0].PayeeID, d.Rules[0].TagID, d.StartDate = &pid, &tid, &start
	labelled := e.rule(e.category, fixedTime.Add(time.Second))
	labelled.LabelID = &lid
	d.Rules = append(d.Rules, labelled)
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := e.repo.GetByID(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != d.Name || got.BudgetID != d.BudgetID || got.CurrencyID != d.CurrencyID {
		t.Errorf("identity mismatch: %+v", got)
	}
	if len(got.Rules) != 2 {
		t.Fatalf("rules = %+v", got.Rules)
	}
	if r := got.Rules[0]; r.ID != d.Rules[0].ID || r.CategoryID.String() != e.category || r.PayeeID == nil || *r.PayeeID != pid ||
		r.TagID == nil || *r.TagID != tid || r.LabelID != nil || !r.CreatedAt.Equal(fixedTime) {
		t.Errorf("first rule = %+v", r)
	}
	if r := got.Rules[1]; r.LabelID == nil || *r.LabelID != lid || r.PayeeID != nil || r.TagID != nil {
		t.Errorf("labelled rule = %+v", r)
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

	// Updating clears the start and rewrites the terms.
	d.StartDate = nil
	d.Name = "Renamed"
	d.Current.Balance = vo.NewDecimal("9000")
	d.CreatedDay = time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatalf("Save update: %v", err)
	}
	got, err = e.repo.GetByID(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Renamed" || got.StartDate != nil || got.Current.Balance.String() != "9000" {
		t.Errorf("after update: %+v", got)
	}
	// the creation day is written once, at insert
	if want := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC); !got.CreatedDay.Equal(want) {
		t.Errorf("created day after update = %v, want %v", got.CreatedDay, want)
	}

	byBudget, err := e.repo.ListByBudget(ctx, d.BudgetID)
	if err != nil || len(byBudget) != 1 || byBudget[0].ID != d.ID || len(byBudget[0].Rules) != 2 {
		t.Errorf("ListByBudget = %v, %v", byBudget, err)
	}
	byBudgets, err := e.repo.ListByBudgets(ctx, []vo.Id{d.BudgetID, vo.NewId()})
	if err != nil || len(byBudgets) != 1 || byBudgets[0].ID != d.ID || len(byBudgets[0].Rules) != 2 {
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

	got, err := e.repo.CandidateTransactions(ctx, []vo.Id{vo.MustParseId(e.category)}, []vo.Id{vo.MustParseId(e.account), vo.MustParseId(eurAcc)})
	if err != nil {
		t.Fatalf("CandidateTransactions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("candidates = %+v, want the one expense", got)
	}
	c := got[0]
	if c.ID.String() != expense || c.CurrencyID.String() != eur || c.CategoryID.String() != e.category || c.Amount.String() != "40" ||
		c.PayeeID != nil || c.TagID != nil {
		t.Errorf("candidate = %+v", c)
	}
}

func TestRepo_ListByClassification(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	payee := e.f.Payee(fixture.Payee{UserID: e.user, Name: "Bank"})
	tag := e.f.Tag(fixture.Tag{UserID: e.user, Name: "Car"})
	label := e.f.Label(fixture.Label{UserID: e.user, Name: "Home"})
	other := e.f.Category(fixture.Category{UserID: e.user, Name: "Other"})
	pid, tid, lid := vo.MustParseId(payee), vo.MustParseId(tag), vo.MustParseId(label)

	withAll := e.debt(fixture.NewID())
	withAll.Rules[0].PayeeID, withAll.Rules[0].TagID = &pid, &tid
	labelled := e.rule(other, fixedTime.Add(time.Second))
	labelled.LabelID = &lid
	withAll.Rules = append(withAll.Rules, labelled)
	plain := e.debt(fixture.NewID())
	plain.CreatedAt = fixedTime.Add(time.Minute)
	for _, d := range []*model.Debt{withAll, plain} {
		if err := e.repo.Save(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		kind string
		id   string
		want []vo.Id
	}{
		{"category", e.category, []vo.Id{withAll.ID, plain.ID}},
		{"category", other, []vo.Id{withAll.ID}},
		{"payee", payee, []vo.Id{withAll.ID}},
		{"tag", tag, []vo.Id{withAll.ID}},
		{"label", label, []vo.Id{withAll.ID}},
		{"payee", fixture.NewID(), nil},
	} {
		got, err := e.repo.ListByClassification(ctx, c.kind, vo.MustParseId(c.id))
		if err != nil {
			t.Fatalf("ListByClassification(%s): %v", c.kind, err)
		}
		var ids []vo.Id
		for _, d := range got {
			ids = append(ids, d.ID)
			if len(d.Rules) == 0 {
				t.Errorf("%s: debt %s loaded without rules", c.kind, d.ID)
			}
		}
		if !slices.Equal(ids, c.want) {
			t.Errorf("ListByClassification(%s) = %v, want %v", c.kind, ids, c.want)
		}
	}
	if _, err := e.repo.ListByClassification(ctx, "currency", pid); err == nil {
		t.Error("an unknown kind must be refused")
	}
}

func TestRepo_SaveReplacesRules(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	other := e.f.Category(fixture.Category{UserID: e.user, Name: "Other"})
	d := e.debt(fixture.NewID())
	kept := d.Rules[0]
	d.Rules = append(d.Rules, e.rule(other, fixedTime.Add(time.Second)))
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	d.Rules = []model.DebtRule{kept}
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, err := e.repo.GetByID(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rules) != 1 || got.Rules[0].ID != kept.ID || got.Rules[0].CategoryID != kept.CategoryID {
		t.Fatalf("rules = %+v, want the kept one with its id", got.Rules)
	}
	d.Rules = nil
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	if got, err := e.repo.GetByID(ctx, d.ID); err != nil || len(got.Rules) != 0 {
		t.Fatalf("a debt without rules: %+v, %v", got, err)
	}
}

func TestRepo_ListByBudgetLoadsRulesInOrder(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	other := e.f.Category(fixture.Category{UserID: e.user, Name: "Other"})
	third := e.f.Category(fixture.Category{UserID: e.user, Name: "Third"})
	older, newer := e.debt(fixture.NewID()), e.debt(fixture.NewID())
	newer.CreatedAt = fixedTime.Add(time.Hour)
	// the rules are saved out of order: created_at, then id, orders them
	late, early := e.rule(other, fixedTime.Add(2*time.Second)), e.rule(third, fixedTime.Add(time.Second))
	older.Rules = []model.DebtRule{late, older.Rules[0], early}
	elsewhere := e.debt(fixture.NewID())
	elsewhere.BudgetID = vo.MustParseId(e.f.Budget(fixture.Budget{UserID: e.user}))
	for _, d := range []*model.Debt{newer, elsewhere, older} {
		if err := e.repo.Save(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.repo.ListByBudget(ctx, vo.MustParseId(e.budget))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != older.ID || got[1].ID != newer.ID {
		t.Fatalf("ListByBudget = %v, want older then newer", got)
	}
	if r := got[0].Rules; len(r) != 3 || r[0].CategoryID.String() != e.category || r[1].ID != early.ID || r[2].ID != late.ID {
		t.Fatalf("rules = %+v, want by created_at", r)
	}
	if len(got[1].Rules) != 1 {
		t.Fatalf("newer rules = %+v", got[1].Rules)
	}
}

func TestRepo_LockBudget(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	err := e.db.TX.WithTx(ctx, func(ctx context.Context) error {
		if err := e.repo.LockBudget(ctx, vo.MustParseId(e.budget)); err != nil {
			return err
		}
		if err := e.repo.LockBudget(ctx, vo.NewId()); err != nil {
			return err
		}
		return e.repo.LockRuleCategories(ctx, []vo.Id{vo.MustParseId(e.category), vo.NewId()})
	})
	if err != nil {
		t.Fatalf("locks: %v", err)
	}
	if err := e.repo.LockRuleCategories(ctx, nil); err != nil {
		t.Fatalf("no categories: %v", err)
	}
}

// Deleting a budget deletes its debts with their rules, snapshots and splits.
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
	for _, table := range []string{"debts_rules", "debts_snapshots", "debts_payments"} {
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

func (e *env) link(t *testing.T, txID string, labelIDs ...string) {
	t.Helper()
	for _, l := range labelIDs {
		if _, err := e.db.Raw.ExecContext(context.Background(), e.db.Rebind("INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)"), txID, l); err != nil {
			t.Fatalf("link label: %v", err)
		}
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

func TestRepo_CandidateTransactionsCarryLabels(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.f.Label(fixture.Label{UserID: e.user, Name: "A"})
	b := e.f.Label(fixture.Label{UserID: e.user, Name: "B"})
	withTwo := e.expense(t, 0, "10")
	withOne := e.expense(t, 0, "20")
	bare := e.expense(t, 0, "30")
	elsewhere := e.f.Category(fixture.Category{UserID: e.user, Name: "Other"})
	foreign := e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: e.account, CategoryID: elsewhere, Type: 0, Amount: "5"})
	e.link(t, withTwo, a, b)
	e.link(t, withOne, b)
	e.link(t, foreign, a)

	got, err := e.repo.CandidateTransactions(ctx, []vo.Id{vo.MustParseId(e.category)}, []vo.Id{vo.MustParseId(e.account)})
	if err != nil || len(got) != 3 {
		t.Fatalf("CandidateTransactions = %v, %v", got, err)
	}
	labels := map[string][]string{}
	for _, c := range got {
		ids := []string{}
		for _, l := range c.LabelIDs {
			ids = append(ids, l.String())
		}
		slices.Sort(ids)
		labels[c.ID.String()] = ids
	}
	wantTwo := []string{a, b}
	slices.Sort(wantTwo)
	if !slices.Equal(labels[withTwo], wantTwo) || !slices.Equal(labels[withOne], []string{b}) || len(labels[bare]) != 0 {
		t.Errorf("labels = %v", labels)
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
	label := e.f.Label(fixture.Label{UserID: e.user, Name: "L"})
	inFirst := e.expense(t, 0, "10")
	inOther := e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: e.account, CategoryID: other, Type: 0, Amount: "20"})
	e.link(t, inOther, label)
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: outside, CategoryID: e.category, Type: 0, Amount: "30"})
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: e.account, CategoryID: ignored, Type: 0, Amount: "40"})
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: e.account, CategoryID: other, Type: 1, Amount: "50"})
	e.link(t, e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: outside, CategoryID: other, Type: 0, Amount: "60"}), label)

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
	if l := byID[inOther].LabelIDs; len(l) != 1 || l[0].String() != label {
		t.Errorf("labels = %v", l)
	}
	for _, empty := range [][2][]vo.Id{{nil, {vo.MustParseId(e.account)}}, {{vo.MustParseId(e.category)}, nil}} {
		if got, err := e.repo.CandidateTransactions(ctx, empty[0], empty[1]); err != nil || len(got) != 0 {
			t.Errorf("no categories or no accounts: %v, %v", got, err)
		}
	}
}

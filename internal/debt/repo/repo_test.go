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
	}
}

func (e *env) debt(id string) *model.Debt {
	d := &model.Debt{
		ID: vo.MustParseId(id), OwnerID: vo.MustParseId(e.user), CurrencyID: vo.MustParseId(fixture.USD),
		CategoryID: vo.MustParseId(e.category), Name: "Car loan",
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
	d := e.debt(fixture.NewID())
	pid, tid := vo.MustParseId(payee), vo.MustParseId(tag)
	start := time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC)
	d.PayeeID, d.TagID, d.StartDate = &pid, &tid, &start
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := e.repo.GetByID(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != d.Name || got.OwnerID != d.OwnerID || got.CurrencyID != d.CurrencyID || got.CategoryID != d.CategoryID {
		t.Errorf("identity mismatch: %+v", got)
	}
	if got.PayeeID == nil || *got.PayeeID != pid || got.TagID == nil || *got.TagID != tid {
		t.Errorf("payee/tag mismatch: %v %v", got.PayeeID, got.TagID)
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

	// Updating clears the optional keys and rewrites the terms.
	d.PayeeID, d.TagID, d.StartDate = nil, nil, nil
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
	if got.Name != "Renamed" || got.PayeeID != nil || got.TagID != nil || got.StartDate != nil || got.Current.Balance.String() != "9000" {
		t.Errorf("after update: %+v", got)
	}
	// the creation day is written once, at insert
	if want := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC); !got.CreatedDay.Equal(want) {
		t.Errorf("created day after update = %v, want %v", got.CreatedDay, want)
	}

	byOwner, err := e.repo.ListByOwners(ctx, []vo.Id{d.OwnerID})
	if err != nil || len(byOwner) != 1 || byOwner[0].ID != d.ID {
		t.Errorf("ListByOwners = %v, %v", byOwner, err)
	}
	byCat, err := e.repo.ListByCategory(ctx, d.CategoryID)
	if err != nil || len(byCat) != 1 || byCat[0].ID != d.ID {
		t.Errorf("ListByCategory = %v, %v", byCat, err)
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

// A payment that moved to another debt's category takes its split with it at
// once, even before the old debt's category is synced again.
func TestRepo_InsertSplitMovesToAnotherDebt(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a, b := e.debt(fixture.NewID()), e.debt(fixture.NewID())
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
	got, err := e.repo.Splits(ctx, []vo.Id{b.ID})
	if err != nil || len(got) != 1 || got[0].TransactionID != txID || got[0].Interest.String() != "7" {
		t.Fatalf("Splits of b = %v, %v", got, err)
	}
	if got, _ := e.repo.Splits(ctx, []vo.Id{a.ID}); len(got) != 0 {
		t.Errorf("Splits of a = %v, want none", got)
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

	got, err := e.repo.CandidateTransactions(ctx, vo.MustParseId(e.category))
	if err != nil {
		t.Fatalf("CandidateTransactions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("candidates = %+v, want the one expense", got)
	}
	c := got[0]
	if c.ID.String() != expense || c.CurrencyID.String() != eur || c.Amount.String() != "40" || c.PayeeID != nil || c.TagID != nil {
		t.Errorf("candidate = %+v", c)
	}
}

func TestRepo_UsesAndReassign(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	payee := e.f.Payee(fixture.Payee{UserID: e.user, Name: "Bank"})
	tag := e.f.Tag(fixture.Tag{UserID: e.user, Name: "Car"})
	d := e.debt(fixture.NewID())
	pid, tid := vo.MustParseId(payee), vo.MustParseId(tag)
	d.PayeeID, d.TagID = &pid, &tid
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	for kind, id := range map[string]vo.Id{"payee": pid, "tag": tid} {
		got, err := e.repo.ListByClassification(ctx, kind, id)
		if err != nil || len(got) != 1 || got[0].ID != d.ID {
			t.Errorf("ListByClassification(%s) = %v, %v", kind, got, err)
		}
	}
	if _, err := e.repo.ListByClassification(ctx, "currency", pid); err == nil {
		t.Error("an unknown kind must be refused")
	}
	newCat := e.f.Category(fixture.Category{UserID: e.user, Name: "Loan 2"})
	newPayee := e.f.Payee(fixture.Payee{UserID: e.user, Name: "Bank 2"})
	newTag := e.f.Tag(fixture.Tag{UserID: e.user, Name: "Car 2"})

	var _ debt.Repository = e.repo
	check := func(what string, got func(vo.Id) (bool, error), id string, want bool) {
		t.Helper()
		v, err := got(vo.MustParseId(id))
		if err != nil || v != want {
			t.Errorf("%s(%s) = %v, %v; want %v", what, id, v, err, want)
		}
	}
	check("UsesCategory", func(id vo.Id) (bool, error) { return e.repo.UsesCategory(ctx, id) }, e.category, true)
	check("UsesCategory", func(id vo.Id) (bool, error) { return e.repo.UsesCategory(ctx, id) }, newCat, false)
	check("UsesPayee", func(id vo.Id) (bool, error) { return e.repo.UsesPayee(ctx, id) }, payee, true)
	check("UsesPayee", func(id vo.Id) (bool, error) { return e.repo.UsesPayee(ctx, id) }, newPayee, false)
	check("UsesTag", func(id vo.Id) (bool, error) { return e.repo.UsesTag(ctx, id) }, tag, true)
	check("UsesTag", func(id vo.Id) (bool, error) { return e.repo.UsesTag(ctx, id) }, newTag, false)

	if err := e.repo.ReassignCategory(ctx, vo.MustParseId(e.category), vo.MustParseId(newCat)); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.ReassignPayee(ctx, pid, vo.MustParseId(newPayee)); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.ReassignTag(ctx, tid, vo.MustParseId(newTag)); err != nil {
		t.Fatal(err)
	}
	got, err := e.repo.GetByID(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CategoryID.String() != newCat || got.PayeeID == nil || got.PayeeID.String() != newPayee || got.TagID == nil || got.TagID.String() != newTag {
		t.Errorf("after reassign: category=%s payee=%v tag=%v", got.CategoryID, got.PayeeID, got.TagID)
	}
	check("UsesCategory", func(id vo.Id) (bool, error) { return e.repo.UsesCategory(ctx, id) }, e.category, false)
}

func TestRepo_LockOwner(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	err := e.db.TX.WithTx(ctx, func(ctx context.Context) error {
		if err := e.repo.LockOwner(ctx, vo.MustParseId(e.user)); err != nil {
			return err
		}
		return e.repo.LockOwner(ctx, vo.MustParseId(fixture.NewID()))
	})
	if err != nil {
		t.Fatalf("LockOwner: %v", err)
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
	byCat, err := e.repo.ListByCategory(ctx, d.CategoryID)
	if err != nil || len(byCat) != 1 || byCat[0].Current != nil {
		t.Errorf("ListByCategory = %v, %v", byCat, err)
	}
	byOwner, err := e.repo.ListByOwners(ctx, []vo.Id{d.OwnerID})
	if err != nil || len(byOwner) != 1 || byOwner[0].Current != nil {
		t.Errorf("ListByOwners = %v, %v", byOwner, err)
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

func TestRepo_LabelRoundTripAndReassign(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	label := e.f.Label(fixture.Label{UserID: e.user, Name: "Car"})
	other := e.f.Label(fixture.Label{UserID: e.user, Name: "House"})
	lid := vo.MustParseId(label)
	d := e.debt(fixture.NewID())
	d.LabelID = &lid
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := e.repo.GetByID(ctx, d.ID)
	if err != nil || got.LabelID == nil || *got.LabelID != lid {
		t.Fatalf("GetByID label = %v, %v", got, err)
	}
	byOwner, err := e.repo.ListByOwners(ctx, []vo.Id{d.OwnerID})
	if err != nil || len(byOwner) != 1 || byOwner[0].LabelID == nil || *byOwner[0].LabelID != lid {
		t.Errorf("ListByOwners label = %v, %v", byOwner, err)
	}
	byLabel, err := e.repo.ListByClassification(ctx, "label", lid)
	if err != nil || len(byLabel) != 1 || byLabel[0].ID != d.ID {
		t.Errorf("ListByClassification(label) = %v, %v", byLabel, err)
	}
	for id, want := range map[string]bool{label: true, other: false} {
		if v, err := e.repo.UsesLabel(ctx, vo.MustParseId(id)); err != nil || v != want {
			t.Errorf("UsesLabel(%s) = %v, %v; want %v", id, v, err, want)
		}
	}
	if err := e.repo.ReassignLabel(ctx, lid, vo.MustParseId(other)); err != nil {
		t.Fatal(err)
	}
	got, err = e.repo.GetByID(ctx, d.ID)
	if err != nil || got.LabelID == nil || got.LabelID.String() != other {
		t.Fatalf("after reassign: %v, %v", got, err)
	}
	if v, _ := e.repo.UsesLabel(ctx, lid); v {
		t.Error("the old label is still in use")
	}
	d.LabelID = nil
	if err := e.repo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	if got, err = e.repo.GetByID(ctx, d.ID); err != nil || got.LabelID != nil {
		t.Errorf("after clearing: %v, %v", got, err)
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

	got, err := e.repo.CandidateTransactions(ctx, vo.MustParseId(e.category))
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

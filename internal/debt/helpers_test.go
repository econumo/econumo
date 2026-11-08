package debt_test

import (
	"context"
	"errors"
	"strconv"
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

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

type fakeCategory struct {
	owner     string
	icon      string
	isExpense bool
}

type fakeClass struct {
	categories map[string]fakeCategory
	payees     map[string]string
	tags       map[string]string
	labels     map[string]string
}

func (c *fakeClass) CategoryOf(_ context.Context, ownerID, id vo.Id) (string, string, bool, error) {
	cat, ok := c.categories[id.String()]
	if !ok || cat.owner != ownerID.String() {
		return "", "", false, errs.NewNotFound("Category not found")
	}
	return "Category", cat.icon, cat.isExpense, nil
}

func (c *fakeClass) PayeeOwned(_ context.Context, ownerID, id vo.Id) (bool, error) {
	return c.payees[id.String()] == ownerID.String(), nil
}

func (c *fakeClass) TagOwned(_ context.Context, ownerID, id vo.Id) (bool, error) {
	return c.tags[id.String()] == ownerID.String(), nil
}

func (c *fakeClass) LabelOwned(_ context.Context, ownerID, id vo.Id) (bool, error) {
	return c.labels[id.String()] == ownerID.String(), nil
}

// fakeRates converts 1:1 except for currencies with a set multiplier into the
// debt's currency.
type fakeRates struct {
	multiplier map[string]float64
	converted  int
}

func (r *fakeRates) ConvertOnDay(_ context.Context, from, to vo.Id, amount vo.DecimalNumber, _ time.Time) (vo.DecimalNumber, error) {
	r.converted++
	m, ok := r.multiplier[from.String()]
	if !ok || from.Equal(to) {
		return amount, nil
	}
	return amount.Mul(vo.NewDecimal(formatFloat(m))), nil
}

func (r *fakeRates) FractionDigits(context.Context, vo.Id) (int, error) { return 2, nil }

// fakeCurrencies refuses the currencies listed as unusable.
type fakeCurrencies struct{ unusable map[string]bool }

func (c *fakeCurrencies) EnsureUsable(_ context.Context, _, currencyID string) error {
	if c.unusable[currencyID] {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "currencyId", Message: "Currency is not available", Code: errs.CodeCurrencyNotAvailable})
	}
	return nil
}

func formatFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

type transferCall struct {
	from        string
	fromType    model.ElementType
	to          string
	toType      model.ElementType
	currency    string
	sum         bool
	first, last string // Y-m-d, "" = open
	after       string // Y-m-d, "" = unset
}

type fakeBudget struct {
	transfers  []transferCall
	hasData    bool
	leftBehind bool
	// calls is the order of the budget calls: "lock", "transfer", "has-data", "remove".
	calls []string
}

func monthString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func (b *fakeBudget) TransferData(_ context.Context, t model.ElementDataTransfer) (bool, error) {
	b.calls = append(b.calls, "transfer")
	b.transfers = append(b.transfers, transferCall{t.FromID.String(), t.FromType, t.ToID.String(), t.ToType, t.CurrencyID.String(),
		t.SumOnClash, monthString(t.FirstDay), monthString(t.LastDay), monthString(t.AfterDay)})
	return b.leftBehind, nil
}

func (b *fakeBudget) HasData(context.Context, vo.Id, model.ElementType) (bool, error) {
	b.calls = append(b.calls, "has-data")
	return b.hasData, nil
}

func (b *fakeBudget) RemoveRows(context.Context, vo.Id, model.ElementType) error {
	b.calls = append(b.calls, "remove")
	return nil
}

func (b *fakeBudget) LockRows(context.Context, vo.Id, model.ElementType) error {
	b.calls = append(b.calls, "lock")
	return nil
}

type fakeOwners struct{ zones map[string]string }

func (o *fakeOwners) GetTimezone(_ context.Context, id vo.Id) (string, error) {
	return o.zones[id.String()], nil
}

type env struct {
	svc      *debt.Service
	repo     *debtrepo.Repo
	db       *dbtest.DB
	f        *fixture.Builder
	clock    *fakeClock
	class    *fakeClass
	rates    *fakeRates
	currency *fakeCurrencies
	budget   *fakeBudget
	owners   *fakeOwners
	user     string
	category string
	account  string
}

var march10 = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func newEnv(t *testing.T) *env {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	user := f.User(fixture.User{Name: "u"})
	e := &env{
		repo: debtrepo.NewRepo(db.Engine, db.TX), db: db, f: f, user: user,
		clock:    &fakeClock{now: march10},
		class:    &fakeClass{categories: map[string]fakeCategory{}, payees: map[string]string{}, tags: map[string]string{}, labels: map[string]string{}},
		rates:    &fakeRates{multiplier: map[string]float64{}},
		currency: &fakeCurrencies{unusable: map[string]bool{}},
		budget:   &fakeBudget{},
		owners:   &fakeOwners{zones: map[string]string{}},
	}
	e.category = e.newCategory(user, true)
	e.account = f.Account(fixture.Account{UserID: user})
	e.svc = debt.NewService(e.repo, db.TX, e.clock, e.class, e.rates, e.currency, e.owners)
	e.svc.SetBudgetElements(e.budget)
	return e
}

func (e *env) newCategory(owner string, expense bool) string {
	typ := 1
	if expense {
		typ = 0
	}
	id := e.f.Category(fixture.Category{UserID: owner, Name: "Loan " + fixture.NewID()[:6], Type: typ})
	e.class.categories[id] = fakeCategory{owner: owner, icon: "i", isExpense: expense}
	return id
}

func (e *env) newPayee() string {
	id := e.f.Payee(fixture.Payee{UserID: e.user, Name: "Bank " + fixture.NewID()[:6]})
	e.class.payees[id] = e.user
	return id
}

func (e *env) newTag() string {
	id := e.f.Tag(fixture.Tag{UserID: e.user, Name: "Tag " + fixture.NewID()[:6]})
	e.class.tags[id] = e.user
	return id
}

func (e *env) newLabel() string {
	id := e.f.Label(fixture.Label{UserID: e.user, Name: "Label " + fixture.NewID()[:6]})
	e.class.labels[id] = e.user
	return id
}

func (e *env) addLabel(t *testing.T, txID, labelID string) {
	t.Helper()
	if _, err := e.db.Raw.ExecContext(context.Background(), e.db.Rebind("INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)"), txID, labelID); err != nil {
		t.Fatal(err)
	}
}

// withoutTerms leaves the balance and the last payment date out: the debt is
// created not set up.
func withoutTerms(r *model.CreateDebtRequest) {
	r.Balance, r.LastPaymentDate = vo.FlexString{}, ""
}

func (e *env) uid() vo.Id { return vo.MustParseId(e.user) }

func ptr(s string) *string { return &s }

func (e *env) createReq() model.CreateDebtRequest {
	return model.CreateDebtRequest{
		Name: "Car loan", CurrencyId: fixture.USD, CategoryId: e.category,
		Balance: vo.NewFlexString("10000"), Payment: vo.NewFlexString("1000"),
		Schedule: "monthly", LastPaymentDate: "2027-03-01",
	}
}

func (e *env) create(t *testing.T, mod func(*model.CreateDebtRequest)) *model.DebtResult {
	t.Helper()
	req := e.createReq()
	if mod != nil {
		mod(&req)
	}
	res, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatalf("CreateDebt: %v", err)
	}
	return res
}

func (e *env) updateReq(res *model.DebtResult) model.UpdateDebtRequest {
	start := res.StartDate
	var startPtr *string
	if start != "" {
		startPtr = &start
	}
	return model.UpdateDebtRequest{
		Id: res.Id, Name: res.Name, CurrencyId: res.CurrencyId, CategoryId: res.CategoryId,
		PayeeId: res.PayeeId, TagId: res.TagId, LabelId: res.LabelId, StartDate: startPtr,
		Balance: vo.NewFlexString(res.Balance), Payment: vo.NewFlexString(res.Payment),
		Schedule: res.Schedule, LastPaymentDate: res.LastPaymentDate,
	}
}

func (e *env) expense(t *testing.T, accountID string, at time.Time, amount, payeeID string) string {
	t.Helper()
	tx := fixture.Transaction{UserID: e.user, AccountID: accountID, CategoryID: e.category, Type: 0, Amount: amount, SpentAt: at}
	tx.PayeeID = payeeID
	return e.f.Transaction(tx)
}

func (e *env) splits(t *testing.T, debtID string) map[string]string {
	t.Helper()
	got, err := e.repo.Splits(context.Background(), []vo.Id{vo.MustParseId(debtID)})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, s := range got {
		out[s.TransactionID.String()] = s.Interest.String()
	}
	return out
}

func (e *env) snapshots(t *testing.T, debtID string) []*model.DebtSnapshot {
	t.Helper()
	got, err := e.repo.Snapshots(context.Background(), []vo.Id{vo.MustParseId(debtID)})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (e *env) sync(t *testing.T) {
	t.Helper()
	if err := e.svc.Sync(context.Background(), []vo.Id{e.uid()}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

func ymd(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var v *errs.ValidationError
	if !errors.As(err, &v) {
		t.Fatalf("err = %v, want a validation error with code %s", err, code)
	}
	if v.MsgCode == code {
		return
	}
	for _, f := range v.Fields {
		if f.Code == code {
			return
		}
	}
	t.Fatalf("validation error %+v has no code %s", v, code)
}

func requireNotFound(t *testing.T, err error) {
	t.Helper()
	if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("err = %v, want not found", err)
	}
}

func requireField(t *testing.T, err error, key, code string) {
	t.Helper()
	var v *errs.ValidationError
	if !errors.As(err, &v) {
		t.Fatalf("err = %v, want a validation error on %s", err, key)
	}
	for _, f := range v.Fields {
		if f.Key == key && f.Code == code {
			return
		}
	}
	t.Fatalf("validation error %+v has no %s/%s field", v, key, code)
}

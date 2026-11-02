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

func formatFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

type transferCall struct {
	from     string
	fromType int16
	to       string
	toType   int16
	currency string
}

type fakeBudget struct {
	transfers  []transferCall
	hasData    bool
	leftBehind bool
}

func (b *fakeBudget) TransferData(_ context.Context, from vo.Id, fromType int16, to vo.Id, toType int16, currency vo.Id) (bool, error) {
	b.transfers = append(b.transfers, transferCall{from.String(), fromType, to.String(), toType, currency.String()})
	return b.leftBehind, nil
}

func (b *fakeBudget) HasData(context.Context, vo.Id, int16) (bool, error) { return b.hasData, nil }

type env struct {
	svc      *debt.Service
	repo     *debtrepo.Repo
	db       *dbtest.DB
	f        *fixture.Builder
	clock    *fakeClock
	class    *fakeClass
	rates    *fakeRates
	budget   *fakeBudget
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
		clock:  &fakeClock{now: march10},
		class:  &fakeClass{categories: map[string]fakeCategory{}, payees: map[string]string{}, tags: map[string]string{}},
		rates:  &fakeRates{multiplier: map[string]float64{}},
		budget: &fakeBudget{},
	}
	e.category = e.newCategory(user, true)
	e.account = f.Account(fixture.Account{UserID: user})
	e.svc = debt.NewService(e.repo, db.TX, e.clock, e.class, e.rates)
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
		PayeeId: res.PayeeId, TagId: res.TagId, StartDate: startPtr,
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

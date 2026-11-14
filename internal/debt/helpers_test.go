package debt_test

import (
	"context"
	"errors"
	"slices"
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

// fakeCategories names the categories the tests made.
type fakeCategories struct {
	items map[string]model.DebtClassification
}

func (c *fakeCategories) Category(_ context.Context, id vo.Id) (model.DebtClassification, error) {
	item, ok := c.items[id.String()]
	if !ok {
		return model.DebtClassification{}, errs.NewNotFound("Category not found")
	}
	return item, nil
}

// fakeRates converts 1:1 except for currencies with a set multiplier into the
// debt's currency.
type fakeRates struct {
	multiplier map[string]float64
	converted  int
	// digits are every currency's fraction digits; 0 = 2
	digits int
	// digitsOf overrides digits per currency id
	digitsOf map[string]int
}

func (r *fakeRates) ConvertOnDay(_ context.Context, from, to vo.Id, amount vo.DecimalNumber, _ time.Time) (vo.DecimalNumber, error) {
	r.converted++
	m, ok := r.multiplier[from.String()]
	if !ok || from.Equal(to) {
		return amount, nil
	}
	return amount.Mul(vo.NewDecimal(formatFloat(m))), nil
}

func (r *fakeRates) FractionDigits(_ context.Context, id vo.Id) (int, error) {
	if d, ok := r.digitsOf[id.String()]; ok {
		return d, nil
	}
	if r.digits == 0 {
		return 2, nil
	}
	return r.digits, nil
}

func formatFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// fakeElements holds the budgets' elements and their rows' plans, keyed by
// budget then element id.
type fakeElements struct {
	items map[string]map[string]model.DebtElementInfo
	plans map[string]map[string]map[string]float64
}

func (f *fakeElements) Element(_ context.Context, budgetID, elementID vo.Id) (model.DebtElementInfo, error) {
	if el, ok := f.items[budgetID.String()][elementID.String()]; ok {
		return el, nil
	}
	return model.DebtElementInfo{ID: elementID}, nil
}

func (f *fakeElements) Plans(_ context.Context, budgetID, elementID vo.Id) (map[string]float64, error) {
	return f.plans[budgetID.String()][elementID.String()], nil
}

func (f *fakeElements) set(budgetID string, el model.DebtElementInfo) {
	if f.items[budgetID] == nil {
		f.items[budgetID] = map[string]model.DebtElementInfo{}
	}
	f.items[budgetID][el.ID.String()] = el
}

// plan sets the element row's plan for a period key "YYYY-MM-01".
func (e *env) setPlan(budgetID, elementID, period string, amount float64) {
	if e.elements.plans[budgetID] == nil {
		e.elements.plans[budgetID] = map[string]map[string]float64{}
	}
	if e.elements.plans[budgetID][elementID] == nil {
		e.elements.plans[budgetID][elementID] = map[string]float64{}
	}
	e.elements.plans[budgetID][elementID][period] = amount
}

func monthString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

const (
	roleOwner   = "owner"
	roleAdmin   = "admin"
	roleUser    = "user"
	roleGuest   = "guest"
	rolePending = "pending"
)

type fakeBudget struct {
	id, name, currency string
	startDay           int
	archived           bool
	owner              string
	roles              map[string]string
	accounts           []string
	// savings are the savings members among accounts: no debt counts them
	savings []string
}

// fakeBudgets answers access by role per user; a pending invite reads as no
// access at all.
type fakeBudgets struct {
	byID  map[string]*fakeBudget
	order []string
}

func ids(raw ...string) []vo.Id {
	out := make([]vo.Id, 0, len(raw))
	for _, r := range raw {
		out = append(out, vo.MustParseId(r))
	}
	return out
}

func (f *fakeBudgets) view(b *fakeBudget, role string) model.DebtBudget {
	startDay := b.startDay
	if startDay == 0 {
		startDay = 1
	}
	return model.DebtBudget{ID: vo.MustParseId(b.id), CurrencyID: vo.MustParseId(b.currency), Name: b.name, PeriodStartDay: startDay,
		IsArchived: b.archived, CanPlan: role != roleGuest, AccountIDs: ids(b.everyday()...)}
}

func (b *fakeBudget) everyday() []string {
	var out []string
	for _, a := range b.accounts {
		if !slices.Contains(b.savings, a) {
			out = append(out, a)
		}
	}
	return out
}

func (f *fakeBudgets) Access(_ context.Context, userID, budgetID vo.Id) (model.DebtBudget, error) {
	b, ok := f.byID[budgetID.String()]
	if !ok {
		return model.DebtBudget{}, errs.NewNotFound("Budget not found")
	}
	role := b.roles[userID.String()]
	if role == "" || role == rolePending {
		return model.DebtBudget{}, errs.NewNotFound("Budget not found")
	}
	return f.view(b, role), nil
}

func (f *fakeBudgets) Readable(_ context.Context, userID vo.Id) ([]model.DebtBudget, error) {
	var out []model.DebtBudget
	for _, id := range f.order {
		b := f.byID[id]
		if role := b.roles[userID.String()]; role != "" && role != rolePending {
			out = append(out, f.view(b, role))
		}
	}
	return out, nil
}

func (f *fakeBudgets) Scope(_ context.Context, budgetID vo.Id) (model.DebtScope, error) {
	b, ok := f.byID[budgetID.String()]
	if !ok {
		return model.DebtScope{BudgetID: budgetID}, nil
	}
	return model.DebtScope{BudgetID: budgetID, AccountIDs: ids(b.everyday()...)}, nil
}

type fakeUsers struct {
	zones map[string]string
}

func (u *fakeUsers) GetTimezone(_ context.Context, id vo.Id) (string, error) {
	return u.zones[id.String()], nil
}

type env struct {
	svc      *debt.Service
	repo     *debtrepo.Repo
	db       *dbtest.DB
	f        *fixture.Builder
	clock    *fakeClock
	cats     *fakeCategories
	rates    *fakeRates
	elements *fakeElements
	budgets  *fakeBudgets
	users    *fakeUsers
	user     string
	category string
	account  string
	// budget is the user's budget, "Family", holding account
	budget string
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
		cats:     &fakeCategories{items: map[string]model.DebtClassification{}},
		rates:    &fakeRates{multiplier: map[string]float64{}},
		elements: &fakeElements{items: map[string]map[string]model.DebtElementInfo{}, plans: map[string]map[string]map[string]float64{}},
		budgets:  &fakeBudgets{byID: map[string]*fakeBudget{}},
		users:    &fakeUsers{zones: map[string]string{}},
	}
	e.account = f.Account(fixture.Account{UserID: user})
	e.budget = e.newBudget(user, "Family", e.account)
	e.category = e.newCategory(user, true)
	e.svc = debt.NewService(e.repo, db.TX, e.clock, e.cats, e.rates, e.users)
	e.svc.SetBudgets(e.budgets)
	e.svc.SetElements(e.elements)
	return e
}

// newBudget is a budget of owner, in USD, counting the accounts.
func (e *env) newBudget(owner, name string, accounts ...string) string {
	id := e.f.Budget(fixture.Budget{UserID: owner, Name: name})
	e.budgets.byID[id] = &fakeBudget{id: id, name: name, currency: fixture.USD, owner: owner,
		roles: map[string]string{owner: roleOwner}, accounts: accounts}
	e.budgets.order = append(e.budgets.order, id)
	return id
}

func (e *env) fakeBudget(id string) *fakeBudget { return e.budgets.byID[id] }

// partner is another user who takes part in the budget with role.
func (e *env) partner(t *testing.T, name, role string) string {
	t.Helper()
	u := e.f.User(fixture.User{Name: name, Email: name + "@example.test"})
	e.fakeBudget(e.budget).roles[u] = role
	return u
}

// newCategory is a category of owner shown in the user's budget as a
// top-level element in USD, eligible when it is an expense category.
func (e *env) newCategory(owner string, expense bool) string {
	typ := 1
	if expense {
		typ = 0
	}
	name := "Loan " + fixture.NewID()[:6]
	id := e.f.Category(fixture.Category{UserID: owner, Name: name, Type: typ})
	e.cats.items[id] = model.DebtClassification{OwnerID: vo.MustParseId(owner), Name: name, Icon: "i", IsExpense: expense}
	e.elements.set(e.budget, model.DebtElementInfo{ID: vo.MustParseId(id), Type: model.DebtElementCategory, Name: name, Icon: "i",
		CurrencyID: vo.MustParseId(fixture.USD), CategoryIDs: ids(id), Eligible: expense})
	return id
}

// newEnvelope is an expense envelope of the user's budget over the
// categories, in USD.
func (e *env) newEnvelope(name string, categories ...string) string {
	id := fixture.NewID()
	e.elements.set(e.budget, model.DebtElementInfo{ID: vo.MustParseId(id), Type: model.DebtElementEnvelope, Name: name, Icon: "env",
		CurrencyID: vo.MustParseId(fixture.USD), CategoryIDs: ids(categories...), Eligible: true})
	for _, c := range categories {
		el := e.elements.items[e.budget][c]
		el.Eligible = false
		e.elements.items[e.budget][c] = el
	}
	return id
}

func (e *env) newTag() string {
	return e.f.Tag(fixture.Tag{UserID: e.user, Name: "Tag " + fixture.NewID()[:6]})
}

// createSpec is a create-debt request plus the loan's facts, which a
// second, update-debt call sets up.
type createSpec struct {
	BudgetId, ElementId     string
	StartDate               *string
	AmountBorrowed, Balance vo.FlexString
	BalanceDate             string
	Payment                 vo.FlexString
	Schedule                string
	LastPaymentDate         string
}

// withoutTerms leaves the loan's facts out: the debt stays not set up.
func withoutTerms(r *createSpec) {
	r.AmountBorrowed, r.StartDate, r.Balance, r.LastPaymentDate = vo.FlexString{}, nil, vo.FlexString{}, ""
}

func (e *env) uid() vo.Id { return vo.MustParseId(e.user) }

func ptr(s string) *string { return &s }

// createReq is a loan of 10000 taken today (march10) on e.category.
func (e *env) createReq() createSpec {
	return createSpec{
		BudgetId: e.budget, ElementId: e.category,
		AmountBorrowed: vo.NewFlexString("10000"), StartDate: ptr("2026-03-10"), Payment: vo.NewFlexString("1000"),
		Schedule: "monthly", LastPaymentDate: "2027-03-01",
	}
}

// tryCreate creates the debt and, when the spec carries the loan's facts,
// sets it up; the first refusal is returned, and a debt whose setup is
// refused is deleted again, as one create used to be all or nothing.
func (e *env) tryCreate(spec createSpec) (*model.DebtResult, error) {
	ctx := context.Background()
	hasFacts := spec.AmountBorrowed.String() != "" || spec.LastPaymentDate != "" || spec.Balance.String() != ""
	create := model.CreateDebtRequest{BudgetId: spec.BudgetId, ElementId: spec.ElementId, Payment: spec.Payment, Schedule: spec.Schedule}
	if !hasFacts {
		create.StartDate = spec.StartDate
	}
	res, err := e.svc.CreateDebt(ctx, e.uid(), create)
	if err != nil || !hasFacts {
		return res, err
	}
	set, err := e.svc.UpdateDebt(ctx, e.uid(), model.UpdateDebtRequest{Id: res.Id, StartDate: spec.StartDate, AmountBorrowed: spec.AmountBorrowed,
		Balance: spec.Balance, BalanceDate: spec.BalanceDate, LastPaymentDate: spec.LastPaymentDate})
	if err != nil {
		if _, derr := e.svc.DeleteDebt(ctx, e.uid(), model.DeleteDebtRequest{Id: res.Id}); derr != nil {
			return nil, derr
		}
		return nil, err
	}
	return set, nil
}

// createDebtReq is the plain create-debt request on e.category.
func (e *env) createDebtReq() model.CreateDebtRequest {
	return model.CreateDebtRequest{BudgetId: e.budget, ElementId: e.category, Payment: vo.NewFlexString("1000"), Schedule: "monthly"}
}

func (e *env) create(t *testing.T, mod func(*createSpec)) *model.DebtResult {
	t.Helper()
	req := e.createReq()
	if mod != nil {
		mod(&req)
	}
	res, err := e.tryCreate(req)
	if err != nil {
		t.Fatalf("CreateDebt: %v", err)
	}
	return res
}

// updateReq sends back what res holds.
func (e *env) updateReq(res *model.DebtResult) model.UpdateDebtRequest {
	start := res.StartDate
	var startPtr *string
	if start != "" {
		startPtr = &start
	}
	return model.UpdateDebtRequest{
		Id: res.Id, StartDate: startPtr,
		AmountBorrowed: vo.NewFlexString(res.AmountBorrowed), Payment: vo.NewFlexString(res.Payment),
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

func (e *env) scope(budgetID string) model.DebtScope {
	scope, _ := e.budgets.Scope(context.Background(), vo.MustParseId(budgetID))
	return scope
}

func (e *env) sync(t *testing.T) {
	t.Helper()
	if err := e.svc.Sync(context.Background(), e.scope(e.budget)); err != nil {
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

func requireArchived(t *testing.T, err error) {
	t.Helper()
	if ae, ok := errs.AsAccessDenied(err); !ok || ae.Code != errs.CodeBudgetArchived {
		t.Fatalf("err = %v, want budget.archived", err)
	}
}

func requireForbidden(t *testing.T, err error) {
	t.Helper()
	if ae, ok := errs.AsAccessDenied(err); !ok || ae.Code != "" {
		t.Fatalf("err = %v, want access denied", err)
	}
}

func uidOf(id string) vo.Id { return vo.MustParseId(id) }

func fixtureExpense(e *env, category string, at time.Time, amount string) fixture.Transaction {
	return fixture.Transaction{UserID: e.user, AccountID: e.account, CategoryID: category, Type: 0, Amount: amount, SpentAt: at}
}

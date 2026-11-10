package onboarding_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/onboarding"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	me       = "11111111-1111-1111-1111-111111111111"
	budgetID = "019a0000-0000-7000-8000-0000000000b0"
	cash     = "019a0000-0000-7000-8000-000000000001"
	deposit  = "019a0000-0000-7000-8000-000000000002"
	gone     = "019a0000-0000-7000-8000-000000000003"
	theirs   = "019a0000-0000-7000-8000-000000000004"
)

type fakeBudgets struct {
	created []model.CreateBudgetRequest
	added   []model.AddAccountRequest
	marked  []vo.Id
	members []vo.Id
	err     error
}

func (f *fakeBudgets) MemberAccountIDs(context.Context, vo.Id, vo.Id) ([]vo.Id, error) {
	return f.members, f.err
}

func (f *fakeBudgets) CreateBudget(_ context.Context, _ vo.Id, req model.CreateBudgetRequest) (*model.CreateBudgetResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.created = append(f.created, req)
	return &model.CreateBudgetResult{Item: model.BudgetResult{}}, nil
}

func (f *fakeBudgets) AddAccount(_ context.Context, _ vo.Id, req model.AddAccountRequest) (*model.AddAccountResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.added = append(f.added, req)
	return &model.AddAccountResult{Item: model.MetaResult{Id: req.BudgetId}}, nil
}

func (f *fakeBudgets) MarkSavingsMembers(_ context.Context, _, _ vo.Id, ids []vo.Id) error {
	f.marked = append(f.marked, ids...)
	return nil
}

type fakeAccounts struct{}

func (fakeAccounts) AccountsByIDs(_ context.Context, ids []vo.Id) ([]model.OnboardingAccount, error) {
	all := map[string]model.OnboardingAccount{
		cash:    {ID: cash, OwnerID: me, Name: "Wallet", CurrencyCode: "EUR"},
		deposit: {ID: deposit, OwnerID: me, Name: "Tagesgeld", CurrencyCode: "CHF"},
		gone:    {ID: gone, OwnerID: me, Name: "Old savings", CurrencyCode: "EUR", IsDeleted: true},
		theirs:  {ID: theirs, OwnerID: "22222222-2222-2222-2222-222222222222", Name: "Their savings", CurrencyCode: "USD"},
	}
	var out []model.OnboardingAccount
	for _, id := range ids {
		if a, ok := all[id.String()]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

type fakeTimezones struct{}

func (fakeTimezones) StoredTimezone(context.Context, vo.Id) (string, error) {
	return "Europe/Zurich", nil
}

type fakeCompleter struct {
	calls int
	user  string
	reply func(ctx context.Context) (string, error)
}

func (f *fakeCompleter) Complete(ctx context.Context, _, _, user string) (string, error) {
	f.calls++
	f.user = user
	return f.reply(ctx)
}

type denyLimiter struct{}

func (denyLimiter) Allow(context.Context, string, string) error {
	return errs.NewTooManyRequests("Too many attempts. Try again later.")
}
func (denyLimiter) Reserve(string, string) error { return nil }

func replying(s string) *fakeCompleter {
	return &fakeCompleter{reply: func(context.Context) (string, error) { return s, nil }}
}

func newSvc(c onboarding.Completer, l onboarding.AttemptLimiter) (*onboarding.Service, *fakeBudgets) {
	b := &fakeBudgets{}
	svc := onboarding.NewService(b, fakeAccounts{}, fakeTimezones{})
	if c != nil {
		svc.SetAdvisor(c, l)
	}
	svc.SetRunner(func(job func()) { job() })
	return svc, b
}

var allIDs = []string{cash, deposit, gone, theirs}

func createReq() model.OnboardingCreateBudgetRequest {
	return model.OnboardingCreateBudgetRequest{Id: budgetID, Name: "My budget", CurrencyId: "cur", AccountIds: allIDs, NewAccountIds: allIDs}
}

func TestCreateBudget_FlagsTheAIPicksInTheBackground(t *testing.T) {
	c := replying("```json\n{\"savings\":[\"" + strings.ToUpper(deposit) + "\",\"" + theirs + "\",\"nope\"]}\n```")
	svc, b := newSvc(c, nil)
	ctx := reqctx.WithLanguage(context.Background(), "de")
	if _, err := svc.CreateBudget(ctx, vo.MustParseId(me), createReq()); err != nil {
		t.Fatal(err)
	}
	if len(b.created) != 1 || b.created[0].Id != budgetID || len(b.created[0].AccountIds) != 4 || b.created[0].SavingsAccountIds != nil {
		t.Fatalf("created = %+v", b.created)
	}
	if len(b.marked) != 1 || b.marked[0].String() != deposit {
		t.Fatalf("marked = %v", b.marked)
	}
	var prompt struct {
		Timezone, Language string
		Accounts           []struct{ ID, Name, Currency string }
	}
	if err := json.Unmarshal([]byte(c.user), &prompt); err != nil {
		t.Fatal(err)
	}
	if prompt.Timezone != "Europe/Zurich" || prompt.Language != "de" {
		t.Fatalf("context = %+v", prompt)
	}
	if len(prompt.Accounts) != 2 || prompt.Accounts[0].Currency != "EUR" || prompt.Accounts[1].Name != "Tagesgeld" || prompt.Accounts[1].Currency != "CHF" {
		t.Fatalf("accounts = %+v", prompt.Accounts)
	}
	for _, leak := range []string{theirs, gone, "Their savings", "Old savings", "profile", "USD"} {
		if strings.Contains(c.user, leak) {
			t.Fatalf("prompt carries %q: %s", leak, c.user)
		}
	}
}

func TestCreateBudget_ExplicitTimezoneWins(t *testing.T) {
	c := replying(`{"savings":[]}`)
	svc, b := newSvc(c, nil)
	loc, _ := time.LoadLocation("Asia/Tokyo")
	if _, err := svc.CreateBudget(reqctx.WithExplicitLocation(context.Background(), loc), vo.MustParseId(me), createReq()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.user, `"timezone":"Asia/Tokyo"`) || len(b.marked) != 0 {
		t.Fatalf("prompt = %s, marked = %v", c.user, b.marked)
	}
}

func TestCreateBudget_NoAIRunsNoJob(t *testing.T) {
	svc, b := newSvc(nil, nil)
	if _, err := svc.CreateBudget(context.Background(), vo.MustParseId(me), createReq()); err != nil {
		t.Fatal(err)
	}
	if len(b.created) != 1 || len(b.marked) != 0 {
		t.Fatalf("budgets = %+v", b)
	}
}

func TestCreateBudget_FailedCreateRunsNoJob(t *testing.T) {
	c := replying(`{"savings":["` + deposit + `"]}`)
	svc, b := newSvc(c, nil)
	b.err = errors.New("in use")
	if _, err := svc.CreateBudget(context.Background(), vo.MustParseId(me), createReq()); err == nil {
		t.Fatal("want the create error")
	}
	if c.calls != 0 {
		t.Fatal("AI asked after a failed create")
	}
}

func TestCreateBudget_ClassifiesOnlyNewAccounts(t *testing.T) {
	c := replying(`{"savings":["` + deposit + `","` + cash + `"]}`)
	svc, b := newSvc(c, nil)
	req := createReq()
	// More accounts than one onboarding call may classify: the budget still takes them all.
	req.AccountIds = make([]string, 0, 60)
	for i := 0; i < 58; i++ {
		req.AccountIds = append(req.AccountIds, vo.NewId().String())
	}
	req.AccountIds = append(req.AccountIds, cash, deposit)
	req.NewAccountIds = []string{deposit}
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateBudget(context.Background(), vo.MustParseId(me), req); err != nil {
		t.Fatal(err)
	}
	if len(b.created[0].AccountIds) != 60 {
		t.Fatalf("accountIds = %d", len(b.created[0].AccountIds))
	}
	if strings.Contains(c.user, cash) || strings.Contains(c.user, "Wallet") || !strings.Contains(c.user, deposit) {
		t.Fatalf("prompt = %s", c.user)
	}
	if len(b.marked) != 1 || b.marked[0].String() != deposit {
		t.Fatalf("marked = %v", b.marked)
	}
}

func TestCreateBudget_NoNewAccountsAsksNothing(t *testing.T) {
	c := replying(`{"savings":[]}`)
	svc, _ := newSvc(c, nil)
	req := createReq()
	req.NewAccountIds = nil
	if _, err := svc.CreateBudget(context.Background(), vo.MustParseId(me), req); err != nil {
		t.Fatal(err)
	}
	if c.calls != 0 {
		t.Fatal("AI asked with no new account")
	}
}

func TestAddBudgetAccounts_ExistingMembersAreNotReclassified(t *testing.T) {
	c := replying(`{"savings":["` + deposit + `","` + cash + `"]}`)
	svc, b := newSvc(c, nil)
	b.members = []vo.Id{vo.MustParseId(deposit)}
	if _, err := svc.AddBudgetAccounts(context.Background(), vo.MustParseId(me), model.OnboardingAddBudgetAccountsRequest{BudgetId: budgetID, AccountIds: []string{cash, deposit}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(c.user, deposit) {
		t.Fatalf("an existing member was sent: %s", c.user)
	}
	if len(b.marked) != 1 || b.marked[0].String() != cash {
		t.Fatalf("marked = %v", b.marked)
	}
}

func TestClassification_FailuresChangeNothing(t *testing.T) {
	cases := map[string]*fakeCompleter{
		"malformed":   replying("the second one"),
		"wrong shape": replying(`{"savings":"all"}`),
		"error":       {reply: func(context.Context) (string, error) { return "", errors.New("boom") }},
		// The job's deadline is at most 15 s away; one that runs out flags nothing.
		"timeout": {reply: func(ctx context.Context) (string, error) {
			if dl, ok := ctx.Deadline(); !ok || time.Until(dl) > 15*time.Second {
				return `{"savings":["` + deposit + `"]}`, nil
			}
			return "", context.DeadlineExceeded
		}},
		"panic": {reply: func(context.Context) (string, error) { panic("boom") }},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			svc, b := newSvc(c, nil)
			if _, err := svc.CreateBudget(context.Background(), vo.MustParseId(me), createReq()); err != nil {
				t.Fatal(err)
			}
			if len(b.marked) != 0 {
				t.Fatalf("marked = %v", b.marked)
			}
		})
	}
}

func TestClassification_OnlyForeignOrDeletedAsksNothing(t *testing.T) {
	c := replying(`{"savings":[]}`)
	svc, _ := newSvc(c, nil)
	req := createReq()
	req.AccountIds = []string{gone, theirs}
	req.NewAccountIds = req.AccountIds
	if _, err := svc.CreateBudget(context.Background(), vo.MustParseId(me), req); err != nil {
		t.Fatal(err)
	}
	if c.calls != 0 {
		t.Fatalf("AI asked %d times", c.calls)
	}
}

func TestClassification_OverTheCapSkipsTheJobNotTheWrite(t *testing.T) {
	c := replying(`{"savings":["` + deposit + `"]}`)
	svc, b := newSvc(c, denyLimiter{})
	if _, err := svc.CreateBudget(context.Background(), vo.MustParseId(me), createReq()); err != nil {
		t.Fatal(err)
	}
	if len(b.created) != 1 || c.calls != 0 {
		t.Fatalf("created = %d, calls = %d", len(b.created), c.calls)
	}
}

func TestAddBudgetAccounts_AddsEverydayThenClassifies(t *testing.T) {
	c := replying(`{"savings":["` + deposit + `"]}`)
	svc, b := newSvc(c, nil)
	res, err := svc.AddBudgetAccounts(context.Background(), vo.MustParseId(me), model.OnboardingAddBudgetAccountsRequest{BudgetId: budgetID, AccountIds: []string{cash, deposit, cash}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.Id != budgetID || len(b.added) != 2 || b.added[0].IsSavings != nil || b.added[1].AccountId != deposit {
		t.Fatalf("added = %+v", b.added)
	}
	if len(b.marked) != 1 || b.marked[0].String() != deposit {
		t.Fatalf("marked = %v", b.marked)
	}
}

func TestAddBudgetAccounts_ErrorStopsBeforeTheJob(t *testing.T) {
	c := replying(`{"savings":[]}`)
	svc, b := newSvc(c, nil)
	b.err = errs.NewAccessDenied("Access denied")
	if _, err := svc.AddBudgetAccounts(context.Background(), vo.MustParseId(me), model.OnboardingAddBudgetAccountsRequest{BudgetId: budgetID, AccountIds: []string{cash}}); err == nil {
		t.Fatal("want the add error")
	}
	if c.calls != 0 {
		t.Fatal("AI asked after a failed add")
	}
}

func TestRequests_Validate(t *testing.T) {
	many := make([]string, model.MaxOnboardingAccountIds+1)
	for i := range many {
		many[i] = cash
	}
	bad := map[string]interface{ Validate() error }{
		"create blank":     model.OnboardingCreateBudgetRequest{},
		"add blank budget": model.OnboardingAddBudgetAccountsRequest{AccountIds: []string{cash}},
		"add no accounts":  model.OnboardingAddBudgetAccountsRequest{BudgetId: budgetID},
		"add too many":     model.OnboardingAddBudgetAccountsRequest{BudgetId: budgetID, AccountIds: many},
		"add not uuid":     model.OnboardingAddBudgetAccountsRequest{BudgetId: budgetID, AccountIds: []string{"x"}},
		"new too many":     model.OnboardingCreateBudgetRequest{Id: budgetID, Name: "B", AccountIds: many, NewAccountIds: many},
		"new not member":   model.OnboardingCreateBudgetRequest{Id: budgetID, Name: "B", AccountIds: []string{cash}, NewAccountIds: []string{deposit}},
		"new not uuid":     model.OnboardingCreateBudgetRequest{Id: budgetID, Name: "B", AccountIds: []string{"x"}, NewAccountIds: []string{"x"}},
	}
	for name, r := range bad {
		var ve *errs.ValidationError
		if err := r.Validate(); !errors.As(err, &ve) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
	if err := (model.OnboardingAddBudgetAccountsRequest{BudgetId: budgetID, AccountIds: []string{cash}}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (model.OnboardingCreateBudgetRequest{Id: budgetID, Name: "B", AccountIds: many, NewAccountIds: []string{strings.ToUpper(cash)}}).Validate(); err != nil {
		t.Fatalf("more than 50 accountIds must pass: %v", err)
	}
}

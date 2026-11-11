package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

type stepClock struct{ t time.Time }

func (c stepClock) Now() time.Time { return c.t }

type recordingCompleter struct {
	mu        sync.Mutex
	prompts   []string
	systems   []string
	deadlines []time.Time
	// reply, when set, answers instead of the empty savings list; bySystem,
	// when set, answers by the request's system prompt instead.
	reply    func() (string, error)
	bySystem func(system string) (string, error)
}

func (c *recordingCompleter) Complete(ctx context.Context, _, system, user string) (string, error) {
	c.mu.Lock()
	deadline, _ := ctx.Deadline()
	c.deadlines = append(c.deadlines, deadline)
	c.prompts = append(c.prompts, user)
	c.systems = append(c.systems, system)
	reply, bySystem := c.reply, c.bySystem
	c.mu.Unlock()
	if bySystem != nil {
		return bySystem(system)
	}
	if reply != nil {
		return reply()
	}
	return `{"savings":[]}`, nil
}

// promptsFor are the prompts sent with a system prompt carrying marker.
func (c *recordingCompleter) promptsFor(marker string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for i, sys := range c.systems {
		if strings.Contains(sys, marker) {
			out = append(out, c.prompts[i])
		}
	}
	return out
}

type onbEnv struct {
	t     *testing.T
	db    *dbtest.DB
	f     *fixture.Builder
	url   string
	user  string
	token string
	ai    *recordingCompleter
	eur   string
}

func newOnbEnv(t *testing.T, now time.Time) *onbEnv {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	user := f.User(fixture.User{Name: "Me"})
	f.DefaultOptions(user)
	token := "eco_pat_onboarding-steps-test-token-000000000000000"
	exp := now.Add(24 * time.Hour)
	f.AccessToken(fixture.AccessToken{UserID: user, Kind: "personal", TokenHash: appuser.HashAccessToken(token), ExpiresAt: &exp})
	eur := f.Currency(fixture.Currency{Code: "EUR", Symbol: "€", Name: "Euro"})
	ai := &recordingCompleter{}
	ts := httptest.NewServer(server.BuildAPI(baseTestConfig(db.Engine), db.Raw, server.Seams{
		Clock:               stepClock{t: now},
		Avatars:             appuser.FixedAvatarPicker(appuser.DefaultAvatar),
		OnboardingCompleter: ai,
		OnboardingRunner:    func(job func()) { job() },
	}))
	t.Cleanup(ts.Close)
	return &onbEnv{t: t, db: db, f: f, url: ts.URL, user: user, token: token, ai: ai, eur: eur}
}

func (e *onbEnv) post(path string, body any) (int, map[string]any) {
	e.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		e.t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, e.url+path, strings.NewReader(string(raw)))
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+e.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "de")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var env map[string]any
	if err := json.Unmarshal(b, &env); err != nil {
		e.t.Fatalf("%s: %s", path, b)
	}
	data, _ := env["data"].(map[string]any)
	if resp.StatusCode != http.StatusOK {
		data = env
	}
	return resp.StatusCode, data
}

func (e *onbEnv) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.Raw.QueryRow(e.db.Rebind(query), args...).Scan(&n); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
	return n
}

func (e *onbEnv) str(query string, args ...any) string {
	e.t.Helper()
	var s string
	if err := e.db.Raw.QueryRow(e.db.Rebind(query), args...).Scan(&s); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
	return s
}

func (e *onbEnv) onboarding() string {
	return e.str(`SELECT value FROM users_options WHERE user_id = ? AND name = 'onboarding'`, e.user)
}

func (e *onbEnv) budgetCount() int {
	return e.count(`SELECT COUNT(*) FROM budgets WHERE user_id = ?`, e.user)
}

func stringMap(t *testing.T, v any) map[string]string {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("not a map: %#v", v)
	}
	out := map[string]string{}
	for k, val := range m {
		out[k] = val.(string)
	}
	return out
}

var stepNow = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func categoriesBody(id string, rows ...[3]string) map[string]any {
	list := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		list = append(list, map[string]any{"key": r[0], "name": r[1], "type": r[2], "icon": "home"})
	}
	return map[string]any{"id": id, "categories": list}
}

func TestSaveCategories_CreatesMergesAndNeedsAccountsForABudget(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	existing := e.f.Category(fixture.Category{UserID: e.user, Name: "Rent", Type: 0})
	op := fixture.NewID()
	body := categoriesBody(op, [3]string{"k1", "Food", "expense"}, [3]string{"k2", " food ", "expense"}, [3]string{"k3", "rent", "expense"}, [3]string{"k4", "Food", "income"})
	status, data := e.post("/api/v1/onboarding/save-categories", body)
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	got := stringMap(t, data["categories"])
	if len(got) != 4 || got["k1"] != got["k2"] || got["k3"] != existing || got["k4"] == got["k1"] {
		t.Fatalf("categories = %v", got)
	}
	if n := e.count(`SELECT COUNT(*) FROM categories WHERE user_id = ?`, e.user); n != 3 {
		t.Fatalf("categories rows = %d", n)
	}
	if data["budgetId"] != "" || e.budgetCount() != 0 {
		t.Fatalf("budget without accounts: %v / %d", data["budgetId"], e.budgetCount())
	}
	if e.onboarding() != "categories" {
		t.Fatalf("onboarding = %q", e.onboarding())
	}

	// The same id again: the same map, no new rows.
	status, again := e.post("/api/v1/onboarding/save-categories", body)
	if status != 200 || len(stringMap(t, again["categories"])) != 4 || stringMap(t, again["categories"])["k1"] != got["k1"] {
		t.Fatalf("replay %d: %v", status, again)
	}
	// A new id once the step is done changes nothing either.
	status, done := e.post("/api/v1/onboarding/save-categories", categoriesBody(fixture.NewID(), [3]string{"k9", "Travel", "expense"}))
	if status != 200 || len(stringMap(t, done["categories"])) != 0 {
		t.Fatalf("done step %d: %v", status, done)
	}
	if n := e.count(`SELECT COUNT(*) FROM categories WHERE user_id = ?`, e.user); n != 3 {
		t.Fatalf("categories rows after replays = %d", n)
	}
}

func TestSaveCategories_OrderAndBudgetWhenAccountsExist(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	acc := e.f.Account(fixture.Account{UserID: e.user, Name: "Wallet"})
	gone := e.f.Account(fixture.Account{UserID: e.user, Name: "Old", Deleted: true})
	salary := e.f.Category(fixture.Category{UserID: e.user, Name: "Salary", Type: 1})
	body := categoriesBody(fixture.NewID(), [3]string{"a", "Food", "expense"}, [3]string{"b", "Cafe", "expense"}, [3]string{"c", "Bonus", "income"})
	body["order"] = []string{"b", "a", "c", salary}
	status, data := e.post("/api/v1/onboarding/save-categories", body)
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	ids := stringMap(t, data["categories"])
	rows, err := e.db.Raw.Query(e.db.Rebind(`SELECT id FROM categories WHERE user_id = ? AND type = 0 ORDER BY sort_key`), e.user)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		order = append(order, id)
	}
	rows.Close()
	if len(order) != 2 || order[0] != ids["b"] || order[1] != ids["a"] {
		t.Fatalf("expense order = %v, want b,a = %v", order, ids)
	}
	budgetID, _ := data["budgetId"].(string)
	if budgetID == "" || e.budgetCount() != 1 {
		t.Fatalf("budget = %q / %d", budgetID, e.budgetCount())
	}
	if name := e.str(`SELECT name FROM budgets WHERE id = ?`, budgetID); name != "Mein Budget" {
		t.Fatalf("budget name = %q", name)
	}
	if n := e.count(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ? AND account_id = ?`, budgetID, acc); n != 1 {
		t.Fatal("live account not a member")
	}
	if n := e.count(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ? AND account_id = ?`, budgetID, gone); n != 0 {
		t.Fatal("deleted account is a member")
	}
	if n := e.count(`SELECT COUNT(*) FROM budgets_elements WHERE budget_id = ? AND external_id = ?`, budgetID, ids["a"]); n != 1 {
		t.Fatal("new category has no budget element")
	}
}

func TestSaveCategories_FailureOnTheLastRowRollsEverythingBack(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	e.f.Account(fixture.Account{UserID: e.user, Name: "Wallet"})
	op := fixture.NewID()
	status, data := e.post("/api/v1/onboarding/save-categories", categoriesBody(op, [3]string{"a", "Food", "expense"}, [3]string{"b", "ab", "expense"}))
	if status != 400 {
		t.Fatalf("status %d: %v", status, data)
	}
	if n := e.count(`SELECT COUNT(*) FROM categories WHERE user_id = ?`, e.user); n != 0 {
		t.Fatalf("categories left: %d", n)
	}
	if e.budgetCount() != 0 || e.onboarding() != "started" {
		t.Fatalf("budget %d, onboarding %q", e.budgetCount(), e.onboarding())
	}
	// The operation id was rolled back with the rest: the corrected call goes through.
	status, data = e.post("/api/v1/onboarding/save-categories", categoriesBody(op, [3]string{"a", "Food", "expense"}, [3]string{"b", "Cafe", "expense"}))
	if status != 200 || len(stringMap(t, data["categories"])) != 2 || data["budgetId"] == "" {
		t.Fatalf("retry %d: %v", status, data)
	}
}

func TestSaveCategories_Validation(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	for name, body := range map[string]any{
		"no id":     categoriesBody("", [3]string{"a", "Food", "expense"}),
		"dup key":   categoriesBody(fixture.NewID(), [3]string{"a", "Food", "expense"}, [3]string{"a", "Cafe", "expense"}),
		"bad type":  categoriesBody(fixture.NewID(), [3]string{"a", "Food", "spending"}),
		"blank key": categoriesBody(fixture.NewID(), [3]string{"", "Food", "expense"}),
	} {
		if status, data := e.post("/api/v1/onboarding/save-categories", body); status != 400 {
			t.Fatalf("%s: status %d: %v", name, status, data)
		}
	}
	if e.onboarding() != "started" {
		t.Fatalf("onboarding = %q", e.onboarding())
	}
}

func accountsBody(id, main string, currencyIDs []string, rows ...[4]string) map[string]any {
	list := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		list = append(list, map[string]any{"key": r[0], "name": r[1], "balance": r[2], "currencyId": r[3], "icon": "wallet"})
	}
	return map[string]any{"id": id, "accounts": list, "currencyIds": currencyIDs, "mainCurrencyId": main}
}

func TestSaveAccounts_AccountsFirstCreatesTheBudget(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	earlier := e.f.Account(fixture.Account{UserID: e.user, Name: "Earlier"})
	e.f.Folder(fixture.Folder{UserID: e.user, Name: "Mine"})
	body := accountsBody(fixture.NewID(), e.eur, []string{fixture.USD},
		[4]string{"w", "Wallet", "", e.eur}, [4]string{"s", "Savings", "100.50", fixture.USD}, [4]string{"d", "Debt", "-20", e.eur})
	status, data := e.post("/api/v1/onboarding/save-accounts", body)
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	ids := stringMap(t, data["accounts"])
	if len(ids) != 3 {
		t.Fatalf("accounts = %v", ids)
	}
	budgetID, _ := data["budgetId"].(string)
	if budgetID == "" || e.budgetCount() != 1 {
		t.Fatalf("budget %q / %d", budgetID, e.budgetCount())
	}
	if n := e.count(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ?`, budgetID); n != 4 {
		t.Fatalf("members = %d, want the earlier account plus three new", n)
	}
	if cur := e.str(`SELECT currency_id FROM budgets WHERE id = ?`, budgetID); cur != e.eur {
		t.Fatalf("budget currency = %s", cur)
	}
	if n := e.count(`SELECT COUNT(*) FROM transactions WHERE account_id IN (?, ?)`, ids["s"], ids["d"]); n != 2 {
		t.Fatalf("opening corrections = %d", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM transactions WHERE account_id = ?`, ids["w"]); n != 0 {
		t.Fatal("zero balance wrote a correction")
	}
	if n := e.count(`SELECT COUNT(*) FROM folders WHERE user_id = ?`, e.user); n != 1 {
		t.Fatalf("folders = %d", n)
	}
	user := data["user"].(map[string]any)
	if user["currency"] != "EUR" || e.onboarding() != "accounts" {
		t.Fatalf("currency %v, onboarding %q", user["currency"], e.onboarding())
	}
	// Only the new accounts reach the classifier, never the earlier one.
	if len(e.ai.prompts) != 1 || strings.Contains(e.ai.prompts[0], earlier) || !strings.Contains(e.ai.prompts[0], ids["s"]) {
		t.Fatalf("prompts = %v", e.ai.prompts)
	}

	status, again := e.post("/api/v1/onboarding/save-accounts", body)
	if status != 200 || stringMap(t, again["accounts"])["s"] != ids["s"] || again["budgetId"] != budgetID {
		t.Fatalf("replay %d: %v", status, again)
	}
	if n := e.count(`SELECT COUNT(*) FROM accounts WHERE user_id = ?`, e.user); n != 4 || len(e.ai.prompts) != 1 {
		t.Fatalf("accounts after replay = %d, prompts = %d", n, len(e.ai.prompts))
	}
}

func TestSaveAccounts_FirstAccountCreatesTheFolderAndVisibleCurrencies(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	chf := e.f.Currency(fixture.Currency{Code: "CHF", Symbol: "Fr", Name: "Franc"})
	status, data := e.post("/api/v1/onboarding/save-accounts", accountsBody(fixture.NewID(), e.eur, []string{chf, fixture.NewID()},
		[4]string{"a", "Wallet", "", e.eur}, [4]string{"b", "Card", "", e.eur}))
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	if n := e.count(`SELECT COUNT(*) FROM folders WHERE user_id = ?`, e.user); n != 1 {
		t.Fatalf("folders = %d", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM accounts_folders`); n != 2 {
		t.Fatalf("folder members = %d", n)
	}
	hidden := e.count(`SELECT COUNT(*) FROM users_hidden_currencies WHERE user_id = ?`, e.user)
	for _, visible := range []string{e.eur, chf, fixture.USD} {
		if e.count(`SELECT COUNT(*) FROM users_hidden_currencies WHERE user_id = ? AND currency_id = ?`, e.user, visible) != 0 {
			t.Fatalf("%s hidden (%d hidden in all)", visible, hidden)
		}
	}
}

func TestSaveAccounts_AddsToTheExistingBudget(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	old := e.f.Account(fixture.Account{UserID: e.user, Name: "Old"})
	e.f.Folder(fixture.Folder{UserID: e.user, Name: "Mine"})
	budget := e.f.Budget(fixture.Budget{UserID: e.user})
	e.f.BudgetAccount(budget, old)
	status, data := e.post("/api/v1/onboarding/save-accounts", accountsBody(fixture.NewID(), fixture.USD, nil, [4]string{"n", "New", "5", fixture.USD}))
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	newID := stringMap(t, data["accounts"])["n"]
	if data["budgetId"] != budget || e.budgetCount() != 1 {
		t.Fatalf("budget %v / %d", data["budgetId"], e.budgetCount())
	}
	if n := e.count(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ?`, budget); n != 2 {
		t.Fatalf("members = %d", n)
	}
	if len(e.ai.prompts) != 1 || strings.Contains(e.ai.prompts[0], old) || !strings.Contains(e.ai.prompts[0], newID) {
		t.Fatalf("prompts = %v", e.ai.prompts)
	}
}

func TestSaveAccounts_FailureOnTheLastRowRollsEverythingBack(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	other := e.f.Account(fixture.Account{UserID: e.user, Name: "Other"})
	revision := e.count(`SELECT transactions_revision FROM accounts WHERE id = ?`, other)
	op := fixture.NewID()
	status, data := e.post("/api/v1/onboarding/save-accounts", accountsBody(op, e.eur, nil,
		[4]string{"a", "Wallet", "10", e.eur}, [4]string{"b", "Card", "", fixture.NewID()}))
	if status == 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	if n := e.count(`SELECT COUNT(*) FROM accounts WHERE user_id = ?`, e.user); n != 1 {
		t.Fatalf("accounts = %d", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM transactions`); n != 0 {
		t.Fatalf("transactions = %d", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM folders WHERE user_id = ?`, e.user); n != 0 {
		t.Fatalf("folders = %d", n)
	}
	if e.budgetCount() != 0 || e.onboarding() != "started" || len(e.ai.prompts) != 0 {
		t.Fatalf("budget %d, onboarding %q, prompts %d", e.budgetCount(), e.onboarding(), len(e.ai.prompts))
	}
	if e.count(`SELECT transactions_revision FROM accounts WHERE id = ?`, other) != revision {
		t.Fatal("revision moved")
	}
	if cur := e.str(`SELECT value FROM users_options WHERE user_id = ? AND name = 'currency'`, e.user); cur != fixture.USD {
		t.Fatalf("profile currency = %s", cur)
	}
	status, data = e.post("/api/v1/onboarding/save-accounts", accountsBody(op, e.eur, nil, [4]string{"a", "Wallet", "10", e.eur}))
	if status != 200 || len(stringMap(t, data["accounts"])) != 1 {
		t.Fatalf("retry %d: %v", status, data)
	}
}

func TestSaveAccounts_Validation(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	for name, body := range map[string]any{
		"bad balance":  accountsBody(fixture.NewID(), e.eur, nil, [4]string{"a", "Wallet", "1,5", e.eur}),
		"no main":      accountsBody(fixture.NewID(), "", nil, [4]string{"a", "Wallet", "", e.eur}),
		"unknown main": accountsBody(fixture.NewID(), fixture.NewID(), nil, [4]string{"a", "Wallet", "", e.eur}),
		"short name":   accountsBody(fixture.NewID(), e.eur, nil, [4]string{"a", "W", "", e.eur}),
	} {
		if status, data := e.post("/api/v1/onboarding/save-accounts", body); status != 400 {
			t.Fatalf("%s: status %d: %v", name, status, data)
		}
	}
	if n := e.count(`SELECT COUNT(*) FROM accounts WHERE user_id = ?`, e.user); n != 0 {
		t.Fatalf("accounts = %d", n)
	}
}

// A user who already has categories and accounts continues without new rows:
// the step is marked, and with no account at all no budget is created.
func TestSaveSteps_NoRows(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	status, data := e.post("/api/v1/onboarding/save-categories", map[string]any{"id": fixture.NewID(), "categories": []any{}})
	if status != 200 || data["budgetId"] != "" || e.onboarding() != "categories" {
		t.Fatalf("categories %d: %v (%q)", status, data, e.onboarding())
	}
	status, data = e.post("/api/v1/onboarding/save-accounts", accountsBody(fixture.NewID(), e.eur, nil))
	if status != 200 || data["budgetId"] != "" || e.onboarding() != "categories,accounts" || e.budgetCount() != 0 {
		t.Fatalf("accounts %d: %v (%q)", status, data, e.onboarding())
	}
	if cur := e.str(`SELECT value FROM users_options WHERE user_id = ? AND name = 'currency'`, e.user); cur != e.eur {
		t.Fatalf("profile currency = %s", cur)
	}
}

func TestSavePlan_WritesTheCurrentPeriodOfAShiftedBudget(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	e.f.Folder(fixture.Folder{UserID: e.user, Name: "Mine"})
	acc := e.f.Account(fixture.Account{UserID: e.user, Name: "Wallet"})
	food := e.f.Category(fixture.Category{UserID: e.user, Name: "Food", Type: 0})
	salary := e.f.Category(fixture.Category{UserID: e.user, Name: "Salary", Type: 1})
	budget := e.f.Budget(fixture.Budget{UserID: e.user, StartedAt: "2026-01-01 00:00:00"})
	e.f.BudgetAccount(budget, acc)
	if _, err := e.db.Raw.Exec(e.db.Rebind(`UPDATE budgets SET period_start_day = 15 WHERE id = ?`), budget); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Raw.Exec(e.db.Rebind(`UPDATE users_options SET value = ? WHERE user_id = ? AND name = 'budget'`), budget, e.user); err != nil {
		t.Fatal(err)
	}
	op := fixture.NewID()
	body := map[string]any{"id": op, "amounts": []map[string]any{{"categoryId": food, "amount": "250.5"}, {"categoryId": salary, "amount": "0"}}}
	status, data := e.post("/api/v1/onboarding/save-plan", body)
	if status != 200 || data["budgetId"] != budget {
		t.Fatalf("status %d: %v", status, data)
	}
	// March 10 with months starting on the 15th is in the period keyed February.
	var periods []string
	rows, err := e.db.Raw.Query(e.db.Rebind(`SELECT l.period, l.amount FROM budgets_elements_limits l JOIN budgets_elements e ON e.id = l.element_id WHERE e.budget_id = ?`), budget)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var p, a string
		_ = rows.Scan(&p, &a)
		periods = append(periods, p[:10]+"="+a)
	}
	rows.Close()
	sort.Strings(periods)
	if len(periods) != 2 || !strings.HasPrefix(periods[0], "2026-02-01=0") || !strings.HasPrefix(periods[1], "2026-02-01=250.5") {
		t.Fatalf("limits = %v", periods)
	}
	if e.onboarding() != "plan" {
		t.Fatalf("onboarding = %q", e.onboarding())
	}
	if status, data := e.post("/api/v1/onboarding/save-plan", body); status != 200 || data["budgetId"] != budget {
		t.Fatalf("replay %d: %v", status, data)
	}
	if n := e.count(`SELECT COUNT(*) FROM budgets_elements_limits`); n != 2 {
		t.Fatalf("limits after replay = %d", n)
	}
}

func TestSavePlan_FailureRollsBackAndNoBudgetIsAnError(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	food := e.f.Category(fixture.Category{UserID: e.user, Name: "Food", Type: 0})
	body := map[string]any{"id": fixture.NewID(), "amounts": []map[string]any{{"categoryId": food, "amount": "10"}}}
	if status, data := e.post("/api/v1/onboarding/save-plan", body); status != 400 {
		t.Fatalf("no budget: status %d: %v", status, data)
	}
	acc := e.f.Account(fixture.Account{UserID: e.user, Name: "Wallet"})
	budget := e.f.Budget(fixture.Budget{UserID: e.user})
	e.f.BudgetAccount(budget, acc)
	body = map[string]any{"id": fixture.NewID(), "budgetId": budget, "amounts": []map[string]any{{"categoryId": food, "amount": "10"}, {"categoryId": fixture.NewID(), "amount": "5"}}}
	if status, data := e.post("/api/v1/onboarding/save-plan", body); status == 200 {
		t.Fatalf("unknown category: status %d: %v", status, data)
	}
	if n := e.count(`SELECT COUNT(*) FROM budgets_elements_limits`); n != 0 || e.onboarding() != "started" {
		t.Fatalf("limits = %d, onboarding %q", n, e.onboarding())
	}
	for name, bad := range map[string]any{
		"negative":   map[string]any{"id": fixture.NewID(), "amounts": []map[string]any{{"categoryId": food, "amount": "-1"}}},
		"blank":      map[string]any{"id": fixture.NewID(), "amounts": []map[string]any{{"categoryId": food, "amount": ""}}},
		"bad budget": map[string]any{"id": fixture.NewID(), "budgetId": "nope", "amounts": []any{}},
	} {
		if status, data := e.post("/api/v1/onboarding/save-plan", bad); status != 400 {
			t.Fatalf("%s: status %d: %v", name, status, data)
		}
	}
}

func TestSavePlan_ForeignBudgetIsRefused(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	other := e.f.User(fixture.User{Email: "other@example.test"})
	budget := e.f.Budget(fixture.Budget{UserID: other})
	body := map[string]any{"id": fixture.NewID(), "budgetId": budget, "amounts": []any{}}
	if status, data := e.post("/api/v1/onboarding/save-plan", body); status != 403 {
		t.Fatalf("status %d: %v", status, data)
	}
}

// operation_requests_ids is shared by every create endpoint, so an id another
// write claimed is refused while the step is still open; once the step is done
// the same id is the no-op replay.
func TestSaveSteps_AnIDClaimedByAnotherWriteIsRefused(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	op := fixture.NewID()
	if status, data := e.post("/api/v1/onboarding/save-categories", categoriesBody(op, [3]string{"a", "Food", "expense"})); status != 200 {
		t.Fatalf("categories %d: %v", status, data)
	}
	status, data := e.post("/api/v1/onboarding/save-accounts", accountsBody(op, e.eur, nil, [4]string{"a", "Wallet", "", e.eur}))
	errsMap, _ := data["errors"].(map[string]any)
	if status != 400 || errsMap["id"] == nil {
		t.Fatalf("reused id %d: %v", status, data)
	}
	if n := e.count(`SELECT COUNT(*) FROM accounts WHERE user_id = ?`, e.user); n != 0 || e.onboarding() != "categories" {
		t.Fatalf("accounts = %d, onboarding %q", n, e.onboarding())
	}
	if status, _ := e.post("/api/v1/onboarding/save-categories", categoriesBody(op, [3]string{"a", "Food", "expense"})); status != 200 {
		t.Fatalf("replay of a done step %d", status)
	}
}

func TestSaveAccounts_ReplayNeverMapsARowToAnOlderAccountOfTheSameName(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	older := e.f.Account(fixture.Account{UserID: e.user, Name: "Cash"})
	op := fixture.NewID()
	body := accountsBody(op, e.eur, nil, [4]string{"c", "Cash", "5", e.eur})
	status, data := e.post("/api/v1/onboarding/save-accounts", body)
	created := stringMap(t, data["accounts"])["c"]
	if status != 200 || created == "" || created == older {
		t.Fatalf("save %d: %v", status, data)
	}
	status, again := e.post("/api/v1/onboarding/save-accounts", body)
	if got := stringMap(t, again["accounts"])["c"]; status != 200 || got != created {
		t.Fatalf("replay mapped c to %s, want %s (older %s)", got, created, older)
	}
}

func TestSaveAccounts_MainCurrencyMustBeUsable(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	other := e.f.User(fixture.User{Name: "Other", Email: "other@onboarding.test"})
	foreign := e.f.Currency(fixture.Currency{Code: "XFO", Symbol: "X", UserID: other, Rate: "2.00000000"})
	gone := e.f.Currency(fixture.Currency{Code: "XGO", Symbol: "G", Deleted: true})
	for name, main := range map[string]string{"foreign custom": foreign, "deleted": gone} {
		status, data := e.post("/api/v1/onboarding/save-accounts", accountsBody(fixture.NewID(), main, nil, [4]string{"a", "Wallet", "", e.eur}))
		errsMap, _ := data["errors"].(map[string]any)
		if status != 400 || errsMap["mainCurrencyId"] == nil || strings.Contains(data["message"].(string), "XFO") {
			t.Fatalf("%s: %d %v", name, status, data)
		}
	}
	if e.onboarding() != "started" {
		t.Fatalf("onboarding = %q", e.onboarding())
	}
}

func TestSaveAccounts_KeepsTheCurrenciesOfExistingAccountsVisible(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	chf := e.f.Currency(fixture.Currency{Code: "CHF", Symbol: "Fr", Name: "Franc"})
	e.f.Account(fixture.Account{UserID: e.user, Name: "Swiss", CurrencyID: chf})
	if status, data := e.post("/api/v1/onboarding/save-accounts", accountsBody(fixture.NewID(), e.eur, nil, [4]string{"a", "Wallet", "", e.eur})); status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	if e.count(`SELECT COUNT(*) FROM users_hidden_currencies WHERE user_id = ? AND currency_id = ?`, e.user, chf) != 0 {
		t.Fatal("the currency of an existing account was hidden")
	}
}

func TestUpdateOnboarding_NeverReopensAStepTheServerMarked(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	if status, _ := e.post("/api/v1/onboarding/save-categories", categoriesBody(fixture.NewID(), [3]string{"a", "Food", "expense"})); status != 200 {
		t.Fatal("save-categories")
	}
	// a stale stepper that still shows Categories open skips it
	if status, data := e.post("/api/v1/user/update-onboarding", map[string]any{"value": "!import,!categories"}); status != 200 {
		t.Fatalf("update %d: %v", status, data)
	}
	if got := e.onboarding(); got != "!import,categories" {
		t.Fatalf("onboarding = %q", got)
	}
}

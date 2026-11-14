package server_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/fixture"
)

func (e *onbEnv) get(path string) (int, map[string]any) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.url+path, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+e.token)
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

func (e *onbEnv) icon(table, id string) string {
	return e.str(`SELECT icon FROM `+table+` WHERE id = ?`, id)
}

func (e *onbEnv) isSavings(budget, account string) bool {
	return e.count(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ? AND account_id = ? AND is_savings = ?`, budget, account, true) == 1
}

func (e *onbEnv) member(budget, account string) bool {
	return e.count(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ? AND account_id = ?`, budget, account) == 1
}

func (e *onbEnv) setProfileCurrency(currencyID string) {
	e.t.Helper()
	if _, err := e.db.Raw.Exec(e.db.Rebind(`UPDATE users_options SET value = ? WHERE user_id = ? AND name = 'currency'`), currencyID, e.user); err != nil {
		e.t.Fatal(err)
	}
}

type importWorld struct {
	food, salary, old string
	rent              string // an icon the user picked
	savingsCard       string // an account icon the user picked
	card, deposit     string
	stash             string // in a hidden folder
}

func seedImport(e *onbEnv) importWorld {
	folder := e.f.Folder(fixture.Folder{UserID: e.user, Name: "Main"})
	hidden := e.f.Folder(fixture.Folder{UserID: e.user, Name: "Old stuff", Hidden: true, Position: 1})
	w := importWorld{
		food:    e.f.Category(fixture.Category{UserID: e.user, Name: "Lebensmittel", Type: 0, Icon: model.ImportCategoryIcon}),
		salary:  e.f.Category(fixture.Category{UserID: e.user, Name: "Gehalt", Type: 1, Icon: model.DefaultIcon}),
		old:     e.f.Category(fixture.Category{UserID: e.user, Name: "Altes", Type: 0, Icon: model.ImportCategoryIcon, Archived: true}),
		rent:    e.f.Category(fixture.Category{UserID: e.user, Name: "Miete", Type: 0, Icon: "home"}),
		card:    e.f.Account(fixture.Account{UserID: e.user, Name: "Girokonto", CurrencyID: e.eur, Icon: "wallet"}),
		deposit: e.f.Account(fixture.Account{UserID: e.user, Name: "Tagesgeld", CurrencyID: e.eur, Icon: "wallet"}),
		stash:   e.f.Account(fixture.Account{UserID: e.user, Name: "Versteck", Icon: "wallet"}),
	}
	w.savingsCard = e.f.Account(fixture.Account{UserID: e.user, Name: "Sparbuch", CurrencyID: e.eur, Icon: "pets"})
	e.f.AccountInFolder(folder, w.savingsCard)
	e.f.AccountInFolder(folder, w.card)
	e.f.AccountInFolder(folder, w.deposit)
	e.f.AccountInFolder(hidden, w.stash)
	return w
}

func TestFinishImport_CreatesTheBudgetOnceAndOnlyMarksImport(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	w := seedImport(e)
	e.setProfileCurrency(e.f.Currency(fixture.Currency{Code: "CHF", Symbol: "Fr", Name: "Franc"}))
	e.ai.reply = func() (string, error) {
		return `{"savings":["` + w.deposit + `","` + fixture.NewID() + `"]}`, nil
	}
	if _, err := e.db.Raw.Exec(e.db.Rebind(`UPDATE users SET timezone = ? WHERE id = ?`), "Europe/Berlin", e.user); err != nil {
		t.Fatal(err)
	}
	op := fixture.NewID()
	status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": op})
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	budget, _ := data["budgetId"].(string)
	if budget == "" || e.budgetCount() != 1 {
		t.Fatalf("budget %q / %d", budget, e.budgetCount())
	}
	if name := e.str(`SELECT name FROM budgets WHERE id = ?`, budget); name != "Mein Budget" {
		t.Fatalf("budget name = %q", name)
	}
	if cur := e.str(`SELECT c.code FROM budgets b JOIN currencies c ON c.id = b.currency_id WHERE b.id = ?`, budget); cur != "CHF" {
		t.Fatalf("budget currency = %s", cur)
	}
	if !e.member(budget, w.card) || !e.member(budget, w.deposit) || !e.member(budget, w.savingsCard) || e.member(budget, w.stash) {
		t.Fatal("members: want the two visible accounts only")
	}
	if n := e.count(`SELECT COUNT(*) FROM budgets_elements WHERE budget_id = ? AND external_id IN (?, ?)`, budget, w.food, w.salary); n != 2 {
		t.Fatalf("category elements = %d", n)
	}
	if got := e.onboarding(); got != "import" {
		t.Fatalf("onboarding = %q", got)
	}
	user := data["user"].(map[string]any)
	if user["id"] != e.user {
		t.Fatalf("user = %v", user)
	}

	// One request ran, after the commit: savings, with timezone and language and
	// never the profile currency, over every live account. The icons are the
	// CSV import's to ask for, so none is asked here.
	if len(e.ai.prompts) != 1 || !strings.Contains(e.ai.systems[0], `{"savings":`) {
		t.Fatalf("prompts = %d", len(e.ai.prompts))
	}
	var p struct {
		Timezone, Language string
		Accounts           []struct{ Key, Name, Currency string }
	}
	raw := e.ai.prompts[0]
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	if p.Language != "de" || p.Timezone != "Europe/Berlin" || strings.Contains(raw, "CHF") || len(p.Accounts) != 4 {
		t.Fatalf("savings prompt = %s", raw)
	}
	if left := time.Until(e.ai.deadlines[0]); left < 60*time.Second {
		t.Fatalf("the request has %s left, want its 90 s budget", left)
	}
	if e.icon("categories", w.food) != model.ImportCategoryIcon || e.icon("accounts", w.card) != "wallet" {
		t.Fatal("finish-import repainted an icon")
	}
	if !e.isSavings(budget, w.deposit) || e.isSavings(budget, w.card) {
		t.Fatal("savings flags")
	}

	// A replay and a new id after the step is done write nothing and start no job.
	for _, id := range []string{op, fixture.NewID()} {
		status, again := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": id})
		if status != 200 || again["budgetId"] != budget {
			t.Fatalf("again %d: %v", status, again)
		}
	}
	if e.budgetCount() != 1 || len(e.ai.prompts) != 1 || e.onboarding() != "import" {
		t.Fatalf("budgets %d, prompts %d, onboarding %q", e.budgetCount(), len(e.ai.prompts), e.onboarding())
	}
}

func TestFinishImport_AIFailuresChangeNothing(t *testing.T) {
	for name, reply := range map[string]func() (string, error){
		"error":     func() (string, error) { return "", errors.New("provider down") },
		"malformed": func() (string, error) { return "I think the deposit is savings", nil },
	} {
		t.Run(name, func(t *testing.T) {
			e := newOnbEnv(t, stepNow)
			w := seedImport(e)
			e.ai.reply = reply
			status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": fixture.NewID()})
			if status != 200 || data["budgetId"] == "" {
				t.Fatalf("status %d: %v", status, data)
			}
			budget := data["budgetId"].(string)
			if len(e.ai.prompts) != 1 || e.icon("categories", w.food) != model.ImportCategoryIcon || e.icon("accounts", w.deposit) != "wallet" || e.isSavings(budget, w.deposit) {
				t.Fatal("a failed job changed something")
			}
		})
	}
}

// A savings request that panics changes nothing and does not fail the step.
func TestFinishImport_SavingsPanicChangesNothing(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	w := seedImport(e)
	e.ai.reply = func() (string, error) { panic("boom") }
	status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": fixture.NewID()})
	if status != 200 || e.isSavings(data["budgetId"].(string), w.deposit) {
		t.Fatalf("status %d: %v", status, data)
	}
}

// With a budget already in place the import reuses it: no account turns
// savings in a budget the user set up, and no request is sent at all.
func TestFinishImport_ExistingBudgetGetsNoSavings(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	w := seedImport(e)
	budget := e.f.Budget(fixture.Budget{UserID: e.user})
	e.f.BudgetAccount(budget, w.deposit)
	e.ai.reply = func() (string, error) {
		return `{"savings":["` + w.deposit + `"]}`, nil
	}
	status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": fixture.NewID()})
	if status != 200 || data["budgetId"] != budget || e.budgetCount() != 1 {
		t.Fatalf("status %d: %v", status, data)
	}
	if e.isSavings(budget, w.deposit) {
		t.Fatal("savings flag set in a budget the import did not create")
	}
	if len(e.ai.prompts) != 0 {
		t.Fatalf("prompts = %d", len(e.ai.prompts))
	}
}

func TestFinishImport_ClaimedIDStartsNoJob(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	seedImport(e)
	op := fixture.NewID()
	if status, _ := e.post("/api/v1/onboarding/save-categories", categoriesBody(op)); status != 200 {
		t.Fatal("save-categories")
	}
	// save-categories created the budget; its id is now claimed
	status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": op})
	if status != 400 || len(e.ai.prompts) != 0 || e.onboarding() != "categories" {
		t.Fatalf("status %d: %v, prompts %d, onboarding %q", status, data, len(e.ai.prompts), e.onboarding())
	}
	if status, _ := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": "nope"}); status != 400 {
		t.Fatalf("bad id: %d", status)
	}
}

func TestFinishImport_NoAccountsNoBudget(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": fixture.NewID()})
	if status != 200 || data["budgetId"] != "" || e.budgetCount() != 0 || e.onboarding() != "import" {
		t.Fatalf("status %d: %v", status, data)
	}
}

func TestSaveCategories_ArchiveAndRestore(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	food := e.f.Category(fixture.Category{UserID: e.user, Name: "Food", Type: 0})
	gone := e.f.Category(fixture.Category{UserID: e.user, Name: "Gone", Type: 0, Archived: true})
	other := e.f.User(fixture.User{Name: "Other", Email: "other@onboarding.test"})
	theirs := e.f.Category(fixture.Category{UserID: other, Name: "Theirs", Type: 0})
	archived := func(id string) int {
		return e.count(`SELECT COUNT(*) FROM categories WHERE id = ? AND is_archived = ?`, id, true)
	}
	for name, body := range map[string]map[string]any{
		"foreign":  {"id": fixture.NewID(), "archive": []string{food, theirs}},
		"unknown":  {"id": fixture.NewID(), "restore": []string{fixture.NewID()}},
		"both":     {"id": fixture.NewID(), "archive": []string{food}, "restore": []string{food}},
		"not uuid": {"id": fixture.NewID(), "archive": []string{"x"}},
	} {
		status, data := e.post("/api/v1/onboarding/save-categories", body)
		if status != 400 {
			t.Fatalf("%s: %d %v", name, status, data)
		}
	}
	if archived(food) != 0 || archived(theirs) != 0 || e.onboarding() != "started" {
		t.Fatal("a refused call archived something")
	}
	status, data := e.post("/api/v1/onboarding/save-categories", map[string]any{
		"id": fixture.NewID(), "archive": []string{strings.ToUpper(food)}, "restore": []string{gone},
		"categories": []map[string]any{{"key": "a", "name": "Travel", "type": "expense", "icon": "home"}},
	})
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	if archived(food) != 1 || archived(gone) != 0 || e.onboarding() != "categories" {
		t.Fatalf("food %d, gone %d, onboarding %q", archived(food), archived(gone), e.onboarding())
	}
}

func TestSaveAccounts_HideAndUnhide(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	main := e.f.Folder(fixture.Folder{UserID: e.user, Name: "Main"})
	fresh := e.f.Account(fixture.Account{UserID: e.user, Name: "Fresh"})
	locked := e.f.Account(fixture.Account{UserID: e.user, Name: "Locked"})
	kept := e.f.Account(fixture.Account{UserID: e.user, Name: "Kept"})
	for _, a := range []string{fresh, locked, kept} {
		e.f.AccountInFolder(main, a)
	}
	budget := e.f.Budget(fixture.Budget{UserID: e.user, StartedAt: "2025-12-01 00:00:00"})
	for _, a := range []string{fresh, locked, kept} {
		e.f.BudgetAccount(budget, a)
	}
	// history in a closed period locks this one in the budget
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: locked, Type: 0, Amount: "5", SpentAt: "2026-01-15 10:00:00"})
	other := e.f.User(fixture.User{Name: "Other", Email: "other@onboarding.test"})
	theirs := e.f.Account(fixture.Account{UserID: other, Name: "Theirs"})

	if status, data := e.post("/api/v1/onboarding/save-accounts", map[string]any{"id": fixture.NewID(), "mainCurrencyId": fixture.USD, "hide": []string{theirs}}); status != 400 {
		t.Fatalf("foreign: %d %v", status, data)
	}
	if status, data := e.post("/api/v1/onboarding/save-accounts", map[string]any{"id": fixture.NewID(), "mainCurrencyId": fixture.USD, "hide": []string{fresh}, "unhide": []string{fresh}}); status != 400 {
		t.Fatalf("both: %d %v", status, data)
	}
	status, data := e.post("/api/v1/onboarding/save-accounts", map[string]any{
		"id": fixture.NewID(), "mainCurrencyId": fixture.USD, "hide": []string{fresh, locked},
	})
	if status != 200 || data["budgetId"] != budget {
		t.Fatalf("status %d: %v", status, data)
	}
	if n := e.count(`SELECT COUNT(*) FROM folders WHERE user_id = ? AND name = ? AND is_visible = ?`, e.user, "Archiv", false); n != 1 {
		t.Fatalf("hidden Archiv folders = %d", n)
	}
	archive := e.str(`SELECT id FROM folders WHERE user_id = ? AND name = 'Archiv'`, e.user)
	for _, a := range []string{fresh, locked} {
		if e.str(`SELECT folder_id FROM accounts_folders WHERE account_id = ?`, a) != archive {
			t.Fatalf("%s not in the archive folder", a)
		}
	}
	if e.member(budget, fresh) || !e.member(budget, locked) || !e.member(budget, kept) {
		t.Fatalf("members: fresh %v, locked %v, kept %v", e.member(budget, fresh), e.member(budget, locked), e.member(budget, kept))
	}
	if e.onboarding() != "accounts" {
		t.Fatalf("onboarding = %q", e.onboarding())
	}
}

func TestSaveAccounts_UnhideIntoAGeneralFolderAndBackIntoTheBudget(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	archive := e.f.Folder(fixture.Folder{UserID: e.user, Name: "Archiv", Hidden: true})
	parked := e.f.Account(fixture.Account{UserID: e.user, Name: "Parked"})
	more := e.f.Account(fixture.Account{UserID: e.user, Name: "More"})
	e.f.AccountInFolder(archive, parked)
	e.f.AccountInFolder(archive, more)
	budget := e.f.Budget(fixture.Budget{UserID: e.user})
	e.f.BudgetAccount(budget, more)
	status, data := e.post("/api/v1/onboarding/save-accounts", map[string]any{
		"id": fixture.NewID(), "mainCurrencyId": fixture.USD, "unhide": []string{parked},
		"hide":     []string{more},
		"accounts": []map[string]any{{"key": "n", "name": "New one", "balance": "", "currencyId": fixture.USD, "icon": "wallet"}},
	})
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	general := e.str(`SELECT id FROM folders WHERE user_id = ? AND name = 'General' AND is_visible = ?`, e.user, true)
	newID := stringMap(t, data["accounts"])["n"]
	for _, a := range []string{parked, newID} {
		if e.str(`SELECT folder_id FROM accounts_folders WHERE account_id = ?`, a) != general {
			t.Fatalf("%s not in General", a)
		}
	}
	if n := e.count(`SELECT COUNT(*) FROM folders WHERE user_id = ?`, e.user); n != 2 {
		t.Fatalf("folders = %d, want Archiv reused and one General", n)
	}
	if !e.member(budget, parked) || !e.member(budget, newID) || e.member(budget, more) {
		t.Fatal("budget membership")
	}
}

// Hidden accounts stay out of a budget the accounts step creates.
func TestSaveAccounts_NewBudgetLeavesHiddenAccountsOut(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	main := e.f.Folder(fixture.Folder{UserID: e.user, Name: "Main"})
	a := e.f.Account(fixture.Account{UserID: e.user, Name: "Alpha"})
	b := e.f.Account(fixture.Account{UserID: e.user, Name: "Beta"})
	e.f.AccountInFolder(main, a)
	e.f.AccountInFolder(main, b)
	status, data := e.post("/api/v1/onboarding/save-accounts", map[string]any{"id": fixture.NewID(), "mainCurrencyId": fixture.USD, "hide": []string{b}})
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	budget := data["budgetId"].(string)
	if !e.member(budget, a) || e.member(budget, b) {
		t.Fatal("membership of the new budget")
	}
}

func suggestions(t *testing.T, data map[string]any) map[string]string {
	t.Helper()
	out := map[string]string{}
	items, _ := data["items"].([]any)
	for _, it := range items {
		m := it.(map[string]any)
		out[m["categoryId"].(string)] = m["amount"].(string)
	}
	return out
}

func TestGetPlanSuggestions_AveragesTheLastThreeCompleteMonths(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	usd := e.f.Account(fixture.Account{UserID: e.user, Name: "Card"})
	eur := e.f.Account(fixture.Account{UserID: e.user, Name: "Euro card", CurrencyID: e.eur})
	other := e.f.Account(fixture.Account{UserID: e.user, Name: "Not in the budget"})
	// someone else's account shared into the budget: its history is theirs
	partner := e.f.User(fixture.User{Name: "Partner", Email: "partner@onboarding.test"})
	theirs := e.f.Account(fixture.Account{UserID: partner, Name: "Partner card"})
	e.f.AccountAccess(theirs, e.user, 1)
	budget := e.f.Budget(fixture.Budget{UserID: e.user, StartedAt: "2026-03-01 00:00:00"})
	e.f.BudgetAccount(budget, usd)
	e.f.BudgetAccount(budget, eur)
	e.f.BudgetAccount(budget, theirs)
	for _, month := range []string{"2025-12-15", "2026-01-15", "2026-02-15"} {
		e.f.Rate(fixture.Rate{CurrencyID: e.eur, Rate: "0.50000000", PublishedAt: month})
	}
	cat := func(name string, typ int, archived bool) string {
		return e.f.Category(fixture.Category{UserID: e.user, Name: name, Type: typ, Archived: archived})
	}
	food, cafe, transport, travel := cat("Food", 0, false), cat("Cafe", 0, false), cat("Transport", 0, false), cat("Travel", 0, false)
	salary, gifts, old, empty := cat("Salary", 1, false), cat("Gifts", 1, false), cat("Old", 0, true), cat("Empty", 0, false)
	tag := e.f.Tag(fixture.Tag{UserID: e.user, Name: "Trip"})
	tx := func(account, category string, typ int, amount, at string) {
		e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: account, CategoryID: category, Type: typ, Amount: amount, SpentAt: at + " 10:00:00"})
	}
	tx(usd, food, 0, "30", "2025-12-05")
	tx(usd, food, 0, "60", "2026-01-10")
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: usd, CategoryID: food, TagID: tag, Type: 0, Amount: "30", SpentAt: "2026-01-20 10:00:00"})
	tx(usd, food, 0, "500", "2025-11-20")   // before the window
	tx(usd, food, 0, "999", "2026-03-05")   // the current month
	tx(other, food, 0, "777", "2026-02-05") // the caller's own account counts, member or not
	tx(theirs, food, 0, "555", "2026-01-05")
	tx(usd, cafe, 0, "12", "2026-02-11")
	tx(usd, transport, 0, "10", "2025-12-01")
	tx(usd, transport, 0, "10", "2026-01-31")
	tx(usd, transport, 0, "0.01", "2026-02-28")
	tx(eur, travel, 0, "10", "2025-12-20")
	tx(eur, travel, 0, "10", "2026-01-20")
	for _, at := range []string{"2025-12-25", "2026-01-25", "2026-02-25"} {
		tx(usd, salary, 1, "1000", at)
		tx(usd, old, 0, "50", at)
	}
	tx(usd, gifts, 1, "100", "2026-02-02")

	status, data := e.get("/api/v1/onboarding/get-plan-suggestions")
	if status != 200 || data["budgetId"] != budget {
		t.Fatalf("status %d: %v", status, data)
	}
	got := suggestions(t, data)
	// food: (30 + 60 + 30 + 777) / 3
	want := map[string]string{food: "299", transport: "6.67", travel: "13.33", salary: "1000"}
	if len(got) != len(want) {
		t.Fatalf("suggestions = %v, want %v (cafe %s, gifts %s, old %s, empty %s)", got, want, cafe, gifts, old, empty)
	}
	for id, amount := range want {
		if got[id] != amount {
			t.Fatalf("%s = %q, want %q (all %v)", id, got[id], amount, got)
		}
	}
	// the same with the budget named, and nothing was written
	if status, again := e.get("/api/v1/onboarding/get-plan-suggestions?budgetId=" + budget); status != 200 || len(suggestions(t, again)) != 4 {
		t.Fatalf("named budget %d: %v", status, again)
	}
	if n := e.count(`SELECT COUNT(*) FROM budgets_elements_limits`); n != 0 || e.onboarding() != "started" {
		t.Fatalf("limits %d, onboarding %q", n, e.onboarding())
	}
}

func TestGetPlanSuggestions_ShiftedPeriods(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	acc := e.f.Account(fixture.Account{UserID: e.user, Name: "Card"})
	budget := e.f.Budget(fixture.Budget{UserID: e.user, StartedAt: "2026-02-01 00:00:00"})
	e.f.BudgetAccount(budget, acc)
	if _, err := e.db.Raw.Exec(e.db.Rebind(`UPDATE budgets SET period_start_day = 15 WHERE id = ?`), budget); err != nil {
		t.Fatal(err)
	}
	food := e.f.Category(fixture.Category{UserID: e.user, Name: "Food", Type: 0})
	cafe := e.f.Category(fixture.Category{UserID: e.user, Name: "Cafe", Type: 0})
	tx := func(category, amount, at string) {
		e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: acc, CategoryID: category, Type: 0, Amount: amount, SpentAt: at})
	}
	// March 10 sits in the period starting Feb 15: the complete ones run
	// Nov 15 - Dec 14, Dec 15 - Jan 14 and Jan 15 - Feb 14.
	tx(food, "30", "2025-12-10 10:00:00")
	tx(food, "60", "2026-01-10 10:00:00")
	tx(food, "500", "2025-11-10 10:00:00")
	tx(food, "999", "2026-02-20 10:00:00")
	tx(cafe, "5", "2026-01-20 10:00:00")
	tx(cafe, "5", "2026-02-14 23:00:00")
	status, data := e.get("/api/v1/onboarding/get-plan-suggestions?budgetId=" + budget)
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	if got := suggestions(t, data); len(got) != 1 || got[food] != "30" {
		t.Fatalf("suggestions = %v (food %s, cafe %s)", got, food, cafe)
	}
}

func TestGetPlanSuggestions_NoBudgetAndAccess(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	status, data := e.get("/api/v1/onboarding/get-plan-suggestions")
	if status != 200 || data["budgetId"] != "" || len(data["items"].([]any)) != 0 {
		t.Fatalf("no budget %d: %v", status, data)
	}
	other := e.f.User(fixture.User{Name: "Other", Email: "other@onboarding.test"})
	theirs := e.f.Budget(fixture.Budget{UserID: other})
	if status, data := e.get("/api/v1/onboarding/get-plan-suggestions?budgetId=" + theirs); status != 403 {
		t.Fatalf("foreign %d: %v", status, data)
	}
	e.f.BudgetAccess(theirs, e.user, 2, true) // guest
	if status, data := e.get("/api/v1/onboarding/get-plan-suggestions?budgetId=" + theirs); status != 403 {
		t.Fatalf("guest %d: %v", status, data)
	}
	if status, _ := e.get("/api/v1/onboarding/get-plan-suggestions?budgetId=nope"); status != 400 {
		t.Fatalf("bad id %d", status)
	}
}

// A savings member carrying a plan is not removed by a hide: the removal is
// never confirmed, so the plan survives and the account stays a member.
func TestSaveAccounts_HideKeepsASavingsMemberWithPlans(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	main := e.f.Folder(fixture.Folder{UserID: e.user, Name: "Main"})
	saver := e.f.Account(fixture.Account{UserID: e.user, Name: "Saver"})
	e.f.AccountInFolder(main, saver)
	budget := e.f.Budget(fixture.Budget{UserID: e.user})
	e.f.BudgetAccount(budget, saver)
	if _, err := e.db.Raw.Exec(e.db.Rebind(`UPDATE budgets_accounts SET is_savings = ? WHERE budget_id = ?`), true, budget); err != nil {
		t.Fatal(err)
	}
	el := e.f.BudgetElement(fixture.BudgetElement{BudgetID: budget, ExternalID: saver, Type: int(model.ElementSavings)})
	e.f.BudgetLimit(fixture.BudgetLimit{ElementID: el, Period: "2026-03-01", Amount: "100"})
	status, data := e.post("/api/v1/onboarding/save-accounts", map[string]any{"id": fixture.NewID(), "mainCurrencyId": fixture.USD, "hide": []string{saver}})
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	if !e.member(budget, saver) || e.count(`SELECT COUNT(*) FROM budgets_elements_limits WHERE element_id = ?`, el) != 1 {
		t.Fatal("the savings member or its plan was dropped")
	}
	if e.count(`SELECT COUNT(*) FROM folders WHERE user_id = ? AND name = 'Archiv' AND is_visible = ?`, e.user, false) != 1 {
		t.Fatal("not moved to the archive folder")
	}
}

// A visible folder that happens to carry the archive name is the user's own:
// it stays visible and the archive gets a numbered name, reused next time.
func TestSaveAccounts_NeverHidesAVisibleFolderOfTheArchiveName(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	mine := e.f.Folder(fixture.Folder{UserID: e.user, Name: "Archiv"})
	acc := e.f.Account(fixture.Account{UserID: e.user, Name: "Card"})
	e.f.AccountInFolder(mine, acc)
	status, data := e.post("/api/v1/onboarding/save-accounts", map[string]any{"id": fixture.NewID(), "mainCurrencyId": fixture.USD, "hide": []string{acc}})
	if status != 200 {
		t.Fatalf("status %d: %v", status, data)
	}
	if e.count(`SELECT COUNT(*) FROM folders WHERE id = ? AND is_visible = ?`, mine, true) != 1 {
		t.Fatal("the user's visible folder was hidden")
	}
	archive := e.str(`SELECT id FROM folders WHERE user_id = ? AND name = 'Archiv 2' AND is_visible = ?`, e.user, false)
	if e.str(`SELECT folder_id FROM accounts_folders WHERE account_id = ?`, acc) != archive {
		t.Fatal("not moved to the numbered archive folder")
	}
}

func TestGetPlanSuggestions_ADefaultBudgetTheUserCannotPlanSuggestsNothing(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	other := e.f.User(fixture.User{Name: "Other", Email: "other@onboarding.test"})
	theirs := e.f.Budget(fixture.Budget{UserID: other})
	e.f.BudgetAccess(theirs, e.user, 2, true) // guest: readable, not plannable
	status, data := e.get("/api/v1/onboarding/get-plan-suggestions")
	if status != 200 || data["budgetId"] != "" || len(data["items"].([]any)) != 0 {
		t.Fatalf("status %d: %v", status, data)
	}
}

// A user who opted out of AI gets the step writes with no job at all: no
// import review after finish-import, no savings classification after
// save-accounts.
func TestOnboarding_OptedOutUserStartsNoAIJob(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	off := "0"
	e.f.Option(e.user, model.OptionAI, &off)
	seedImport(e)
	if status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": fixture.NewID()}); status != 200 {
		t.Fatalf("finish-import %d: %v", status, data)
	}
	body := accountsBody(fixture.NewID(), e.eur, nil, [4]string{"s", "Savings", "100", e.eur})
	if status, data := e.post("/api/v1/onboarding/save-accounts", body); status != 200 {
		t.Fatalf("save-accounts %d: %v", status, data)
	}
	if len(e.ai.prompts) != 0 {
		t.Fatalf("prompts = %d", len(e.ai.prompts))
	}
}

// Opting out after a job was scheduled but before it calls the provider: no
// request goes out, for the import's savings job.
func TestOnboarding_OptOutAfterSchedulingSendsNothing(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	seedImport(e)
	e.beforeJob = func() {
		off := "0"
		e.f.Option(e.user, model.OptionAI, &off)
		e.beforeJob = nil
	}
	if status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": fixture.NewID()}); status != 200 {
		t.Fatalf("finish-import %d: %v", status, data)
	}
	if len(e.ai.prompts) != 0 {
		t.Fatalf("prompts = %d", len(e.ai.prompts))
	}
}

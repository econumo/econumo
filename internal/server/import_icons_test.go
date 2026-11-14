package server_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/fixture"
)

const iconsMapping = `{"account":"Account","date":"Date","amount":"Amount","category":"Category","description":"Note","payee":"Payee","tag":null}`

// importCSV posts a CSV to the transaction import, fails unless it imported,
// and lets the icon review's quiet period end.
func (e *onbEnv) importCSV(csv string) {
	e.t.Helper()
	e.importChunk(csv)
	e.flushIcons()
}

// importChunk posts one request of an import, leaving the quiet period running.
func (e *onbEnv) importChunk(csv string) {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "import.csv")
	_, _ = fw.Write([]byte(csv))
	_ = mw.WriteField("mapping", iconsMapping)
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, e.url+"/api/v1/transaction/import-transaction-list", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+e.token)
	req.Header.Set("Accept-Language", "de")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var env struct {
		Data struct{ Imported int } `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil || resp.StatusCode != 200 || env.Data.Imported == 0 {
		e.t.Fatalf("import: status %d, %+v, %v", resp.StatusCode, env, err)
	}
}

// The existing world: a category and an account still on a default icon that
// this import does not create, so they are never asked about.
type iconWorld struct{ salary, giro string }

func seedIconWorld(e *onbEnv) iconWorld {
	folder := e.f.Folder(fixture.Folder{UserID: e.user, Name: "Main"})
	w := iconWorld{
		salary: e.f.Category(fixture.Category{UserID: e.user, Name: "Gehalt", Type: 1, Icon: model.DefaultIcon}),
		giro:   e.f.Account(fixture.Account{UserID: e.user, Name: "Girokonto", CurrencyID: e.eur, Icon: model.ImportAccountIcon}),
	}
	e.f.AccountInFolder(folder, w.giro)
	return w
}

// New: the Visa account, the Lebensmittel (expense) and Bonus (income)
// categories. Existing: Girokonto and Gehalt.
const iconsCSV = "Date,Account,Amount,Category,Payee,Note\n" +
	"2026-11-01,Visa,-12.50,Lebensmittel,,\n" +
	"2026-11-02,Girokonto,3000,Gehalt,,\n" +
	"2026-11-03,Girokonto,200,Bonus,,\n"

func (e *onbEnv) idByName(table, name string) string {
	return e.str(`SELECT id FROM `+table+` WHERE user_id = ? AND name = ?`, e.user, name)
}

type iconPrompt struct {
	Timezone, Language string
	Categories         []struct{ ID, Name, Type string }
	Accounts           []struct{ ID, Name, Currency string }
}

func (e *onbEnv) iconPrompts() map[string]iconPrompt {
	e.t.Helper()
	out := map[string]iconPrompt{}
	for i, raw := range e.ai.prompts {
		var p iconPrompt
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			e.t.Fatal(err)
		}
		switch sys := e.ai.systems[i]; {
		case strings.Contains(sys, `{"categories":`):
			out["categories"] = p
		case strings.Contains(sys, `{"accounts":`):
			out["accounts"] = p
		case strings.Contains(sys, `{"savings":`):
			out["savings"] = p
		}
	}
	return out
}

func TestCSVImport_ReviewsTheIconsOfWhatItCreated(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	w := seedIconWorld(e)
	if _, err := e.db.Raw.Exec(e.db.Rebind(`UPDATE users SET timezone = ? WHERE id = ?`), "Europe/Berlin", e.user); err != nil {
		t.Fatal(err)
	}
	e.ai.bySystem = func(string) (string, error) {
		food, bonus, visa := e.idByName("categories", "Lebensmittel"), e.idByName("categories", "Bonus"), e.idByName("accounts", "Visa")
		return `{"categories":[{"id":"` + strings.ToUpper(food) + `","icon":"shopping_cart"},{"id":"` + bonus + `","icon":"not_an_icon"},{"id":"` + w.salary + `","icon":"payments"}],` +
			`"accounts":[{"id":"` + visa + `","icon":"credit_card"},{"id":"` + w.giro + `","icon":"savings"}]}`, nil
	}
	e.importCSV(iconsCSV)
	food, bonus, visa := e.idByName("categories", "Lebensmittel"), e.idByName("categories", "Bonus"), e.idByName("accounts", "Visa")

	if len(e.ai.prompts) != 2 {
		t.Fatalf("prompts = %d, want category and account icons", len(e.ai.prompts))
	}
	schemas := append([]string(nil), e.ai.schemas...)
	sort.Strings(schemas)
	if strings.Join(schemas, ",") != "account_icons,category_icons" {
		t.Fatalf("schemas = %v", e.ai.schemas)
	}
	parts := e.iconPrompts()
	cats, accts := parts["categories"], parts["accounts"]
	if len(cats.Categories) != 2 || len(accts.Accounts) != 1 || accts.Accounts[0].ID != visa || accts.Accounts[0].Currency != "USD" {
		t.Fatalf("prompts = %+v", parts)
	}
	for _, p := range []iconPrompt{cats, accts} {
		if p.Language != "de" || p.Timezone != "Europe/Berlin" {
			t.Fatalf("prompt facts = %+v", p)
		}
	}
	for i, raw := range e.ai.prompts {
		if strings.Contains(raw, "Gehalt") || strings.Contains(raw, "Girokonto") {
			t.Fatalf("prompt asks about an item the import did not create: %s", raw)
		}
		if left := time.Until(e.ai.deadlines[i]); left < 60*time.Second {
			t.Fatalf("request %d has %s left, want its own 90 s budget", i, left)
		}
	}
	if e.icon("categories", food) != "shopping_cart" || e.icon("categories", bonus) != model.ImportCategoryIcon || e.icon("accounts", visa) != "credit_card" {
		t.Fatalf("created icons: food %s, bonus %s, visa %s", e.icon("categories", food), e.icon("categories", bonus), e.icon("accounts", visa))
	}
	if e.icon("categories", w.salary) != model.DefaultIcon || e.icon("accounts", w.giro) != model.ImportAccountIcon {
		t.Fatal("an item the import did not create was repainted")
	}

	// a second import that creates nothing asks nothing
	e.importCSV("Date,Account,Amount,Category,Payee,Note\n2026-11-04,Visa,-3,Lebensmittel,,\n")
	if len(e.ai.prompts) != 2 {
		t.Fatalf("prompts = %d after an import that created nothing", len(e.ai.prompts))
	}
}

// A file sent in chunks is reviewed once, over everything the chunks created.
func TestCSVImport_ChunksAreReviewedOnce(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	seedIconWorld(e)
	e.importChunk("Date,Account,Amount,Category,Payee,Note\n2026-11-01,Visa,-12.50,Lebensmittel,,\n")
	e.importChunk("Date,Account,Amount,Category,Payee,Note\n2026-11-02,Girokonto,200,Bonus,,\n")
	e.importChunk("Date,Account,Amount,Category,Payee,Note\n2026-11-03,Cash,-5,Kino,,\n")
	if len(e.ai.prompts) != 0 {
		t.Fatalf("prompts = %d before the import went quiet", len(e.ai.prompts))
	}
	e.flushIcons()
	if len(e.ai.prompts) != 2 {
		t.Fatalf("prompts = %d, want one categories and one accounts request", len(e.ai.prompts))
	}
	parts := e.iconPrompts()
	if len(parts["categories"].Categories) != 3 || len(parts["accounts"].Accounts) != 2 {
		t.Fatalf("prompts = %+v", parts)
	}
}

// Chunks that create nothing keep the review waiting: a slow import whose
// middle requests only add rows to existing accounts is still reviewed once,
// after its last request, over the first and the last chunk's new items.
func TestCSVImport_EmptyChunksKeepOneReview(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	seedIconWorld(e)
	e.importChunk("Date,Account,Amount,Category,Payee,Note\n2026-11-01,Visa,-12.50,Lebensmittel,,\n")
	e.importChunk("Date,Account,Amount,Category,Payee,Note\n2026-11-02,Girokonto,3000,Gehalt,,\n")
	e.importChunk("Date,Account,Amount,Category,Payee,Note\n2026-11-03,Visa,-4,Lebensmittel,,\n")
	e.importChunk("Date,Account,Amount,Category,Payee,Note\n2026-11-04,Cash,-5,Kino,,\n")
	e.timersMu.Lock()
	live := 0
	for _, tm := range e.timers {
		if !tm.stopped {
			live++
		}
	}
	scheduled := len(e.timers)
	e.timersMu.Unlock()
	if live != 1 || scheduled != 4 {
		t.Fatalf("timers: %d live of %d, want every chunk to reset one", live, scheduled)
	}
	e.flushIcons()
	if len(e.ai.prompts) != 2 {
		t.Fatalf("prompts = %d, want one review", len(e.ai.prompts))
	}
	parts := e.iconPrompts()
	cats, accts := parts["categories"].Categories, parts["accounts"].Accounts
	if len(cats) != 2 || len(accts) != 2 {
		t.Fatalf("prompts = %+v", parts)
	}
}

func TestCSVImport_OptedOutUserGetsNoIconReview(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	e.f.Option(e.user, model.OptionAI, ptr("0"))
	seedIconWorld(e)
	e.importCSV(iconsCSV)
	if len(e.ai.prompts) != 0 {
		t.Fatalf("prompts = %d", len(e.ai.prompts))
	}
	if e.icon("accounts", e.idByName("accounts", "Visa")) != model.ImportAccountIcon {
		t.Fatal("icon changed")
	}
}

// Opting out between the import and the provider call sends nothing.
func TestCSVImport_OptOutBeforeTheCallSendsNothing(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	seedIconWorld(e)
	e.beforeJob = func() {
		e.f.Option(e.user, model.OptionAI, ptr("0"))
		e.beforeJob = nil
	}
	e.importCSV(iconsCSV)
	if len(e.ai.prompts) != 0 {
		t.Fatalf("prompts = %d", len(e.ai.prompts))
	}
}

// An icon the user picks while the AI answers wins; a failed compare writes
// nothing at all.
func TestCSVImport_UserPickedIconWinsOverTheAI(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	seedIconWorld(e)
	e.ai.bySystem = func(string) (string, error) {
		food, visa := e.idByName("categories", "Lebensmittel"), e.idByName("accounts", "Visa")
		for _, q := range []struct{ sql, id string }{
			{`UPDATE categories SET icon = 'pets', updated_at = '2001-01-01 00:00:00' WHERE id = ?`, food},
			{`UPDATE accounts SET icon = 'face', updated_at = '2001-01-01 00:00:00' WHERE id = ?`, visa},
		} {
			if _, err := e.db.Raw.Exec(e.db.Rebind(q.sql), q.id); err != nil {
				return "", err
			}
		}
		bonus := e.idByName("categories", "Bonus")
		return `{"categories":[{"id":"` + food + `","icon":"shopping_cart"},{"id":"` + bonus + `","icon":"payments"}],"accounts":[{"id":"` + visa + `","icon":"credit_card"}]}`, nil
	}
	e.importCSV(iconsCSV)
	food, bonus, visa := e.idByName("categories", "Lebensmittel"), e.idByName("categories", "Bonus"), e.idByName("accounts", "Visa")
	if e.icon("categories", food) != "pets" || e.icon("accounts", visa) != "face" || e.icon("categories", bonus) != "payments" {
		t.Fatalf("icons: food %s, visa %s, bonus %s", e.icon("categories", food), e.icon("accounts", visa), e.icon("categories", bonus))
	}
	for table, id := range map[string]string{"categories": food, "accounts": visa} {
		if got := e.str(`SELECT updated_at FROM `+table+` WHERE id = ?`, id); !strings.HasPrefix(got, "2001-01-01") {
			t.Fatalf("%s updated_at = %s", table, got)
		}
	}
}

// Each icon request stands alone: one that fails, panics or answers garbage
// leaves the other to apply, and the import itself never fails.
func TestCSVImport_EachIconPartAppliesWhenTheOtherFails(t *testing.T) {
	parts := map[string]string{"categories": `{"categories":`, "accounts": `{"accounts":`}
	for broken := range parts {
		for _, how := range []string{"error", "garbage", "panic"} {
			t.Run(broken+"_"+how, func(t *testing.T) {
				e := newOnbEnv(t, stepNow)
				seedIconWorld(e)
				e.ai.bySystem = func(system string) (string, error) {
					if strings.Contains(system, parts[broken]) {
						switch how {
						case "error":
							return "", errors.New("provider down")
						case "panic":
							panic("boom")
						}
						return "not json at all", nil
					}
					food, visa := e.idByName("categories", "Lebensmittel"), e.idByName("accounts", "Visa")
					return `{"categories":[{"id":"` + food + `","icon":"shopping_cart"}],"accounts":[{"id":"` + visa + `","icon":"credit_card"}]}`, nil
				}
				e.importCSV(iconsCSV)
				if got := e.icon("categories", e.idByName("categories", "Lebensmittel")) == "shopping_cart"; got != (broken != "categories") {
					t.Fatalf("category icon applied = %v", got)
				}
				if got := e.icon("accounts", e.idByName("accounts", "Visa")) == "credit_card"; got != (broken != "accounts") {
					t.Fatalf("account icon applied = %v", got)
				}
			})
		}
	}
}

// During onboarding the CSV import asks for the icons and finish-import only
// for the savings: the icons are never asked for twice.
func TestCSVImport_InOnboardingIconsAreAskedOnce(t *testing.T) {
	e := newOnbEnv(t, stepNow)
	seedIconWorld(e)
	e.importCSV(iconsCSV)
	if len(e.ai.prompts) != 2 {
		t.Fatalf("prompts after the import = %d", len(e.ai.prompts))
	}
	if status, data := e.post("/api/v1/onboarding/finish-import", map[string]any{"id": fixture.NewID()}); status != 200 {
		t.Fatalf("finish-import %d: %v", status, data)
	}
	parts := e.iconPrompts()
	if len(e.ai.prompts) != 3 || len(parts["savings"].Accounts) != 2 {
		t.Fatalf("prompts = %d, parts = %+v", len(e.ai.prompts), parts)
	}
}

func ptr(s string) *string { return &s }

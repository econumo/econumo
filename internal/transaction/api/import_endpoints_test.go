package api_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// importResult mirrors the import response data.
type importResult struct {
	Imported int              `json:"imported"`
	Skipped  int              `json:"skipped"`
	Errors   map[string][]int `json:"errors"`
}

// doImport posts a multipart import request: csv bytes, a mapping JSON, and
// optional extra form fields. Returns status + parsed envelope.
func (h *harness) doImport(t *testing.T, token, csv, mappingJSON string, extra map[string]string) (int, envelope) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if csv != "" {
		fw, _ := mw.CreateFormFile("file", "import.csv")
		fw.Write([]byte(csv))
	}
	if mappingJSON != "" {
		mw.WriteField("mapping", mappingJSON)
	}
	for k, v := range extra {
		mw.WriteField(k, v)
	}
	mw.Close()

	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/transaction/import-transaction-list", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("doImport: %v", err)
	}
	defer resp.Body.Close()
	var env envelope
	dec := json.NewDecoder(resp.Body)
	if derr := dec.Decode(&env); derr != nil {
		t.Fatalf("decode import response (status %d): %v", resp.StatusCode, derr)
	}
	return resp.StatusCode, env
}

const importMapping = `{"account":"Account","date":"Date","amount":"Amount","category":"Category","description":"Note","payee":"Payee","tag":null}`

func TestImport_CreatesTransactions_FindOrCreate(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	// Row 1 uses the seeded "Cash" account + seeded "Food" category (expense).
	// Row 2 names a new account "Savings" + new category "Salary" (income).
	csv := "Account,Date,Amount,Category,Note,Payee\n" +
		"Cash,2024-03-01,-42.50,Food,groceries,Market\n" +
		"Savings,2024-03-02,1000,Salary,march pay,Employer\n"
	status, env := h.doImport(t, tok, csv, importMapping, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 2 || res.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d want 2/0; errors=%v", res.Imported, res.Skipped, res.Errors)
	}

	// Both transactions show up in the list (across visible accounts).
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	list := mustUnmarshal[listResult](t, listEnv.Data)
	if len(list.Items) != 2 {
		t.Fatalf("list has %d items, want 2", len(list.Items))
	}
	var sawExpense, sawIncome bool
	for _, it := range list.Items {
		switch it.Type {
		case "expense":
			sawExpense = true
			if it.Amount != "42.5" {
				t.Fatalf("expense amount=%q want 42.5 (stored abs)", it.Amount)
			}
		case "income":
			sawIncome = true
			if it.Amount != "1000" {
				t.Fatalf("income amount=%q want 1000", it.Amount)
			}
		}
	}
	if !sawExpense || !sawIncome {
		t.Fatalf("want one expense + one income; items=%+v", list.Items)
	}
}

// TestImport_NewAccount_PrefersOwnCustomCurrencyOverGlobalCode: importing a row
// that names an unknown account creates it in the base currency CODE
// (config.CurrencyBase = "USD" in this harness), resolved user-aware — so when
// the importing user has their OWN custom currency coded "USD", the new
// account must use THAT currency, not the global USD.
func TestImport_NewAccount_PrefersOwnCustomCurrencyOverGlobalCode(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	ownUSD := f.Currency(fixture.Currency{Code: "USD", UserID: seedUserID})

	csv := "Account,Date,Amount,Category,Note,Payee\n" +
		"Savings,2024-03-02,1000,Salary,march pay,Employer\n"
	status, env := h.doImport(t, tok, csv, importMapping, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 1 {
		t.Fatalf("imported=%d want 1; errors=%v", res.Imported, res.Errors)
	}

	var currencyID string
	if err := h.db.QueryRow("SELECT currency_id FROM accounts WHERE user_id = ? AND name = ?", seedUserID, "Savings").Scan(&currencyID); err != nil {
		t.Fatalf("query new account currency: %v", err)
	}
	if currencyID != ownUSD {
		t.Fatalf("new account currency = %q, want own custom USD %q (not the global one)", currencyID, ownUSD)
	}
}

func TestImport_OverrideAccountAndDate(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	// Mapping omits account+date; overrides supply them. Amount only.
	csv := "Amount\n-15\n-25\n"
	mapping := `{"amount":"Amount"}`
	status, env := h.doImport(t, tok, csv, mapping, map[string]string{
		"accountId": accountID,
		"date":      "2024-05-01",
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 2 || res.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d want 2/0; errors=%v", res.Imported, res.Skipped, res.Errors)
	}
}

func TestImport_DualAmountMode(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	csv := "Account,Date,In,Out\n" +
		"Cash,2024-03-01,,30\n" + // outflow -> expense 30
		"Cash,2024-03-02,200,\n" // inflow -> income 200
	mapping := `{"account":"Account","date":"Date","amountInflow":"In","amountOutflow":"Out"}`
	status, env := h.doImport(t, tok, csv, mapping, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 2 || res.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d want 2/0; errors=%v", res.Imported, res.Skipped, res.Errors)
	}
}

func TestImport_BadDateRow_SkippedWithErrorMap(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	csv := "Account,Date,Amount\n" +
		"Cash,2024-03-01,-10\n" +
		"Cash,not-a-date,-20\n"
	mapping := `{"account":"Account","date":"Date","amount":"Amount"}`
	status, env := h.doImport(t, tok, csv, mapping, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 1 || res.Skipped != 1 {
		t.Fatalf("imported=%d skipped=%d want 1/1", res.Imported, res.Skipped)
	}
	rows, ok := res.Errors["Invalid date format 'not-a-date'"]
	if !ok || len(rows) != 1 || rows[0] != 3 {
		t.Fatalf("errors=%v want row 3 under invalid-date", res.Errors)
	}
}

func TestImport_MissingMapping_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	// No account/date mapping and no overrides -> top-level error (still 200, with
	// the failure reported as an error entry rather than an HTTP error status).
	csv := "Amount\n-10\n"
	status, env := h.doImport(t, tok, csv, `{"amount":"Amount"}`, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 0 {
		t.Fatalf("imported=%d want 0", res.Imported)
	}
	if _, ok := res.Errors[`Mapping must include "account" and "date" fields`]; !ok {
		t.Fatalf("missing mapping error; errors=%v", res.Errors)
	}
}

func TestImport_NoToken_401(t *testing.T) {
	h := newHarness(t)
	status, _ := h.doImport(t, "", "Amount\n-1\n", `{"amount":"Amount"}`, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", status)
	}
}

package api_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/backend"
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

// A row whose Category cell is empty imports as an uncategorized transaction
// (categoryId null) rather than being skipped or inventing a category — the
// import counterpart of a categoryless create.
func TestImport_EmptyCategoryCell_ImportsUncategorized(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	csv := "Account,Date,Amount,Category,Note,Payee\n" +
		"Cash,2024-03-01,-42.50,,no category,Market\n"
	status, env := h.doImport(t, tok, csv, importMapping, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 1 || res.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d want 1/0; errors=%v", res.Imported, res.Skipped, res.Errors)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	list := mustUnmarshal[listResult](t, listEnv.Data)
	if len(list.Items) != 1 {
		t.Fatalf("list has %d items, want 1", len(list.Items))
	}
	if list.Items[0].CategoryID != nil {
		t.Fatalf("categoryId=%v want nil (uncategorized)", *list.Items[0].CategoryID)
	}
	// the empty cell must not have created a category named ""
	var blank int
	h.db.QueryRow(`SELECT COUNT(*) FROM categories WHERE name = ''`).Scan(&blank)
	if blank != 0 {
		t.Fatalf("import created %d blank-named categories", blank)
	}
}

// A whitespace-only Category cell counts as "no category name": it must import
// uncategorized rather than find-or-create a category named " ".
func TestImport_WhitespaceCategoryCell_ImportsUncategorized(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	csv := "Account,Date,Amount,Category,Note,Payee\n" +
		"Cash,2024-03-01,-42.50,   ,blank category,Market\n"
	status, env := h.doImport(t, tok, csv, importMapping, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 1 || res.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d want 1/0; errors=%v", res.Imported, res.Skipped, res.Errors)
	}
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	list := mustUnmarshal[listResult](t, listEnv.Data)
	if len(list.Items) != 1 || list.Items[0].CategoryID != nil {
		t.Fatalf("want 1 uncategorized item; got %+v", list.Items)
	}
	var blank int
	h.db.QueryRow(`SELECT COUNT(*) FROM categories WHERE TRIM(name) = ''`).Scan(&blank)
	if blank != 0 {
		t.Fatalf("import created %d blank/whitespace-named categories", blank)
	}
}

// The same, with the Category column left unmapped entirely.
func TestImport_UnmappedCategoryColumn_ImportsUncategorized(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	csv := "Account,Date,Amount,Note\nCash,2024-03-01,-42.50,no category\n"
	mapping := `{"account":"Account","date":"Date","amount":"Amount","description":"Note"}`
	status, env := h.doImport(t, tok, csv, mapping, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 1 || res.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d want 1/0; errors=%v", res.Imported, res.Skipped, res.Errors)
	}
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	list := mustUnmarshal[listResult](t, listEnv.Data)
	if len(list.Items) != 1 || list.Items[0].CategoryID != nil {
		t.Fatalf("want 1 uncategorized item; got %+v", list.Items)
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

// TestImportSplitsLabelsOnDefaultSemicolon: with no labelsSeparator override
// the mapped labels cell splits on ";" — a match against the seeded "Label
// One" resolves to its existing id, and the unmatched piece auto-creates a
// new label owned by the importing user, exactly as the tag column does for
// a single value.
func TestImportSplitsLabelsOnDefaultSemicolon(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	csv := "Account,Date,Amount,Labels\n" +
		"Cash,2024-03-01,-42.50,Label One;New Label\n"
	mapping := `{"account":"Account","date":"Date","amount":"Amount","labels":"Labels"}`
	status, env := h.doImport(t, tok, csv, mapping, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 1 || res.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d want 1/0; errors=%v", res.Imported, res.Skipped, res.Errors)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	list := mustUnmarshal[listResult](t, listEnv.Data)
	if len(list.Items) != 1 {
		t.Fatalf("list has %d items, want 1", len(list.Items))
	}
	got := list.Items[0].LabelIds
	if len(got) != 2 {
		t.Fatalf("labelIds = %v, want 2 (seeded Label One + auto-created New Label)", got)
	}
	if got[0] != label1ID && got[1] != label1ID {
		t.Fatalf("labelIds %v missing seeded label1ID %q", got, label1ID)
	}
	var newLabelCount int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM labels WHERE user_id = ? AND name = ?`, seedUserID, "New Label").Scan(&newLabelCount); err != nil {
		t.Fatalf("query new label: %v", err)
	}
	if newLabelCount != 1 {
		t.Fatalf("want exactly 1 auto-created 'New Label' for the seeded user, got %d", newLabelCount)
	}
}

// TestImportSplitsLabelsOnCustomSeparator: a CSV from another tool may use any
// delimiter, so the labelsSeparator form field controls the split — here "|"
// on a cell containing two seeded label names. A hardcoded ";" implementation
// would treat the whole cell as one unmatched name and auto-create a bogus
// label containing a literal "|", which the assertions below catch.
func TestImportSplitsLabelsOnCustomSeparator(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	csv := "Account,Date,Amount,Labels\n" +
		"Cash,2024-03-01,-10,Label One|Label Two\n"
	mapping := `{"account":"Account","date":"Date","amount":"Amount","labels":"Labels"}`
	status, env := h.doImport(t, tok, csv, mapping, map[string]string{"labelsSeparator": "|"})
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 1 || res.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d want 1/0; errors=%v", res.Imported, res.Skipped, res.Errors)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	list := mustUnmarshal[listResult](t, listEnv.Data)
	if len(list.Items) != 1 {
		t.Fatalf("list has %d items, want 1", len(list.Items))
	}
	got := list.Items[0].LabelIds
	wantSet := map[string]bool{label1ID: true, label2ID: true}
	if len(got) != 2 || !wantSet[got[0]] || !wantSet[got[1]] {
		t.Fatalf("labelIds = %v, want exactly [%s %s] (both seeded labels matched by the custom separator)", got, label1ID, label2ID)
	}
	var bogusCount int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM labels WHERE name LIKE '%|%'`).Scan(&bogusCount); err != nil {
		t.Fatalf("query bogus label: %v", err)
	}
	if bogusCount != 0 {
		t.Fatalf("a label containing the raw unsplit cell was created; the separator was not applied")
	}
}

// TestImportLabelsBlankAndDuplicateHandling: an empty cell attaches no
// labels; within a non-empty cell, blank pieces are dropped and duplicate
// names dedupe case-insensitively (first occurrence wins) before resolution,
// so "Kid A;kid a; ;Kid A" resolves to exactly one label.
func TestImportLabelsBlankAndDuplicateHandling(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	csv := "Account,Date,Amount,Labels\n" +
		"Cash,2024-03-01,-10,\n" +
		"Cash,2024-03-02,-20,Kid A;kid a; ;Kid A\n"
	mapping := `{"account":"Account","date":"Date","amount":"Amount","labels":"Labels"}`
	status, env := h.doImport(t, tok, csv, mapping, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 2 || res.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d want 2/0; errors=%v", res.Imported, res.Skipped, res.Errors)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	list := mustUnmarshal[listResult](t, listEnv.Data)
	if len(list.Items) != 2 {
		t.Fatalf("list has %d items, want 2", len(list.Items))
	}
	var blankRow, kidRow *txItem
	for i := range list.Items {
		it := &list.Items[i]
		if len(it.Date) >= 10 && it.Date[:10] == "2024-03-01" {
			blankRow = it
		} else {
			kidRow = it
		}
	}
	if blankRow == nil || kidRow == nil {
		t.Fatalf("could not locate both rows by date; items=%+v", list.Items)
	}
	if len(blankRow.LabelIds) != 0 {
		t.Fatalf("blank cell labelIds = %v, want none", blankRow.LabelIds)
	}
	if len(kidRow.LabelIds) != 1 {
		t.Fatalf("kidRow labelIds = %v, want exactly 1 (deduped case-insensitively)", kidRow.LabelIds)
	}
	var count int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM labels WHERE user_id = ? AND lower(name) = 'kid a'`, seedUserID).Scan(&count); err != nil {
		t.Fatalf("query kid a label: %v", err)
	}
	if count != 1 {
		t.Fatalf("want exactly 1 label named 'Kid A' (any case), got %d", count)
	}
}

// TestImportLabelIdsOverrideAppliesToEveryRow: the labelIds override, when
// present, replaces the per-row label resolution entirely and applies the
// same set to every imported row.
func TestImportLabelIdsOverrideAppliesToEveryRow(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	csv := "Account,Date,Amount\n" +
		"Cash,2024-03-01,-10\n" +
		"Cash,2024-03-02,-20\n"
	mapping := `{"account":"Account","date":"Date","amount":"Amount"}`
	status, env := h.doImport(t, tok, csv, mapping, map[string]string{"labelIds": label1ID + "," + label2ID})
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 2 || res.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d want 2/0; errors=%v", res.Imported, res.Skipped, res.Errors)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	list := mustUnmarshal[listResult](t, listEnv.Data)
	if len(list.Items) != 2 {
		t.Fatalf("list has %d items, want 2", len(list.Items))
	}
	for _, it := range list.Items {
		if len(it.LabelIds) != 2 {
			t.Fatalf("row %s labelIds = %v, want 2 (override applied to every row)", it.ID, it.LabelIds)
		}
	}
}

// TestImportLabelIdsOverrideRejectsForeignLabel: the labelIds override is
// belongs-to checked against the account owner exactly like tagId — a label
// owned by a different user aborts the whole import with the same top-level
// error style as an unknown tagId, rather than silently skipping it.
func TestImportLabelIdsOverrideRejectsForeignLabel(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	csv := "Account,Date,Amount\nCash,2024-03-01,-10\n"
	mapping := `{"account":"Account","date":"Date","amount":"Amount"}`
	status, env := h.doImport(t, tok, csv, mapping, map[string]string{"labelIds": label1ID + "," + otherLabelID})
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 0 {
		t.Fatalf("imported=%d want 0 (foreign label aborts the whole import)", res.Imported)
	}
	if _, ok := res.Errors["Label not found for provided labelIds"]; !ok {
		t.Fatalf("errors=%v want top-level label-not-found error", res.Errors)
	}
}

// maxTestImportLabelOverride mirrors internal/transaction/import.go's
// unexported maxLabelsPerImportRow (10) -- this test package cannot see it
// directly, so the boundary is duplicated here; keep the two in sync.
const maxTestImportLabelOverride = 10

// TestImportLabelIdsOverrideExceedsCap_Rejected: the labelIds override
// applies to every imported row, so it is capped at the same width as the
// per-row mapped-cell cap; one over aborts the whole import with a dedicated
// top-level error (not the generic "not found" one), before any row is
// processed.
func TestImportLabelIdsOverrideExceedsCap_Rejected(t *testing.T) {
	h := newHarness(t)
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	ids := make([]string, maxTestImportLabelOverride+1)
	for i := range ids {
		ids[i] = f.Label(fixture.Label{UserID: seedUserID, Name: fixture.NewID()})
	}
	tok := h.token(t)
	csv := "Account,Date,Amount\nCash,2024-03-01,-10\n"
	mapping := `{"account":"Account","date":"Date","amount":"Amount"}`
	status, env := h.doImport(t, tok, csv, mapping, map[string]string{"labelIds": strings.Join(ids, ",")})
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 0 {
		t.Fatalf("imported=%d want 0 (the cap aborts the whole import)", res.Imported)
	}
	var found bool
	for msg := range res.Errors {
		if strings.Contains(msg, "Too many labelIds") {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors=%v want a top-level too-many-labelIds error", res.Errors)
	}
}

// TestImportLabelIdsOverrideDedupesRepeatedIds: the same id repeated in the
// override CSV counts once against the cap and attaches once per row, not
// once per occurrence.
func TestImportLabelIdsOverrideDedupesRepeatedIds(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	csv := "Account,Date,Amount\nCash,2024-03-01,-10\n"
	mapping := `{"account":"Account","date":"Date","amount":"Amount"}`
	status, env := h.doImport(t, tok, csv, mapping, map[string]string{"labelIds": label1ID + "," + label1ID + "," + label1ID})
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 1 || res.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d want 1/0; errors=%v", res.Imported, res.Skipped, res.Errors)
	}
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	list := mustUnmarshal[listResult](t, listEnv.Data)
	if len(list.Items) != 1 || len(list.Items[0].LabelIds) != 1 {
		t.Fatalf("labelIds = %+v, want exactly 1 (deduped)", list.Items)
	}
}

// Label ownership scoping for the accountId OVERRIDE path (auto-create and
// the labelIds belongs-to check must use the resolved ACCOUNT OWNER, never
// the caller) is covered at the unit level in
// internal/transaction/import_ownership_test.go, with a stub Importer the
// test controls directly — the real production account adapter only ever
// resolves an accountId override to the CALLER's own account (see
// TransactionImportAccounts.AccountByID's "only available (own) accounts
// qualify" comment), so there is no way to reach accountOwnerID != userID
// through THAT path in this harness's real wiring.
//
// The ROW-MAPPED account path is different: findOrCreateAccount resolves a
// mapped account NAME against AvailableAccounts, which does include accepted
// shared accounts (ListAvailableAccounts's "own OR ACCEPTED shared" query),
// so a caller importing into a shared account by name — no accountId
// override — really can reach a resolved account whose owner isn't the
// caller. That combination is exercised end-to-end below.

// TestImport_RowMappedSharedAccount_LabelsOwnedByAccountOwner: seedUserID (a
// user-role grantee, no accountId override) imports a row whose mapped
// Account cell names the shared "Shared" account owned by ownerTwoID, with a
// Labels cell. The label must be created under ownerTwoID — the account's
// ACTUAL owner — not the caller, else ownerTwoID can never edit their own
// transaction afterwards: resolveLabels requires a label's owner to equal
// the account owner, so a caller-owned label makes any subsequent
// update-transaction (even one that just echoes the current labelIds back)
// fail with transaction.transaction.not_available. The update-transaction
// call at the end is the actual regression guard — pre-fix it 400s.
func TestImport_RowMappedSharedAccount_LabelsOwnedByAccountOwner(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleUser, true)
	tok := h.token(t)

	csv := "Account,Date,Amount,Labels\n" +
		"Shared,2024-03-01,-10,New Label\n"
	mapping := `{"account":"Account","date":"Date","amount":"Amount","labels":"Labels"}`
	status, env := h.doImport(t, tok, csv, mapping, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[importResult](t, env.Data)
	if res.Imported != 1 || res.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d want 1/0; errors=%v", res.Imported, res.Skipped, res.Errors)
	}

	var labelOwner string
	if err := h.db.QueryRow(`SELECT user_id FROM labels WHERE name = ?`, "New Label").Scan(&labelOwner); err != nil {
		t.Fatalf("query new label: %v", err)
	}
	if labelOwner != ownerTwoID {
		t.Fatalf("label owner = %q, want the shared account's owner %q (not the caller %q)", labelOwner, ownerTwoID, seedUserID)
	}

	// The account OWNER must see the imported transaction and be able to
	// resolve its own label.
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list?accountId="+sharedAcctID, ownerTwoID, nil)
	list := mustUnmarshal[listResult](t, listEnv.Data)
	if len(list.Items) != 1 {
		t.Fatalf("owner's list = %+v, want 1 item", list.Items)
	}
	item := list.Items[0]
	if len(item.LabelIds) != 1 {
		t.Fatalf("item labelIds = %v, want exactly 1", item.LabelIds)
	}

	// The account owner echoing the same labelIds back on an update must
	// succeed, not 400 with transaction.transaction.not_available.
	updateReq := map[string]any{
		"id": item.ID, "type": "expense", "amount": "10", "accountId": sharedAcctID,
		"date": "2024-03-01 00:00:00", "description": item.Description, "labelIds": item.LabelIds,
	}
	status2, env2 := h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", ownerTwoID, updateReq)
	if status2 != http.StatusOK {
		t.Fatalf("account owner's update-transaction echoing labelIds: status=%d want 200; body=%s", status2, env2.raw)
	}
}

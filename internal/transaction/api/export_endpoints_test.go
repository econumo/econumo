package api_test

import (
	"encoding/csv"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// getRaw performs a GET and returns the status, response headers, and raw body
// (the export returns text/csv, not the JSON envelope, so the standard `do`
// helper's JSON decode can't be used).
func (h *harness) getRaw(t *testing.T, path, token string) (int, http.Header, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, h.srv.URL+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("getRaw: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(body)
}

func TestExportTransactionList_HeaderOnlyWhenEmpty(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, hdr, body := h.getRaw(t, "/api/v1/transaction/export-transaction-list", tok)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	if ct := hdr.Get("Content-Type"); ct != "text/csv; charset=UTF-8" {
		t.Fatalf("Content-Type=%q want text/csv; charset=UTF-8", ct)
	}
	if cd := hdr.Get("Content-Disposition"); cd != `attachment; filename="transactions.csv"` {
		t.Fatalf("Content-Disposition=%q", cd)
	}
	records := parseCSV(t, body)
	if len(records) != 1 {
		t.Fatalf("rows=%d want 1 (header only)\n%s", len(records), body)
	}
	want := []string{"transaction_id", "account_name", "account_currency", "category", "description", "tag", "labels", "payee", "amount", "date"}
	if strings.Join(records[0], ",") != strings.Join(want, ",") {
		t.Fatalf("header=%v want %v", records[0], want)
	}
}

func TestExportTransactionList_ExpenseAndIncomeSigns(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	// expense -> negative; income -> positive.
	if st, e := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, createReq(txID1, "expense", "42.50")); st != 200 {
		t.Fatalf("create expense=%d body=%s", st, e.raw)
	}
	const txID2 = "bbbb1111-0000-7000-8000-000000000002"
	inc := createReq(txID2, "income", "100")
	if st, e := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, inc); st != 200 {
		t.Fatalf("create income=%d body=%s", st, e.raw)
	}

	status, _, body := h.getRaw(t, "/api/v1/transaction/export-transaction-list", tok)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	records := parseCSV(t, body)
	if len(records) != 3 {
		t.Fatalf("rows=%d want 3 (header + 2)\n%s", len(records), body)
	}

	// Entity ids are server-minted (not the txID1/txID2 operation ids), so key
	// rows by their amount column instead.
	byAmount := map[string][]string{}
	for _, r := range records[1:] {
		byAmount[r[8]] = r
	}
	exp := byAmount["-42.5"]
	if exp == nil {
		t.Fatalf("no row for expense tx (amount -42.5)\n%s", body)
	}
	// amount column (index 8) negative for expense; category resolved.
	if exp[2] != "USD" {
		t.Fatalf("expense currency=%q want USD", exp[2])
	}
	if exp[3] != "Food" {
		t.Fatalf("expense category=%q want Food", exp[3])
	}
	if exp[1] != "Cash" {
		t.Fatalf("expense account_name=%q want Cash", exp[1])
	}
	in := byAmount["100"]
	if in == nil {
		t.Fatalf("no row for income tx (amount 100, positive)\n%s", body)
	}
}

func TestExportTransactionList_InvalidAccountId_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	// '@' is outside the allowed hex/comma/dash/space charset.
	status, _, body := h.getRaw(t, "/api/v1/transaction/export-transaction-list?accountId=@bad", tok)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400; body=%s", status, body)
	}
}

func TestExportTransactionList_NoToken_401(t *testing.T) {
	h := newHarness(t)
	status, _, _ := h.getRaw(t, "/api/v1/transaction/export-transaction-list", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", status)
	}
}

// TestExportTransactionList_LabelsColumn_PositionOrder is the end-to-end
// counterpart to the internal/transaction package's exportLabelIndex.cell unit
// tests: it exercises the real wiring (server.TransactionLabelNameLookup ->
// the label repo's GetByID -> internal/transaction's buildExportLabelIndex),
// not just the in-package helper. The two seeded labels' ids sort ascending
// as [labelLowID, labelHighID] -- exactly the order the repo's
// LabelsByTransactionIDs returns (ORDER BY label_id) -- but their positions
// are reversed relative to that, so a correct export must still read
// "Kid A;Kid B" (position order), not "Kid B;Kid A" (id/read order).
func TestExportTransactionList_LabelsColumn_PositionOrder(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const (
		labelLowID  = "eeee0000-0000-0000-0000-000000000001" // sorts first by id
		labelHighID = "eeee0000-0000-0000-0000-000000000002" // sorts second by id
	)
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	f.Label(fixture.Label{ID: labelLowID, UserID: seedUserID, Name: "Kid B", Position: 1})
	f.Label(fixture.Label{ID: labelHighID, UserID: seedUserID, Name: "Kid A", Position: 0})

	if st, e := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		createReqWithLabels(txID1, "expense", "7.25", []string{labelLowID, labelHighID})); st != 200 {
		t.Fatalf("create expense = %d; body=%s", st, e.raw)
	}

	status, _, body := h.getRaw(t, "/api/v1/transaction/export-transaction-list", tok)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	records := parseCSV(t, body)
	if len(records) != 2 {
		t.Fatalf("rows=%d want 2 (header + 1)\n%s", len(records), body)
	}
	const labelsCol = 6
	if got := records[1][labelsCol]; got != "Kid A;Kid B" {
		t.Fatalf("labels column = %q, want %q (position order)\n%s", got, "Kid A;Kid B", body)
	}
}

func parseCSV(t *testing.T, body string) [][]string {
	t.Helper()
	recs, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v\nbody: %s", err, body)
	}
	return recs
}

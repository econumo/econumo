package transaction_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// TestExportTransactionList_NamesAndTransfer exercises the export name resolution
// (category + tag + payee, via the per-request name cache) and the transfer path
// (auto "Transfer of …" note + the recipient-account row). It self-seeds a tag,
// payee, and a second account on the harness DB, then creates an expense with all
// three metadata fields plus a transfer between the two accounts.
func TestExportTransactionList_NamesAndTransfer(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	// Seed extra entities on the same DB the harness migrated/seeded.
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm})
	tagID := f.Tag(fixture.Tag{UserID: seedUserID, Name: "Travel"})
	payeeID := f.Payee(fixture.Payee{UserID: seedUserID, Name: "Airline"})
	const acct2 = "aaaa1111-0000-0000-0000-0000000000a2"
	f.Account(fixture.Account{ID: acct2, UserID: seedUserID, CurrencyID: usdID, Name: "Savings"})
	f.AccountInFolder(folderID, acct2)
	f.AccountOption(acct2, seedUserID, 1)

	// Expense with category + tag + payee (covers all three resolveNames branches).
	const expID = "bbbb1111-0000-7000-8000-0000000000e1"
	exp := map[string]any{
		"id": expID, "type": "expense", "amount": "12.00", "accountId": accountID,
		"categoryId": catID, "tagId": tagID, "payeeId": payeeID,
		"date": "2024-03-02 10:00:00", "description": "trip",
	}
	if st, e := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, exp); st != 200 {
		t.Fatalf("create expense = %d; body=%s", st, e.raw)
	}

	// Transfer accountID -> acct2 (covers the transfer note + recipient row).
	const trID = "bbbb1111-0000-7000-8000-0000000000e2"
	tr := map[string]any{
		"id": trID, "type": "transfer", "amount": "5.00", "accountId": accountID,
		"accountRecipientId": acct2, "amountRecipient": "5.00",
		"date": "2024-03-03 10:00:00", "description": "move",
	}
	if st, e := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, tr); st != 200 {
		t.Fatalf("create transfer = %d; body=%s", st, e.raw)
	}

	status, _, body := h.getRaw(t, "/api/v1/transaction/export-transaction-list", tok)
	if status != http.StatusOK {
		t.Fatalf("export status = %d; body=%s", status, body)
	}
	records := parseCSV(t, body)
	// header + expense (1) + transfer (2: source row + recipient row) = 4.
	if len(records) != 4 {
		t.Fatalf("rows = %d, want 4 (header + expense + transfer x2)\n%s", len(records), body)
	}
	for _, want := range []string{"Food", "Travel", "Airline", "Transfer of"} {
		if !strings.Contains(body, want) {
			t.Errorf("export body missing %q\n%s", want, body)
		}
	}
}

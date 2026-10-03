package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

// A hidden folder is a display preference, not an access boundary: the
// app-wide list leaves its accounts out, but the account's own list (opened
// from global search) must still load.
func TestGetTransactionList_HiddenFolderAccount(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	f := h.fixtures(t)
	hiddenFolder := f.Folder(fixture.Folder{UserID: seedUserID, Name: "Archive", Position: 1, Hidden: true})
	hiddenAccount := f.Account(fixture.Account{UserID: seedUserID, CurrencyID: usdID, Name: "Old brokerage"})
	f.AccountInFolder(hiddenFolder, hiddenAccount)
	f.AccountOption(hiddenAccount, seedUserID, 1)
	txID := f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: hiddenAccount, Type: 1, Amount: "12.40", Description: "Dividend payout"})

	status, env := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list?accountId="+hiddenAccount, tok, nil)
	if status != http.StatusOK {
		t.Fatalf("own list of a hidden-folder account: status %d, want 200", status)
	}
	list := mustUnmarshal[listResult](t, env.Data)
	if len(list.Items) != 1 || list.Items[0].ID != txID {
		t.Fatalf("own list = %+v, want the one transaction %s", list.Items, txID)
	}

	_, allEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	for _, item := range mustUnmarshal[listResult](t, allEnv.Data).Items {
		if item.ID == txID {
			t.Fatalf("the app-wide list must keep leaving hidden-folder accounts out")
		}
	}
}

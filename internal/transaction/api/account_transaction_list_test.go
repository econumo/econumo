package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

const accountTxListPath = "/api/v1/transaction/get-account-transaction-list"

type accountTxItem struct {
	ID                 string   `json:"id"`
	AuthorID           string   `json:"authorId"`
	AccountID          string   `json:"accountId"`
	AccountRecipientID *string  `json:"accountRecipientId"`
	LabelIds           []string `json:"labelIds"`
	IsImported         int      `json:"isImported"`
}

type accountTxList struct {
	Revisions map[string]int64 `json:"revisions"`
	Items     []accountTxItem  `json:"items"`
	Authors   []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Avatar string `json:"avatar"`
	} `json:"authors"`
}

func (l accountTxList) itemIDs() []string {
	ids := make([]string, len(l.Items))
	for i, it := range l.Items {
		ids[i] = it.ID
	}
	return ids
}

func (h *harness) getAccountTxList(t *testing.T, query string) accountTxList {
	t.Helper()
	status, env := h.do(t, http.MethodGet, accountTxListPath+query, h.token(t), nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	return mustUnmarshal[accountTxList](t, env.Data)
}

func (h *harness) setRevision(t *testing.T, accountID string, rev int64) {
	t.Helper()
	if _, err := h.db.Exec(`UPDATE accounts SET transactions_revision = ? WHERE id = ?`, rev, accountID); err != nil {
		t.Fatalf("set revision: %v", err)
	}
}

func (h *harness) ownAccount(t *testing.T, f *fixture.Builder, name string, position int, opts fixture.Account) string {
	t.Helper()
	opts.UserID = seedUserID
	opts.CurrencyID = usdID
	opts.Name = name
	id := f.Account(opts)
	f.AccountInFolder(folderID, id)
	f.AccountOption(id, seedUserID, position)
	return id
}

func TestGetAccountTransactionList_AllAccessibleWhenIdsAbsent(t *testing.T) {
	h := newHarness(t)
	f := h.fixtures(t)

	hiddenFolder := f.Folder(fixture.Folder{UserID: seedUserID, Name: "Archive", Position: 1, Hidden: true})
	hiddenAcct := f.Account(fixture.Account{UserID: seedUserID, CurrencyID: usdID, Name: "Old"})
	f.AccountInFolder(hiddenFolder, hiddenAcct)
	f.AccountOption(hiddenAcct, seedUserID, 1)

	h.shareAccount(t, roleGuest, true)
	f.User(fixture.User{ID: "44444444-4444-4444-4444-444444444444", Name: "Stranger"})
	foreign := f.Account(fixture.Account{UserID: "44444444-4444-4444-4444-444444444444", CurrencyID: usdID, Name: "Theirs"})
	deleted := h.ownAccount(t, f, "Gone", 2, fixture.Account{Deleted: true})

	own := f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "5"})
	hid := f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: hiddenAcct, Type: 0, Amount: "6"})
	shared := f.Transaction(fixture.Transaction{UserID: ownerTwoID, AccountID: sharedAcctID, Type: 0, Amount: "7"})
	f.Transaction(fixture.Transaction{UserID: "44444444-4444-4444-4444-444444444444", AccountID: foreign, Type: 0, Amount: "8"})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: deleted, Type: 0, Amount: "9"})

	got := h.getAccountTxList(t, "")
	if len(got.Revisions) != 3 {
		t.Fatalf("revisions = %v, want own + hidden-folder + shared accounts only", got.Revisions)
	}
	for _, id := range []string{accountID, hiddenAcct, sharedAcctID} {
		if _, ok := got.Revisions[id]; !ok {
			t.Errorf("revisions lacks %s: %v", id, got.Revisions)
		}
	}
	ids := strings.Join(got.itemIDs(), ",")
	for _, want := range []string{own, hid, shared} {
		if !strings.Contains(ids, want) {
			t.Errorf("items lack %s: %v", want, got.itemIDs())
		}
	}
	if len(got.Items) != 3 {
		t.Errorf("items = %d, want 3 (no foreign or deleted-account rows)", len(got.Items))
	}
}

func TestGetAccountTransactionList_IdsNarrowResult(t *testing.T) {
	h := newHarness(t)
	f := h.fixtures(t)
	second := h.ownAccount(t, f, "Second", 1, fixture.Account{})
	first := f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1"})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: second, Type: 0, Amount: "2"})

	got := h.getAccountTxList(t, "?accountIds="+accountID)
	if len(got.Revisions) != 1 {
		t.Fatalf("revisions = %v, want only %s", got.Revisions, accountID)
	}
	if ids := got.itemIDs(); len(ids) != 1 || ids[0] != first {
		t.Fatalf("items = %v, want [%s]", ids, first)
	}
}

func TestGetAccountTransactionList_TransferAppearsOnce(t *testing.T) {
	h := newHarness(t)
	f := h.fixtures(t)
	second := h.ownAccount(t, f, "Second", 1, fixture.Account{})
	transfer := f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, AccountRecipientID: second, Type: 2, Amount: "10", AmountRecipient: "10"})

	got := h.getAccountTxList(t, "?accountIds="+accountID+","+second+","+accountID)
	if ids := got.itemIDs(); len(ids) != 1 || ids[0] != transfer {
		t.Fatalf("items = %v, want the transfer once", ids)
	}
	if r := got.Items[0].AccountRecipientID; r == nil || *r != second {
		t.Errorf("accountRecipientId = %v, want %s", r, second)
	}
}

func TestGetAccountTransactionList_TransferOnlyRecipientRequested(t *testing.T) {
	h := newHarness(t)
	f := h.fixtures(t)
	second := h.ownAccount(t, f, "Second", 1, fixture.Account{})
	transfer := f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, AccountRecipientID: second, Type: 2, Amount: "10", AmountRecipient: "10"})

	got := h.getAccountTxList(t, "?accountIds="+second)
	if ids := got.itemIDs(); len(ids) != 1 || ids[0] != transfer {
		t.Fatalf("items = %v, want the transfer via its recipient account", ids)
	}
}

func TestGetAccountTransactionList_InaccessibleIdsOmitted(t *testing.T) {
	h := newHarness(t)
	f := h.fixtures(t)
	h.shareAccountPending(t, roleUser)
	f.User(fixture.User{ID: "44444444-4444-4444-4444-444444444444", Name: "Stranger"})
	foreign := f.Account(fixture.Account{UserID: "44444444-4444-4444-4444-444444444444", CurrencyID: usdID, Name: "Theirs"})
	deleted := h.ownAccount(t, f, "Gone", 1, fixture.Account{Deleted: true})
	f.Transaction(fixture.Transaction{UserID: "44444444-4444-4444-4444-444444444444", AccountID: foreign, Type: 0, Amount: "8"})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: deleted, Type: 0, Amount: "9"})
	own := f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1"})
	missing := "99999999-9999-4999-8999-999999999999"

	q := "?accountIds=" + strings.Join([]string{accountID, foreign, sharedAcctID, deleted, missing}, ",")
	got := h.getAccountTxList(t, q)
	if len(got.Revisions) != 1 {
		t.Fatalf("revisions = %v, want only the own account", got.Revisions)
	}
	if ids := got.itemIDs(); len(ids) != 1 || ids[0] != own {
		t.Fatalf("items = %v, want [%s]", ids, own)
	}
}

func TestGetAccountTransactionList_NothingAccessibleIsEmptyNotNull(t *testing.T) {
	h := newHarness(t)
	status, env := h.do(t, http.MethodGet, accountTxListPath+"?accountIds=99999999-9999-4999-8999-999999999999", h.token(t), nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	for _, want := range []string{`"revisions":{}`, `"items":[]`, `"authors":[]`} {
		if !strings.Contains(string(env.raw), want) {
			t.Errorf("body lacks %s: %s", want, env.raw)
		}
	}
}

func TestGetAccountTransactionList_RevisionsMatchColumn(t *testing.T) {
	h := newHarness(t)
	f := h.fixtures(t)
	second := h.ownAccount(t, f, "Second", 1, fixture.Account{})
	h.setRevision(t, accountID, 42)
	h.setRevision(t, second, 7)

	got := h.getAccountTxList(t, "")
	if got.Revisions[accountID] != 42 || got.Revisions[second] != 7 {
		t.Fatalf("revisions = %v, want %s:42 %s:7", got.Revisions, accountID, second)
	}
}

func TestGetAccountTransactionList_AuthorsDedupedAndSorted(t *testing.T) {
	h := newHarness(t)
	f := h.fixtures(t)
	h.shareAccount(t, roleUser, true)
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1"})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "2"})
	f.Transaction(fixture.Transaction{UserID: ownerTwoID, AccountID: sharedAcctID, Type: 0, Amount: "3"})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: sharedAcctID, Type: 0, Amount: "4"})

	got := h.getAccountTxList(t, "")
	if len(got.Items) != 4 {
		t.Fatalf("items = %d, want 4", len(got.Items))
	}
	if len(got.Authors) != 2 {
		t.Fatalf("authors = %+v, want 2 distinct", got.Authors)
	}
	if got.Authors[0].ID != seedUserID || got.Authors[1].ID != ownerTwoID {
		t.Errorf("authors not sorted by id: %+v", got.Authors)
	}
	if got.Authors[0].Name != seedName {
		t.Errorf("author name = %q, want %q", got.Authors[0].Name, seedName)
	}
	known := map[string]bool{seedUserID: true, ownerTwoID: true}
	for _, it := range got.Items {
		if !known[it.AuthorID] {
			t.Errorf("item %s authorId %q is not in authors", it.ID, it.AuthorID)
		}
	}
}

func TestGetAccountTransactionList_CarriesLabelsAndImportedFlag(t *testing.T) {
	h := newHarness(t)
	f := h.fixtures(t)
	id := f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1"})
	if _, err := h.db.Exec(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)`, id, label1ID); err != nil {
		t.Fatalf("label link: %v", err)
	}

	got := h.getAccountTxList(t, "")
	if len(got.Items) != 1 || len(got.Items[0].LabelIds) != 1 || got.Items[0].LabelIds[0] != label1ID {
		t.Fatalf("items = %+v, want one item carrying %s", got.Items, label1ID)
	}
	if got.Items[0].IsImported != 0 {
		t.Errorf("isImported = %d, want 0", got.Items[0].IsImported)
	}
}

func TestGetAccountTransactionList_ValidationErrors(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	ids := make([]string, 201)
	for i := range ids {
		ids[i] = fmt.Sprintf("aaaa0000-0000-4000-8000-%012d", i)
	}
	cases := []struct {
		name  string
		query string
		code  string
	}{
		{"present but empty", "?accountIds=", "This value should not be blank."},
		{"only commas", "?accountIds=,,", "This value should not be blank."},
		{"not a uuid", "?accountIds=nope", "This value is not a valid UUID."},
		{"one bad among good", "?accountIds=" + accountID + ",nope", "This value is not a valid UUID."},
		{"empty segment", "?accountIds=" + accountID + ",,", "This value is not a valid UUID."},
		{"201 ids", "?accountIds=" + strings.Join(ids, ","), "This value is too long."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, env := h.do(t, http.MethodGet, accountTxListPath+tc.query, tok, nil)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
			}
			msgs := env.errorsMap()["accountIds"]
			if len(msgs) == 0 || msgs[0] != tc.code {
				t.Errorf("errors[accountIds] = %v, want [%q]", msgs, tc.code)
			}
		})
	}
}

func TestGetAccountTransactionList_Exactly200IdsAllowed(t *testing.T) {
	h := newHarness(t)
	ids := make([]string, 200)
	for i := range ids {
		ids[i] = fmt.Sprintf("aaaa0000-0000-4000-8000-%012d", i)
	}
	h.getAccountTxList(t, "?accountIds="+strings.Join(ids, ","))
}

func TestGetAccountTransactionList_Unauthenticated(t *testing.T) {
	h := newHarness(t)
	status, _ := h.do(t, http.MethodGet, accountTxListPath, "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}
